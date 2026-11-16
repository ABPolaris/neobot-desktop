package launcher

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var downloads = &http.Client{
	Timeout: 2 * time.Minute,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" || len(via) >= 10 {
			return errors.New("无效的下载重定向")
		}
		return nil
	},
}

func (m *Manager) Available() ([]Version, error) {
	m.mu.Lock()
	index := m.state.Sources.Index
	m.mu.Unlock()
	versions, err := availableAt(index)
	if err != nil && index != officialIndex {
		return availableAt(officialIndex)
	}
	return versions, err
}

func availableAt(index string) ([]Version, error) {
	req, err := http.NewRequest(http.MethodGet, index+"/neobot-app/", nil)
	if err != nil {
		return nil, errors.New("软件包索引地址无效")
	}
	req.Header.Set("Accept", "application/vnd.pypi.simple.v1+json")
	resp, err := downloads.Do(req)
	if err != nil {
		return nil, errors.New("无法获取版本列表，请检查网络后重试")
	}
	defer resp.Body.Close()
	var list struct {
		Files []struct {
			Filename string `json:"filename"`
			Yanked   any    `json:"yanked"`
		} `json:"files"`
	}
	if resp.StatusCode != 200 {
		return nil, errors.New("无法读取 NeoBot 版本列表")
	}
	available := map[string]bool{}
	add := func(filename string) {
		parts := strings.Split(filename, "-")
		if len(parts) < 2 || (parts[0] != "neobot_app" && parts[0] != "neobot.app") {
			return
		}
		version := strings.TrimSuffix(strings.TrimSuffix(parts[1], ".tar.gz"), ".zip")
		if versionPattern.MatchString(version) {
			available[version] = true
		}
	}
	if strings.Contains(resp.Header.Get("Content-Type"), "json") {
		if json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&list) != nil {
			return nil, errors.New("无法解析 NeoBot 版本列表")
		}
		for _, f := range list.Files {
			yanked, _ := f.Yanked.(bool)
			_, hasReason := f.Yanked.(string)
			if !yanked && !hasReason {
				add(f.Filename)
			}
		}
	} else {
		if err := readSimpleHTML(io.LimitReader(resp.Body, 8<<20), add); err != nil {
			return nil, err
		}
	}
	result := make([]Version, 0, len(available))
	for version := range available {
		result = append(result, versionOf(version))
	}
	sort.Slice(result, func(i, j int) bool { return compareVersions(result[i].Name, result[j].Name) > 0 })
	return result, nil
}

func readSimpleHTML(r io.Reader, add func(string)) error {
	// PEP 503 indexes may return HTML when they have not implemented JSON.
	body, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	for _, match := range simpleLink.FindAllStringSubmatch(string(body), -1) {
		if strings.Contains(strings.ToLower(match[0]), "data-yanked") {
			continue
		}
		u, err := url.Parse(match[1])
		if err == nil {
			add(filepath.Base(u.Path))
		}
	}
	return nil
}

func (m *Manager) Install(version string, official, start bool) error {
	if !supported() {
		return errors.New("首版仅支持 Windows x64")
	}
	if !versionPattern.MatchString(version) {
		return errors.New("请选择 NeoBot 发行版本")
	}
	m.mu.Lock()
	if m.closing || m.job.State == "running" {
		m.mu.Unlock()
		return errors.New("已有安装任务正在运行")
	}
	if start && (m.bot != nil || m.preparing) {
		m.mu.Unlock()
		return errors.New("请先停止正在运行的 NeoBot")
	}
	if _, installed := m.installed[version]; installed {
		m.mu.Unlock()
		if !start {
			return nil
		}
		if err := m.SelectVersion(version); err != nil {
			return err
		}
		return m.Start(false)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Minute)
	m.jobCancel = cancel
	done := make(chan struct{})
	m.jobDone = done
	sources := m.state.Sources
	if official {
		sources = defaultSources("official")
	}
	m.job = Job{Version: version, Step: "uv", State: "running", Total: 3, StartAfter: start}
	m.mu.Unlock()
	m.notify()

	go func() {
		defer cancel()
		defer close(done)
		err := m.install(ctx, version, sources)
		m.mu.Lock()
		if err == nil {
			m.installed[version] = versionOf(version)
			if start || m.state.Selected == "" {
				before := m.state.Selected
				m.state.Selected = version
				if err = m.persist(); err != nil {
					m.state.Selected = before
				}
			}
		}
		m.jobCancel = nil
		if err != nil {
			m.job.State, m.job.Message = "failed", err.Error()
		} else {
			m.job.State, m.job.Completed, m.job.Message = "done", 3, "NeoBot 安装完成"
		}
		m.mu.Unlock()
		m.notify()
		if err == nil && start {
			if err := m.Start(false); err != nil {
				m.setRuntimeError(err.Error())
			}
		}
	}()
	return nil
}

func (m *Manager) setStep(step, message string, done int) {
	m.mu.Lock()
	m.job.Step, m.job.Message, m.job.Completed = step, message, done
	m.mu.Unlock()
	m.notify()
}

func (m *Manager) install(ctx context.Context, version string, sources Sources) error {
	for _, dir := range []string{"tools", "python", "cache", "versions"} {
		if err := os.MkdirAll(filepath.Join(m.root, dir), 0700); err != nil {
			return err
		}
	}
	m.setStep("uv", "正在准备 uv", 0)
	uv, err := m.ensureUV(ctx, sources)
	if err != nil {
		return fmt.Errorf("下载 uv 失败：%w", err)
	}
	env := m.installEnv()
	m.setStep("python", "正在准备 Python 3.13", 1)
	if err := m.ensurePython(ctx, uv, env, sources); err != nil {
		return fmt.Errorf("安装 Python 失败：%w", err)
	}

	m.setStep("neobot", "正在安装 NeoBot "+version, 2)
	dir := m.versionDir(version)
	python := filepath.Join(dir, "Scripts", "python.exe")
	if !isFile(python) {
		if err := runUV(ctx, uv, env, "venv", "--allow-existing", "--python", "3.13", "--no-python-downloads", dir); err != nil {
			return fmt.Errorf("创建 NeoBot 环境失败：%w", err)
		}
	}
	if err := runUV(ctx, uv, env, "pip", "install", "--python", python, "--default-index", sources.Index, "neobot-app=="+version); err != nil {
		return fmt.Errorf("安装 NeoBot 失败：%w", err)
	}
	if err := runUV(ctx, uv, env, "pip", "check", "--python", python); err != nil {
		return fmt.Errorf("检查 NeoBot 依赖失败：%w", err)
	}
	if !isFile(m.binary(version)) {
		return errors.New("安装完成后没有找到 NeoBot 启动程序")
	}
	return writeInstallRecord(dir, installRecord{Name: version, Preview: versionOf(version).Preview})
}

func (m *Manager) uvEnv() []string {
	return []string{
		"UV_CACHE_DIR=" + filepath.Join(m.root, "cache"),
		"UV_PYTHON_INSTALL_DIR=" + filepath.Join(m.root, "python"),
		"UV_PYTHON_PREFERENCE=only-managed",
	}
}

func (m *Manager) installEnv() []string {
	return append(m.uvEnv(), "UV_NO_CONFIG=1")
}

func (m *Manager) ensureUV(ctx context.Context, sources Sources) (string, error) {
	uv := filepath.Join(m.root, "tools", "uv-"+uvVersion+".exe")
	if !isFile(uv) {
		if err := installUV(ctx, uv, sources.UV); err != nil {
			return "", err
		}
	}
	return uv, nil
}

func (m *Manager) ensurePython(ctx context.Context, uv string, env []string, sources Sources) error {
	args := []string{"python", "install", "3.13", "--no-bin", "--no-registry"}
	if sources.Python != "" {
		args = append(args, "--mirror", sources.Python)
	}
	return runUV(ctx, uv, env, args...)
}

func cleanEnv(clearUV bool, extra ...string) []string {
	env := make([]string, 0)
	for _, kv := range os.Environ() {
		name := strings.ToUpper(strings.SplitN(kv, "=", 2)[0])
		if clearUV && (strings.HasPrefix(name, "UV_") || name == "VIRTUAL_ENV" || name == "PYTHONHOME" || name == "PYTHONPATH") {
			continue
		}
		if !clearUV && (strings.HasPrefix(name, "NEOBOT_") || name == "PYTHONHOME" || name == "PYTHONPATH") {
			continue
		}
		env = append(env, kv)
	}
	return append(env, extra...)
}

func runUV(ctx context.Context, uv string, env []string, args ...string) error {
	cmd := exec.CommandContext(ctx, uv, args...)
	cmd.Env = cleanEnv(true, env...)
	hideWindow(cmd)
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return errors.New("任务已取消或超时")
		}
		// uv output can contain credentials inherited through proxy configuration.
		return errors.New("命令执行失败，请检查下载源、网络或磁盘空间后重试")
	}
	return nil
}

func getDownload(ctx context.Context, address string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	resp, err := downloads.Do(req)
	if err != nil {
		return nil, errors.New("无法连接下载源")
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("下载源返回 HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

func installUV(ctx context.Context, target, source string) error {
	// GitHub's digest for the release ZIP; mirrors need no connection to GitHub.
	const expectedSHA256 = "ddbfcee1ac615a0499f6aa97b5ec8ebdf3ee4a7714a48055ec2ba0030e3cf810"
	resp, err := getDownload(ctx, source+"/"+uvVersion+"/"+uvArchive)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	tmp, err := os.CreateTemp(filepath.Dir(target), ".uv-*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(resp.Body, 64<<20))
	closeErr := tmp.Close()
	if err != nil || closeErr != nil || n == 64<<20 || expectedSHA256 != hex.EncodeToString(hash.Sum(nil)) {
		return errors.New("uv 下载不完整或校验失败")
	}
	return extractUV(tmp.Name(), target)
}

func extractUV(archive, target string) error {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, file := range r.File {
		if (file.Name != "uv.exe" && file.Name != strings.TrimSuffix(uvArchive, ".zip")+"/uv.exe") || !file.Mode().IsRegular() {
			continue
		}
		if file.UncompressedSize64 > 96<<20 {
			return errors.New("uv 文件过大")
		}
		in, err := file.Open()
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.CreateTemp(filepath.Dir(target), ".uv-*.exe")
		if err != nil {
			return err
		}
		defer os.Remove(out.Name())
		_, err = io.Copy(out, in)
		if closeErr := out.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
		return os.Rename(out.Name(), target)
	}
	return errors.New("压缩包缺少 uv.exe")
}

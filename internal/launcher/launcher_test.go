package launcher

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestVersionSwitchBacksUpAndRestoresCompatibleData(t *testing.T) {
	root := t.TempDir()
	m := Manager{root: root, state: diskState{Backups: map[string]string{}}}
	write := func(dir, value string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	check := func(dir, want string) {
		t.Helper()
		value, err := os.ReadFile(filepath.Join(dir, "config.toml"))
		if err != nil || string(value) != want {
			t.Fatalf("got %q, %v; want %q", value, err, want)
		}
	}
	active, err := m.prepareData("1.0.0", false)
	if err != nil {
		t.Fatal(err)
	}
	write(active, "old")
	if err := os.Mkdir(filepath.Join(active, ".neobot"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = m.prepareData("1.1.0", false); err != nil {
		t.Fatal(err)
	}
	write(active, "new")
	if _, err = m.prepareData("1.0.0", false); err == nil {
		t.Fatal("restore must require confirmation")
	}
	if _, err = m.prepareData("0.9.0", true); err == nil {
		t.Fatal("restore must require a compatible snapshot")
	}
	check(active, "new")
	backup := m.state.Backups["1.0.0"]
	if _, err := os.Stat(filepath.Join(root, "backups", backup, ".neobot")); !os.IsNotExist(err) {
		t.Fatalf("control credentials must not be backed up: %v", err)
	}
	if _, err = m.prepareData("1.0.0", true); err != nil {
		t.Fatal(err)
	}
	check(active, "old")
	check(filepath.Join(root, "backups", m.state.Backups["1.1.0"]), "new")
	preview, err := m.prepareData("2.0.0a1", false)
	if err != nil || preview != active || m.state.DataVersion != "1.0.0" {
		t.Fatalf("preview should share stable data: %s, %v", preview, err)
	}
	m.state.IsolateData = true
	isolated, err := m.prepareData("2.0.0a1", false)
	if err != nil || isolated == active || !isDir(isolated) || m.state.DataVersion != "1.0.0" {
		t.Fatalf("preview should use isolated data when enabled: %s, %v", isolated, err)
	}
}

func TestSnapshotReportsDataDirectoryInUse(t *testing.T) {
	root := t.TempDir()
	m := &Manager{root: root, state: diskState{Selected: "1.2.0", Backups: map[string]string{}}}
	if got, want := m.Snapshot().DataDir, filepath.Join(root, "data"); got != want {
		t.Fatalf("shared data dir = %q; want %q", got, want)
	}
	m.state.IsolateData = true
	if got, want := m.Snapshot().DataDir, filepath.Join(root, "isolated", "1.2.0"); got != want {
		t.Fatalf("isolated data dir = %q; want %q", got, want)
	}
	m.state.Selected = ""
	if got, want := m.Snapshot().DataDir, filepath.Join(root, "isolated"); got != want {
		t.Fatalf("unselected isolated data dir = %q; want %q", got, want)
	}
}

func TestBackupFailureLeavesActiveVersion(t *testing.T) {
	root := t.TempDir()
	m := Manager{root: root, state: diskState{Backups: map[string]string{}}}
	active, err := m.prepareData("1.0.0", false)
	if err != nil {
		t.Fatal(err)
	}
	backups := filepath.Join(root, "backups")
	if err := os.Remove(backups); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backups, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.prepareData("1.1.0", false); err == nil {
		t.Fatal("expected backup failure")
	}
	if m.state.DataVersion != "1.0.0" {
		t.Fatal("version changed despite backup failure")
	}
	if _, err := os.Stat(active); err != nil {
		t.Fatal(err)
	}
}

func TestControlCapabilityAndIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sample-token" {
			t.Error("missing local control authentication")
		}
		data := map[string]any{"instance_id": "owned"}
		switch r.URL.Path {
		case "/v1/status":
			data["capabilities"] = []string{"runtime.stop"}
		case "/v1/setup/status":
			data["instance_id"] = "old"
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"schema_version": 1, "ok": true, "data": data})
	}))
	defer server.Close()
	d := discovery{URL: server.URL, Token: "sample-token", InstanceID: "owned"}
	if supported, err := d.supported(context.Background()); err != nil || supported {
		t.Fatalf("legacy detection: %v %v", supported, err)
	}
	if err := d.call(context.Background(), "/v1/setup/status", nil, new(SetupStatus)); err == nil {
		t.Fatal("accepted a different instance")
	}
}

func TestLegacyDashboardUsesReportedLoopbackAddress(t *testing.T) {
	line := "\x1b[32m2026-11-14 13:50:15.449\x1b[0m | INFO | \x1b[36mplugin.dashboard\x1b[0m | 网页面板已启动 | url=http://<服务器IP>:19981/bot/, local_url=http://127.0.0.1:19981/bot/, listen_host=0.0.0.0"
	const want = "http://127.0.0.1:19981/bot/"
	if got := dashboardAddress(line); got != want {
		t.Fatalf("address = %q; want %q", got, want)
	}
	for _, invalid := range []string{
		strings.Replace(line, "plugin.dashboard", "plugin.other", 1),
		strings.Replace(line, want, "http://example.com:19981/bot/", 1),
	} {
		if got := dashboardAddress(invalid); got != "" {
			t.Fatalf("accepted unrelated or remote address %q", got)
		}
	}
}

func buildZip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for name, content := range entries {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// The two entry names and digests were checked against the published artifacts:
// the GitHub zip is flat (uv.exe at the root) and the wheel keeps it under
// uv-<version>.data/scripts/. This case is self-referential, so it cannot catch
// a wrong constant.
func TestUVArchiveExtractsOnlyPinnedExecutable(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "uv.zip")
	if err := os.WriteFile(archive, buildZip(t, map[string]string{
		"../outside":   "escaped",
		uvArchiveEntry: "archive",
		uvWheelEntry:   "wheel",
	}), 0600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		entry string
		want  string
	}{
		{"archive", uvArchiveEntry, "archive"},
		{"wheel", uvWheelEntry, "wheel"},
	} {
		t.Run(test.name, func(t *testing.T) {
			target := filepath.Join(dir, test.name+".exe")
			if err := extractUV(archive, target, test.entry); err != nil {
				t.Fatal(err)
			}
			if content, err := os.ReadFile(target); err != nil || string(content) != test.want {
				t.Fatalf("extracted %q, %v; want %q", content, err, test.want)
			}
			if isFile(filepath.Join(dir, "..", "outside")) {
				t.Fatal("path traversal entry was written")
			}
		})
	}
	other := filepath.Join(dir, "other.zip")
	if err := os.WriteFile(other, buildZip(t, map[string]string{uvArchiveEntry: "archive"}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := extractUV(other, filepath.Join(dir, "missing.exe"), uvWheelEntry); err == nil || !strings.Contains(err.Error(), "压缩包缺少 uv.exe") {
		t.Fatalf("missing entry was not reported: %v", err)
	}
}

func TestAcceleratedUVComesFromMirroredWheel(t *testing.T) {
	wheel := buildZip(t, map[string]string{uvWheelEntry: "wheel uv"})
	sum := sha256.Sum256(wheel)
	win32 := "uv-" + uvVersion + "-py3-none-win32.whl"
	name := "uv-" + uvVersion + "-py3-none-win_amd64.whl"
	serve := func(withWheel bool) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/uv/":
				files := `[{"filename":"` + win32 + `","url":"../../packages/win32.whl","yanked":false}]`
				if withWheel {
					files = `[{"filename":"` + win32 + `","url":"../../packages/win32.whl","yanked":false},` +
						`{"filename":"` + name + `","url":"../../packages/` + name + `","yanked":false}]`
				}
				w.Header().Set("Content-Type", "application/vnd.pypi.simple.v1+json")
				_, _ = w.Write([]byte(`{"files":` + files + `}`))
			case "/packages/" + name:
				_, _ = w.Write(wheel)
			default:
				t.Errorf("unexpected request %s", r.URL.Path)
				http.NotFound(w, r)
			}
		}))
	}
	missing := serve(false)
	defer missing.Close()
	manager := &Manager{root: t.TempDir()}
	accelerated := Sources{Preset: "accelerated", Index: missing.URL}
	if _, err := manager.ensureUV(context.Background(), accelerated); err == nil || !strings.Contains(err.Error(), "下载源没有提供 uv "+uvVersion) {
		t.Fatalf("index without the wheel was not reported: %v", err)
	}
	if isFile(filepath.Join(manager.root, "tools", "uv-"+uvVersion+".exe")) {
		t.Fatal("failed uv install published an executable")
	}

	mirror := serve(true)
	defer mirror.Close()
	address, err := uvWheelURL(context.Background(), mirror.URL)
	if err != nil {
		t.Fatal(err)
	}
	if want := mirror.URL + "/packages/" + name; address != want {
		t.Fatalf("wheel address = %q; want %q", address, want)
	}

	target := filepath.Join(t.TempDir(), "uv.exe")
	if err := installUV(context.Background(), target, address, strings.Repeat("0", 64), uvWheelEntry); err == nil || !strings.Contains(err.Error(), "校验失败") {
		t.Fatalf("bad checksum was not reported: %v", err)
	}
	if isFile(target) {
		t.Fatal("unverified download was published")
	}
	if err := installUV(context.Background(), target, address, hex.EncodeToString(sum[:]), uvWheelEntry); err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(target); err != nil || string(content) != "wheel uv" {
		t.Fatalf("extracted %q, %v; want the wheel's uv.exe", content, err)
	}

	gone := httptest.NewServer(http.NotFoundHandler())
	defer gone.Close()
	goneTarget := filepath.Join(t.TempDir(), "uv.exe")
	if err := installUV(context.Background(), goneTarget, gone.URL+"/packages/"+name, hex.EncodeToString(sum[:]), uvWheelEntry); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("missing wheel download was not reported: %v", err)
	}
	if isFile(goneTarget) {
		t.Fatal("failed download published an executable")
	}
}

func TestAcceleratedUVUsesIndexWheel(t *testing.T) {
	win32 := "uv-" + uvVersion + "-py3-none-win32.whl"
	name := "uv-" + uvVersion + "-py3-none-win_amd64.whl"
	yanked := "uv-" + uvVersion + "-1-py3-none-win_amd64.whl"
	serve := func(contentType, body string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/uv/" {
				t.Errorf("unexpected request %s", r.URL.Path)
			}
			if contentType != "" {
				w.Header().Set("Content-Type", contentType)
			}
			_, _ = w.Write([]byte(body))
		}))
	}
	const jsonType = "application/vnd.pypi.simple.v1+json"

	t.Run("html", func(t *testing.T) {
		server := serve("", `<a href="../../packages/`+win32+`">win32</a>
			<a href="../../packages/`+yanked+`" data-yanked="">yanked</a>
			<a href="../../packages/`+name+`#sha256=871e884c">win_amd64</a>`)
		defer server.Close()
		address, err := uvWheelURL(context.Background(), server.URL)
		if err != nil {
			t.Fatal(err)
		}
		if want := server.URL + "/packages/" + name; address != want {
			t.Fatalf("wheel address = %q; want %q", address, want)
		}
	})

	t.Run("yanked reason", func(t *testing.T) {
		server := serve(jsonType, `{"files":[`+
			`{"filename":"`+name+`","url":"../../packages/yanked.whl","yanked":"broken"},`+
			`{"filename":"`+name+`","url":"../../packages/`+name+`","yanked":false}]}`)
		defer server.Close()
		address, err := uvWheelURL(context.Background(), server.URL)
		if err != nil {
			t.Fatal(err)
		}
		if want := server.URL + "/packages/" + name; address != want {
			t.Fatalf("wheel address = %q; want %q", address, want)
		}
	})

	t.Run("empty url", func(t *testing.T) {
		server := serve(jsonType, `{"files":[`+
			`{"filename":"`+name+`","url":""},`+
			`{"filename":"`+name+`","url":"../../packages/`+name+`"}]}`)
		defer server.Close()
		address, err := uvWheelURL(context.Background(), server.URL)
		if err != nil {
			t.Fatal(err)
		}
		if want := server.URL + "/packages/" + name; address != want {
			t.Fatalf("wheel address = %q; want %q", address, want)
		}
	})

	t.Run("duplicate url", func(t *testing.T) {
		server := serve(jsonType, `{"files":[`+
			`{"filename":"`+name+`","url":"../../packages/`+name+`"},`+
			`{"filename":"`+name+`","url":"../../packages/`+name+`"}]}`)
		defer server.Close()
		address, err := uvWheelURL(context.Background(), server.URL)
		if err != nil {
			t.Fatal(err)
		}
		if want := server.URL + "/packages/" + name; address != want {
			t.Fatalf("wheel address = %q; want %q", address, want)
		}
	})

	t.Run("ambiguous", func(t *testing.T) {
		server := serve(jsonType, `{"files":[`+
			`{"filename":"`+name+`","url":"../../packages/one.whl"},`+
			`{"filename":"`+name+`","url":"../../packages/two.whl"}]}`)
		defer server.Close()
		if _, err := uvWheelURL(context.Background(), server.URL); err == nil || !strings.Contains(err.Error(), "提供了多个") {
			t.Fatalf("ambiguous index was not reported: %v", err)
		}
	})
}

func TestSimpleFilesResolvesLinksAgainstFinalURL(t *testing.T) {
	name := "uv-" + uvVersion + "-py3-none-win_amd64.whl"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/uv/":
			http.Redirect(w, r, "/mirror/simple/uv/", http.StatusFound)
		case "/mirror/simple/uv/":
			w.Header().Set("Content-Type", "application/vnd.pypi.simple.v1+json")
			_, _ = w.Write([]byte(`{"files":[{"filename":"` + name + `","url":"../../packages/` + name + `"}]}`))
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	// The shared client refuses non-HTTPS redirects, which an httptest server
	// cannot satisfy; this case is about the redirect itself. It swaps a
	// package-level client, so it must not be marked t.Parallel.
	shared := downloads
	downloads = &http.Client{Timeout: shared.Timeout}
	t.Cleanup(func() { downloads = shared })

	address, err := uvWheelURL(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if want := server.URL + "/mirror/packages/" + name; address != want {
		t.Fatalf("wheel address = %q; want %q", address, want)
	}
}

func TestResolveLinkRejectsNonWebSchemes(t *testing.T) {
	base, err := url.Parse("https://mirror.example.com/simple/uv/")
	if err != nil {
		t.Fatal(err)
	}
	for _, link := range []string{"", "#sha256=abc", "file:///etc/passwd", "data:text/plain,uv", "javascript:alert(1)"} {
		if got := resolveLink(base, link); got != nil {
			t.Fatalf("resolveLink(%q) = %v; want nil", link, got)
		}
	}
	for _, test := range []struct{ link, want string }{
		{"../../packages/uv-0.12.9-py3-none-win_amd64.whl", "https://mirror.example.com/packages/uv-0.12.9-py3-none-win_amd64.whl"},
		{"//cdn.example.com/uv.whl", "https://cdn.example.com/uv.whl"},
		{"https://other.example.com/uv.whl", "https://other.example.com/uv.whl"},
	} {
		got := resolveLink(base, test.link)
		if got == nil || got.String() != test.want {
			t.Fatalf("resolveLink(%q) = %v; want %q", test.link, got, test.want)
		}
	}
}

func TestDefaultSourcesPreferAccelerated(t *testing.T) {
	got, err := validSources(Sources{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Preset != "accelerated" || got.Index != "https://pypi.tuna.tsinghua.edu.cn/simple" || got.Python != mirrorPython || got.UV != officialUV {
		t.Fatalf("empty preset = %+v; want the accelerated defaults", got)
	}
	if official := defaultSources("official"); official.Preset != "official" || official.Index != officialIndex || official.Python != "" || official.UV != officialUV {
		t.Fatalf("official preset = %+v; want the upstream defaults", official)
	}
}

func TestValidSourcesRejectsEmptyCustomFields(t *testing.T) {
	if _, err := validSources(Sources{Preset: "custom", UV: "https://example.com/uv"}); err == nil || !strings.Contains(err.Error(), "自定义下载源需要填写完整地址") {
		t.Fatalf("missing index was not reported: %v", err)
	}
	if _, err := validSources(Sources{Preset: "custom", UV: "https://example.com/uv", Index: "https://example.com/simple"}); err != nil {
		t.Fatalf("an empty python prefix must stay valid: %v", err)
	}
}

func TestShouldFallBack(t *testing.T) {
	versions := []Version{{Name: "1.0.0"}}
	for _, test := range []struct {
		name     string
		sources  Sources
		versions []Version
		err      error
		want     bool
	}{
		{"official never falls back", Sources{Preset: "official", Index: officialIndex}, nil, errors.New("boom"), false},
		{"accelerated empty list", Sources{Preset: "accelerated", Index: mirrorIndex}, nil, nil, true},
		{"accelerated with versions", Sources{Preset: "accelerated", Index: mirrorIndex}, versions, nil, false},
		{"custom empty list", Sources{Preset: "custom", Index: "https://packages.example.com/simple"}, nil, nil, false},
		{"custom error", Sources{Preset: "custom", Index: "https://packages.example.com/simple"}, nil, errors.New("boom"), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := shouldFallBack(test.sources, test.versions, test.err); got != test.want {
				t.Fatalf("shouldFallBack = %v; want %v", got, test.want)
			}
		})
	}
}

func TestEnsureUVAcceleratedInstallsFromWheel(t *testing.T) {
	wheel := buildZip(t, map[string]string{uvWheelEntry: "wheel uv"})
	sum := sha256.Sum256(wheel)
	// This case rewrites a package-level variable, so it must not be marked
	// t.Parallel.
	original := uvWheelSHA256
	uvWheelSHA256 = hex.EncodeToString(sum[:])
	t.Cleanup(func() { uvWheelSHA256 = original })

	name := "uv-" + uvVersion + "-py3-none-win_amd64.whl"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/uv/":
			w.Header().Set("Content-Type", "application/vnd.pypi.simple.v1+json")
			_, _ = w.Write([]byte(`{"files":[{"filename":"` + name + `","url":"../../packages/` + name + `"}]}`))
		case "/packages/" + name:
			_, _ = w.Write(wheel)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tools"), 0700); err != nil {
		t.Fatal(err)
	}
	m := &Manager{root: root}
	uv, err := m.ensureUV(context.Background(), Sources{Preset: "accelerated", Index: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "tools", "uv-"+uvVersion+".exe"); uv != want {
		t.Fatalf("uv = %q; want %q", uv, want)
	}
	if content, err := os.ReadFile(uv); err != nil || string(content) != "wheel uv" {
		t.Fatalf("installed %q, %v; want the wheel's uv.exe", content, err)
	}
}

func TestEnsureUVRemovesStaleTemporaryFiles(t *testing.T) {
	root := t.TempDir()
	toolsDir := filepath.Join(root, "tools")
	if err := os.MkdirAll(toolsDir, 0700); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(toolsDir, ".uv-stale.zip")
	if err := os.WriteFile(stale, []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/uv/" {
			t.Errorf("unexpected request %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/vnd.pypi.simple.v1+json")
		_, _ = w.Write([]byte(`{"files":[]}`))
	}))
	defer server.Close()
	m := &Manager{root: root}
	if _, err := m.ensureUV(context.Background(), Sources{Preset: "accelerated", Index: server.URL}); err == nil {
		t.Fatal("an index without the wheel must fail")
	}
	if isFile(stale) {
		t.Fatal("stale temporary file was not removed")
	}
}

func TestEnsureUVDispatch(t *testing.T) {
	name := "uv-" + uvVersion + "-py3-none-win_amd64.whl"
	var mu sync.Mutex
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		if r.URL.Path == "/uv/" {
			w.Header().Set("Content-Type", "application/vnd.pypi.simple.v1+json")
			_, _ = w.Write([]byte(`{"files":[{"filename":"` + name + `","url":"../../packages/` + name + `"}]}`))
			return
		}
		_, _ = w.Write([]byte("not the pinned container"))
	}))
	defer server.Close()
	recorded := func() string {
		mu.Lock()
		defer mu.Unlock()
		return strings.Join(paths, " ")
	}

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tools"), 0700); err != nil {
		t.Fatal(err)
	}
	m := &Manager{root: root}
	// Both presets fail on the digest; the requested paths show the dispatch.
	if _, err := m.ensureUV(context.Background(), Sources{Preset: "accelerated", Index: server.URL}); err == nil {
		t.Fatal("accelerated uv install accepted an unverified container")
	}
	if got, want := recorded(), "/uv/ /packages/"+name; got != want {
		t.Fatalf("accelerated requests = %q; want %q", got, want)
	}

	mu.Lock()
	paths = nil
	mu.Unlock()
	if _, err := m.ensureUV(context.Background(), Sources{Preset: "official", UV: server.URL}); err == nil {
		t.Fatal("official uv install accepted an unverified container")
	}
	if got, want := recorded(), "/"+uvVersion+"/"+uvArchive; got != want {
		t.Fatalf("official requests = %q; want %q", got, want)
	}
	if isFile(filepath.Join(root, "tools", "uv-"+uvVersion+".exe")) {
		t.Fatal("failed uv install published an executable")
	}
}

func TestEnsureUVReusesInstalledExecutable(t *testing.T) {
	root := t.TempDir()
	toolsDir := filepath.Join(root, "tools")
	if err := os.MkdirAll(toolsDir, 0700); err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(toolsDir, "uv-"+uvVersion+".exe")
	if err := os.WriteFile(installed, []byte("installed"), 0600); err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.NotFound(w, r)
	}))
	defer server.Close()
	m := &Manager{root: root}
	uv, err := m.ensureUV(context.Background(), Sources{Preset: "accelerated", Index: server.URL})
	if err != nil || uv != installed {
		t.Fatalf("ensureUV = %q, %v; want the installed executable", uv, err)
	}
	if requests != 0 {
		t.Fatalf("ensureUV made %d requests for an installed executable", requests)
	}
}

func TestOpenDefaultsToAcceleratedSources(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("实例锁仅由 Windows 实现")
	}
	m, err := Open(filepath.Join(t.TempDir(), "data"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if got := m.state.Sources; got.Preset != "accelerated" || got.Index != "https://pypi.tuna.tsinghua.edu.cn/simple" || got.Python != mirrorPython || got.UV != officialUV {
		t.Fatalf("sources = %+v; want the accelerated defaults", got)
	}
}

func TestOpenKeepsSavedOfficialSources(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("实例锁仅由 Windows 实现")
	}
	root := filepath.Join(t.TempDir(), "data")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	saved := `{"sources":{"preset":"official","uv":"` + officialUV + `","python":"","index":"` + officialIndex + `"}}`
	if err := os.WriteFile(filepath.Join(root, "settings.json"), []byte(saved), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := Open(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if got := m.state.Sources; got.Preset != "official" || got.Index != officialIndex || got.Python != "" || got.UV != officialUV {
		t.Fatalf("sources = %+v; want the saved official sources", got)
	}
}

func TestOpenUpgradesSavedAcceleratedIndex(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("实例锁仅由 Windows 实现")
	}
	root := filepath.Join(t.TempDir(), "data")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	saved := `{"sources":{"preset":"accelerated","uv":"` + officialUV + `","python":"` + mirrorPython +
		`","index":"https://mirrors.ustc.edu.cn/pypi/simple"}}`
	if err := os.WriteFile(filepath.Join(root, "settings.json"), []byte(saved), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := Open(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if got := m.state.Sources; got.Preset != "accelerated" || got.Index != mirrorIndex {
		t.Fatalf("sources = %+v; want the accelerated preset on the current mirror", got)
	}
}

func TestOpenMigratesSettingsWithoutSources(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("实例锁仅由 Windows 实现")
	}
	root := filepath.Join(t.TempDir(), "data")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "settings.json"), []byte(`{"selected":""}`), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := Open(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if got := m.state.Sources; got.Preset != "accelerated" || got.Index != mirrorIndex || got.Python != mirrorPython {
		t.Fatalf("sources = %+v; want the accelerated defaults", got)
	}
}

func TestMirrorListsStableAndPreviewButNotYanked(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<a href="/neobot_app-1.0.0-py3-none-any.whl#sha256=x">stable</a>
			<a href="/neobot_app-2.0.0a1-py3-none-any.whl">preview</a>
			<a href="/neobot_app-3.0.0-py3-none-any.whl" data-yanked="">yanked</a>`))
	}))
	defer server.Close()
	versions, err := availableAt(server.URL)
	if err != nil || len(versions) != 2 || !versions[0].Preview || versions[1].Name != "1.0.0" {
		t.Fatalf("unexpected versions: %v, %v", versions, err)
	}
}

func TestMirrorJSONListsStableAndPreviewButNotYanked(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/neobot-app/" {
			t.Errorf("unexpected request %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/vnd.pypi.simple.v1+json")
		_, _ = w.Write([]byte(`{"files":[` +
			`{"filename":"neobot_app-1.0.0-py3-none-any.whl","url":"../../packages/neobot_app-1.0.0-py3-none-any.whl","yanked":false},` +
			`{"filename":"neobot_app-2.0.0a1-py3-none-any.whl","url":"../../packages/neobot_app-2.0.0a1-py3-none-any.whl"},` +
			`{"filename":"neobot_app-3.0.0-py3-none-any.whl","url":"../../packages/neobot_app-3.0.0-py3-none-any.whl","yanked":"broken"},` +
			`{"filename":"neobot_app-4.0.0.tar.gz","url":"../../packages/neobot_app-4.0.0.tar.gz","yanked":true}]}`))
	}))
	defer server.Close()
	versions, err := availableAt(server.URL)
	if err != nil || len(versions) != 2 || !versions[0].Preview || versions[1].Name != "1.0.0" {
		t.Fatalf("unexpected versions: %v, %v", versions, err)
	}
}

func TestWindowsSupervisesSuspendedProcess(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows process lifecycle")
	}
	cmd := exec.Command("cmd.exe", "/c", "echo started")
	hideWindow(cmd)
	suspendAtLaunch(cmd)
	var output strings.Builder
	cmd.Stdout = &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	closeTree, _, err := trackProcessTree(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	defer closeTree()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil || !strings.Contains(output.String(), "started") {
			t.Fatalf("process did not run %q, %v", output.String(), err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("process did not resume")
	}
}

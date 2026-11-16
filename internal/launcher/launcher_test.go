package launcher

import (
	"archive/zip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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

func TestUVArchiveExtractsOnlyPinnedExecutable(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "uv.zip")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	for _, name := range []string{"../outside", strings.TrimSuffix(uvArchive, ".zip") + "/uv.exe"} {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte("fixture")); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "uv.exe")
	if err := extractUV(archive, target); err != nil {
		t.Fatal(err)
	}
	if !isFile(target) || isFile(filepath.Join(dir, "..", "outside")) {
		t.Fatal("unexpected extracted files")
	}
}

func TestUVMirrorFailureDoesNotPublishAnUnverifiedExecutable(t *testing.T) {
	attempt := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		if r.URL.Path != "/"+uvVersion+"/"+uvArchive {
			t.Errorf("wrong download path: %s", r.URL.Path)
		}
		if attempt == 1 {
			http.Error(w, "missing", http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte("not the official archive"))
	}))
	defer server.Close()
	target := filepath.Join(t.TempDir(), "uv.exe")
	if err := installUV(context.Background(), target, server.URL); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("missing mirror release was not reported: %v", err)
	}
	if err := installUV(context.Background(), target, server.URL); err == nil || !strings.Contains(err.Error(), "校验失败") {
		t.Fatalf("bad checksum was not reported: %v", err)
	}
	if isFile(target) {
		t.Fatal("unverified download was published")
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

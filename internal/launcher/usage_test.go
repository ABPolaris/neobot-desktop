package launcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// usageTestWrite 在 dir 下创建一个 size 字节的文件，用来拼出可预期的目录占用。
func usageTestWrite(t *testing.T, dir, name string, size int) {
	t.Helper()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	content := make([]byte, size)
	for i := range content {
		content[i] = 'x'
	}
	if err := os.WriteFile(filepath.Join(dir, name), content, 0600); err != nil {
		t.Fatal(err)
	}
}

// usageTestWait 等一次异步统计结束并返回结果。RefreshUsage 在返回前已经把
// Scanning 置为 true，所以这里只要等到它重新变回 false 即可。
func usageTestWait(t *testing.T, m *Manager) Usage {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		usage := m.Snapshot().Usage
		if !usage.Scanning && usage.ScannedAt != 0 {
			return usage
		}
		if time.Now().After(deadline) {
			t.Fatalf("磁盘占用统计没有在限定时间内完成：%+v", usage)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestUsageDirSizeSumsNestedFiles(t *testing.T) {
	root := t.TempDir()
	usageTestWrite(t, root, "a.bin", 10)
	usageTestWrite(t, filepath.Join(root, "sub"), "b.bin", 20)
	usageTestWrite(t, filepath.Join(root, "sub", "deep"), "c.bin", 30)
	if got := usageDirSize(root); got != 60 {
		t.Fatalf("usageDirSize = %d; want 60", got)
	}
	// 目录不存在时返回 0 而不是报错：调用方会拿它统计还没生成的目录。
	if got := usageDirSize(filepath.Join(root, "missing")); got != 0 {
		t.Fatalf("missing dir size = %d; want 0", got)
	}
	// 传进来的是普通文件时统计它自己。生产代码只会传目录，这里锁定的是「不 panic、
	// 也不把文件当成要展开的树」这一行为。
	if got := usageDirSize(filepath.Join(root, "a.bin")); got != 10 {
		t.Fatalf("file size = %d; want 10", got)
	}
}

func TestRefreshUsageReportsEveryCategory(t *testing.T) {
	root := t.TempDir()
	for name, size := range map[string]int{
		"python": 100, "cache": 200, "tools": 300, "tmp": 400,
		"backups": 500, "data": 600, "isolated": 700,
	} {
		usageTestWrite(t, filepath.Join(root, name), "payload.bin", size)
	}
	usageTestWrite(t, filepath.Join(root, "versions", "1.0.0"), "payload.bin", 1000)
	usageTestWrite(t, filepath.Join(root, "versions", "1.1.0"), "payload.bin", 3000)
	usageTestWrite(t, filepath.Join(root, "versions", "dev-branch-a"), "payload.bin", 2000)
	m := &Manager{root: root, installed: map[string]Version{
		"1.0.0":        {Name: "1.0.0"},
		"1.1.0":        {Name: "1.1.0"},
		"dev-branch-a": {Name: "dev-branch-a", Branch: "dev"},
		// 磁盘上没有目录的版本也要列出来，界面才知道它占了 0。
		"1.2.0": {Name: "1.2.0", Preview: true},
	}}
	if err := m.RefreshUsage(); err != nil {
		t.Fatal(err)
	}
	usage := usageTestWait(t, m)
	want := map[string]int64{"python": 100, "cache": 200, "tools": 300, "tmp": 400, "backups": 500, "data": 600, "isolated": 700}
	got := map[string]int64{
		"python": usage.Python, "cache": usage.Cache, "tools": usage.Tools, "tmp": usage.Temp,
		"backups": usage.Backups, "data": usage.Data, "isolated": usage.Isolated,
	}
	for name, size := range want {
		if got[name] != size {
			t.Fatalf("%s = %d; want %d", name, got[name], size)
		}
	}
	if usage.Scanning || usage.Error != "" {
		t.Fatalf("扫描结束后仍是 Scanning=%v Error=%q", usage.Scanning, usage.Error)
	}
	// 版本按占用降序；同大小时按名字排序，保证顺序稳定。
	names := make([]string, 0, len(usage.Versions))
	for _, version := range usage.Versions {
		names = append(names, version.Name)
	}
	if strings.Join(names, ",") != "1.1.0,dev-branch-a,1.0.0,1.2.0" {
		t.Fatalf("版本顺序 = %v; want 1.1.0,dev-branch-a,1.0.0,1.2.0", names)
	}
	if usage.Versions[1].Label != "分支 dev" || usage.Versions[1].Bytes != 2000 {
		t.Fatalf("分支版本的标签或占用不对：%+v", usage.Versions[1])
	}
	if usage.Versions[3].Bytes != 0 {
		t.Fatalf("没有目录的版本占用 = %d; want 0", usage.Versions[3].Bytes)
	}
	// Total 覆盖上面所有类别，包括界面没有逐项展示的 tools。
	if want := int64(2800 + 6000); usage.Total != want {
		t.Fatalf("Total = %d; want %d", usage.Total, want)
	}
}

// 日志要单独列出来，又不能和 data/isolated 重复计算：宿主日志本来就躺在数据目录里。
func TestUsageCountsLogsWithoutDoubleCounting(t *testing.T) {
	root := t.TempDir()
	usageTestWrite(t, filepath.Join(root, "logs"), "launcher.log", 100)
	usageTestWrite(t, filepath.Join(root, "data"), "db.bin", 1000)
	usageTestWrite(t, filepath.Join(root, "data", "logs"), "neobot.log", 200)
	usageTestWrite(t, filepath.Join(root, "isolated", "2.0.1"), "db.bin", 2000)
	usageTestWrite(t, filepath.Join(root, "isolated", "2.0.1", "logs"), "neobot.log", 300)
	m := &Manager{root: root}
	if err := m.RefreshUsage(); err != nil {
		t.Fatal(err)
	}
	usage := usageTestWait(t, m)
	if usage.Logs != 600 {
		t.Fatalf("日志占用 = %d; want 600", usage.Logs)
	}
	if usage.Data != 1000 {
		t.Fatalf("共用数据占用 = %d; want 1000", usage.Data)
	}
	if usage.Isolated != 2000 {
		t.Fatalf("隔离数据占用 = %d; want 2000", usage.Isolated)
	}
	if want := int64(3600); usage.Total != want {
		t.Fatalf("Total = %d; want %d", usage.Total, want)
	}
}

func TestRefreshUsageKeepsRunningScanWhenCalledAgain(t *testing.T) {
	root := t.TempDir()
	usageTestWrite(t, filepath.Join(root, "cache"), "payload.bin", 12)
	m := &Manager{root: root}
	m.mu.Lock()
	m.usageBusy = true
	m.usage = Usage{ScannedAt: 42}
	m.mu.Unlock()
	if err := m.RefreshUsage(); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	busy, usage := m.usageBusy, m.usage
	m.mu.Unlock()
	// 重入的调用不能打断已经在进行中的统计，也不能把界面打回「尚未统计」。
	if !busy || usage.ScannedAt != 42 || usage.Scanning {
		t.Fatalf("重入的 RefreshUsage 改动了正在进行的统计：busy=%v usage=%+v", busy, usage)
	}
}

func TestCleanUsageRejectsUnsafeState(t *testing.T) {
	cases := []struct {
		name    string
		prepare func(*Manager)
		want    string
	}{
		{"closing", func(m *Manager) { m.closing = true }, "正在关闭"},
		{"installing", func(m *Manager) { m.job = Job{State: "running"} }, "任务正在进行"},
		{"starting", func(m *Manager) { m.preparing = true }, "正在启动或停止"},
		{"stopping", func(m *Manager) { m.stopping = true }, "正在启动或停止"},
		{"running", func(m *Manager) { m.bot = &botProcess{} }, "请先停止 NeoBot"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			usageTestWrite(t, filepath.Join(root, "cache"), "payload.bin", 100)
			m := &Manager{root: root, state: diskState{Backups: map[string]string{}}}
			test.prepare(m)
			freed, err := m.CleanUsage("cache")
			if err == nil || freed != 0 || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("CleanUsage = %d, %v; want a refusal containing %q", freed, err, test.want)
			}
			if !isFile(filepath.Join(root, "cache", "payload.bin")) {
				t.Fatal("被拒绝的清理删掉了文件")
			}
		})
	}
}

func TestCleanUsageRefusesWithoutDataRoot(t *testing.T) {
	m := &Manager{}
	if freed, err := m.CleanUsage("cache"); err == nil || freed != 0 || !strings.Contains(err.Error(), "数据目录尚未就绪") {
		t.Fatalf("CleanUsage = %d, %v; want a refusal for an empty root", freed, err)
	}
}

func TestCleanUsageRejectsProtectedAndUnknownTargets(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"python", "versions", "data", "isolated"} {
		usageTestWrite(t, filepath.Join(root, name), "payload.bin", 40)
	}
	m := &Manager{root: root, state: diskState{Backups: map[string]string{}}}
	for _, target := range []string{"python", "versions", "data", "isolated", "root", ""} {
		freed, err := m.CleanUsage(target)
		if err == nil || freed != 0 || !strings.Contains(err.Error(), "未知的清理目标") {
			t.Fatalf("CleanUsage(%q) = %d, %v; want 未知的清理目标", target, freed, err)
		}
		for _, name := range []string{"python", "versions", "data", "isolated"} {
			if !isFile(filepath.Join(root, name, "payload.bin")) {
				t.Fatalf("清理 %q 时删掉了 %s", target, name)
			}
		}
	}
}

func TestCleanUsageRemovesOnlyRequestedDirectory(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"cache", "tmp", "backups", "python", "data"} {
		usageTestWrite(t, filepath.Join(root, name), "payload.bin", 128)
	}
	usageTestWrite(t, filepath.Join(root, "versions", "1.0.0"), "payload.bin", 256)
	m := &Manager{root: root, state: diskState{Backups: map[string]string{}},
		installed: map[string]Version{"1.0.0": {Name: "1.0.0"}}}
	for _, target := range []string{"cache", "tmp"} {
		freed, err := m.CleanUsage(target)
		if err != nil || freed != 128 {
			t.Fatalf("CleanUsage(%q) = %d, %v; want 128", target, freed, err)
		}
		if _, statErr := os.Stat(filepath.Join(root, target)); !os.IsNotExist(statErr) {
			t.Fatalf("清理 %q 后目录仍然存在：%v", target, statErr)
		}
		for _, keep := range []string{"backups", "python", "data", "versions"} {
			if !isDir(filepath.Join(root, keep)) {
				t.Fatalf("清理 %q 时误删了 %s", target, keep)
			}
		}
		usage := usageTestWait(t, m)
		size := usage.Cache
		if target == "tmp" {
			size = usage.Temp
		}
		if size != 0 {
			t.Fatalf("清理 %q 后占用 = %d; want 0", target, size)
		}
	}
	// 目标目录已经不存在时返回 0 而不是报错，界面据此显示「没有可清理的内容」。
	if freed, err := m.CleanUsage("cache"); err != nil || freed != 0 {
		t.Fatalf("重复清理 = %d, %v; want 0, nil", freed, err)
	}
	usage := usageTestWait(t, m)
	if usage.Cache != 0 || usage.Total == 0 {
		t.Fatalf("清理后的统计不对：%+v", usage)
	}
}

func TestCleanUsageBackupsClearsSnapshotRecords(t *testing.T) {
	root := t.TempDir()
	const snapshot = "1234567890123456789"
	usageTestWrite(t, filepath.Join(root, "backups", snapshot), "config.toml", 64)
	m := &Manager{root: root, state: diskState{Backups: map[string]string{"1.0.0": snapshot}}}
	if err := m.persist(); err != nil {
		t.Fatal(err)
	}
	freed, err := m.CleanUsage("backups")
	if err != nil || freed != 64 {
		t.Fatalf("CleanUsage(backups) = %d, %v; want 64", freed, err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "backups", snapshot)); !os.IsNotExist(statErr) {
		t.Fatalf("快照目录没有被删除：%v", statErr)
	}
	m.mu.Lock()
	records := len(m.state.Backups)
	m.mu.Unlock()
	if records != 0 {
		t.Fatalf("内存里还剩 %d 条快照记录", records)
	}
	// settings.json 里必须同步清空，否则下次降级会按旧记录去找已经不存在的快照。
	var saved diskState
	if err := readJSON(filepath.Join(root, "settings.json"), &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.Backups) != 0 {
		t.Fatalf("settings.json 仍然指向已删除的快照：%v", saved.Backups)
	}
	usage := usageTestWait(t, m)
	if usage.Backups != 0 {
		t.Fatalf("清理后快照占用 = %d; want 0", usage.Backups)
	}
}

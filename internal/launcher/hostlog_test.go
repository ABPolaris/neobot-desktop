package launcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// hostLogTestWrite 写一个文件并固定修改时间，便于验证按时间排序。
func hostLogTestWrite(t *testing.T, path, content string, at time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

func TestHostLogFilesListsRotatedLogsNewestFirst(t *testing.T) {
	root := t.TempDir()
	logs := filepath.Join(root, "data", "logs")
	base := time.Now().Add(-time.Hour)
	hostLogTestWrite(t, filepath.Join(logs, "neobot.log"), "now\n", base.Add(30*time.Minute))
	hostLogTestWrite(t, filepath.Join(logs, "neobot.2026-11-19_10-00-00.log"), "old\n", base)
	hostLogTestWrite(t, filepath.Join(logs, "notes.txt"), "ignore\n", base)
	if err := os.MkdirAll(filepath.Join(logs, "neobot.folder"), 0700); err != nil {
		t.Fatal(err)
	}
	m := &Manager{root: root}
	files, err := m.HostLogFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("只应列出 neobot.log 系列，实际 %+v", files)
	}
	if files[0].Name != "neobot.log" || files[1].Name != "neobot.2026-11-19_10-00-00.log" {
		t.Fatalf("排序错误：%+v", files)
	}
	if files[0].Bytes != 4 || files[0].At == 0 {
		t.Fatalf("元信息错误：%+v", files[0])
	}
}

func TestHostLogFilesWithoutDirectoryIsEmpty(t *testing.T) {
	m := &Manager{root: t.TempDir()}
	files, err := m.HostLogFiles()
	if err != nil {
		t.Fatalf("数据目录不存在不应报错：%v", err)
	}
	if len(files) != 0 {
		t.Fatalf("files = %+v", files)
	}
}

func TestReadHostLogTailAndPaging(t *testing.T) {
	root := t.TempDir()
	var content strings.Builder
	for i := 1; i <= 1200; i++ {
		fmt.Fprintf(&content, "line %d\n", i)
	}
	hostLogTestWrite(t, filepath.Join(root, "data", "logs", "neobot.log"), content.String(), time.Now())
	m := &Manager{root: root}

	tail, err := m.ReadHostLog("neobot.log", -1, 500)
	if err != nil {
		t.Fatal(err)
	}
	if tail.Total != 1200 || tail.Start != 700 || len(tail.Lines) != 500 {
		t.Fatalf("尾页 = %d 行，起点 %d，总数 %d", len(tail.Lines), tail.Start, tail.Total)
	}
	if tail.Lines[0] != "line 701" || tail.Lines[499] != "line 1200" {
		t.Fatalf("尾页内容错误：%q ... %q", tail.Lines[0], tail.Lines[499])
	}

	older, err := m.ReadHostLog("neobot.log", tail.Start-500, 500)
	if err != nil {
		t.Fatal(err)
	}
	if older.Start != 200 || len(older.Lines) != 500 || older.Lines[0] != "line 201" {
		t.Fatalf("上一页 = 起点 %d，%d 行，首行 %q", older.Start, len(older.Lines), older.Lines[0])
	}

	first, err := m.ReadHostLog("neobot.log", 0, 10)
	if err != nil || first.Start != 0 || len(first.Lines) != 10 || first.Lines[0] != "line 1" {
		t.Fatalf("第一页 = %+v, %v", first, err)
	}

	// count 上限 2000：超出的部分被截断，而不是把整个文件塞进内存。
	capped, err := m.ReadHostLog("neobot.log", 0, 99999)
	if err != nil || len(capped.Lines) != 1200 {
		t.Fatalf("capped = %d 行, %v", len(capped.Lines), err)
	}

	// 起点越过末尾：返回空页，但总行数依然正确。
	empty, err := m.ReadHostLog("neobot.log", 5000, 100)
	if err != nil || len(empty.Lines) != 0 || empty.Start != 1200 || empty.Total != 1200 {
		t.Fatalf("越界页 = %+v, %v", empty, err)
	}
}

func TestReadHostLogStripsAnsiAndSecrets(t *testing.T) {
	root := t.TempDir()
	line := "\x1b[31mapi_key = \"sk-secret-123\"\x1b[0m 完成\n"
	hostLogTestWrite(t, filepath.Join(root, "data", "logs", "neobot.log"), line, time.Now())
	m := &Manager{root: root}
	page, err := m.ReadHostLog("neobot.log", -1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Lines) != 1 {
		t.Fatalf("lines = %q", page.Lines)
	}
	if strings.Contains(page.Lines[0], "sk-secret") || strings.Contains(page.Lines[0], "\x1b") {
		t.Fatalf("没有清理干净：%q", page.Lines[0])
	}
	if !strings.Contains(page.Lines[0], "[已隐藏]") {
		t.Fatalf("凭据没有打码：%q", page.Lines[0])
	}
}

func TestReadHostLogRejectsUnsafeNames(t *testing.T) {
	root := t.TempDir()
	hostLogTestWrite(t, filepath.Join(root, "data", "logs", "neobot.log"), "ok\n", time.Now())
	hostLogTestWrite(t, filepath.Join(root, "data", "settings.json"), "{}\n", time.Now())
	hostLogTestWrite(t, filepath.Join(root, "neobot.log"), "outside\n", time.Now())
	m := &Manager{root: root}
	names := []string{
		"", "..", "../settings.json", `..\settings.json`, "settings.json", "C:neobot.log",
		"neobot.log/../settings.json", "neobot.logx",
	}
	for _, name := range names {
		if _, err := m.ReadHostLog(name, 0, 10); err == nil {
			t.Fatalf("%q 应该被拒绝", name)
		}
	}
	// 目录即使叫 neobot.log 也不能读。
	if err := os.MkdirAll(filepath.Join(root, "data", "logs", "neobot.dir"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ReadHostLog("neobot.dir", 0, 10); err == nil {
		t.Fatal("目录不该被当成日志文件")
	}
}

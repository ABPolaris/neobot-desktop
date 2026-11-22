//go:build linux

package launcher

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLockInstanceRejectsSecondHolder 验证同一个数据目录只能被一个启动器锁定，
// 释放后可以重新获取（进程退出时内核也会自动释放）。
func TestLockInstanceRejectsSecondHolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "desktop.lock")
	release, err := lockInstance(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lockInstance(path); err == nil || !strings.Contains(err.Error(), "已经在运行") {
		t.Fatalf("second lock = %v; want the already-running error", err)
	}
	release()
	again, err := lockInstance(path)
	if err != nil {
		t.Fatalf("released lock could not be taken again: %v", err)
	}
	again()
}

// TestTrackProcessTreeKillsGroup 验证 Linux 用进程组替代 Job Object：
// 组内所有进程都能被判为「自己启动的」，强制停止会把整组收掉。
func TestTrackProcessTreeKillsGroup(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 30 & wait")
	prepareLaunch(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	closeTree, ownsPID, err := trackProcessTree(cmd.Process.Pid)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal(err)
	}
	if !ownsPID(cmd.Process.Pid) {
		t.Fatal("the launched process was not recognized as owned")
	}
	if ownsPID(os.Getpid()) {
		t.Fatal("the launcher itself was reported as owned")
	}
	closeTree()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("process group was not terminated")
	}
}

func TestAppDataUsesXDGDataHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	got, err := AppData()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "NeoBotDesktop"); got != want {
		t.Fatalf("AppData = %q; want %q", got, want)
	}
}

func TestFreeDiskSpaceReportsAvailableBytes(t *testing.T) {
	free, err := freeDiskSpace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if free <= 0 {
		t.Fatalf("free disk space = %d; want a positive number", free)
	}
}

// TestSamePathIsCaseSensitive 验证本机控制接口的 data_dir 校验不会把大小写不同的
// 两个目录当成同一个（Windows 上则相反，见 platform_windows.go）。
func TestSamePathIsCaseSensitive(t *testing.T) {
	if samePath("/tmp/neobot", "/tmp/NeoBot") {
		t.Fatal("case-different paths were treated as the same")
	}
	if !samePath("/tmp/./neobot", "/tmp/neobot") {
		t.Fatal("cleaned paths were not treated as the same")
	}
}

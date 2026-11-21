//go:build windows

package launcher

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// openTerminalWindow 用系统 cmd.exe 打开一个新控制台，并进入已经准备好的环境。
//
// 这里用 `cmd /c start` 而不是直接给 cmd.exe 设置 CREATE_NEW_CONSOLE：Wails
// 是 GUI 进程，os/exec 默认会把子进程的标准句柄接到 NUL；交给 start 创建
// 新控制台，Windows 会把标准句柄正确接到新窗口上。Windows 11 的「默认终端
// 应用」设置成 Windows Terminal 时，start 创建的控制台也会自动由它承载。
func openTerminalWindow(dir string, env []string) error {
	shell := comspec()
	cmd := exec.Command(shell, "/d", "/c", "start", "", shell, "/d", "/k", "chcp 65001>nul")
	cmd.Dir = dir
	cmd.Env = env
	// 不调用 hideWindow：终端窗口必须可见。
	return cmd.Run()
}

// comspec 返回系统自带的命令解释器。优先 ComSpec，其次 SystemRoot 下的
// cmd.exe；ComSpec 指向不存在的路径时也要能回退，避免完全打不开终端。
func comspec() string {
	if value := strings.Trim(strings.TrimSpace(os.Getenv("ComSpec")), `"`); value != "" && isFile(value) {
		return value
	}
	if root := os.Getenv("SystemRoot"); root != "" {
		if candidate := filepath.Join(root, "System32", "cmd.exe"); isFile(candidate) {
			return candidate
		}
	}
	return "cmd.exe"
}

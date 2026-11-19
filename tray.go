package main

// trayCommands 是托盘菜单能触发的动作。所有回调都在非 UI 线程被调用，
// 涉及窗口的调用必须自己切回 Wails 的运行时。
type trayCommands struct {
	Show      func()
	Start     func()
	Stop      func()
	Dashboard func()
	OpenData  func()
	Diagnose  func()
	Quit      func()
}

// trayTipLimit 是托盘提示文本能容纳的字符数：Windows 的 szTip 有 128 个
// UTF-16 单元，末尾还要留一个字符串结束符。
const trayTipLimit = 127

// clampTrayTip 按 UTF-16 单元数截断托盘提示；超长的提示会被 Windows 整条忽略，
// 所以必须在写进 NOTIFYICONDATA 之前截短。
func clampTrayTip(text string) string {
	units := 0
	for i, r := range text {
		units++
		if r > 0xFFFF { // 基本平面以外的字符在 UTF-16 里占两个单元
			units++
		}
		if units > trayTipLimit {
			return text[:i]
		}
	}
	return text
}

// 托盘的生命周期由平台文件实现：
//   tray_windows.go  用 Win32 的 Shell_NotifyIcon 真的挂一个托盘图标；
//   tray_other.go    其他平台没有实现，托盘不可用。
//
// 下面这些函数在两边签名一致，startTray 不能阻塞也不能 panic，
// 初始化失败后 trayAvailable 必须返回 false，主窗口行为保持原样。

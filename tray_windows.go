//go:build windows

package main

import (
	"errors"
	"os"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	// trayWindowClass 是托盘隐藏窗口的类名，整个进程只注册一次。
	trayWindowClass = "NeoBotDesktopTray"

	// trayIconID 是托盘图标在这个窗口下的编号。
	trayIconID = 1

	// trayCallbackMessage = WM_APP + 1，托盘把鼠标事件发到这条消息上。
	trayCallbackMessage = 0x8001

	// Shell_NotifyIcon 的动作。
	trayIconAdd    = 0x00000000
	trayIconModify = 0x00000001
	trayIconDelete = 0x00000002

	// NOTIFYICONDATA.uFlags。
	trayFlagMessage = 0x00000001
	trayFlagIcon    = 0x00000002
	trayFlagTip     = 0x00000004

	// 窗口消息。
	wmDestroy       = 0x0002
	wmClose         = 0x0010
	wmCommand       = 0x0111
	wmNull          = 0x0000
	wmLButtonDblClk = 0x0203
	wmRButtonUp     = 0x0205

	// 窗口样式：托盘窗口不显示，样式留空即可。
	wsOverlapped = 0x00000000

	// taskbarCreatedMessage 是资源管理器重启时广播的消息，用来重新挂图标。

	// 菜单项 ID。
	trayMenuShow      = 1
	trayMenuStart     = 2
	trayMenuStop      = 3
	trayMenuDashboard = 4
	trayMenuOpenData  = 5
	trayMenuDiagnose  = 6
	trayMenuQuit      = 7

	// 菜单标志。
	menuString    = 0x00000000
	menuGrayed    = 0x00000001
	menuSeparator = 0x00000800

	// TrackPopupMenu 标志：右键弹出，并且自己返回被点的菜单项。
	trackRightButton = 0x0002
	trackReturnCmd   = 0x0100

	// defaultAppIcon 是 IDI_APPLICATION：取不到程序图标时的兜底。
	defaultAppIcon = 32512
)

// wndClassExW 对应 WNDCLASSEXW。
type wndClassExW struct {
	size       uint32
	style      uint32
	wndProc    uintptr
	clsExtra   int32
	wndExtra   int32
	instance   windows.Handle
	icon       windows.Handle
	cursor     windows.Handle
	background windows.Handle
	menuName   *uint16
	className  *uint16
	iconSmall  windows.Handle
}

// msgW 对应 MSG。字段顺序和大小必须和 Windows 一致。
type msgW struct {
	hwnd    windows.Handle
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      pointW
	private uint32
}

// pointW 对应 POINT。
type pointW struct {
	x int32
	y int32
}

// notifyIconDataW 对应 NOTIFYICONDATAW。字段顺序和大小必须和 Windows 一致，
// cbSize 传整个结构的大小，Windows 会按这个大小判断能用到哪个版本。
type notifyIconDataW struct {
	size      uint32
	wnd       windows.Handle
	id        uint32
	flags     uint32
	callback  uint32
	icon      windows.Handle
	tip       [128]uint16
	state     uint32
	stateMask uint32
	info      [256]uint16
	version   uint32
	infoTitle [64]uint16
	infoFlags uint32
	guid      windows.GUID
	balloon   windows.Handle
}

var (
	trayUser32   = windows.NewLazySystemDLL("user32.dll")
	trayShell32  = windows.NewLazySystemDLL("shell32.dll")
	trayKernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procRegisterClassExW    = trayUser32.NewProc("RegisterClassExW")
	trayProcRegisterMsg     = trayUser32.NewProc("RegisterWindowMessageW")
	procCreateWindowExW     = trayUser32.NewProc("CreateWindowExW")
	procDefWindowProcW      = trayUser32.NewProc("DefWindowProcW")
	procDestroyWindow       = trayUser32.NewProc("DestroyWindow")
	procGetMessageW         = trayUser32.NewProc("GetMessageW")
	procTranslateMessage    = trayUser32.NewProc("TranslateMessage")
	procDispatchMessageW    = trayUser32.NewProc("DispatchMessageW")
	procPostQuitMessage     = trayUser32.NewProc("PostQuitMessage")
	procPostMessageW        = trayUser32.NewProc("PostMessageW")
	procCreatePopupMenu     = trayUser32.NewProc("CreatePopupMenu")
	procAppendMenuW         = trayUser32.NewProc("AppendMenuW")
	procDestroyMenu         = trayUser32.NewProc("DestroyMenu")
	procTrackPopupMenu      = trayUser32.NewProc("TrackPopupMenu")
	procSetForegroundWindow = trayUser32.NewProc("SetForegroundWindow")
	procGetCursorPos        = trayUser32.NewProc("GetCursorPos")
	procLoadIconW           = trayUser32.NewProc("LoadIconW")
	procShellNotifyIconW    = trayShell32.NewProc("Shell_NotifyIconW")
	procExtractIconExW      = trayShell32.NewProc("ExtractIconExW")
	procGetModuleHandleW    = trayKernel32.NewProc("GetModuleHandleW")
)

var (
	// trayMu 保护托盘在「调用方线程」和「托盘线程」之间共享的状态。
	trayMu             sync.Mutex
	trayWindow         windows.Handle
	trayIconData       notifyIconDataW
	trayCmds           trayCommands
	trayRunning        bool
	trayLastTip        string
	trayTaskbarCreated uint32
	trayStarted        bool
	trayBroken         bool // 初始化失败，之后一律当作没有托盘
	trayStopped        bool // 已经显式停止，之后不能再把窗口藏起来
)

// trayAvailable 报告托盘现在是否可用。初始化失败或已经停止后返回 false，
// 这样主窗口就不会把窗口藏进一个不存在的托盘里。
// trayInstalled 报告托盘图标是否真的挂上了。trayAvailable 在初始化完成前也为 true，
// 所以判断「这台机器上有托盘」必须用这个函数。
func trayInstalled() bool {
	trayMu.Lock()
	defer trayMu.Unlock()
	return trayWindow != 0 && !trayBroken && !trayStopped
}

func trayAvailable() bool {
	trayMu.Lock()
	defer trayMu.Unlock()
	return !trayBroken && !trayStopped
}

// startTray 在独立的 OS 线程上创建托盘图标和它自己的消息循环，函数立即返回。
// 任何一步失败都必须静默降级：托盘不可用时主窗口行为保持原样。
func startTray(commands trayCommands) {
	trayMu.Lock()
	if trayStarted {
		trayMu.Unlock()
		return
	}
	trayStarted = true
	trayCmds = commands
	trayMu.Unlock()
	go runTray()
}

// runTray 是托盘专用的消息循环。主线程被 Wails 的消息循环占着，
// 而且窗口消息只会投递给创建窗口的线程，所以这里必须锁住自己的 OS 线程。
func runTray() {
	defer func() {
		// Win32 调用出意外不该把整个启动器带走：托盘不可用就静默降级。
		if recover() != nil {
			markTrayBroken()
		}
	}()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := createTrayWindow(); err != nil {
		markTrayBroken()
		return
	}
	var message msgW
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if int32(ret) <= 0 { // 0 是 WM_QUIT，-1 表示取消息出错
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&message)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&message)))
	}
	finishTray()
}

// createTrayWindow 注册窗口类、建隐藏窗口并把图标挂到托盘上。
func createTrayWindow() error {
	instance, _, _ := procGetModuleHandleW.Call(0)
	if instance == 0 {
		return errors.New("取不到应用程序模块句柄")
	}
	className, err := windows.UTF16PtrFromString(trayWindowClass)
	if err != nil {
		return err
	}
	class := wndClassExW{
		wndProc:   windows.NewCallback(trayWindowProc),
		instance:  windows.Handle(instance),
		className: className,
	}
	class.size = uint32(unsafe.Sizeof(class))
	if atom, _, _ := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&class))); atom == 0 {
		return errors.New("注册托盘窗口类失败")
	}
	hwnd, _, _ := procCreateWindowExW.Call(0,
		uintptr(unsafe.Pointer(className)), 0, wsOverlapped, 0, 0, 0, 0, 0, 0, instance, 0)
	if hwnd == 0 {
		return errors.New("创建托盘窗口失败")
	}
	// 托盘可能在初始化途中就被要求停止（用户很快点了退出），这时不要留下图标。
	trayMu.Lock()
	stopped := trayStopped
	trayMu.Unlock()
	if stopped {
		procDestroyWindow.Call(hwnd)
		return errors.New("托盘已经停止")
	}
	data := notifyIconDataW{
		wnd:      windows.Handle(hwnd),
		id:       trayIconID,
		flags:    trayFlagMessage | trayFlagIcon | trayFlagTip,
		callback: trayCallbackMessage,
		icon:     extractAppIcon(),
	}
	data.size = uint32(unsafe.Sizeof(data))
	copyTrayText(data.tip[:], "NeoBot Desktop")
	if added, _, _ := procShellNotifyIconW.Call(trayIconAdd, uintptr(unsafe.Pointer(&data))); added == 0 {
		procDestroyWindow.Call(hwnd)
		return errors.New("添加托盘图标失败")
	}
	// 资源管理器重启会重建通知区域，届时它广播一条 TaskbarCreated；记下消息号以便重挂图标。
	if name, convErr := windows.UTF16PtrFromString("TaskbarCreated"); convErr == nil {
		if id, _, _ := trayProcRegisterMsg.Call(uintptr(unsafe.Pointer(name))); id != 0 {
			trayTaskbarCreated = uint32(id)
		}
	}
	trayMu.Lock()
	trayWindow, trayIconData = windows.Handle(hwnd), data
	trayMu.Unlock()
	return nil
}

// extractAppIcon 取可执行文件里编号 0 的图标；取不到就退回系统默认应用图标。
// 图标句柄跟着进程走，不主动销毁：系统图标是共享句柄，退出时系统会自己回收。
func extractAppIcon() windows.Handle {
	if path, err := os.Executable(); err == nil {
		if name, convErr := windows.UTF16PtrFromString(path); convErr == nil {
			var large, small windows.Handle
			count, _, _ := procExtractIconExW.Call(uintptr(unsafe.Pointer(name)), 0,
				uintptr(unsafe.Pointer(&large)), uintptr(unsafe.Pointer(&small)), 1)
			if count > 0 && large != 0 {
				return large
			}
		}
	}
	icon, _, _ := procLoadIconW.Call(0, defaultAppIcon)
	return windows.Handle(icon)
}

// trayWindowProc 是托盘窗口的消息处理函数。它跑在托盘线程上，绝不能长时间阻塞，
// 所以菜单动作一律交给新的 goroutine。
func trayWindowProc(hwnd windows.Handle, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case trayCallbackMessage:
		switch lParam {
		case wmLButtonDblClk:
			runTrayAction(trayMenuShow)
		case wmRButtonUp:
			showTrayMenu(hwnd)
		}
		return 0
	case wmCommand:
		runTrayAction(int(wParam & 0xffff))
		return 0
	case wmClose:
		procDestroyWindow.Call(uintptr(hwnd))
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	if trayTaskbarCreated != 0 && message == trayTaskbarCreated {
		// 通知区域被重建：重挂一次图标，否则图标永远消失，而窗口还能被藏起来。
		trayMu.Lock()
		data, window := trayIconData, trayWindow
		trayLastTip = ""
		trayMu.Unlock()
		if window != 0 {
			if ok, _, _ := procShellNotifyIconW.Call(trayIconAdd, uintptr(unsafe.Pointer(&data))); ok == 0 {
				markTrayBroken()
			}
		}
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return ret
}

// showTrayMenu 在鼠标位置弹出托盘菜单。TrackPopupMenu 会自己跑一段模态消息循环，
// 前后必须处理前台窗口，否则点菜单外面时菜单不会消失。
func showTrayMenu(hwnd windows.Handle) {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)
	trayMu.Lock()
	running := trayRunning
	trayMu.Unlock()
	appendTrayItem(menu, trayMenuShow, "显示主窗口", false)
	appendTrayItem(menu, trayMenuStart, "启动 NeoBot", running)
	appendTrayItem(menu, trayMenuStop, "停止 NeoBot", !running)
	appendTraySeparator(menu)
	appendTrayItem(menu, trayMenuDashboard, "打开网页管理", false)
	appendTrayItem(menu, trayMenuOpenData, "打开数据目录", false)
	appendTraySeparator(menu)
	appendTrayItem(menu, trayMenuDiagnose, "运行体检", false)
	appendTraySeparator(menu)
	appendTrayItem(menu, trayMenuQuit, "退出", false)

	var point pointW
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&point)))
	procSetForegroundWindow.Call(uintptr(hwnd))
	command, _, _ := procTrackPopupMenu.Call(menu, trackRightButton|trackReturnCmd,
		uintptr(point.x), uintptr(point.y), 0, uintptr(hwnd), 0)
	// 菜单关掉后补一条空消息，让窗口退出前台状态，否则下一次点托盘图标会被吞掉。
	procPostMessageW.Call(uintptr(hwnd), wmNull, 0, 0)
	if command != 0 {
		runTrayAction(int(command))
	}
}

// runTrayAction 执行一个托盘菜单动作。动作可能阻塞好几秒（停止 NeoBot、
// 打开浏览器），必须放到新的 goroutine 里，否则托盘消息循环会跟着卡住。
func runTrayAction(id int) {
	trayMu.Lock()
	commands := trayCmds
	trayMu.Unlock()
	var work func()
	switch id {
	case trayMenuShow:
		work = commands.Show
	case trayMenuStart:
		work = commands.Start
	case trayMenuStop:
		work = commands.Stop
	case trayMenuDashboard:
		work = commands.Dashboard
	case trayMenuOpenData:
		work = commands.OpenData
	case trayMenuDiagnose:
		work = commands.Diagnose
	case trayMenuQuit:
		work = commands.Quit
	}
	if work != nil {
		go work()
	}
}

// updateTrayStatus 更新托盘提示文本（运行状态 + 版本）。只发 NIF_TIP：
// 图标不重画，避免每次状态变化托盘都闪一下。
func updateTrayStatus(running bool, tooltip string) {
	trayMu.Lock()
	data, ready := trayIconData, trayWindow != 0 && !trayBroken
	trayRunning = running
	trayMu.Unlock()
	if !ready {
		return
	}
	// 运行中 poll 每秒都会同步一次状态，文本没变就不必再打一次系统调用。
	trayMu.Lock()
	if trayLastTip == tooltip {
		trayMu.Unlock()
		return
	}
	trayMu.Unlock()
	data.flags = trayFlagTip
	copyTrayText(data.tip[:], clampTrayTip(tooltip))
	// 只有真写成功了才记下来：explorer 重启后图标没了，这里要能重试。
	if ok, _, _ := procShellNotifyIconW.Call(trayIconModify, uintptr(unsafe.Pointer(&data))); ok != 0 {
		trayMu.Lock()
		trayLastTip = tooltip
		trayMu.Unlock()
	}
}

// stopTray 移除托盘图标并结束它的消息循环，可重复调用。
func stopTray() {
	trayMu.Lock()
	hwnd, data, started := trayWindow, trayIconData, trayStarted
	trayStopped = true
	trayWindow = 0
	trayMu.Unlock()
	if !started || hwnd == 0 {
		return
	}
	procShellNotifyIconW.Call(trayIconDelete, uintptr(unsafe.Pointer(&data)))
	// 窗口只能在创建它的线程里销毁，这里只投递一条关闭消息。
	procPostMessageW.Call(uintptr(hwnd), wmClose, 0, 0)
}

// finishTray 在消息循环结束后清掉可能残留的托盘图标。循环不是 stopTray 结束的，
// 说明托盘自己出了问题，之后不能再把主窗口藏起来。
func finishTray() {
	trayMu.Lock()
	hwnd, data := trayWindow, trayIconData
	trayWindow = 0
	stopped := trayStopped
	trayMu.Unlock()
	if hwnd != 0 {
		procShellNotifyIconW.Call(trayIconDelete, uintptr(unsafe.Pointer(&data)))
	}
	if !stopped {
		markTrayBroken()
	}
}

// markTrayBroken 把托盘标记成不可用。
func markTrayBroken() {
	trayMu.Lock()
	trayBroken, trayWindow = true, 0
	trayMu.Unlock()
}

// copyTrayText 把字符串写进固定长度的 UTF-16 缓冲区，并保证末尾有结束符。
func copyTrayText(buffer []uint16, text string) {
	clear(buffer)
	units, err := windows.UTF16FromString(text)
	if err != nil || len(units) == 0 {
		return
	}
	if len(units) > len(buffer) {
		units = units[:len(buffer)]
		units[len(units)-1] = 0
	}
	copy(buffer, units)
}

// appendTrayItem 往弹出菜单里加一项。
func appendTrayItem(menu uintptr, id int, label string, grayed bool) {
	text, err := windows.UTF16PtrFromString(label)
	if err != nil {
		return
	}
	flags := uintptr(menuString)
	if grayed {
		flags |= menuGrayed
	}
	procAppendMenuW.Call(menu, flags, uintptr(id), uintptr(unsafe.Pointer(text)))
}

// appendTraySeparator 往弹出菜单里加一条分隔线。
func appendTraySeparator(menu uintptr) {
	procAppendMenuW.Call(menu, menuSeparator, 0, 0)
}

//go:build !windows

package launcher

import "errors"

// setAutostartEntry 只在 Windows 上有实现：开机自启依赖 HKCU 的 Run 项，
// 其他系统请用系统自带的启动项设置。
func setAutostartEntry(bool) error {
	return errors.New("当前系统不支持自动写入开机自启，请在系统的启动项设置里手动添加")
}

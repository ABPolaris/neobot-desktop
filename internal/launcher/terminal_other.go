//go:build !windows

package launcher

import "errors"

func openTerminalWindow(string, []string) error {
	return errors.New("终端仅支持 Windows")
}

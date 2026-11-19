//go:build !windows

package launcher

import (
	"errors"
	"os"
	"os/exec"
)

func freeDiskSpace(string) (int64, error) {
	return 0, errors.New("暂不支持读取磁盘剩余空间")
}
func hideWindow(*exec.Cmd)                    {}
func replaceFile(source, target string) error { return os.Rename(source, target) }
func lockInstance(string) (func(), error)     { return nil, errors.New("首版仅支持 Windows x64") }
func suspendAtLaunch(*exec.Cmd)               {}
func trackProcessTree(int) (func(), func(int) bool, error) {
	return nil, nil, errors.New("首版仅支持 Windows x64")
}

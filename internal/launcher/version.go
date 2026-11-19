package launcher

// currentVersion 是启动器自身的版本号。qegg 只维护 frontend/package.json，
// main 包启动时把它读出来注入这里，Go 侧不再单独维护一份版本号。
var currentVersion = "0.0.0"

// SetCurrentVersion 注入启动器版本号。
func SetCurrentVersion(v string) {
	if v != "" {
		currentVersion = v
	}
}

// CurrentVersion 返回启动器版本号。
func CurrentVersion() string { return currentVersion }

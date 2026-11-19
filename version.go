package main

import (
	_ "embed"
	"encoding/json"
)

//go:embed frontend/package.json
var frontendPackageJSON []byte

// appVersion 读取 frontend/package.json 的版本号。发布流程只改这一个文件，
// 因此 Go 侧（自更新比较、诊断包）也必须从这里取，避免两处版本号漂移。
func appVersion() string {
	var meta struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(frontendPackageJSON, &meta); err != nil || meta.Version == "" {
		return "0.0.0"
	}
	return meta.Version
}

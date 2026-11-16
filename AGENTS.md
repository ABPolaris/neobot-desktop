# AGENTS.md

补充规则见 [`.rules`](.rules)，编写前必须阅读。处理相关任务时，请遵循其中适用的规则。

## 项目与源码入口

- `neobot-desktop` 是基于 Wails v2 的桌面应用，后端用 Go，前端用 React、TypeScript 和 Vite。Go 版本以 `go.mod` 为准。
- NeoBot 主项目仓库：[SuperQuail/NeoBot](https://github.com/SuperQuail/NeoBot)。
- `main.go` 是应用入口，负责窗口配置、前端资源嵌入和 Go 对象绑定。`app.go` 定义当前暴露给前端的 Go 方法。
- `frontend/src/` 是前端源码；`wails.json` 定义 Wails 调用的前端安装、开发和构建命令。
- `build/` 包含应用图标和平台打包配置。`build/bin/` 是构建产物，打包所需的源文件应保留在版本库中。

## 开发与验证

- 在仓库根目录运行 `wails dev` 启动桌面开发环境，运行 `wails build` 构建应用。
- 改动 Go 代码时，用 `gofmt` 格式化改动文件，并在根目录运行 `go test ./...`。
- 只改前端时，可在 `frontend/` 运行 `npm run build`，完成 TypeScript 检查和 Vite 构建。缺少依赖时先运行 `npm install`；缺少 Go 绑定时先按下节生成。
- 改动 Go 与前端之间的接口时，重新生成绑定、构建前端，并在 `wails dev` 中验证受影响的调用。改动窗口配置或打包配置时，验证实际构建产物。
- 只改文档时，检查链接、路径和命令即可。汇报时说明实际运行了哪些检查，以及哪些没能运行。

## Wails 生成文件

- `frontend/wailsjs/` 下的 Go 绑定、TypeScript 声明和运行时由 Wails 生成。修改绑定时改 Go 源码，不手改或提交生成文件。
- `wails dev` 和 `wails build` 默认生成绑定。只需刷新绑定时，可在仓库根目录运行 `wails generate module`。
- `main.go` 嵌入了 `frontend/dist/`。全新检出时，如果直接运行 `go test ./...` 或 `wails generate module` 遇到嵌入路径不存在的错误，先运行 `wails build` 初始化构建产物。
- `frontend/wailsjs/`、`frontend/dist/` 和 `build/bin/` 均由 Git 忽略。

## 版本与发布

- QuailEgg 的版本管理范围以 `.quailegg.toml` 为准：根目录的 `go@neobot-desktop` 和 `frontend/` 下的 `npm@frontend`。前端包不发布到 npm；`@wailsapp/runtime` 不纳入版本管理。
- 面向用户的变更按 [`.changes/README.md`](.changes/README.md) 添加变更记录。用 `qegg plan` 预览发布计划；只需要生成记录时，用 `qegg commit --no-commit`。
- 发布由 `.github/workflows/qegg-release.yml` 处理。日常功能和修复通过变更记录表达版本影响，版本号和发布标签交给发布流程。

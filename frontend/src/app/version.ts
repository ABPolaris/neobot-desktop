// 启动器版本号，构建时由 vite 从 frontend/package.json 注入。
export const APP_VERSION = typeof __APP_VERSION__ === 'string' ? __APP_VERSION__ : 'unknown'

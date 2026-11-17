import {defineConfig} from 'vite'
import react from '@vitejs/plugin-react'
import pkg from './package.json'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  // 版本号只有 package.json 一处来源，由发布流程统一升级。
  define: {__APP_VERSION__: JSON.stringify(pkg.version)},
  server: {
    host: '127.0.0.1',
    port: 5187,
    strictPort: true,
  },
})

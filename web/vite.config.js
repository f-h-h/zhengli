import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// 开发期把 /api 代理到本地 Go 后端，生产构建后走同源。
export default defineConfig({
  plugins: [vue()],
  build: {
    // Scene3D 独立 chunk 含 three.js，阈值放宽以消除噪音警告。
    chunkSizeWarningLimit: 600,
  },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})

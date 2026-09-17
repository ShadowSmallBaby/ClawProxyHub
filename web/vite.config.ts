import { writeFileSync } from 'node:fs'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [
    vue(),
    // dist/.gitkeep 入库占位，让 go:embed 在未构建时也能编译；构建会清空 dist，这里补回
    { name: 'keep-dist-placeholder', closeBundle: () => writeFileSync('dist/.gitkeep', '') },
  ],
  server: {
    port: 5173,
    proxy: {
      '/admin': 'http://127.0.0.1:8080',
      '/v1': 'http://127.0.0.1:8080',
      '/assets': 'http://127.0.0.1:8080',
    },
  },
  build: {
    outDir: 'dist',
  },
})

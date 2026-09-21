import { writeFileSync } from 'node:fs'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// vendor 拆分：框架 / TDesign / echarts 独立 chunk，业务改动不影响其缓存命中
function manualChunks(id: string): string | undefined {
  if (!id.includes('node_modules')) return undefined
  if (id.includes('echarts') || id.includes('zrender')) return 'echarts'
  if (id.includes('tdesign')) return 'tdesign'
  if (id.includes('/vue/') || id.includes('vue-router') || id.includes('vue-i18n') || id.includes('pinia') || id.includes('@vue/')) return 'vue'
  return 'vendor'
}

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
    rollupOptions: {
      output: { manualChunks },
    },
    chunkSizeWarningLimit: 700,
  },
})

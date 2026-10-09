import { readFileSync, writeFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { execFileSync } from 'node:child_process'
import { fileURLToPath, URL } from 'node:url'
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

export default defineConfig(({ mode }) => {
  const python = process.env.CPH_PYTHON || (process.platform === 'win32' ? 'python' : 'python3')
  const project = JSON.parse(execFileSync(python, [fileURLToPath(new URL('../scripts/project_config.py', import.meta.url)), 'export'], { encoding: 'utf8' }))
  const profile = process.env.CPH_WEB_PROFILE || (['development', 'production', 'test'].includes(mode) ? 'web-full' : mode)
  const config = JSON.parse(readFileSync(new URL('../build-config.json', import.meta.url), 'utf8'))
  const selected = config.frontends[profile] as { surface: 'app' | 'web'; out: string; packages: string[] } | undefined
  if (!selected) throw new Error(`Unknown frontend profile: ${profile}`)
  const outDir = fileURLToPath(new URL(`../${selected.out}`, import.meta.url))
  return {
    base: selected.surface === 'app' ? './' : '/',
    define: { __CPH_PROFILE__: JSON.stringify(profile), __CPH_UPDATE_REPOSITORY__: JSON.stringify(project.updates.repository) },
    resolve: {
      // @/ -> src/（与 tsconfig paths 对齐）
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
        '@profile': fileURLToPath(new URL(`./src/features/profiles/${selected.surface}.ts`, import.meta.url)),
      },
    },
    plugins: [
      vue(),
      // 保留 Web 占位文件，让 go:embed 在未构建前端时也能编译。
      { name: 'keep-build-placeholder', closeBundle: () => writeFileSync(resolve(outDir, '.gitkeep'), '') },
      {
        name: 'verify-profile-modules',
        generateBundle(_options, bundle) {
          const modules = Object.values(bundle).flatMap(output => output.type === 'chunk' ? Object.keys(output.modules) : [])
            .map(id => id.replaceAll('\\', '/')).filter(id => id.startsWith(fileURLToPath(new URL('./src/', import.meta.url)).replaceAll('\\', '/')))
          if (selected.surface === 'app' && modules.some(id => /\/views\/plugins\/Plugins\.vue$/.test(id))) this.error('App workspace must not include the plugin manager')
          this.emitFile({ type: 'asset', fileName: 'profile.json', source: JSON.stringify({ profile, version: project.components.frontend.version, packages: selected.packages, modules: modules.map(id => id.slice(id.lastIndexOf('/src/') + 1)) }, null, 2) })
        },
      },
    ],
    server: {
      port: 5173,
      proxy: {
        '/admin': 'http://127.0.0.1:8080',
        '/v1': 'http://127.0.0.1:8080',
        '/assets/plugins': 'http://127.0.0.1:8080',
        '/extension-assets': 'http://127.0.0.1:8080',
      },
    },
    build: {
      outDir,
      manifest: true,
      rollupOptions: {
        output: { manualChunks },
      },
      chunkSizeWarningLimit: 700,
    },
  }
})

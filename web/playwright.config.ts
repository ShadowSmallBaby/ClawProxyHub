import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './tests',
  testMatch: '*.spec.ts',
  workers: 1,
  use: { headless: true, viewport: { width: 1280, height: 900 } },
  webServer: ['web-full', 'app-full'].map((profile, index) => ({
    command: `pnpm exec vite preview --mode ${profile} --host 127.0.0.1 --port ${4173 + index} --strictPort`,
    url: `http://127.0.0.1:${4173 + index}`,
    reuseExistingServer: false,
  })),
})

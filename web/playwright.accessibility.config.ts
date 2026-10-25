import { defineConfig } from '@playwright/test'

// The native Go owner supplies PostgreSQL, production portals and private IPC.
// This is Chromium engine evidence, not stable retail browser/device acceptance.
export default defineConfig({
  testDir: './accessibility', testMatch: ['native.spec.ts', 'admission.spec.ts'],
  outputDir: 'test-results-accessibility-native', workers: 1, retries: 0,
  timeout: 60_000, expect: { timeout: 10_000 },
  reporter: [['./accessibility/provenance.ts'], ['list'], ['json', { outputFile: 'test-results-accessibility-native/results.json' }]],
  use: { viewport: { width: 1280, height: 900 }, trace: 'retain-on-failure', screenshot: 'only-on-failure' },
  projects: [{ name: 'chromium', use: { browserName: 'chromium' } }],
})

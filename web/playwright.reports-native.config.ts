import { defineConfig } from '@playwright/test'
const viewport = process.env.HACKBOX_REPORT_VIEWPORT
if (viewport !== 'desktop' && viewport !== 'mobile') throw new Error('Run through native Go TestTeamReportsUIBrowsers with its private PostgreSQL fixture.')
export default defineConfig({ testDir: './report-native', outputDir: `test-results-reports-native/${viewport}`, workers: 1, retries: 0, timeout: 120_000, expect: { timeout: 15_000 }, use: { actionTimeout: 15_000, navigationTimeout: 15_000, browserName: 'chromium', trace: 'retain-on-failure', screenshot: 'only-on-failure' }, reporter: [['list'], ['json', { outputFile: `test-results-reports-native/${viewport}/results.json` }]] })

import { defineConfig } from '@playwright/test'
const scope = process.env.HACKBOX_ADVISORY_SCOPE
if (!scope || !/^(admin|public)-(desktop|mobile)$/.test(scope)) throw new Error('Run through the owned Go TestMentorAdvisoryBrowsers fixture.')
const output = `test-results-advisory-native/${scope}`
export default defineConfig({ testDir: './advisory-native', outputDir: output, workers: 1, retries: 0, timeout: 120_000, expect: { timeout: 12_000 }, use: { actionTimeout: 15_000, navigationTimeout: 15_000, browserName: 'chromium', trace: 'retain-on-failure', screenshot: 'only-on-failure' }, reporter: [['list'], ['json', { outputFile: `${output}/results.json` }]] })

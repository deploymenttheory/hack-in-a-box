import { defineConfig } from '@playwright/test'
import base from './playwright.config'
const participant = process.env.TODAY_PORTAL === 'participant', port = participant ? 4343 : 4342
export default defineConfig({ ...base, testMatch: ['today.spec.ts'], workers: 1, outputDir: `test-results-today-${participant ? 'participant' : 'admin'}`, projects: base.projects?.map(project => ({ ...project, metadata: { portal: participant ? 'participant' : 'administrator' } })), use: { ...base.use, baseURL: `http://127.0.0.1:${port}` }, webServer: { command: `${participant ? 'npm run preview' : 'npm run preview:admin'} -- --port ${port}`, port, reuseExistingServer: false } })

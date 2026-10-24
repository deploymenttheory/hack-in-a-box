import { defineConfig } from '@playwright/test'
import base from './playwright.surveys.config'
export default defineConfig({ ...base, testMatch: ['survey-visual-evidence.spec.ts'], workers: 1 })

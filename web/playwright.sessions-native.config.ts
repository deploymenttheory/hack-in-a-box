import { defineConfig } from '@playwright/test'
export default defineConfig({testDir:'./session-native',outputDir:'test-results-sessions-native',workers:1,retries:0,timeout:90_000,expect:{timeout:12_000},use:{actionTimeout:15_000,navigationTimeout:15_000,browserName:'chromium',trace:'retain-on-failure',screenshot:'only-on-failure'},reporter:'list'})

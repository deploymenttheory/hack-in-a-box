import { defineConfig } from '@playwright/test'
export default defineConfig({testDir:'./session-native',outputDir:'test-results-sessions-native',workers:1,retries:0,timeout:90_000,expect:{timeout:12_000},use:{browserName:'chromium',trace:'retain-on-failure',screenshot:'only-on-failure'},reporter:'list'})

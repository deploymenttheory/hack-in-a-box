import {defineConfig} from '@playwright/test'
import {native} from './freeze-native/native'
export default defineConfig({testDir:'./freeze-native',outputDir:`test-results-freeze-native/${native.viewport}`,workers:1,retries:0,timeout:60000,use:{trace:'retain-on-failure'},projects:[{name:native.viewport,use:{browserName:'chromium'}}]})

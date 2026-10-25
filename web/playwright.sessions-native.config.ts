import { defineConfig } from '@playwright/test'
const viewport=process.env.HACKBOX_SESSION_VIEWPORT
if(viewport!=='desktop'&&viewport!=='mobile')throw new Error('Run the native Go session browser harness with its owned viewport fixture.')
const output=`test-results-sessions-native/${viewport}`
export default defineConfig({testDir:'./session-native',outputDir:output,workers:1,retries:0,timeout:90_000,expect:{timeout:12_000},use:{actionTimeout:15_000,navigationTimeout:15_000,browserName:'chromium',trace:'retain-on-failure',screenshot:'only-on-failure'},reporter:[['list'],['json',{outputFile:`${output}/results.json`}]]})

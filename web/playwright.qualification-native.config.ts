import {defineConfig} from '@playwright/test'
const scope=process.env.HACKBOX_QUALIFICATION_SCOPE
if(!scope||!/^(admin|public)-(desktop|mobile)-(initial|final)$/.test(scope))throw new Error('Use the owned native Go qualification browser fixture.')
const output=`test-results-qualification-native/${scope}`
export default defineConfig({testDir:'./qualification-native',outputDir:output,workers:1,retries:0,timeout:90_000,expect:{timeout:12_000},use:{actionTimeout:15_000,navigationTimeout:15_000,browserName:'chromium',trace:'retain-on-failure',screenshot:'only-on-failure'},reporter:[['list'],['json',{outputFile:`${output}/results.json`}]]})

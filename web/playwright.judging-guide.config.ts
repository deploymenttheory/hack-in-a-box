import { defineConfig } from '@playwright/test'
import { resolve } from 'node:path'
import base from './playwright.styleguide.config'
export default defineConfig({...base,workers:1,retries:0,outputDir:'test-results-judging-guide',use:{...base.use,baseURL:'http://127.0.0.1:4196'},webServer:{command:'go run ../cmd/styleguide',url:'http://127.0.0.1:4196/readyz',reuseExistingServer:false,env:{HACKBOX_HTTP_ADDR:'127.0.0.1:4196',HACKBOX_WEB_DIR:resolve('dist-styleguide')}}})

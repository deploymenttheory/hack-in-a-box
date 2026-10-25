import {defineConfig} from '@playwright/test'
import {resolve} from 'node:path'
import base from './playwright.styleguide.config'
export default defineConfig({...base,testMatch:['qualification.spec.ts'],outputDir:'test-results-qualification-guide',workers:1,use:{...base.use,baseURL:'http://127.0.0.1:4498'},webServer:{command:'go run ../cmd/styleguide',url:'http://127.0.0.1:4498/readyz',reuseExistingServer:false,env:{HACKBOX_HTTP_ADDR:'127.0.0.1:4498',HACKBOX_WEB_DIR:resolve('dist-styleguide')}}})

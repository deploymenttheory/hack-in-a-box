import {defineConfig} from '@playwright/test'
import base from './playwright.config'
export default defineConfig({...base,testMatch:['qualification.spec.ts'],testIgnore:[],outputDir:'test-results-qualification',workers:1,use:{...base.use,baseURL:'http://127.0.0.1:4497'},webServer:{command:'npm run preview:admin -- --port 4497 --strictPort',port:4497,reuseExistingServer:false}})

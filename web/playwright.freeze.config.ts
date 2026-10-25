import {defineConfig} from '@playwright/test'
import base from './playwright.config'
export default defineConfig({...base,testMatch:['submission-freeze.spec.ts'],testIgnore:[],outputDir:'test-results-freeze',webServer:{command:'npm run preview:admin -- --port 4397 --strictPort',port:4397,reuseExistingServer:false},use:{...base.use,baseURL:'http://127.0.0.1:4397'},workers:1})

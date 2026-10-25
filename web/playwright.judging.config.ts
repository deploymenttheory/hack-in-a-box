import { defineConfig } from '@playwright/test'
import base from './playwright.config'
const regular=process.env.HACKBOX_JUDGING_PORTAL==='regular',port=regular?4295:4195
export default defineConfig({...base,testMatch:['judging.spec.ts','judging-safety.spec.ts'],testIgnore:[],workers:1,retries:0,outputDir:regular?'test-results-judging-regular':'test-results-judging-admin',projects:base.projects?.map(project=>({...project,metadata:{...project.metadata,portal:regular?'regular':'administrator'}})),use:{...base.use,baseURL:`http://127.0.0.1:${port}`},webServer:{command:`npm run ${regular?'preview':'preview:admin'} -- --port ${port} --strictPort`,port,reuseExistingServer:false}})

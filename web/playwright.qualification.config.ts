import {defineConfig} from '@playwright/test'
import base from './playwright.config'
const regular=process.env.HACKBOX_QUALIFICATION_REGULAR==='1',port=regular?4499:4497
export default defineConfig({...base,testMatch:['qualification.spec.ts'],testIgnore:[],outputDir:regular?'test-results-qualification-public':'test-results-qualification',workers:1,projects:base.projects?.map(project=>({...project,metadata:{...project.metadata,portal:regular?'regular':'administrator'}})),use:{...base.use,baseURL:`http://127.0.0.1:${port}`},webServer:{command:`npm run ${regular?'preview':'preview:admin'} -- --port ${port} --strictPort`,port,reuseExistingServer:false}})

import { defineConfig } from '@playwright/test'
import base from './playwright.planning.config'
import publicBase from './playwright.public.config'
const full=process.env.HACKBOX_PLANNING_FULL==='1'
export default defineConfig({...base,testMatch:full?publicBase.testMatch:['event-planning-public.spec.ts','participant-guidance.spec.ts','public-portal.spec.ts'],testIgnore:[],grepInvert:publicBase.grepInvert,outputDir:full?'test-results-planning-full-public':'test-results-planning-public',projects:full?publicBase.projects:base.projects?.map(project=>({...project,name:(project.name??'admin').replace('admin','participant')})),use:{...base.use,baseURL:'http://127.0.0.1:4396'},webServer:{command:'npm run preview -- --port 4396 --strictPort',port:4396,reuseExistingServer:false}})

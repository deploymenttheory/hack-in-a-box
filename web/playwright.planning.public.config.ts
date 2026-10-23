import { defineConfig } from '@playwright/test'
import base from './playwright.planning.config'
export default defineConfig({...base, testMatch:['event-planning-public.spec.ts','participant-guidance.spec.ts','public-portal.spec.ts'],outputDir:'test-results-planning-public',projects:base.projects?.map(project=>({...project,name:(project.name??'admin').replace('admin','participant')})),use:{...base.use,baseURL:'http://127.0.0.1:4396'},webServer:{command:'npm run preview -- --port 4396 --strictPort',port:4396,reuseExistingServer:false}})

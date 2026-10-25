import { defineConfig } from '@playwright/test'
const portal=process.env.SESSION_PORTAL==='participant'?'participant':'administrator'
const root=process.env.SESSION_BASELINE_ROOT
if(!root)throw new Error('Provide the isolated reviewed-main baseline root.')
const port=portal==='participant'?4355:4354
export default defineConfig({testDir:'./e2e',testMatch:'session-bookings.visual.ts',outputDir:`test-results-sessions-before/${portal}`,workers:1,retries:0,use:{baseURL:`http://127.0.0.1:${port}`,trace:'retain-on-failure'},projects:[{name:'desktop',use:{viewport:{width:1440,height:1000}}},{name:'mobile',use:{viewport:{width:390,height:844}}}],webServer:{command:`${root}/web/node_modules/.bin/vite preview --outDir ${root}/web/${portal==='participant'?'dist':'dist-admin'} --host 127.0.0.1 --port ${port} --strictPort`,port,reuseExistingServer:false}})

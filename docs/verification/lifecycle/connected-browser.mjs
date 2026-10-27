// Host browser actions on the very same events as TestLifecycleComposeJourney.
// The controller supplies no sessions, application responses or native writes.
import assert from 'node:assert/strict'
import {createEventInFleet} from './browser-event-creation.mjs'
import {checkLiveRoles} from './browser-live-roles.mjs'
import {recoverLiveHelp} from './browser-live-recovery.mjs'
import {execFile} from 'node:child_process'
import {promisify} from 'node:util'
import {createRequire} from 'node:module'
import {dirname,resolve,join} from 'node:path'
import {fileURLToPath} from 'node:url'
import {readFile,writeFile,chmod,stat} from 'node:fs/promises'
import {setTimeout as pause} from 'node:timers/promises'

const root=resolve(dirname(fileURLToPath(import.meta.url)),'../../..')
const require=createRequire(join(root,'web/package.json'))
const {chromium,expect}=require('@playwright/test')
const docker=promisify(execFile)
const output=process.env.HACKBOX_LIFECYCLE_BROWSER_ARTIFACTS
const source=process.env.HACKBOX_LIFECYCLE_BROWSER_SOURCE
const runner=process.env.HACKBOX_LIFECYCLE_BROWSER_RUNNER
assert.ok(output);assert.match(source??'',/^[a-f0-9]{40}$/)
assert.match(runner??'',/^hackbox-connected-browser-[0-9]+-runner$/)
async function readPhase(sequence,budget){
  const file=join(output,`phase-${sequence}.json`)
  const end=Date.now()+budget
  for(;;){
   try{await docker('docker',['cp',`${runner}:/artifacts/browser/phase-${sequence}.json`,file]);break}
   catch{
    const state=await docker('docker',['inspect','--format','{{.State.Status}}',runner])
    assert.equal(state.stdout.trim(),'running','Native journey exited before its required browser phase')
    assert.ok(Date.now()<end,'Native browser phase was not reached within its bounded wait')
    await pause(500)
   }
  }
  return JSON.parse(await readFile(file,'utf8'))
}
async function acknowledge(ack){
 const sequence=ack.sequence,result=join(output,`result-${sequence}.json`)
 await writeFile(result,JSON.stringify({...ack,completed_by_ui:true})+'\n',{mode:0o600})
 await chmod(result,0o644)
 assert.equal((await stat(result)).mode&0o777,0o644)
 await docker('docker',['cp',result,`${runner}:/artifacts/browser/result-${sequence}.tmp`])
 await docker('docker',['exec',runner,'mv',`/artifacts/browser/result-${sequence}.tmp`,`/artifacts/browser/result-${sequence}.json`])
}
const errors=[],receipts=[]
let recovery,liveRoles
const creations=[]

let step='launch'
const browser=await chromium.launch({headless:true})
try{
 const context=await browser.newContext({viewport:{width:1440,height:1100},proxy:{server:'http://127.0.0.1:18088',bypass:'<-loopback>'}})
 const page=await context.newPage();page.on('pageerror',error=>errors.push(error.message))
 step='normal-owner-oidc'
 await page.goto('http://admin.localhost:8080/auth/login')
 await expect(page.getByRole('heading',{name:'Synthetic identity provider',exact:true})).toBeVisible()
 await page.getByLabel('Synthetic actor',{exact:true}).selectOption('owner')
 await page.getByRole('button',{name:'Continue through normal OIDC',exact:true}).click()
 await expect(page.getByRole('heading',{name:'Your events',exact:true})).toBeVisible()
 for(let sequence=1;sequence<=2;sequence++){
  if(process.env.HACKBOX_LIFECYCLE_BROWSER_PHASES==='ui-journey'){
   step=`create-event-${sequence}`
   const creation=await readPhase(99+sequence,180000)
   const created=await createEventInFleet({page,expect,phase:creation,output})
   await acknowledge({sequence:creation.sequence,action:creation.action,event_id:created.event_id,saved_by_ui:true})
   creations.push(created)
  }
  step=`await-phase-${sequence}`
  const phase=await readPhase(sequence,180000)
  assert.equal(phase.sequence,sequence);assert.equal(phase.action,'save-branding')
  assert.match(phase.event_id,/^[a-f0-9-]{36}$/);assert.match(phase.primary,/^#[a-f0-9]{6}$/)
  assert.ok(Number.isFinite(Date.parse(phase.domain_now)))
  // Date follows this same fixture's domain phase; browser timers, sessions,
  // OIDC expiry, HTTP budgets and the native backend's gates remain real.
  await page.clock.setFixedTime(new Date(phase.domain_now))
  step=`open-branding-${sequence}`
  await page.goto(`http://admin.localhost:8080/events/${phase.event_id}?section=branding`)
  await expect(page.getByRole('heading',{name:'Event branding',exact:true})).toBeVisible()
  await page.getByRole('navigation',{name:'Event branding sections',exact:true}).getByRole('button',{name:'Colours',exact:true}).click()
  await page.getByLabel('Primary colour',{exact:true}).fill(phase.primary)
  await page.getByRole('navigation',{name:'Event branding sections',exact:true}).getByRole('button',{name:'Preview',exact:true}).click()
  const preview=page.getByRole('region',{name:'Branding preview',exact:true})
  await expect(preview).toBeVisible()
  assert.equal(await preview.evaluate(el=>getComputedStyle(el).getPropertyValue('--primary').trim()),phase.primary)
  step=`save-branding-${sequence}`
  const response=page.waitForResponse(r=>new URL(r.url()).pathname===`/api/v1/events/${phase.event_id}/branding`&&r.request().method()==='PUT')
  await page.getByRole('button',{name:'Save branding',exact:true}).click()
  assert.equal((await response).status(),200)
  await expect(page.getByRole('status').filter({hasText:'Branding saved.'})).toBeVisible()
  await page.reload()
  await expect(page.getByRole('heading',{name:'Event branding',exact:true})).toBeVisible()
  await page.getByRole('navigation',{name:'Event branding sections',exact:true}).getByRole('button',{name:'Colours',exact:true}).click()
  await expect(page.getByLabel('Primary colour',{exact:true})).toHaveValue(phase.primary)
  await page.screenshot({path:join(output,`native-event-${sequence}-branding.png`),fullPage:true})
  const ack={sequence,action:phase.action,event_id:phase.event_id,saved_by_ui:true}
  await acknowledge(ack)
  receipts.push({...ack,preview_matches:true,persisted_after_reload:true})
 }
 if(['live-recovery','role-matrix','ui-journey'].includes(process.env.HACKBOX_LIFECYCLE_BROWSER_PHASES)){
  step='await-live-recovery'
  const phase=await readPhase(3,720000)
  step='live-help-response-loss'
  recovery=await recoverLiveHelp({browser,expect,phase,output,errors})
  await acknowledge({sequence:phase.sequence,action:phase.action,event_id:phase.event_id,saved_by_ui:true,help:recovery})
 }
 if(['role-matrix','ui-journey'].includes(process.env.HACKBOX_LIFECYCLE_BROWSER_PHASES)){
  step='live-role-matrix'
  liveRoles=await checkLiveRoles({browser,expect,owner:page,readPhase,acknowledge,output,errors})
 }
 step='native-journey-completion'
 const completed=await docker('docker',['wait',runner],{timeout:720000})
 assert.equal(completed.stdout.trim(),'0','Same-event native journey failed after browser actions')
 assert.deepEqual(errors,[])
 await writeFile(join(output,'connected-browser-proof.json'),JSON.stringify({source_revision:source,kind:'same-event-native-browser-planning',complete_lifecycle:false,normal_oidc:true,fulfilled_application_routes:0,session_injections:0,page_errors:0,native_journey_exit_code:0,event_creation:creations,planning:receipts,live_help_recovery:recovery,live_role_matrix:liveRoles},null,2)+'\n',{mode:0o600})
 console.log('Same-event browser branding actions and subsequent native journey PASS; full browser acceptance remains open.')
}catch(error){
 console.error(`Connected browser failed during ${step}; private original evidence retained.`)
 throw error
}finally{await browser.close()}

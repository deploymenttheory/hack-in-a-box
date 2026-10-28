import {planFinalJudgingInFleet} from './browser-judging-setup.mjs'
import {rehearseJudgeInFleet} from './browser-judge-practice.mjs'
import {reviewSubmissionFreezeInFleet} from './browser-submission-freeze.mjs'
import {submitTeamEvidenceInFleet} from './browser-team-submissions.mjs'
import {askClarificationInFleet,answerClarificationInFleet} from './browser-clarification-authoring.mjs'
import {decideEventInFleet} from './browser-event-decisions.mjs'
import {readPublicationInFleet} from './browser-publication-reading.mjs'
import {createObjectiveInFleet} from './browser-objective-authoring.mjs'
import {authorChallengeInFleet} from './browser-challenge-authoring.mjs'
import {configureRubricInFleet} from './browser-rubric-planning.mjs'
// Host browser actions on the very same events as TestLifecycleComposeJourney.
// The controller supplies no sessions, application responses or native writes.
import assert from 'node:assert/strict'
import {changeTeamInFleet} from './browser-team-allocation.mjs'
import {publishRegistrationFormInFleet} from './browser-registration-forms.mjs'
import {openRegistrationInFleet,applyInFleet,selectFacilitatorInFleet,competitionGoInFleet,respondToAdmissionInFleet,configureOffersInFleet} from './browser-registration-applications.mjs'
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
const creations=[],applications=[],selections=[],admissionDecisions=[],forms=[],offerPolicies=[],teamChanges=[],rubrics=[],challenges=[],teamWithdrawals=[],objectives=[],readings=[],eventDecisions=[],clarifications=[],submissions=[],freezeChanges=[],practiceSubmissions=[],judgingSetupChanges=[]

let step='launch'
const browser=await chromium.launch({headless:true})
async function withdrawTeamMember(sequence){
 const phase=await readPhase(sequence,180000)
 assert.equal(phase.response.action,'withdraw')
 const result=await respondToAdmissionInFleet({browser,expect,phase,output,errors})
 teamWithdrawals.push({...result,event_id:phase.event_id,sequence})
 await acknowledge({sequence:phase.sequence,action:phase.action,event_id:phase.event_id,saved_by_ui:true,application_id:result.application_id})
}
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
  if(process.env.HACKBOX_LIFECYCLE_BROWSER_PHASES==='ui-journey'){
   step=`create-objective-${sequence}`
   const objectivePhase=await readPhase(701+(sequence-1)*20,180000)
   const objective=await createObjectiveInFleet({page,expect,phase:objectivePhase,output})
   objectives.push(objective)
   await acknowledge({sequence:objectivePhase.sequence,action:objectivePhase.action,event_id:objectivePhase.event_id,objective_id:objective.objective_id,saved_by_ui:true})
   const base=200+(sequence-1)*20
   step=`publish-attendee-form-${sequence}`
   const attendeeForm=await readPhase(base+16,180000)
   const published=await publishRegistrationFormInFleet({page,expect,phase:attendeeForm,output})
   await acknowledge({sequence:attendeeForm.sequence,action:attendeeForm.action,event_id:attendeeForm.event_id,saved_by_ui:true,form_id:published.form_id})
   forms.push(published)
   step=`open-registration-${sequence}`
   const opening=await readPhase(base,180000)
   await openRegistrationInFleet({page,expect,phase:opening})
   await acknowledge({sequence:opening.sequence,action:opening.action,event_id:opening.event_id,saved_by_ui:true})
   step=`approve-offer-policy-${sequence}`
   const policy=await readPhase(base+19,180000)
   offerPolicies.push(await configureOffersInFleet({page,expect,phase:policy,output}))
   await acknowledge({sequence:policy.sequence,action:policy.action,event_id:policy.event_id,saved_by_ui:true})
   for(let slot=0;slot<4;slot++){
    if(slot<2){
     step=`publish-facilitator-form-${sequence}-${slot}`
     const phase=await readPhase(base+17+slot,180000)
     const published=await publishRegistrationFormInFleet({page,expect,phase,output})
     await acknowledge({sequence:phase.sequence,action:phase.action,event_id:phase.event_id,saved_by_ui:true,form_id:published.form_id})
     forms.push(published)
    }
    step=`application-${sequence}-${slot}`
    const phase=await readPhase(base+slot+1,180000)
    const applied=await applyInFleet({browser,expect,phase,output,errors})
    await acknowledge({sequence:phase.sequence,action:phase.action,event_id:phase.event_id,saved_by_ui:true,application_id:applied.application_id})
    applications.push(applied)
    if(slot<2){
     step=`select-facilitator-${sequence}-${slot}`
     const selection=await readPhase(base+11+slot,180000)
     const selected=await selectFacilitatorInFleet({page,expect,phase:selection,output})
     await acknowledge({sequence:selection.sequence,action:selection.action,event_id:selection.event_id,saved_by_ui:true,application_id:selected.application_id})
     selections.push(selected)
    }
   }
   step=`competition-go-${sequence}`
   const go=await readPhase(base+13,180000)
   admissionDecisions.push(await competitionGoInFleet({page,expect,phase:go,output}))
   await acknowledge({sequence:go.sequence,action:go.action,event_id:go.event_id,saved_by_ui:true})
   for(const responseSequence of [base+14,base+15]){
    step=`respond-admission-${responseSequence}`
    const phase=await readPhase(responseSequence,180000)
    const responded=await respondToAdmissionInFleet({browser,expect,phase,output,errors})
    await acknowledge({sequence:phase.sequence,action:phase.action,event_id:phase.event_id,saved_by_ui:true,application_id:responded.application_id})
    admissionDecisions.push(responded)
   }
   for(let command=0;command<46;command++){
    if(command===36)for(const withdrawal of [601,602,603]){step=`allocation-withdrawal-${sequence}-${withdrawal}`;await withdrawTeamMember(withdrawal+(sequence-1)*20)}
    step=`team-change-${sequence}-${command}`
    const phase=await readPhase(301+(sequence-1)*100+command,180000)
    const changed=await changeTeamInFleet({page,expect,phase,output})
    teamChanges.push(changed)
    await acknowledge({sequence:phase.sequence,action:phase.action,event_id:phase.event_id,saved_by_ui:true})
    assert.equal(phase.team_change.last,command===45,'Both reviewed team publications are required')
   }
   for(const rubricSequence of [501+(sequence-1)*20,502+(sequence-1)*20]){
    step=`rubric-${rubricSequence}`
    const phase=await readPhase(rubricSequence,180000)
    rubrics.push(await configureRubricInFleet({page,expect,phase,output}))
    await acknowledge({sequence:phase.sequence,action:phase.action,event_id:phase.event_id,saved_by_ui:true})
   }
   for(const challengeSequence of [503,504,505].map(n=>n+(sequence-1)*20)){
    step=`challenge-${challengeSequence}`
    const phase=await readPhase(challengeSequence,180000)
    const authored=await authorChallengeInFleet({page,expect,phase,output})
    challenges.push(authored)
    await acknowledge({sequence:phase.sequence,action:phase.action,event_id:phase.event_id,content_id:authored.content_id,saved_by_ui:true})
   }
  }
 }
 if(process.env.HACKBOX_LIFECYCLE_BROWSER_PHASES==='ui-journey'){
  for(const sequence of [347,447]){
   step=`hard-freeze-${sequence}`
   const phase=await readPhase(sequence,180000)
   assert.equal(phase.team_change.command.action,'freeze')
   teamChanges.push(await changeTeamInFleet({page,expect,phase,output}))
   await acknowledge({sequence:phase.sequence,action:phase.action,event_id:phase.event_id,saved_by_ui:true})
  }
  for(const sequence of [801,802,821,822]){
   step=`read-common-publication-${sequence}`
   const phase=await readPhase(sequence,180000)
   const reading=await readPublicationInFleet({browser,expect,phase,output,errors})
   readings.push(reading)
   await acknowledge({sequence:phase.sequence,action:phase.action,event_id:phase.event_id,content_id:reading.content_id,saved_by_ui:true})
  }
  for(const sequence of [901,921]){
   step=`postpone-published-event-${sequence}`
   const phase=await readPhase(sequence,180000)
   eventDecisions.push(await decideEventInFleet({page,expect,phase,output}))
   await acknowledge({sequence:phase.sequence,action:phase.action,event_id:phase.event_id,saved_by_ui:true})
  }
  for(const sequence of [611,612,613,631,632,633]){step=`frozen-team-withdrawal-${sequence}`;await withdrawTeamMember(sequence)}
  for(const base of [1301,1321])for(let offset=0;offset<6;offset++){
   const sequence=base+offset
   step=`submission-branch-${sequence}`
   const phase=await readPhase(sequence,180000)
   const result=await reviewSubmissionFreezeInFleet({page,expect,phase,output})
   freezeChanges.push(result)
   await acknowledge({sequence:phase.sequence,action:phase.action,event_id:phase.event_id,command_id:result.command_id,saved_by_ui:true})
  }
  for(const base of [1101,1121])for(let offset=0;offset<6;offset++){
   const sequence=base+offset
   step=`private-team-draft-${sequence}`
   const phase=await readPhase(sequence,180000)
   submissions.push(await submitTeamEvidenceInFleet({browser,expect,phase,output,errors}))
   await acknowledge({sequence:phase.sequence,action:phase.action,event_id:phase.event_id,saved_by_ui:true})
  }
  for(let eventIndex=0;eventIndex<2;eventIndex++){
   for(let offset=0;offset<7;offset++){
    const sequence=1601+eventIndex*20+offset
    step=`final-judging-setup-${sequence}`
    const phase=await readPhase(sequence,180000)
    const result=await planFinalJudgingInFleet({page,expect,phase,output})
    judgingSetupChanges.push(result)
    await acknowledge({sequence:phase.sequence,action:phase.action,event_id:phase.event_id,command_id:result.command_id,run_id:result.run_id,saved_by_ui:true})
   }
   for(let offset=0;offset<3;offset++){
    const sequence=1501+eventIndex*20+offset
    step=`fictional-judge-practice-${sequence}`
    const phase=await readPhase(sequence,180000)
    const result=await rehearseJudgeInFleet({browser,expect,phase,output,errors})
    practiceSubmissions.push(result)
    await acknowledge({sequence:phase.sequence,action:phase.action,event_id:phase.event_id,command_id:result.command_id,saved_by_ui:true})
   }
  }
  for(const sequence of [902,922]){
   step=`approve-live-attendance-exception-${sequence}`
   const phase=await readPhase(sequence,180000)
   eventDecisions.push(await decideEventInFleet({page,expect,phase,output}))
   await acknowledge({sequence:phase.sequence,action:phase.action,event_id:phase.event_id,saved_by_ui:true})
  }
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
 if(process.env.HACKBOX_LIFECYCLE_BROWSER_PHASES==='ui-journey'){
  for(const base of [1001,1021])for(let offset=0;offset<7;offset++){
   const sequence=base+offset
   step=`common-clarification-${sequence}`
   const phase=await readPhase(sequence,180000)
   const result=offset===0?await askClarificationInFleet({browser,expect,phase,output,errors}):await answerClarificationInFleet({page,expect,phase,output})
   clarifications.push(result)
   await acknowledge({sequence:phase.sequence,action:phase.action,event_id:phase.event_id,question_id:result.question_id,command_id:result.command_id,saved_by_ui:true})
  }
 }
 if(process.env.HACKBOX_LIFECYCLE_BROWSER_PHASES==='ui-journey'){
  for(const base of [1201,1221])for(let offset=0;offset<4;offset++){
   const sequence=base+offset
   step=`formal-team-submission-${sequence}`
   const phase=await readPhase(sequence,180000)
   submissions.push(await submitTeamEvidenceInFleet({browser,expect,phase,output,errors}))
   await acknowledge({sequence:phase.sequence,action:phase.action,event_id:phase.event_id,saved_by_ui:true})
  }
 }
 if(process.env.HACKBOX_LIFECYCLE_BROWSER_PHASES==='ui-journey'){
  for(const base of [1401,1421])for(let offset=0;offset<3;offset++){
   const sequence=base+offset
   step=`cutoff-reconciliation-${sequence}`
   const phase=await readPhase(sequence,180000)
   const result=await reviewSubmissionFreezeInFleet({page,expect,phase,output})
   freezeChanges.push(result)
   await acknowledge({sequence:phase.sequence,action:phase.action,event_id:phase.event_id,command_id:result.command_id,saved_by_ui:true})
  }
 }
 step='native-journey-completion'
 const completed=await docker('docker',['wait',runner],{timeout:720000})
 assert.equal(completed.stdout.trim(),'0','Same-event native journey failed after browser actions')
 assert.deepEqual(errors,[])
 await writeFile(join(output,'connected-browser-proof.json'),JSON.stringify({source_revision:source,kind:'same-event-native-browser-planning',complete_lifecycle:false,normal_oidc:true,fulfilled_application_routes:0,session_injections:0,page_errors:0,native_journey_exit_code:0,event_creation:creations,objective_authoring:objectives,publication_reading:readings,event_decisions:eventDecisions,clarification_authoring:clarifications,team_submissions:submissions,submission_freeze:freezeChanges,fictional_practice:practiceSubmissions,judging_setup:judgingSetupChanges,offer_policies:offerPolicies,registration_forms:forms,registration_applications:applications,facilitator_selections:selections,admission_decisions:admissionDecisions,team_changes:teamChanges,team_withdrawals:teamWithdrawals,rubric_configuration:rubrics,challenge_authoring:challenges,planning:receipts,live_help_recovery:recovery,live_role_matrix:liveRoles},null,2)+'\n',{mode:0o600})
 console.log('Same-event browser branding actions and subsequent native journey PASS; full browser acceptance remains open.')
}catch(error){
 console.error(`Connected browser failed during ${step}; private original evidence retained.`)
 throw error
}finally{await browser.close()}

import {test,expect} from '@playwright/test'
import {mockWorkspace,firstID} from './fixtures'
import {qualificationGuidePage} from '../src/styleguide/qualification-fixture'
import {qualificationFingerprint,type QualificationCommand} from '../src/qualification-model'
const initial=qualificationGuidePage(),run=initial.round.run_id,path=`/events/${firstID}/panel/${run}`
async function fixture(page:import('@playwright/test').Page){
 await mockWorkspace(page)
 const state={page:structuredClone(initial),denied:false}
 await page.route(`**/api/v1/events/${firstID}/mentor-qualification/${run}`,route=>state.denied?route.fulfill({status:403,json:{error:{code:'forbidden',message:'Synthetic current scope denied',request_id:'synthetic'}}}):route.fulfill({json:state.page}))
 await page.route('**/moderation-huddles/*',route=>route.fulfill({status:403,json:{error:{code:'forbidden',message:'Synthetic source-only denial',request_id:'synthetic'}}}))
 return state
}
test('native panel reader separates source-specific authority and uses organiser build destination',async({page})=>{
 await fixture(page);await page.goto(path);await expect(page.getByRole('heading',{name:'Moderation and selections',exact:true})).toBeVisible()
 const team=initial.teams[0].team_id;await page.getByRole('button',{name:`Inspect Team ${team}`,exact:true}).click()
 await expect(page.getByText('This source is not disclosed under current source-specific authority. A panel selection does not grant private huddle access.',{exact:true})).toBeVisible()
 await expect(page.getByRole('link',{name:'Inspect formal build and frozen code',exact:true})).toHaveAttribute('href',`/events/${firstID}/teams/${team}/build`)
 await expect(page.getByRole('region',{name:'Moderated final card',exact:true})).toContainText('Synthetic recorded moderation reason')
 expect(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth)).toBe(false)
})
test('finalist preparation is private and independent from category preview',async({page})=>{
 await fixture(page);await page.goto(path)
 await page.getByRole('button',{name:'Preview finalist preparation',exact:true}).click()
 const preview=page.getByRole('region',{name:'Private finalist preparation',exact:true});await expect(preview).toBeVisible();await expect(preview).not.toContainText('User experience');await expect(page.getByRole('region',{name:'Private category preparation',exact:true})).toHaveCount(0)
 await page.getByRole('button',{name:'Preview category preparation',exact:true}).click();await expect(page.getByRole('region',{name:'Private finalist preparation',exact:true})).toHaveCount(0);await expect(page.getByRole('region',{name:'Private category preparation',exact:true})).toContainText('User experience')
 await expect(page.getByRole('button',{name:/^(Publish|Send|Release) /})).toHaveCount(0)
})
test('changed signed source removes old private preparation and selected evidence',async({page})=>{
 const state=await fixture(page);await page.goto(path);await page.getByRole('button',{name:`Inspect Team ${initial.teams[0].team_id}`,exact:true}).click();await page.getByRole('button',{name:'Preview finalist preparation',exact:true}).click()
 state.page.current=false;state.page.round.signoff=null;state.page.preview!.blockers=['unresolved_tie:synthetic-finalist-seat'];await page.evaluate(()=>dispatchEvent(new Event('focus')))
 await expect(page.getByRole('region',{name:'Private finalist preparation',exact:true})).toHaveCount(0);await expect(page.getByRole('region',{name:'Moderated final card',exact:true})).toHaveCount(0);await expect(page.getByText(/unresolved tie:synthetic-finalist-seat/).first()).toBeVisible()
})
test('current native denial clears private panel data and requires an explicit fresh read',async({page})=>{
 const state=await fixture(page);await page.goto(path);await page.getByRole('button',{name:'Preview category preparation',exact:true}).click();state.denied=true;await page.evaluate(()=>dispatchEvent(new Event('focus')))
 await expect(page.getByText('Current panel access is unavailable. Private selections, evidence and earlier review context have been cleared.',{exact:true})).toBeVisible();await expect(page.getByRole('heading',{name:'Moderation and selections',exact:true})).toHaveCount(0);await expect(page.getByRole('region',{name:'Private category preparation',exact:true})).toHaveCount(0)
 state.denied=false;state.page.round.cards=[];state.page.round.signoff=null;state.page.current=false;await page.getByRole('button',{name:'Check current panel access',exact:true}).click();await expect(page.getByRole('heading',{name:'Moderation and selections',exact:true})).toBeVisible();await expect(page.getByText('Synthetic recorded moderation reason',{exact:false})).toHaveCount(0)
})
test('current native discovery and editable panel validation support keyboard review/cancel and exact unknown recovery',async({page})=>{
 const state=await fixture(page),organiser=initial.round.panel.organiser_id,mentor=initial.round.panel.mentor_ids[0],writes:QualificationCommand[]=[]
 let releaseStaff:()=>void=()=>{};const staffResponse=new Promise<void>(resolve=>{releaseStaff=resolve})
 state.page.round.version=0;state.page.round.cards=[];state.page.round.signoff=null;state.page.round.panel={organiser_id:'00000000-0000-0000-0000-000000000000',mentor_ids:[]};state.page.current=false;state.page.preview=null
 state.page.capabilities={can_read_panel:false,can_configure_panel:true,can_propose:false,can_sign_off:false,record_locked:false,read_only:false}
 await page.route(`**/api/v1/events/${firstID}/mentor-qualification`,route=>route.fulfill({json:{items:[{run_id:run,previous_run_id:null,name:'Synthetic current final panel',version:0,can_configure_panel:true,record_locked:false}]}}))
 await page.route(`**/api/v1/events/${firstID}/staff`,async route=>{await staffResponse;return route.fulfill({json:{items:[{person_id:organiser,display_name:'Current organising lead',role:'organiser',active:true,changed_at:'2027-06-20T15:00:00Z'},{person_id:mentor,display_name:'Current panel mentor',role:'mentor',active:true,changed_at:'2027-06-20T15:00:00Z'},{person_id:'ffffffff-ffff-4fff-8fff-ffffffffffff',display_name:'Revoked mentor canary',role:'mentor',active:false,changed_at:'2027-06-20T15:00:00Z'}]}})})
 await page.route(`**/api/v1/events/${firstID}/mentor-qualification/${run}`,route=>{if(route.request().method()==='POST'){writes.push(route.request().postDataJSON() as QualificationCommand);return route.abort('failed')}return state.denied?route.fulfill({status:403,json:{error:{code:'forbidden',message:'Synthetic source denial',request_id:'synthetic'}}}):route.fulfill({json:state.page})})
 await page.goto(`/events/${firstID}/panel`);await page.getByRole('link',{name:'Open Synthetic current final panel',exact:true}).press('Enter')
 await page.getByRole('button',{name:'Configure intended panel',exact:true}).press('Enter')
 const submit=page.getByRole('button',{name:'Review entered panel decision',exact:true});await expect(submit).toBeDisabled();releaseStaff();await expect(submit).toBeEnabled();await submit.press('Enter')
 await expect(page.getByLabel('Organising lead',{exact:true})).toBeFocused();await expect(page.getByText('Choose at least one current mentor.',{exact:true})).toBeVisible();expect(writes).toHaveLength(0)
 await page.getByLabel('Organising lead',{exact:true}).selectOption(organiser);await page.getByRole('checkbox',{name:'Current panel mentor',exact:true}).press('Space');await expect(page.getByText('Revoked mentor canary',{exact:true})).toHaveCount(0)
 await submit.press('Enter');await expect(page.getByLabel('Recorded panel reason',{exact:true})).toBeFocused()
 await page.getByLabel('Recorded panel reason',{exact:true}).fill('Independent <native> & panel reason\u2028😀')
 await submit.press('Enter');const dialog=page.getByRole('alertdialog',{name:'Review private panel command',exact:true});await expect(dialog).toContainText('Panel version 0');await dialog.getByRole('button',{name:'Cancel',exact:true}).press('Enter');await expect(submit).toBeFocused();await expect(page.getByLabel('Recorded panel reason',{exact:true})).toHaveValue('Independent <native> & panel reason\u2028😀')
 await submit.press('Enter');await dialog.getByRole('button',{name:'Confirm reviewed panel command',exact:true}).press('Enter');await expect(page.getByRole('heading',{name:'Original panel outcome unknown',exact:true})).toBeVisible();expect(writes).toHaveLength(1)
 const freshBase=page.getByRole('button',{name:'Review current panel starting point',exact:true});await freshBase.press('Enter');const baseReview=page.getByRole('alertdialog',{name:'Review current panel starting point',exact:true});await expect(baseReview).toContainText('Earlier original requests remain unknown in history');await baseReview.getByRole('button',{name:'Cancel',exact:true}).press('Enter');await expect(freshBase).toBeFocused();await freshBase.press('Enter');await baseReview.getByRole('button',{name:'Use reviewed current panel starting point',exact:true}).press('Enter');await expect(baseReview).toHaveCount(0);await expect(page.getByLabel('Original panel command history')).toContainText('Outcome unknown');expect(writes).toHaveLength(1)
 const body=writes[0],fingerprint=await qualificationFingerprint(body,state.page.revision)
 expect(body.version).toBe(0);expect(body.panel).toEqual({organiser_id:organiser,mentor_ids:[mentor]})
 state.denied=true;await page.evaluate(()=>dispatchEvent(new Event('focus')));await expect(page.getByLabel('Recorded panel reason',{exact:true})).toHaveCount(0);await expect(page.getByLabel('Original panel command history')).toContainText('Expected panel version 0')
 await page.route(`**/api/v1/events/${firstID}/mentor-qualification/${run}/commands/${body.request_id}`,route=>route.fulfill({json:{receipt:{request_id:body.request_id,run_id:run,action:body.action,expected_revision:state.page.revision,expected_version:0,version:1,fingerprint}}}))
 await page.getByRole('button',{name:'Read original panel acknowledgement',exact:true}).press('Enter');await expect(page.getByText('Original configure panel acknowledged at panel version 1. Current panel state must be read separately.',{exact:true})).toBeFocused();expect(writes).toHaveLength(1)
 expect(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth)).toBe(false)
})

async function pendingPanel(page:import('@playwright/test').Page,response:'ack'|'408'){
 const state=await fixture(page),writes:QualificationCommand[]=[],organiser=initial.round.panel.organiser_id,mentor=initial.round.panel.mentor_ids[0]
 state.page.round.version=0;state.page.round.cards=[];state.page.round.signoff=null;state.page.current=false;state.page.preview=null;state.page.capabilities={can_read_panel:false,can_configure_panel:true,can_propose:false,can_sign_off:false,read_only:false,record_locked:false}
 let release:()=>void=()=>{};const gate=new Promise<void>(resolve=>{release=resolve})
 await page.route(`**/api/v1/events/${firstID}/staff`,route=>route.fulfill({json:{items:[{person_id:organiser,display_name:'Current organising lead',role:'organiser',active:true,changed_at:'2027-06-20T15:00:00Z'},{person_id:mentor,display_name:'Current panel mentor',role:'mentor',active:true,changed_at:'2027-06-20T15:00:00Z'}]}}))
 await page.route(`**/api/v1/events/${firstID}/mentor-qualification/${run}`,async route=>{
  if(route.request().method()!=='POST')return state.denied?route.fulfill({status:403,json:{error:{code:'forbidden',message:'Synthetic reader denied',request_id:'synthetic'}}}):route.fulfill({json:state.page})
  const body=route.request().postDataJSON() as QualificationCommand;writes.push(body)
  const receipt={request_id:body.request_id,run_id:run,action:body.action,expected_revision:state.page.revision,expected_version:body.version,version:body.version+1,fingerprint:await qualificationFingerprint(body,state.page.revision)}
  await gate
  if(response==='408')return route.fulfill({status:408,json:{error:{code:'timeout',message:'Synthetic response timeout after dispatch',request_id:'synthetic'}}})
  return route.fulfill({json:{...state.page,receipt}})
 })
 await page.goto(path);await page.getByRole('button',{name:'Configure intended panel',exact:true}).press('Enter');await expect(page.getByRole('button',{name:'Review entered panel decision',exact:true})).toBeEnabled();await page.getByLabel('Recorded panel reason',{exact:true}).fill('Private panel reason must disappear on current denial');await page.getByRole('button',{name:'Review entered panel decision',exact:true}).press('Enter');await page.getByRole('alertdialog',{name:'Review private panel command',exact:true}).getByRole('button',{name:'Confirm reviewed panel command',exact:true}).press('Enter')
 await expect.poll(()=>writes.length).toBe(1)
 return {state,writes,release}
}
test('HTTP408 retains the original panel outcome as unknown without replay',async({page})=>{
 const task=await pendingPanel(page,'408');task.release()
 await expect(page.getByLabel('Original panel command history')).toContainText('Outcome unknown')
 await expect(page.getByRole('heading',{name:'Original panel outcome unknown',exact:true})).toBeVisible()
 expect(task.writes).toHaveLength(1)
})
for(const response of ['ack','408'] as const)test(`sent panel metadata protects pending and ${response} departure after private reader denial`,async({page})=>{
 const task=await pendingPanel(page,response)
 try{
  task.state.denied=true;await page.evaluate(()=>dispatchEvent(new Event('focus')));await expect(page.getByLabel('Recorded panel reason',{exact:true})).toHaveCount(0)
  await expect(page.getByLabel('Original panel command history')).toContainText('Awaiting acknowledgement')
  expect(await page.evaluate(()=>dispatchEvent(new Event('planning-departure',{cancelable:true})))).toBe(false)
  expect(await page.evaluate(()=>dispatchEvent(new Event('beforeunload',{cancelable:true})))).toBe(false)
  await page.getByRole('link',{name:test.info().project.metadata.portal==='regular'?'Hack in a Box home':'Hack in a Box · Admin home',exact:true}).click();await expect(page).toHaveURL(new RegExp(`${path}$`))
  task.release()
  if(response==='ack'){await expect(page.getByLabel('Original panel command history')).toHaveCount(0);expect(await page.evaluate(()=>dispatchEvent(new Event('planning-departure',{cancelable:true})))).toBe(true);return}
  await expect(page.getByLabel('Original panel command history')).toContainText('Outcome unknown')
  page.on('dialog',dialog=>void dialog.dismiss());expect(await page.evaluate(()=>dispatchEvent(new Event('planning-departure',{cancelable:true})))).toBe(false)
  await page.getByRole('link',{name:test.info().project.metadata.portal==='regular'?'Hack in a Box home':'Hack in a Box · Admin home',exact:true}).click();await expect(page).toHaveURL(new RegExp(`${path}$`))
  page.removeAllListeners('dialog');page.once('dialog',dialog=>void dialog.accept());await page.getByRole('link',{name:test.info().project.metadata.portal==='regular'?'Hack in a Box home':'Hack in a Box · Admin home',exact:true}).click();await expect(page).toHaveURL(/\/$/)
  task.state.denied=false;await page.goBack();await expect(page.getByLabel('Original panel command history')).toContainText('Outcome unknown');expect(task.writes).toHaveLength(1)
 }finally{task.release()}
})
test('authoritative excluded-target visibility change disposes already-open private history before its next poll',async({page})=>{
 const state=await fixture(page),excluded=state.page.teams[0],visible=state.page.teams[1],canary='EXCLUDED-TARGET-PRIVATE-DECISION-CANARY'
 state.page.current=false;state.page.round.signoff=null;state.page.preview=null;state.page.capabilities.record_locked=false;excluded.eligible=false;excluded.freeze_status='unresolved';state.page.round.cards=state.page.round.cards.filter(card=>card.team_id===visible.team_id)
 state.page.round.reason=canary;state.page.round.eligibility=[{team_id:excluded.team_id,binding:excluded.freeze_binding,decision:'ineligible',reason:canary,evidence:[{kind:'note',id:null,version:null,note:'Private supporting exclusion note'}],actor_id:state.page.round.panel.organiser_id,at:'2027-06-20T16:00:00Z'}]
 const original=structuredClone(state.page.round);let withheld=false,historyReads=0
 await page.route(`**/api/v1/events/${firstID}/mentor-qualification/${run}/history*`,route=>{historyReads++;return route.fulfill({json:{items:[{snapshot:withheld?state.page.round:original,current:false}],next_cursor:null}})})
 await page.goto(path);await page.getByRole('button',{name:'Read recorded panel history',exact:true}).press('Enter');const history=page.getByRole('region',{name:'Currently authorised panel history',exact:true});await history.getByText(/Panel version 4 · sign off/).press('Enter');await expect(history.getByText(`Reason: ${canary}`,{exact:true})).toBeVisible();expect(historyReads).toBe(1)
 withheld=true;state.page.round.eligibility=[];state.page.round.reason='';await page.getByRole('button',{name:'Read current panel source',exact:true}).press('Enter')
 await expect(page.getByText(canary,{exact:false})).toHaveCount(0);expect(historyReads).toBe(1)
 await page.getByRole('button',{name:'Read recorded panel history',exact:true}).press('Enter');await history.getByText(/Panel version 4 · sign off/).press('Enter');await expect(history).not.toContainText(canary);await expect(history).toContainText(`Team ${visible.team_id} · Card version 2`)
})

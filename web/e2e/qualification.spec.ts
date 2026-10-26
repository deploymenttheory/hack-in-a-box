import {test,expect} from '@playwright/test'
import {mockWorkspace,firstID} from './fixtures'
import {qualificationGuidePage} from '../src/styleguide/qualification-fixture'
import {confirmReviewedDialog} from '../qualification-native/confirmation'
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
 await page.getByRole('link',{name:'Back to panel',exact:true}).press('Enter');await expect(page.getByRole('button',{name:`Inspect Team ${team}`,exact:true})).toBeFocused()
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

async function pendingPanel(page:import('@playwright/test').Page,response:'ack'|'408',beforeLoad?:(state:Awaited<ReturnType<typeof fixture>>)=>Promise<void>){
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
 if(beforeLoad)await beforeLoad(state)
 await page.goto(path);await page.getByRole('button',{name:'Configure intended panel',exact:true}).press('Enter');await expect(page.getByRole('button',{name:'Review entered panel decision',exact:true})).toBeEnabled();await page.getByLabel('Recorded panel reason',{exact:true}).fill('Private panel reason must disappear on current denial');await page.getByRole('button',{name:'Review entered panel decision',exact:true}).press('Enter');await page.getByRole('alertdialog',{name:'Review private panel command',exact:true}).getByRole('button',{name:'Confirm reviewed panel command',exact:true}).press('Enter')
 await expect.poll(()=>writes.length).toBe(1)
 return {state,writes,release}
}
test('HTTP408 retains the original panel outcome as unknown without replay',async({page})=>{
 const task=await pendingPanel(page,'408');task.release()
 await expect(page.getByLabel('Original panel command history')).toContainText('Outcome unknown')
 await expect(page.getByRole('heading',{name:'Original panel outcome unknown',exact:true})).toBeVisible()
 await expect(page.getByRole('button',{name:'Configure intended panel',exact:true})).toHaveAttribute('disabled','')
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
test('retired formal output never presents a retained historical card as current scoring evidence',async({page})=>{
 const state=await fixture(page),team=state.page.teams[0]
 team.outcome='withdrawn';team.eligible=false;team.card_source_current=false;team.can_save_card=false;team.can_sign_card=false;team.source=null;state.page.current=false;state.page.round.signoff=null;state.page.preview=null
 await page.goto(path);await page.getByRole('button',{name:`Inspect Team ${team.team_id}`,exact:true}).press('Enter')
 await expect(page.getByRole('region',{name:'Moderated final card',exact:true})).toContainText('No current moderated card is disclosed. Missing evidence is not a zero score.')
 await expect(page.getByText('Recorded reason: Synthetic recorded moderation reason',{exact:true})).toHaveCount(0)
 await expect(page.getByRole('button',{name:'Edit reasoned final card',exact:true})).toHaveCount(0);await expect(page.getByRole('button',{name:'Review saved final card confirmation',exact:true})).toHaveCount(0)
})
test('pending current read preserves keyboard return to the same team without permitting an action',async({page})=>{
 const state=await fixture(page),team=initial.teams[0].team_id;let hold=false,release:()=>void=()=>{}
 const gate=new Promise<void>(resolve=>{release=resolve})
 await page.route(`**/api/v1/events/${firstID}/mentor-qualification/${run}`,async route=>{if(hold)await gate;return route.fulfill({json:state.page})})
 try{
  await page.goto(path);await page.getByRole('button',{name:`Inspect Team ${team}`,exact:true}).press('Enter')
  hold=true;await page.evaluate(()=>dispatchEvent(new Event('focus')))
  await page.getByRole('link',{name:'Back to panel',exact:true}).press('Enter')
  const source=page.getByRole('button',{name:`Inspect Team ${team}`,exact:true});await expect(source).toBeDisabled();await expect(source).toBeFocused()
  await source.press('Enter');await expect(page.getByRole('region',{name:'Moderated final card',exact:true})).toHaveCount(0)
  release();await expect(source).toBeEnabled();await expect(source).toBeFocused();await source.press('Enter')
  await expect(page.getByRole('region',{name:'Moderated final card',exact:true})).toContainText('Synthetic recorded moderation reason')
 }finally{release()}
})
test('matching late original acknowledgement preserves the next action after current polling disposes its private editor',async({page})=>{
 const state=await fixture(page),team=state.page.teams[0],card=state.page.round.cards[0]
 state.page.current=false;state.page.round.signoff=null;state.page.preview=null;state.page.capabilities={can_read_panel:true,can_configure_panel:false,can_propose:false,can_sign_off:false,record_locked:false,read_only:false}
 card.version=1;card.state='draft';team.card_version=1;team.card_state='draft';team.can_sign_card=true;team.source=card.source
 let posted=false,release:()=>void=()=>{};const gate=new Promise<void>(resolve=>{release=resolve})
 await page.route(`**/api/v1/events/${firstID}/mentor-qualification/${run}`,async route=>{
  if(route.request().method()!=='POST')return route.fulfill({json:state.page})
  const body=route.request().postDataJSON() as QualificationCommand,revision=state.page.revision
  const receipt={request_id:body.request_id,run_id:run,action:body.action,expected_revision:revision,expected_version:body.version,version:body.version+1,fingerprint:await qualificationFingerprint(body,revision)}
  state.page.revision++;state.page.round.version++;card.version=2;card.state='signed';team.card_version=2;team.card_state='signed';team.can_sign_card=false;posted=true
  await gate;return route.fulfill({json:{...state.page,receipt}})
 })
 try{
  await page.goto(path);await page.getByRole('button',{name:`Inspect Team ${team.team_id}`,exact:true}).press('Enter');await expect(page.getByRole('heading',{name:`Team ${team.team_id}`,exact:true,level:2})).toBeFocused();await page.getByRole('button',{name:'Review saved final card confirmation',exact:true}).press('Enter')
  await page.getByLabel('Recorded panel reason',{exact:true}).fill('Private captured confirmation reason')
  await page.getByRole('button',{name:'Review entered panel decision',exact:true}).press('Enter');await page.getByRole('alertdialog',{name:'Review private panel command',exact:true}).getByRole('button',{name:'Confirm reviewed panel command',exact:true}).press('Enter')
  await expect.poll(()=>posted).toBe(true);await page.evaluate(()=>dispatchEvent(new Event('focus')))
  await expect(page.getByRole('region',{name:'Moderated final card',exact:true})).toHaveCount(0);await expect(page.getByLabel('Original panel command history')).toContainText('Awaiting acknowledgement')
  release()
  await expect(page.getByRole('region',{name:'Moderated final card',exact:true})).toContainText('Version 2 · signed')
  await expect(page.getByRole('status').filter({hasText:'Original sign card acknowledged for request'})).toBeVisible()
  await expect(page.getByRole('button',{name:'Review another team',exact:true})).toBeFocused()
  // Pin the closing return frame until a newer manual inspector owns focus.
  // Inspector autofocus is a real passive effect; no test directly focuses it.
  await page.evaluate(()=>{
   const request=window.requestAnimationFrame.bind(window),cancel=window.cancelAnimationFrame.bind(window),frames=new Map<number,FrameRequestCallback>();let id=1_000_000
   window.requestAnimationFrame=callback=>{frames.set(++id,callback);return id}
   window.cancelAnimationFrame=value=>{if(!frames.delete(value))cancel(value)}
   Object.assign(window,{finishHeldQualificationFrames:async()=>{window.requestAnimationFrame=request;window.cancelAnimationFrame=cancel;await Promise.all([...frames.values()].map(callback=>new Promise<void>(resolve=>request(time=>{callback(time);resolve()}))))}})
  })
  await page.keyboard.press('Enter')
  for(const target of [state.page.teams[1],state.page.teams[1],team]){
   const inspect=page.getByRole('button',{name:`Inspect Team ${target.team_id}`,exact:true});await expect(inspect).toBeEnabled();await inspect.press('Enter')
   await expect(page.getByRole('heading',{name:`Team ${target.team_id}`,exact:true,level:2})).toBeFocused()
   if(target===state.page.teams[1]){
    await page.evaluate(async()=>{const owner=window as unknown as {finishHeldQualificationFrames?:()=>Promise<void>};await owner.finishHeldQualificationFrames?.();delete owner.finishHeldQualificationFrames})
    await expect(page.getByRole('heading',{name:`Team ${target.team_id}`,exact:true,level:2})).toBeFocused()
   }
   await page.getByRole('link',{name:'Back to panel',exact:true}).press('Enter');await expect(inspect).toBeFocused()
  }
  await expect(page.getByRole('status').filter({hasText:'Original sign card acknowledged for request'})).toBeVisible()
 }finally{release()}
})
test('pending current reads retain saved-card keyboard focus while confirmation stays blocked',async({page})=>{
 const state=await fixture(page),team=state.page.teams[0],card=state.page.round.cards[0]
 state.page.current=false;state.page.round.signoff=null;state.page.preview=null;state.page.capabilities.record_locked=false;card.state='draft';team.can_sign_card=true;team.source=card.source
 let hold=false,release:()=>void=()=>{};const gate=new Promise<void>(resolve=>{release=resolve})
 await page.route(`**/api/v1/events/${firstID}/mentor-qualification/${run}`,async route=>{if(hold)await gate;return route.fulfill({json:state.page})})
 try{
  await page.goto(path);await page.getByRole('button',{name:`Inspect Team ${team.team_id}`,exact:true}).press('Enter')
  const confirm=page.getByRole('button',{name:'Review saved final card confirmation',exact:true})
  await confirm.press('Tab');await page.keyboard.press('Shift+Tab');await expect(confirm).toBeFocused()
  hold=true;await page.evaluate(()=>dispatchEvent(new Event('focus')))
  await expect(confirm).toBeDisabled();await expect(confirm).toBeFocused();await page.keyboard.press('Enter')
  await expect(page.getByLabel('Recorded panel reason',{exact:true})).toHaveCount(0)
  release();await expect(confirm).toBeEnabled();await expect(confirm).toBeFocused();await page.keyboard.press('Enter')
  await expect(page.getByLabel('Recorded panel reason',{exact:true})).toBeVisible()
 }finally{release()}
})
test('resolved retained tie records do not report an unresolved tie after exact panel signoff',async({page})=>{
 const state=await fixture(page)
 state.page.preview!.ties=[{id:'retained-category-tie',kind:'category',category_id:state.page.criteria.categories[0].id,team_ids:state.page.teams.map(team=>team.team_id),seats:1}]
 state.page.preview!.blockers=[];state.page.round.proposal=structuredClone(state.page.preview)
 await page.goto(path);await expect(page.getByText('Current private panel signoff',{exact:true})).toBeVisible()
 await expect(page.getByText(/ties require evidence review and recorded panel decisions/)).toHaveCount(0)
 await page.getByRole('button',{name:'Preview category preparation',exact:true}).press('Enter');await expect(page.getByRole('region',{name:'Private category preparation',exact:true})).toBeVisible()
})

for(const mode of ['accepted','interrupted','changed','denied','408','lost_ack','invalid_ack','wrong_target'] as const)test(`native reviewed confirmation owns accepted input and preserves barriers: ${mode}`,async({page})=>{
 const state=await fixture(page),team=state.page.teams[0],card=state.page.round.cards[0]
 state.page.current=false;state.page.round.signoff=null;state.page.preview=null;state.page.capabilities.record_locked=false;card.state='draft';team.can_sign_card=true;team.source=card.source
 let hold=false,writes=0,release:()=>void=()=>{};const gate=new Promise<void>(resolve=>{release=resolve})
 await page.route(`**/api/v1/events/${firstID}/mentor-qualification/${run}`,async route=>{
  if(route.request().method()==='POST'){
   writes++;if(mode==='lost_ack')return route.abort('failed');if(mode==='408')return route.fulfill({status:408,json:{error:{code:'timeout',message:'Synthetic accepted request lost its response',request_id:'synthetic'}}})
   const body=route.request().postDataJSON() as QualificationCommand
   return route.fulfill({json:{...state.page,receipt:{request_id:mode==='invalid_ack'?'00000000-0000-4000-8000-999999999999':body.request_id,run_id:run,action:body.action,expected_revision:state.page.revision,expected_version:body.version,version:body.version+1,fingerprint:await qualificationFingerprint(body,state.page.revision)}}})
  }
  if(hold)await gate
  return state.denied?route.fulfill({status:403,json:{error:{code:'forbidden',message:'Synthetic current denial',request_id:'synthetic'}}}):route.fulfill({json:state.page})
 })
 const press=page.keyboard.press.bind(page.keyboard)
 try{
  await page.goto(path);await page.getByRole('button',{name:`Inspect Team ${team.team_id}`,exact:true}).press('Enter');await expect(page.getByRole('heading',{name:`Team ${team.team_id}`,exact:true,level:2})).toBeFocused();await page.getByRole('button',{name:'Review saved final card confirmation',exact:true}).press('Enter')
  await page.getByLabel('Recorded panel reason',{exact:true}).fill('Exact reviewed card remains unsent during current read')
  await page.getByRole('button',{name:'Review entered panel decision',exact:true}).press('Enter')
  const dialog=page.getByRole('alertdialog',{name:'Review private panel command',exact:true}),confirm=dialog.getByRole('button',{name:'Confirm reviewed panel command',exact:true})
  let interrupted=false,keys=0,redirected=false
  page.keyboard.press=async(key,options)=>{
   if(key==='Enter')keys++
   if(key==='Enter'&&mode==='wrong_target'&&!redirected){redirected=true;await press('Shift+Tab')}
   if(key==='Enter'&&!interrupted&&['interrupted','changed','denied'].includes(mode)){interrupted=true;hold=true;await page.evaluate(()=>dispatchEvent(new Event('focus')));await expect(confirm).toBeDisabled()}
   await press(key,options)
   if(interrupted&&hold){
    expect(writes).toBe(0);await expect(dialog).toContainText('Exact reviewed card remains unsent during current read');await expect(page.getByLabel('Original panel command history')).toHaveCount(0)
    if(mode==='changed'){state.page.revision++;state.page.round.version++;team.freeze_binding='changed-current-final-source';team.card_source_current=false;team.can_sign_card=false}
    if(mode==='denied')state.denied=true
    release();hold=false
   }
  }
  const result=confirmReviewedDialog(dialog,`/api/v1/events/${firstID}/mentor-qualification/${run}`)
  if(mode==='changed'||mode==='denied'||mode==='wrong_target'){
   await expect(result).rejects.toThrow();expect(writes).toBe(0);expect(keys).toBe(1);await expect(dialog).toHaveCount(0)
   if(mode==='denied')await expect(page.getByText('Current panel access is unavailable. Private selections, evidence and earlier review context have been cleared.',{exact:true})).toBeVisible()
   if(mode==='wrong_target')await expect(page.getByLabel('Recorded panel reason',{exact:true})).toHaveValue('Exact reviewed card remains unsent during current read')
   else await expect(page.getByLabel('Recorded panel reason',{exact:true})).toHaveCount(0)
  }else if(mode==='408'||mode==='lost_ack'||mode==='invalid_ack'){
   await expect(result).rejects.toThrow();expect(writes).toBe(1);expect(keys).toBe(1)
   await expect(page.getByLabel('Original panel command history')).toContainText('Outcome unknown');await expect(page.getByRole('status').filter({hasText:'Original sign card acknowledged for request'})).toHaveCount(0)
  }else{
   await expect(result).resolves.toEqual({acceptedClicks:1,posts:1,resumed:mode==='interrupted'});expect(writes).toBe(1);expect(keys).toBe(mode==='interrupted'?2:1)
   await expect(page.getByRole('status').filter({hasText:'Original sign card acknowledged for request'})).toBeVisible()
  }
 }finally{page.keyboard.press=press;release()}
})

for(const label of ['Read recorded panel history','Read current panel source'])test(`${label} keeps native keyboard focus during a current source pause without opening or writing`,async({page})=>{
 const state=await fixture(page)
 let held=false,reads=0,historyReads=0,writes=0,release:()=>void=()=>{}
 const gate=new Promise<void>(resolve=>{release=resolve})
 await page.route(`**/api/v1/events/${firstID}/mentor-qualification/${run}`,async route=>{
  reads++;if(held)await gate
  await route.fulfill({json:state.page})
 })
 await page.route(`**/api/v1/events/${firstID}/mentor-qualification/${run}/history?*`,route=>{historyReads++;return route.fulfill({json:{items:[],next_cursor:null}})})
 await page.goto(path)
 const historyButton=page.getByRole('button',{name:label,exact:true})
 const {reach}=await import('../qualification-native/keyboard')
 await reach(historyButton)
 page.on('request',request=>{if(!['GET','HEAD','OPTIONS'].includes(request.method()))writes++})
 try{
  const priorReads=reads
  held=true;await page.evaluate(()=>dispatchEvent(new Event('focus')))
  await expect.poll(()=>reads).toBeGreaterThan(priorReads)
  await expect(historyButton).toBeDisabled()
  await expect(historyButton).toBeFocused({timeout:750})
  await page.keyboard.press('Enter')
  expect(historyReads).toBe(0);expect(writes).toBe(0)
  await expect(page.getByRole('region',{name:'Currently authorised panel history',exact:true})).toHaveCount(0)
  held=false;release()
  await expect(historyButton).toBeEnabled();await expect(historyButton).toBeFocused()
  const beforeActivation=reads
  await page.keyboard.press('Enter')
  if(label==='Read recorded panel history'){await expect(page.getByRole('region',{name:'Currently authorised panel history',exact:true})).toBeVisible();expect(historyReads).toBe(1)}
  else{await expect.poll(()=>reads).toBe(beforeActivation+1);expect(historyReads).toBe(0)}
  expect(writes).toBe(0)
 }finally{held=false;release()}
})

for(const preparing of [false,true])test(`${preparing?'Unsent review':'Final-card editor'} keeps focus only during a benign read pause and accepts no disabled action`,async({page})=>{
 const state=await fixture(page)
 state.page.capabilities.record_locked=false;state.page.teams[0].can_save_card=true
 let held=false,reads=0,writes=0,release:()=>void=()=>{}
 const gate=new Promise<void>(resolve=>{release=resolve})
 await page.route(`**/api/v1/events/${firstID}/mentor-qualification/${run}`,async route=>{reads++;if(held)await gate;await route.fulfill({json:state.page})})
 await page.goto(path)
 await page.getByRole('button',{name:`Inspect Team ${state.page.teams[0].team_id}`,exact:true}).click()
 const edit=page.getByRole('button',{name:'Edit reasoned final card',exact:true}),reason=page.getByLabel('Recorded panel reason',{exact:true})
 if(preparing){await edit.click();await reason.fill('Retained private unsent final-card observation')}
 const target=preparing?page.getByRole('button',{name:'Review entered panel decision',exact:true}):edit
 const {reach}=await import('../qualification-native/keyboard');await reach(target)
 page.on('request',request=>{if(!['GET','HEAD','OPTIONS'].includes(request.method()))writes++})
 try{
  const priorReads=reads;held=true;await page.evaluate(()=>dispatchEvent(new Event('focus')))
  await expect.poll(()=>reads).toBeGreaterThan(priorReads)
  await expect(target).toBeDisabled();await expect(target).toBeFocused({timeout:750})
  await page.keyboard.press('Enter')
  await expect(page.getByRole('alertdialog',{name:'Review private panel command',exact:true})).toHaveCount(0)
  if(!preparing)await expect(reason).toHaveCount(0)
  expect(writes).toBe(0)
  held=false;release();await expect(target).toBeEnabled();await expect(target).toBeFocused()
  await page.keyboard.press('Enter')
  if(preparing){await expect(page.getByRole('alertdialog',{name:'Review private panel command',exact:true})).toBeVisible();await expect(reason).toHaveValue('Retained private unsent final-card observation')}
  else await expect(reason).toBeVisible()
  expect(writes).toBe(0)
 }finally{held=false;release()}
})

for(const empty of [false,true])test(`declared original record accepts exact receipt with ${empty?'empty rows':'delayed unrelated labels'}`,async({page})=>{
 let releaseLabels:()=>void=()=>{};const gate=new Promise<void>(resolve=>{releaseLabels=resolve})
 const task=await pendingPanel(page,'408',async(state)=>{if(empty)state.page.teams=[];await page.route(`**/api/v1/events/${firstID}/teams`,async route=>{await gate;await route.fulfill({json:{teams:[{id:initial.teams[0].team_id,name:'Disclosed current team'}]}})})})
 task.release()
 const history=page.getByLabel('Original panel command history')
 await expect(history).toContainText('Outcome unknown')
 await expect(page.getByRole('heading',{name:'Original panel outcome unknown',exact:true})).toBeVisible()
 if(empty)await expect(page.getByRole('button',{name:/^Inspect /})).toHaveCount(0)
 const label=page.getByRole('button',{name:'Inspect Disclosed current team',exact:true})
 const ack=page.getByRole('button',{name:'Read original panel acknowledgement',exact:true}),element=ack.elementHandle.bind(ack)
 let snapshots=0,ackReads=0,enters=0
 const body=task.writes[0],fingerprint=await qualificationFingerprint(body,task.state.page.revision)
 await page.route(`**/api/v1/events/${firstID}/mentor-qualification/${run}/commands/${body.request_id}`,route=>{ackReads++;return route.fulfill({json:{receipt:{request_id:body.request_id,run_id:run,action:body.action,expected_revision:task.state.page.revision,expected_version:0,version:1,fingerprint}}})})
 ack.elementHandle=async(...args)=>{const handle=await element(...args);if(handle){const capture=handle.evaluateHandle.bind(handle);handle.evaluateHandle=(async(...values:Parameters<typeof handle.evaluateHandle>)=>{const scope=await capture(...values),evaluate=scope.evaluate.bind(scope);scope.evaluate=(async(...values:Parameters<typeof scope.evaluate>)=>{const result=await evaluate(...values);if(typeof result==='string'&&result.startsWith('{"url"')&&snapshots++===0){releaseLabels();if(!empty)await expect(label).toBeVisible();await expect(history).toContainText('Outcome unknown')}return result}) as typeof scope.evaluate;return scope}) as typeof handle.evaluateHandle}return handle}
 const press=page.keyboard.press.bind(page.keyboard);page.keyboard.press=async(key,options)=>{if(key==='Enter')enters++;return press(key,options)}
 const {readOriginalAcknowledgement}=await import('../qualification-native/keyboard')
 try{await readOriginalAcknowledgement(ack,page.getByText('Original configure panel acknowledged at panel version 1. Current panel state must be read separately.',{exact:true}))}catch(error){console.log(JSON.stringify({ackReads,enters,writes:task.writes.length}));throw error}finally{releaseLabels()}
 expect(task.writes).toHaveLength(1);expect(ackReads).toBe(1);expect(enters).toBe(1);expect(snapshots).toBeGreaterThan(0)
})


test('owner awaits actual casting projection and ignores pending stale or different tie reads',async({page})=>{
 const frozenAt=new Date()
 // Let startup timers run, then pause at the same captured browser-clock anchor.
 await page.clock.install({time:new Date(frozenAt.getTime()-60_000)})
 const state=await fixture(page),category=state.page.criteria.categories.find(value=>value.label==='User experience')!,tieID='synthetic-ux-tie'
 state.page.current=false;state.page.round.signoff=null;state.page.capabilities.record_locked=false
 state.page.preview!.ties=[{id:tieID,kind:'category',category_id:category.id,team_ids:state.page.teams.map(value=>value.team_id),seats:1}]
 state.page.tie_actions=[{tie_id:tieID,seat:1,candidate_team_ids:[],can_vote:false,has_voted:false,can_resolve:false,resolve_team_ids:[],casting:false,required_votes:2,votes_complete:false}]
 let hold=false,reads=0,writes=0,release:()=>void=()=>{}
 const pending=new Promise<void>(resolve=>{release=resolve})
 await page.route(`**/api/v1/events/${firstID}/mentor-qualification/${run}`,async route=>{
  if(route.request().method()!=='GET'){writes++;return route.abort()}
  const projection=structuredClone(state.page);reads++
  if(hold)await pending
  await route.fulfill({json:projection})
 })
 await page.goto(path)
 const tie=page.getByRole('region',{name:'Affected tie: User experience · Seat 1',exact:true})
 await expect(tie).toContainText('Required current ballots incomplete')
 const setupRead=page.waitForResponse(response=>response.url()===new URL(`/api/v1/events/${firstID}/mentor-qualification/${run}`,page.url()).href&&response.request().method()==='GET'&&response.status()===200)
 await page.clock.pauseAt(frozenAt);await (await setupRead).finished()
 await expect(page.getByRole('button',{name:'Read current panel source',exact:true})).toBeEnabled()
 // This GET began before the second ballot and retains the earlier projection.
 hold=true;const before=reads;await page.evaluate(()=>dispatchEvent(new Event('focus')));await expect.poll(()=>reads).toBeGreaterThan(before)
 const {waitForCastingRead}=await import('../qualification-native/current-panel-read')
 let settled=false
 const current=waitForCastingRead(page,new URL(`/api/v1/events/${firstID}/mentor-qualification/${run}`,page.url()).href,category.id,1).then(()=>{settled=true})
 hold=false;release()
 // A following native read with a complete OTHER tie must not satisfy the barrier.
 state.page.tie_actions[0]={...state.page.tie_actions[0],votes_complete:true,casting:true,seat:2}
 await page.clock.runFor(15_000)
 await expect(tie).toHaveCount(0)
 expect(settled).toBe(false)
 state.page.tie_actions[0].seat=1;state.page.preview!.ties[0].category_id=state.page.criteria.categories[0].id
 await page.clock.runFor(15_000)
 await expect(page.getByRole('region',{name:'Affected tie: Demo · Seat 1',exact:true})).toContainText('Current ballots complete')
 expect(settled).toBe(false)
 state.page.preview!.ties[0].category_id=category.id
 await page.clock.runFor(15_000);await current
 await expect(tie).toContainText('Current ballots complete');await expect(tie).toContainText('Casting decision required')
 const {openLocal}=await import('../qualification-native/keyboard')
 const beforeExplicit=reads
 await openLocal(page.getByRole('button',{name:'Read current panel source',exact:true}),page.getByRole('heading',{name:'Moderation and selections',exact:true}),'visible')
 await expect.poll(()=>reads).toBe(beforeExplicit+1)
 expect(writes).toBe(0)
})

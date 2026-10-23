import { openEventSection } from './event-navigation-fixture'
import { expect, test } from '@playwright/test'
import { firstID, mockWorkspace } from './fixtures'
import type { components } from '../src/api/schema.gen'

test('focused setup keeps mounted edits and reviews the exact expected revision before a deliberate save',async({page})=>{
 const {events}=await mockWorkspace(page),writes:{revision:string;body:components['schemas']['EventInput']}[]=[]
 await page.route(`**/events/${firstID}`,async route=>{
  if(route.request().method()!=='PUT')return route.fallback()
  expect(route.request().headers()['x-csrf-token']).toBe('synthetic-csrf-token-32-characters')
  const body=route.request().postDataJSON();writes.push({revision:route.request().headers()['if-match'],body})
  Object.assign(events[0],{name:body.name,configuration:body.configuration,revision:2});await route.fulfill({json:events[0]})
 })
 await page.goto('/events/'+firstID);await openEventSection(page, 'Setup')
 await page.getByLabel('Event name',{exact:true}).fill('Reviewed event plan')
 await page.getByLabel('Description',{exact:true}).fill('Private planning draft')
 const sections=page.getByRole('navigation',{name:'Event setup sections'})
 await sections.getByRole('button',{name:'Attendance and dates',exact:true}).focus();await page.keyboard.press('Enter')
 await page.getByLabel('Minimum confirmed competitors',{exact:true}).fill('35')
 await sections.getByRole('button',{name:'Business calendar',exact:true}).click()
 await page.getByLabel('Excluded dates, one YYYY-MM-DD date per line').fill('2027-06-18')
 await sections.getByRole('button',{name:'Event details',exact:true}).click()
 await expect(page.getByLabel('Event name',{exact:true})).toHaveValue('Reviewed event plan')
 await page.getByRole('button',{name:'Save event',exact:true}).click()
 const review=page.getByRole('region',{name:'Review event configuration'})
 await expect(review).toContainText('35 confirmed competitors');await expect(review).toContainText('Expected revision')
 expect(writes).toHaveLength(0)
 await review.getByRole('button',{name:'Confirm save event',exact:true}).click()
 await expect(page.getByRole('status')).toContainText('Event saved. Revision 2.')
 await expect(page.getByRole('button',{name:'Review event readiness',exact:true})).toBeVisible()
 expect(writes).toHaveLength(1);expect(writes[0]).toMatchObject({revision:'"1"',body:{name:'Reviewed event plan',configuration:{minimum_attendance:35,calendar:{excluded_dates:['2027-06-18']}}}})
 expect(await page.evaluate(()=>JSON.stringify([localStorage,sessionStorage]))).not.toContain('Private planning draft')
})

test('stale setup retains input and only a reviewed native revision becomes the new write base',async({page})=>{
 const {events}=await mockWorkspace(page),revisions:string[]=[]
 await page.route(`**/events/${firstID}`,async route=>{
  if(route.request().method()!=='PUT')return route.fallback()
  revisions.push(route.request().headers()['if-match'])
  if(revisions.length===1){events[0].revision=7;return route.fulfill({status:409,json:{error:{code:'revision_conflict',request_id:'synthetic'}}})}
  const body=route.request().postDataJSON();Object.assign(events[0],{name:body.name,configuration:body.configuration,revision:8});return route.fulfill({json:events[0]})
 })
 await page.goto('/events/'+firstID);await openEventSection(page, 'Setup')
 await page.getByLabel('Event name',{exact:true}).fill('Retained proposal')
 await page.getByRole('button',{name:'Save event',exact:true}).click();await page.getByRole('button',{name:'Confirm save event',exact:true}).click()
 await expect(page.getByRole('heading',{name:'Event revision changed',exact:true})).toBeVisible()
 await page.getByRole('button',{name:'Read current event for review',exact:true}).click()
 await expect(page.getByText(/Current native revision 7/)).toBeVisible();expect(revisions).toEqual(['"1"'])
 await expect(page.getByLabel('Event name',{exact:true})).toHaveValue('Retained proposal')
 await page.getByRole('button',{name:'Use current event revision and keep my edits',exact:true}).click()
 await page.getByRole('button',{name:'Save event',exact:true}).click();await page.getByRole('button',{name:'Confirm save event',exact:true}).click()
 await expect(page.getByRole('status')).toContainText('Revision 8');expect(revisions).toEqual(['"1"','"7"'])
})

test('unknown setup outcome cannot replay and a latest resource remains distinct from a save receipt',async({page})=>{
 const {events}=await mockWorkspace(page);let writes=0
 await page.route(`**/events/${firstID}`,async route=>{
  if(route.request().method()!=='PUT')return route.fallback()
  writes++;const body=route.request().postDataJSON();Object.assign(events[0],{name:body.name,configuration:body.configuration,revision:2})
  return route.fulfill({status:503,json:{error:{code:'unavailable',request_id:'synthetic'}}})
 })
 await page.goto('/events/'+firstID);await openEventSection(page, 'Setup');await page.getByLabel('Event name',{exact:true}).fill('Unknown write')
 await page.getByRole('button',{name:'Save event',exact:true}).click();await page.getByRole('button',{name:'Confirm save event',exact:true}).click()
 await expect(page.getByRole('heading',{name:'Save outcome unconfirmed',exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'Save event',exact:true})).toBeDisabled()
 await page.getByRole('button',{name:'Read current event for review',exact:true}).click()
 await expect(page.getByText(/Current native revision 2/)).toBeVisible();await expect(page.getByText('Current native state is not an exact receipt for the attempted save.',{exact:false})).toBeVisible()
 expect(writes).toBe(1);await expect(page.getByLabel('Event name',{exact:true})).toHaveValue('Unknown write')
})

test('transient event authority failure pauses mounted input and definite denial removes it',async({page})=>{
 await mockWorkspace(page);let outcome:'ready'|'failed'|'denied'='ready',writes=0
 await page.route(`**/events/${firstID}`,async route=>{
  if(route.request().method()!=='GET'){writes++;return route.fallback()}
  if(outcome==='ready')return route.fallback()
  await route.fulfill({status:outcome==='failed'?503:403,json:{error:{code:outcome==='failed'?'unavailable':'forbidden',request_id:'synthetic'}}})
 })
 await page.goto('/events/'+firstID);await openEventSection(page, 'Setup');await page.getByLabel('Description',{exact:true}).fill('Private retained description')
 outcome='failed';await page.evaluate(()=>window.dispatchEvent(new Event('visibilitychange')))
 const check=page.getByRole('button',{name:'Check current event access',exact:true});await expect(check).toBeVisible()
 await expect(page.getByLabel('Description',{exact:true})).toHaveValue('Private retained description');await expect(page.getByRole('button',{name:'Save event',exact:true})).toBeDisabled();expect(writes).toBe(0)
 outcome='denied';await check.click();await expect(page.getByLabel('Description',{exact:true})).toHaveCount(0)
 expect(await page.evaluate(()=>JSON.stringify([localStorage,sessionStorage]))).not.toContain('Private retained description')
})

test('DST review preserves wall time until native resolution and uses explicit earlier occurrence',async({page})=>{
 await mockWorkspace(page);let writes=0;const resolved:{wall_time:string;timezone:string;dst_choice:string}[]=[]
 await page.route(`**/events/${firstID}`,route=>{if(route.request().method()==='PUT')writes++;return route.fallback()})
 await page.route('**/api/v1/time/resolve',route=>{
  const body=route.request().postDataJSON();resolved.push(body)
  if(body.wall_time==='2027-10-31T01:30:00'&&!body.dst_choice)return route.fulfill({status:409,json:{error:{code:'event_blocked',request_id:'synthetic',blockers:[{field:'wall_time',code:'ambiguous_time',message:'Choose the earlier or later occurrence.'}]}}})
  return route.fulfill({json:{instant:body.wall_time==='2027-10-31T01:30:00'?'2027-10-31T00:30:00Z':body.wall_time+'Z'}})
 })
 await page.goto('/events/'+firstID);await openEventSection(page, 'Setup');await page.getByRole('button',{name:'Attendance and dates',exact:true}).click();await page.getByLabel('Live start',{exact:true}).fill('2027-10-31T01:30')
 await page.getByRole('button',{name:'Preview dates and blockers',exact:true}).click();await expect(page.getByRole('alert')).toContainText('Choose the earlier or later occurrence.')
 await expect(page.getByLabel('Live start',{exact:true})).toHaveValue('2027-10-31T01:30');expect(writes).toBe(0)
 await page.getByLabel('When a local time occurs twice',{exact:true}).selectOption('earlier');await page.getByRole('button',{name:'Preview dates and blockers',exact:true}).click()
 await expect(page.getByRole('region',{name:'Event preview'})).toBeVisible();expect(resolved).toContainEqual({wall_time:'2027-10-31T01:30:00',timezone:'Europe/London',dst_choice:'earlier'});expect(writes).toBe(0)
})

const rulesID='53333333-3333-4333-8333-333333333333', resourceID='54444444-4444-4444-8444-444444444444'
async function rulesFixture(page:import('@playwright/test').Page){
 await mockWorkspace(page)
 const doc:components['schemas']['ContentDocument']={title:'Competition rules',markdown:'# Rules\n\nBuild with care.',purpose:'rules',require_acknowledgement:true,contacts:[],audience:{confirmed:true,roles:[],teams:[]},attachments:[],scheduled_at:null}
 const rules:components['schemas']['ContentPage']={id:rulesID,kind:'page',revision:3,version:1,state:'released',document:doc,released_version:1,released_at:'2027-06-01T09:00:00Z',attachment_details:[],acknowledged_at:null,can_manage:true,message:''}
 const state={failed:false,denied:false,unknown:false,writes:[] as {revision:string;body:unknown}[],gets:0}
 await page.route(`**/events/${firstID}/content**`,async route=>{
  const request=route.request(),url=new URL(request.url()),path=url.pathname;state.gets++
  if(state.denied)return route.fulfill({status:403,json:{error:{code:'forbidden'}}})
  if(request.method()==='GET'){
   if(state.failed)return route.fulfill({status:503,json:{error:{code:'unavailable'}}})
   if(path.endsWith('/content'))return route.fulfill({json:{items:url.searchParams.has('cursor')?[rules]:[{...rules,id:resourceID,document:{...doc,purpose:'',title:'General reference'}}],next_cursor:url.searchParams.has('cursor')?null:'synthetic-guidance-cursor'}})
   if(path.endsWith('/history'))return route.fulfill({json:{items:[],next_cursor:0}})
   return route.fulfill({json:rules})
  }
  expect(request.headers()['x-csrf-token']).toBe('synthetic-csrf-token-32-characters');expect(request.headers()['if-match']).toBe('"'+rules.revision+'"')
  const body=request.postDataJSON();state.writes.push({revision:request.headers()['if-match'],body});rules.revision++
  if(request.method()==='PUT'){rules.document=body.document;rules.version++;rules.state='draft'}else {rules.state=body.action==='review'?'review':body.action==='approve'?'approved':body.action==='release'?'released':'cancelled';if(body.action==='release')rules.released_version=rules.version}
  if(state.unknown)return route.fulfill({status:503,json:{error:{code:'unavailable'}}})
  return route.fulfill({json:rules})
 })
 return {state,rules}
}

test('rules planning honestly pages authorised content and keeps exact save review approval release versions',async({page})=>{
 const {state,rules}=await rulesFixture(page);await page.goto('/events/'+firstID);await openEventSection(page, 'Rules')
 await expect(page.getByRole('status').filter({hasText:'0 rules/contact pages in 1 loaded'})).toBeVisible();await page.getByRole('button',{name:'More records',exact:true}).click()
 await page.getByRole('button',{name:'Plan Competition rules',exact:true}).click();await expect(page.getByLabel('Markdown',{exact:true})).toHaveValue('# Rules\n\nBuild with care.')
 await page.getByLabel('Markdown',{exact:true}).fill('# Revised rules\n\nKeep the preparation week.')
 await page.getByRole('button',{name:'Release audience',exact:true}).click();await page.getByRole('form',{name:'Content editor',exact:true}).getByLabel('Logistics',{exact:true}).check()
 await page.getByRole('button',{name:'Page content',exact:true}).click();await expect(page.getByLabel('Markdown',{exact:true})).toHaveValue('# Revised rules\n\nKeep the preparation week.')
 await page.getByRole('button',{name:'Save new draft',exact:true}).click();await expect(page.getByRole('status')).toContainText('Draft version 2 saved');expect(rules.released_version).toBe(1)
 await page.getByRole('button',{name:'Mark ready for review',exact:true}).click();await page.getByRole('button',{name:'Approve version 2',exact:true}).click();await expect(page.getByRole('status')).toContainText('Version approved')
 await page.getByRole('button',{name:'Release approved version',exact:true}).click();await expect(page.getByRole('status')).toContainText('Version released to its audience.')
 await page.getByRole('button',{name:'Close editor',exact:true}).click();await expect(page.getByRole('button',{name:'Plan Competition rules',exact:true})).toBeFocused();await expect(page.getByRole('link',{name:'Competition rules',exact:true})).toHaveCount(0)
 expect(rules.released_version).toBe(2);expect(rules.document.require_acknowledgement).toBe(true);expect(state.writes.map(write=>write.revision)).toEqual(['"3"','"4"','"5"','"6"'])
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
})

test('unknown rules write exposes current-source review without replay or latest-state receipt inference',async({page})=>{
 const {state}=await rulesFixture(page);await page.goto('/events/'+firstID);await openEventSection(page, 'Rules');await page.getByRole('button',{name:'More records',exact:true}).click();await page.getByRole('button',{name:'Plan Competition rules',exact:true}).click()
 await page.getByLabel('Markdown',{exact:true}).fill('Private uncertain rules');state.unknown=true
 await page.getByRole('button',{name:'Save new draft',exact:true}).click();await expect(page.getByText(/Write outcome is unconfirmed/)).toBeVisible();await expect(page.getByRole('button',{name:'Save new draft',exact:true})).toBeDisabled()
 state.unknown=false;await page.getByRole('button',{name:'Review latest saved version',exact:true}).click();await expect(page.getByRole('region',{name:'Latest saved content'})).toContainText('Version 2')
 expect(state.writes).toHaveLength(1);await expect(page.getByLabel('Markdown',{exact:true})).toHaveValue('Private uncertain rules')
 await page.getByLabel('I reviewed the latest saved version',{exact:true}).check();await page.getByRole('button',{name:'Keep my edits for a new draft',exact:true}).click()
 expect(state.writes).toHaveLength(1)
})

test('staff grant conflicts leave active and waitlisted claims unchanged and require explicit withdrawal',async({page})=>{
 await mockWorkspace(page)
 const claims=[{personID:'58888888-8888-4888-8888-888888888881',state:'confirmed'},{personID:'58888888-8888-4888-8888-888888888882',state:'waitlisted'}] as const
 const requests:{person_id:string;role:string;active:boolean;acknowledge_competition_restriction:boolean}[]=[]
 await page.route(`**/events/${firstID}/staff`,route=>{if(route.request().method()!=='PUT')return route.fallback();const body=route.request().postDataJSON();requests.push(body);expect(claims.some(claim=>claim.personID===body.person_id)).toBe(true);return route.fulfill({status:409,json:{error:{code:'withdrawal_required',request_id:'synthetic'}}})})
 await page.goto('/events/'+firstID);await openEventSection(page,'Staff')
 for(const claim of claims)for(const role of ['organiser','early_brief_access']){
  await page.getByLabel('Person account ID').fill(claim.personID);await page.getByLabel('Role',{exact:true}).selectOption(role)
  await page.getByLabel(/Acknowledge the lasting competition restriction/).check();await page.getByRole('button',{name:'Assign role',exact:true}).click()
  await expect(page.getByRole('alert')).toContainText('explicitly withdraw their active or waitlisted attendee registration')
  await expect(page.getByLabel('Person account ID')).toHaveValue(claim.personID)
 }
 expect(requests.map(request=>[request.person_id,request.role,request.active,request.acknowledge_competition_restriction])).toEqual(claims.flatMap(claim=>['organiser','early_brief_access'].map(role=>[claim.personID,role,true,true])))
 expect(claims.map(claim=>claim.state)).toEqual(['confirmed','waitlisted'])
})

for(const leave of ['event','account'] as const)test(`late rules save response after ${leave} departure cannot refill private inputs or continue another command`,async({page})=>{
 const {state}=await rulesFixture(page);let release!:()=>void,started!:()=>void
 const pending=new Promise<void>(resolve=>{release=resolve}),sending=new Promise<void>(resolve=>{started=resolve})
 await page.route(`**/events/${firstID}/content/${rulesID}`,async route=>{if(route.request().method()!=='PUT')return route.fallback();started();await pending;return route.fallback()})
 await page.goto('/events/'+firstID);await openEventSection(page,'Rules');await page.getByRole('button',{name:'More records',exact:true}).click();await page.getByRole('button',{name:'Plan Competition rules',exact:true}).click()
 await page.getByLabel('Markdown',{exact:true}).fill('Private late response marker');await page.getByRole('button',{name:'Save new draft',exact:true}).click();await sending
 if(leave==='event'){await page.getByRole('link',{name:'← Switch event',exact:true}).click();await expect(page).toHaveURL(new RegExp('/events/'+firstID+'$'));await page.evaluate(()=>window.dispatchEvent(new Event('session-expired')));await expect(page.getByRole('button',{name:'Save new draft',exact:true})).toHaveCount(0)}
 else {await page.evaluate(()=>window.dispatchEvent(new Event('session-expired')));await expect(page.getByRole('button',{name:'Save new draft',exact:true})).toHaveCount(0)}
 const response=page.waitForResponse(response=>response.request().method()==='PUT'&&new URL(response.url()).pathname.endsWith('/content/'+rulesID));release();await response
 await expect(page.getByLabel('Markdown',{exact:true})).toHaveCount(0);await expect(page.getByText('Private late response marker',{exact:true})).toHaveCount(0)
 expect(state.writes).toHaveLength(1);expect(await page.evaluate(()=>JSON.stringify([localStorage,sessionStorage]))).not.toContain('Private late response marker')
})

test('confirmed-attendance shortfall remains a native go gate and post-publication exception is deliberate',async({page})=>{
 const {events}=await mockWorkspace(page),decisions:unknown[]=[];Object.assign(events[0],{stage:'preparation',published_at:'2027-06-13T09:00:00Z',confirmed_attendees:28})
 await page.route(`**/events/${firstID}/transition`,route=>{
  const request=route.request(),body=request.postDataJSON();expect(request.headers()['if-match']).toBe('"1"');expect(request.headers()['x-csrf-token']).toBe('synthetic-csrf-token-32-characters');decisions.push(body)
  if(!body.attendance_exception)return route.fulfill({status:409,json:{error:{code:'event_blocked',blockers:[{field:'minimum_attendance',code:'attendance_shortfall',message:'Only confirmed competitors count. Two more are required.'}]}}})
  Object.assign(events[0],{stage:'live',revision:2});return route.fulfill({json:events[0]})
 })
 await page.goto('/events/'+firstID);await expect(page.getByText('Only confirmed competitors count toward the minimum.',{exact:false})).toBeVisible()
 await page.getByRole('button',{name:'Advance to live',exact:true}).click();await expect(page.getByRole('alert')).toContainText('Two more are required')
 await page.getByLabel('Approve proceeding below minimum attendance after post-publication withdrawals',{exact:true}).check();await page.getByLabel('Reason for an exception, cancellation or reopening',{exact:true}).fill('Approved after recorded withdrawals')
 await page.getByRole('button',{name:'Advance to live',exact:true}).click();await expect(page.getByRole('button',{name:'Advance to assessment',exact:true})).toBeVisible()
 expect(decisions).toEqual([{stage:'live',reason:'',attendance_exception:false},{stage:'live',reason:'Approved after recorded withdrawals',attendance_exception:true}])
})

test('schedule approval shows native event-local times and retains a separate approved state',async({page})=>{
 await mockWorkspace(page);const id='56666666-6666-4666-8666-666666666666';let approved=false,writes=0
 await page.route(`**/events/${firstID}/schedule**`,route=>{
  const request=route.request();if(request.method()==='POST'){writes++;expect(request.headers()['if-match']).toBe('"1"');expect(request.headers()['x-csrf-token']).toBe('synthetic-csrf-token-32-characters');approved=true}
  return route.fulfill({json:{items:[{item:{id,title:'Venue walkthrough',audience:['logistics'],anchor:'live_start',days:-1,minutes:0,business_days:false},due_at:'2027-06-19T09:00:00Z',status:approved?'approved':'draft',revision:approved?2:1}]}})
 })
 await page.goto('/events/'+firstID);await openEventSection(page,'Schedule');await expect(page.getByText('19 Jun 2027, 10:00 · draft',{exact:true})).toBeVisible();expect(writes).toBe(0)
 await page.getByRole('button',{name:'Approve Venue walkthrough',exact:true}).focus();await page.keyboard.press('Enter');await expect(page.getByText('19 Jun 2027, 10:00 · approved',{exact:true})).toBeVisible()
 expect(writes).toBe(1);await expect(page.getByRole('button',{name:'Approve Venue walkthrough',exact:true})).toHaveCount(0)
})

test('branding unknown write keeps the unsent palette separate from current native state and never replays',async({page})=>{
 await mockWorkspace(page);let writes=0,primary='#007e67',revision=1
 await page.route(`**/events/${firstID}/branding`,route=>{
  if(route.request().method()==='PUT'){writes++;expect(route.request().headers()['if-match']).toBe('"1"');primary=route.request().postDataJSON().primary;revision=4;return route.fulfill({status:503,json:{error:{code:'unavailable'}}})}
  return route.fulfill({json:{revision,branding:{primary,background:'#ffffff',foreground:'#192147',accent:'#007e67',assets:[]}}})
 })
 await page.goto('/events/'+firstID);await openEventSection(page,'Branding');await page.getByLabel('Primary colour',{exact:true}).fill('#233c78');await page.getByRole('button',{name:'Save branding',exact:true}).click()
 await expect(page.getByRole('status')).toContainText('outcome is unconfirmed');await expect(page.getByRole('button',{name:'Save branding',exact:true})).toBeDisabled()
 await page.getByRole('button',{name:'Read current branding for review',exact:true}).click();await expect(page.getByText(/Current native revision 4/)).toBeVisible();await expect(page.getByLabel('Primary colour',{exact:true})).toHaveValue('#233c78');expect(writes).toBe(1)
 await expect(page.getByText(/Latest native state is not an exact receipt/)).toBeVisible()
})

test('branding transient read failure retains mounted fields while definite denial removes them',async({page})=>{
 await mockWorkspace(page);let outcome:'ready'|'failed'|'denied'='ready'
 await page.route(`**/events/${firstID}/branding`,route=>outcome==='ready'?route.fallback():route.fulfill({status:outcome==='failed'?503:403,json:{error:{code:outcome==='failed'?'unavailable':'forbidden'}}}))
 await page.goto('/events/'+firstID);await openEventSection(page,'Branding');await page.getByLabel('Primary colour',{exact:true}).fill('#233c78')
 outcome='failed';await page.evaluate(()=>window.dispatchEvent(new Event('visibilitychange')));const check=page.getByRole('button',{name:'Check event branding access',exact:true});await expect(check).toBeVisible()
 await expect(page.getByLabel('Primary colour',{exact:true})).toHaveValue('#233c78');await expect(page.getByRole('button',{name:'Save branding',exact:true})).toBeDisabled()
 outcome='denied';await check.click();await expect(page.getByLabel('Primary colour',{exact:true})).toHaveCount(0)
})

test('review coverage navigation consumes its current native capability without inferring staff access',async({page})=>{
 const {events}=await mockWorkspace(page);Object.assign(events[0],{can_read_staff:false,can_read_review_coverage:true,can_edit:false,can_manage_owner:false,can_transition:false})
 await page.goto('/events/'+firstID);await openEventSection(page,'Review coverage')
 await expect(page.getByRole('link',{name:'Open current support and assessment dashboard',exact:true})).toHaveAttribute('href',`/events/${firstID}/review`)
 const select=page.getByRole('combobox',{name:'Event section',exact:true})
 if(await select.isVisible())await expect(select.locator('option[value="staff"]')).toHaveCount(0)
 else await expect(page.getByRole('navigation',{name:'Event sections',exact:true}).getByRole('button',{name:'Staff',exact:true})).toHaveCount(0)
})

test('native setup preview pauses editor input and a subsequent edit invalidates the reviewed snapshot',async({page})=>{
 await mockWorkspace(page);let release!:()=>void,started!:()=>void,writes=0
 const waiting=new Promise<void>(resolve=>{release=resolve}),sending=new Promise<void>(resolve=>{started=resolve})
 await page.route('**/api/v1/event-preview',async route=>{started();await waiting;return route.fallback()})
 await page.route(`**/events/${firstID}`,route=>{if(route.request().method()==='PUT')writes++;return route.fallback()})
 await page.goto('/events/'+firstID);await openEventSection(page,'Setup');await page.getByLabel('Event name',{exact:true}).fill('First reviewed name')
 await page.getByRole('button',{name:'Save event',exact:true}).click();await sending
 await expect(page.getByLabel('Event name',{exact:true})).toBeDisabled();await expect(page.getByRole('button',{name:'Save event',exact:true})).toBeDisabled();expect(writes).toBe(0)
 release();await expect(page.getByRole('region',{name:'Review event configuration',exact:true})).toContainText('First reviewed name')
 await page.getByLabel('Event name',{exact:true}).fill('New unsent name')
 await expect(page.getByRole('region',{name:'Review event configuration',exact:true})).toHaveCount(0);await expect(page.getByRole('region',{name:'Event preview',exact:true})).toHaveCount(0)
 await expect(page.getByLabel('Event name',{exact:true})).toHaveValue('New unsent name');expect(writes).toBe(0)
})

for (const source of ['preview','failure'] as const) test(`native ${source} blocker reveals and focuses its actual hidden setup field by keyboard`,async({page})=>{
 await mockWorkspace(page)
 const blocker={field:'configuration.calendar.excluded_dates',code:'invalid_date',message:'Review the excluded date reported by the server.'}
 await page.route('**/api/v1/event-preview',route=>route.fulfill(source==='preview'?{json:{challenge_publication:null,blockers:[blocker]}}:{status:422,json:{error:{code:'event_blocked',message:'Native date check',blockers:[blocker]}}}))
 await page.goto('/events/'+firstID);await openEventSection(page,'Setup')
 await page.getByRole('button',{name:'Preview dates and blockers',exact:true}).click()
 const link=page.getByRole('link',{name:blocker.message,exact:true});await expect(link).toBeVisible();await link.focus();await page.keyboard.press('Enter')
 const field=page.getByLabel('Excluded dates, one YYYY-MM-DD date per line');await expect(field).toBeVisible();await expect(field).toBeFocused()
 await field.fill('2027-06-18');await expect(field).toHaveValue('2027-06-18')
})

test('unknown setup capsule survives deliberate new reviewed command and section replacement',async({page})=>{
 const {events}=await mockWorkspace(page),revisions:string[]=[]
 await page.route(`**/events/${firstID}`,route=>{
  if(route.request().method()!=='PUT')return route.fallback()
  revisions.push(route.request().headers()['if-match']);const body=route.request().postDataJSON();Object.assign(events[0],{name:body.name,configuration:body.configuration,revision:revisions.length===1?4:5})
  return route.fulfill(revisions.length===1?{status:503,json:{error:{code:'unavailable'}}}:{json:events[0]})
 })
 await page.goto('/events/'+firstID);await openEventSection(page,'Setup');await page.getByLabel('Event name',{exact:true}).fill('Original uncertain command')
 await page.getByRole('button',{name:'Save event',exact:true}).click();await page.getByRole('button',{name:'Confirm save event',exact:true}).click()
 const history=page.getByRole('complementary',{name:'Unconfirmed command history'});await expect(history).toContainText('Expected revision 1')
 await page.getByRole('button',{name:'Read current event for review',exact:true}).click();await expect(page.getByText(/Current native revision 4/)).toBeVisible()
 await page.getByRole('button',{name:'Use current event revision and keep my edits',exact:true}).click()
 await page.getByLabel('Event name',{exact:true}).fill('New deliberate command')
 await page.getByRole('button',{name:'Save event',exact:true}).click();expect(revisions).toEqual(['"1"'])
 await page.getByRole('button',{name:'Confirm save event',exact:true}).click();await expect(page.getByRole('status').filter({hasText:'Event saved. Revision 5.'})).toBeVisible()
 expect(revisions).toEqual(['"1"','"4"']);await expect(history).toContainText('Original outcome remains unconfirmed');await page.screenshot({path:test.info().outputPath('uncertain-command-history.png'),fullPage:true})
 await openEventSection(page,'Overview');await openEventSection(page,'Setup');await expect(history).toContainText('Expected revision 1')
 await expect(page.getByRole('button',{name:'Save event',exact:true})).toBeEnabled()
 events[0].can_edit=false;await page.evaluate(()=>window.dispatchEvent(new Event('visibilitychange')));await expect(history).toHaveCount(0)
})

test('unknown branding capsule survives fresh base selection and later successful command',async({page})=>{
 await mockWorkspace(page);let revision=1,primary='#007e67';const writes:string[]=[]
 await page.route(`**/events/${firstID}/branding`,route=>{
  if(route.request().method()==='PUT'){writes.push(route.request().headers()['if-match']);primary=route.request().postDataJSON().primary;revision=writes.length===1?4:5;return route.fulfill(writes.length===1?{status:503,json:{error:{code:'unavailable'}}}:{json:{revision,branding:{primary,background:'#ffffff',foreground:'#192147',accent:'#007e67',assets:[]}}})}
  return route.fulfill({json:{revision,branding:{primary,background:'#ffffff',foreground:'#192147',accent:'#007e67',assets:[]}}})
 })
 await page.goto('/events/'+firstID);await openEventSection(page,'Branding');await page.getByLabel('Primary colour',{exact:true}).fill('#233c78');await page.getByRole('button',{name:'Save branding',exact:true}).click()
 const history=page.getByRole('complementary',{name:'Unconfirmed command history'});await expect(history).toContainText('Branding save')
 await page.getByRole('button',{name:'Read current branding for review',exact:true}).click();await expect(page.getByText(/Current native revision 4/)).toBeVisible();await page.getByRole('button',{name:'Use reviewed branding revision and keep my edits',exact:true}).click()
 await page.getByLabel('Primary colour',{exact:true}).fill('#273e74');expect(writes).toEqual(['"1"'])
 await page.getByRole('button',{name:'Save branding',exact:true}).click();await expect(page.getByRole('status').filter({hasText:'Branding saved.'})).toBeVisible();expect(writes).toEqual(['"1"','"4"'])
 await openEventSection(page,'Overview');await expect(history).toContainText('Expected revision 1');await expect(history).toContainText('Original outcome remains unconfirmed')
})

for (const action of ['approve','release'] as const) test(`clean rules ${action} pending and unknown guard local section route and close departure`,async({page})=>{
 const {state,rules}=await rulesFixture(page);rules.state=action==='approve'?'review':'approved'
 let release!:()=>void,started!:()=>void;const held=new Promise<void>(resolve=>release=resolve),sent=new Promise<void>(resolve=>started=resolve)
 await page.route(`**/events/${firstID}/content/${rulesID}/actions`,async route=>{started();await held;return route.fallback()})
 await page.goto('/events/'+firstID);await openEventSection(page,'Rules');await page.getByRole('button',{name:'More records',exact:true}).click();await page.getByRole('button',{name:'Plan Competition rules',exact:true}).click()
 state.unknown=true;await page.getByRole('button',{name:action==='approve'?'Approve version 1':'Release approved version',exact:true}).click();await sent
 await expect(page.getByRole('button',{name:'Close editor',exact:true})).toBeDisabled();await openEventSection(page,'Overview');await expect(page.getByRole('heading',{name:'Edit Competition rules',exact:true})).toBeVisible()
 await page.getByRole('link',{name:'← Switch event',exact:true}).click();await expect(page).toHaveURL(new RegExp('/events/'+firstID+'$'))
 release();await expect(page.getByRole('status').filter({hasText:'Release action outcome is unconfirmed'})).toBeVisible()
 await expect(page.getByRole('button',{name:'Close editor',exact:true})).toBeDisabled();await openEventSection(page,'Overview');await expect(page.getByRole('heading',{name:'Edit Competition rules',exact:true})).toBeVisible()
 state.unknown=false;await page.getByRole('button',{name:'Review latest saved version',exact:true}).click();await expect(page.getByRole('region',{name:'Latest saved content'})).toBeVisible()
 await expect(page.getByRole('button',{name:'Close editor',exact:true})).toBeDisabled();expect(state.writes).toHaveLength(1)
 await page.getByRole('button',{name:'Discard my edits and load latest',exact:true}).click();await page.getByRole('button',{name:'Close editor',exact:true}).click()
 await page.getByRole('button',{name:'Plan Competition rules',exact:true}).click();await expect(page.getByRole('complementary',{name:'Unconfirmed command history'})).toContainText('Content '+action)
 expect(state.writes).toHaveLength(1)
})

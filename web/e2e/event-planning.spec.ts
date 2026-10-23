import { openEventSection } from './event-navigation-fixture'
import { expect, test } from '@playwright/test'
import { firstID, mockWorkspace, person } from './fixtures'
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

test('staff grant conflicts leave lasting restrictions and input explicit for active and waitlisted people',async({page})=>{
 await mockWorkspace(page);const roles:string[]=[]
 await page.route(`**/events/${firstID}/staff`,route=>{if(route.request().method()!=='PUT')return route.fallback();roles.push(route.request().postDataJSON().role);return route.fulfill({status:409,json:{error:{code:'event_blocked',request_id:'synthetic',blockers:[{field:'person_id',code:'competition_conflict',message:'Active or waitlisted competitors must withdraw before this grant.'}]}}})})
 await page.goto('/events/'+firstID);await openEventSection(page, 'Staff');await page.getByLabel('Person account ID').fill(person.id)
 for(const role of ['organiser','early_brief_access']){await page.getByLabel('Role',{exact:true}).selectOption(role);await page.getByLabel(/Acknowledge the lasting competition restriction/).check();await page.getByRole('button',{name:'Assign role',exact:true}).click();await expect(page.getByRole('alert')).toContainText('Active or waitlisted competitors must withdraw');await expect(page.getByLabel('Person account ID')).toHaveValue(person.id)}
 expect(roles).toEqual(['organiser','early_brief_access'])
})

for(const leave of ['event','account'] as const)test(`late rules save response after ${leave} departure cannot refill private inputs or continue another command`,async({page})=>{
 const {state}=await rulesFixture(page);let release!:()=>void,started!:()=>void
 const pending=new Promise<void>(resolve=>{release=resolve}),sending=new Promise<void>(resolve=>{started=resolve})
 await page.route(`**/events/${firstID}/content/${rulesID}`,async route=>{if(route.request().method()!=='PUT')return route.fallback();started();await pending;return route.fallback()})
 await page.goto('/events/'+firstID);await openEventSection(page,'Rules');await page.getByRole('button',{name:'More records',exact:true}).click();await page.getByRole('button',{name:'Plan Competition rules',exact:true}).click()
 await page.getByLabel('Markdown',{exact:true}).fill('Private late response marker');await page.getByRole('button',{name:'Save new draft',exact:true}).click();await sending
 if(leave==='event'){page.once('dialog',dialog=>dialog.accept());await page.getByRole('link',{name:'← Switch event',exact:true}).click();await page.getByRole('link',{name:'Southern Lights Hack',exact:true}).click();await expect(page.getByRole('heading',{name:'Southern Lights Hack',exact:true})).toBeVisible()}
 else {await page.evaluate(()=>window.dispatchEvent(new Event('session-expired')));await expect(page.getByRole('button',{name:'Save new draft',exact:true})).toHaveCount(0)}
 const response=page.waitForResponse(response=>response.request().method()==='PUT'&&new URL(response.url()).pathname.endsWith('/content/'+rulesID));release();await response
 await expect(page.getByLabel('Markdown',{exact:true})).toHaveCount(0);await expect(page.getByText('Private late response marker',{exact:true})).toHaveCount(0)
 expect(state.writes).toHaveLength(1);expect(await page.evaluate(()=>JSON.stringify([localStorage,sessionStorage]))).not.toContain('Private late response marker')
})

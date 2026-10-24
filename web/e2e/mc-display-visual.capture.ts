import { test, expect } from '@playwright/test'
import { mockWorkspace, person, firstID } from './fixtures'
import { openEventSection } from './event-navigation-fixture'
import type { components } from '../src/api/schema.gen'
const output='/private/tmp/hackbox-fleet-mc-display-resumed/docs/verification/fleet-mc-display'
const current={id:firstID,title:'Build something useful',text:'Work with your team.\nAsk a mentor when you need help.',url:'https://example.org/hack',duration_seconds:120,content_id:null}
test('matching actual MC or room source',async({page},info)=>{
 const before=info.project.name.startsWith('before'),surface=info.project.name.includes('-mc-')?'mc':'room',width=info.project.name.endsWith('mobile')?'mobile':'desktop'
 const writes:string[]=[],unexpected:string[]=[]
 if(surface==='mc'){
  const data=await mockWorkspace(page),event=data.events[0];event.can_present=true;event.can_edit=false;event.can_read_staff=false;event.can_manage_owner=false;event.can_transition=false
  await page.route('**/api/v1/session',route=>route.fulfill({json:{person:{...person,platform_admin:false},draft_scope:'22222222-2222-4222-8222-222222222222',csrf_token:'synthetic-csrf-token-32-characters',can_create_events:false}}))
  const source:components['schemas']['PresentationPage']={revision:7,can_control:true,server_now:'2030-06-14T09:00:00Z',timezone:'Europe/London',draft:[current],published:[current],current_index:0,mode:'held',remaining_seconds:120,timer_ends_at:null,override:null,override_mode:'held',override_remaining_seconds:0,override_timer_ends_at:null,pairings:[]}
  await page.route(`**/events/${firstID}/presentation**`,route=>{if(route.request().method()!=='GET'){writes.push(route.request().url());return route.fulfill({status:500})}return route.fulfill({json:source})})
  await page.goto(`/events/${firstID}`);await openEventSection(page,'MC console')
  await page.getByLabel('Reason for room changes').fill('Synthetic reviewed room update')
  if(!before)await page.getByRole('button',{name:'Prepare urgent announcement',exact:true}).click()
  await page.getByLabel('Urgent title').fill('Synthetic room update')
  await page.getByLabel('Urgent room text').fill('Return to your teams. The planned building session resumes when the MC chooses.')
  await page.getByLabel('Urgent countdown in seconds').fill('120')
  if(!before){await page.getByRole('button',{name:'Activate urgent announcement',exact:true}).click();await expect(page.getByRole('heading',{name:'Review urgent announcement',exact:true})).toBeFocused()}
 }else{
  const snapshot:components['schemas']['DisplaySnapshot']={event_id:firstID,event_name:'Synthetic summer hack',timezone:'Europe/London',sequence:7,server_now:'2030-06-14T09:00:00Z',current:{...current,content_title:'Released challenge'},next:{...current,title:'Share your progress',text:'',url:'',content_title:''},mode:'held',remaining_seconds:120,timer_ends_at:null,primary_colour:'#173a35',background_colour:'#f8f9f5',foreground_colour:'#173a35',override_active:false,assets:[]}
  await page.route('**/api/**',route=>{unexpected.push(route.request().url());return route.fulfill({status:500})})
  await page.route('**/display/stream',route=>route.fulfill({contentType:'text/event-stream',headers:{'Cache-Control':'no-store'},body:`id: 7\nevent: snapshot\ndata: ${JSON.stringify(snapshot)}\n\n`}))
  await page.goto(`/display/${firstID}`);await page.getByLabel('Display key',{exact:true}).fill('SYNTHETIC-DISPLAY-KEY-DO-NOT-USE');await page.getByRole('button',{name:'Connect display',exact:true}).click()
  await expect(page.getByRole('heading',{name:'Build something useful',exact:true})).toBeVisible();await expect(page.locator('.room-countdown')).toHaveText('02:00');await expect(page.locator('.room-qr svg')).toBeVisible();await expect(page.getByRole('status')).toContainText('Connection lost')
 }
 await page.evaluate(()=>document.fonts.ready)
 await page.screenshot({path:`${output}/${before?'before':'after'}-${surface}-${width}.png`,fullPage:true})
 expect(writes).toEqual([]);expect(unexpected).toEqual([])
})

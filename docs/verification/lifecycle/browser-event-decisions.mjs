import assert from 'node:assert/strict'
import {join} from 'node:path'
import {wallTime} from './browser-event-creation.mjs'

export async function decideEventInFleet({page,expect,phase,output}){
 assert.ok(['postpone-event','approve-attendance-live'].includes(phase.action))
 const input=phase.event_decision,before=input.before,path=`/api/v1/events/${phase.event_id}`,postpone=phase.action==='postpone-event',method=postpone?'PUT':'POST',target=postpone?path:path+'/transition',writes=[]
 await page.clock.setFixedTime(new Date(phase.domain_now))
 await page.goto(`http://admin.localhost:8080/events/${phase.event_id}${postpone?'?section=setup':''}`)
 const count=request=>{if(new URL(request.url()).pathname===target&&request.method()===method)writes.push(request)}
 page.on('request',count)
 try{
  let confirm
  if(postpone){
   const c=input.configuration,sections=page.getByRole('navigation',{name:'Event setup sections',exact:true})
   await sections.getByRole('button',{name:'Attendance and dates',exact:true}).click()
   for(const [field,label] of [['live_start','Live start'],['live_end','Live end'],['submission_cutoff','Submission cutoff']])await page.getByLabel(label,{exact:true}).fill(wallTime(c[field],c.timezone))
   await sections.getByRole('button',{name:'Exceptional postponement',exact:true}).click()
   await page.getByLabel('Postponement reason',{exact:true}).fill(input.reason)
   await page.getByRole('checkbox',{name:'Approve the exceptional postponement and revised deadlines',exact:true}).check()
   await page.getByRole('button',{name:'Preview dates and blockers',exact:true}).click()
   await expect(page.getByRole('region',{name:'Event preview',exact:true})).toBeVisible()
   assert.equal(writes.length,0,'Postponement preview must not save dates')
   await page.getByRole('button',{name:'Save event',exact:true}).click()
   const review=page.getByRole('region',{name:'Review event configuration',exact:true})
   await expect(review).toContainText(before.name);await expect(review).toContainText(String(before.revision))
   await expect(page.getByLabel('Postponement reason',{exact:true})).toHaveValue(input.reason)
   confirm=review.getByRole('button',{name:'Confirm save event',exact:true})
  }else{
   await expect(page.getByRole('heading',{name:'Organiser decision',exact:true})).toBeVisible()
   await page.getByLabel('Reason for an exception, cancellation or reopening',{exact:true}).fill(input.reason)
   await page.getByRole('checkbox',{name:'Approve proceeding below minimum attendance after post-publication withdrawals',exact:true}).check()
   confirm=page.getByRole('button',{name:'Advance to live',exact:true})
  }
  assert.equal(writes.length,0,'Reviewing an explicit exception must not perform its native write')
  const response=page.waitForResponse(r=>new URL(r.url()).pathname===target&&r.request().method()===method)
  await confirm.click()
  const native=await response;assert.equal(native.status(),200)
  const saved=await native.json();assert.equal(saved.id,before.id);assert.equal(saved.revision,before.revision+1)
  assert.equal(writes.length,1);assert.equal(writes[0].headers()['if-match'],`"${before.revision}"`)
  const sent=writes[0].postDataJSON();assert.equal(sent.reason,input.reason)
  if(postpone){
   assert.equal(sent.approve_postponement,true);assert.equal(saved.stage,before.stage);assert.equal(saved.published_at,before.published_at)
   assert.equal(Date.parse(sent.configuration.live_start),Date.parse(input.configuration.live_start))
   assert.equal(Date.parse(sent.configuration.live_end),Date.parse(input.configuration.live_end))
   assert.equal(Date.parse(sent.configuration.submission_cutoff),Date.parse(input.configuration.submission_cutoff))
   await expect(page.getByRole('status').filter({hasText:`Event saved. Revision ${saved.revision}.`})).toBeVisible()
  }else{
   assert.equal(sent.stage,'live');assert.equal(sent.attendance_exception,true)
   assert.equal(saved.stage,'live');assert.equal(saved.confirmed_attendees,29)
   await expect(page.getByRole('button',{name:'Advance to assessment',exact:true})).toBeVisible()
  }
  await page.reload()
  if(postpone){
   await page.getByRole('navigation',{name:'Event setup sections',exact:true}).getByRole('button',{name:'Attendance and dates',exact:true}).click()
   await expect(page.getByLabel('Live start',{exact:true})).toHaveValue(wallTime(input.configuration.live_start,input.configuration.timezone))
  }else await expect(page.getByRole('button',{name:'Advance to assessment',exact:true})).toBeVisible()
  assert.equal(writes.length,1)
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
  await page.screenshot({path:join(output,`native-event-decision-${phase.sequence}.png`),fullPage:true})
  return {event_id:saved.id,action:phase.action,revision:saved.revision,stage:saved.stage,confirmed_attendees:saved.confirmed_attendees,review_without_write:true,one_confirmed_command:true,reloaded:true}
 }finally{page.off('request',count)}
}

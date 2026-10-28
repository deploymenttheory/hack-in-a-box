// Exercise the actual event form, date resolver, preview/review and native save.
import assert from 'node:assert/strict'
import {join} from 'node:path'

export function wallTime(instant,timezone){
 const parts=Object.fromEntries(new Intl.DateTimeFormat('en-GB',{timeZone:timezone,year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',second:'2-digit',hourCycle:'h23'}).formatToParts(new Date(instant)).map(part=>[part.type,part.value]))
 return `${parts.year}-${parts.month}-${parts.day}T${parts.hour}:${parts.minute}${parts.second==='00'?'':':'+parts.second}`
}

export async function createEventInFleet({page,expect,phase,output}){
 assert.equal(phase.action,'create-event');assert.ok([100,101].includes(phase.sequence))
 assert.equal(phase.event_id,'00000000-0000-0000-0000-000000000000')
 const input=phase.create,c=input.configuration
 await page.clock.setFixedTime(new Date(phase.domain_now))
 await page.goto('http://admin.localhost:8080/')
 await expect(page.getByRole('heading',{name:'Your events',exact:true})).toBeVisible()
 await page.getByRole('link',{name:'Create event',exact:true}).click()
 await expect(page.getByRole('heading',{name:'Create an event',exact:true})).toBeVisible()
 let writes=0;const observe=request=>{if(new URL(request.url()).pathname==='/api/v1/events'&&request.method()==='POST')writes++}
 page.on('request',observe)
 try{
  const navigation=page.getByRole('navigation',{name:'Event setup sections',exact:true})
  await page.getByLabel('Event name',{exact:true}).fill(input.name)
  await page.getByLabel('Event slug',{exact:true}).fill(input.slug)
  await page.getByLabel('Description',{exact:true}).fill(c.description)
  await page.getByLabel('Venue',{exact:true}).fill(c.venue)
  await page.getByLabel('Event timezone',{exact:true}).fill(c.timezone)
  await navigation.getByRole('button',{name:'Attendance and dates',exact:true}).click()
  await page.getByLabel('Event capacity',{exact:true}).fill(String(c.capacity))
  await page.getByLabel('Minimum confirmed competitors',{exact:true}).fill(String(c.minimum_attendance))
  for(const [field,label]of [['admission_cutoff','Final admission'],['team_freeze','Team freeze'],['live_start','Live start'],['submission_cutoff','Submission cutoff'],['live_end','Live end']])await page.getByLabel(label,{exact:true}).fill(wallTime(c[field],c.timezone))
  await page.getByLabel('When a local time occurs twice',{exact:true}).selectOption(c.dst_choice)
  await navigation.getByRole('button',{name:'Business calendar',exact:true}).click()
  for(const [index,day]of ['Sunday','Monday','Tuesday','Wednesday','Thursday','Friday','Saturday'].entries())await page.getByRole('checkbox',{name:day,exact:true}).setChecked(c.calendar.working_days.includes(index))
  await page.getByLabel('Excluded dates, one YYYY-MM-DD date per line',{exact:true}).fill(c.calendar.excluded_dates.join('\n'))
  await navigation.getByRole('button',{name:'Organiser responsibility',exact:true}).click()
  await page.getByRole('checkbox',{name:'I understand that becoming this event’s organiser permanently prevents me from competing in it, even if my role is later removed.',exact:true}).check()
  await page.getByRole('button',{name:'Preview dates and blockers',exact:true}).click()
  await expect(page.getByRole('region',{name:'Event preview',exact:true})).toBeVisible()
  assert.equal(writes,0,'Preview must not create a native event')
  await page.getByRole('button',{name:'Save draft event',exact:true}).click()
  const review=page.getByRole('region',{name:'Review event configuration',exact:true})
  await expect(review).toBeVisible();await expect(review).toContainText(input.name)
  assert.equal(writes,0,'Review must not create a native event')
  const response=page.waitForResponse(r=>new URL(r.url()).pathname==='/api/v1/events'&&r.request().method()==='POST')
  await review.getByRole('button',{name:'Confirm save draft event',exact:true}).click()
  const saved=await response;assert.equal(saved.status(),200);const value=await saved.json()
  assert.match(value.id,/^[a-f0-9-]{36}$/);assert.equal(value.name,input.name);assert.equal(value.slug,input.slug);assert.equal(value.stage,'draft')
  await expect(page).toHaveURL(`http://admin.localhost:8080/events/${value.id}`)
  await expect(page.getByRole('heading',{name:input.name,exact:true})).toBeVisible()
  await page.reload();await expect(page.getByRole('heading',{name:input.name,exact:true})).toBeVisible()
  assert.equal(writes,1,'Create/reload must issue exactly one event POST')
  await page.screenshot({path:join(output,`native-event-create-${phase.sequence}.png`),fullPage:true})
  return {event_id:value.id,preview_without_write:true,review_without_write:true,event_post_attempts:writes,persisted_after_reload:true}
 }catch(error){await page.screenshot({path:join(output,`event-create-${phase.sequence}-failure.png`),fullPage:true}).catch(()=>{});throw error}
 finally{page.off('request',observe)}
}

import assert from 'node:assert/strict'
import {join} from 'node:path'

export async function openRegistrationInFleet({page,expect,phase}){
 assert.equal(phase.action,'open-registration')
 await page.clock.setFixedTime(new Date(phase.domain_now))
 await page.goto(`http://admin.localhost:8080/events/${phase.event_id}`)
 const response=page.waitForResponse(r=>new URL(r.url()).pathname===`/api/v1/events/${phase.event_id}/transition`&&r.request().method()==='POST')
 await page.getByRole('button',{name:'Advance to registration',exact:true}).click()
 const saved=await response;assert.equal(saved.status(),200);assert.equal((await saved.json()).stage,'registration')
 await expect(page.getByRole('button',{name:'Advance to preparation',exact:true})).toBeVisible()
}

export async function applyInFleet({browser,expect,phase,output,errors}){
 assert.equal(phase.action,'apply-registration')
 const input=phase.application,context=await browser.newContext({viewport:{width:1280,height:1000},proxy:{server:'http://127.0.0.1:18088',bypass:'<-loopback>'}})
 const page=await context.newPage();page.on('pageerror',error=>errors.push(error.message))
 try{
  await page.goto('http://public.localhost:8080/auth/login')
  await expect(page.getByRole('heading',{name:'Synthetic identity provider',exact:true})).toBeVisible()
  await page.getByLabel('Synthetic actor',{exact:true}).selectOption(input.actor)
  await page.getByRole('button',{name:'Continue through normal OIDC',exact:true}).click()
  await expect(page.getByRole('heading',{name:'Your events',exact:true})).toBeVisible()
  await page.clock.setFixedTime(new Date(phase.domain_now))
  await page.goto(`http://public.localhost:8080/register/${phase.event_id}`)
  await page.getByLabel('Application type',{exact:true}).selectOption(input.form.definition.id)
  for(const field of input.form.definition.fields){
   assert.equal(field.type,'text')
   await page.getByLabel(field.label+(field.required?' (required)':''),{exact:true}).fill(input.answer)
  }
  if(input.form.definition.audience==='attendee')await page.getByLabel('Team preference',{exact:true}).selectOption('solo')
  let writes=0;const path=`/api/v1/events/${phase.event_id}/registrations`
  page.on('request',request=>{if(new URL(request.url()).pathname===path&&request.method()==='POST')writes++})
  await page.getByRole('button',{name:'Review application',exact:true}).click()
  await expect(page.getByRole('heading',{name:'Review your application',exact:true})).toBeVisible()
  assert.equal(writes,0)
  const response=page.waitForResponse(r=>new URL(r.url()).pathname===path&&r.request().method()==='POST')
  await page.getByRole('button',{name:'Submit application',exact:true}).click()
  const saved=await response;assert.equal(saved.status(),200);const value=await saved.json()
  assert.equal(value.event_id,phase.event_id);assert.equal(value.form_id,input.form.definition.id);assert.equal(value.form_version,input.form.published_version);assert.equal(value.state,input.state)
  const status=page.getByRole('region',{name:`${input.form.definition.audience} application status`,exact:true})
  await expect(status).toBeVisible();await expect(status).toContainText(value.id)
  await page.reload();await expect(status).toContainText(value.id)
  assert.equal(writes,1)
  await page.screenshot({path:join(output,`native-application-${phase.sequence}.png`),fullPage:true})
  return {application_id:value.id,actor:input.actor,audience:value.audience,submission_state:value.state,post_attempts:writes,persisted_after_reload:true}
 }catch(error){await page.screenshot({path:join(output,`application-${phase.sequence}-failure.png`),fullPage:true}).catch(()=>{});throw error}
 finally{await context.close()}
}

export async function selectFacilitatorInFleet({page,expect,phase,output}){
 assert.equal(phase.action,'select-facilitator')
 const input=phase.selection,application=input.application
 assert.equal(application.kind,'facilitator');assert.equal(application.state,'received')
 await page.clock.setFixedTime(new Date(phase.domain_now))
 await page.goto(`http://admin.localhost:8080/events/${phase.event_id}?section=applicants`)
 await expect(page.getByRole('heading',{name:'Applicant review',exact:true})).toBeVisible()
 const review=page.locator(`[data-record-focus="${application.id}"]`)
 await review.click()
 const details=page.getByRole('region',{name:'Applicant details',exact:true})
 await expect(details).toContainText('received')
 await details.getByLabel('Decision or exception reason',{exact:true}).fill(input.reason)
 const path=`/api/v1/events/${phase.event_id}/registrations/${application.id}/decision`
 let writes=0
 const count=request=>{if(new URL(request.url()).pathname===path&&request.method()==='POST')writes++}
 page.on('request',count)
 try{
  await details.getByRole('button',{name:'Select facilitator',exact:true}).click()
  const dialog=page.getByRole('dialog',{name:'Review applicant decision',exact:true})
  await expect(dialog).toContainText(input.reason)
  await expect(dialog).toContainText(String(application.revision))
  assert.equal(writes,0)
  const response=page.waitForResponse(r=>new URL(r.url()).pathname===path&&r.request().method()==='POST')
  await dialog.getByRole('button',{name:'Confirm decision',exact:true}).click()
  const saved=await response;assert.equal(saved.status(),200);const value=await saved.json()
  assert.equal(value.id,application.id);assert.equal(value.event_id,phase.event_id)
  assert.equal(value.state,'selected');assert.equal(value.revision,application.revision+1)
  await expect(details.getByRole('status').filter({hasText:'Decision recorded.'})).toBeVisible()
  await page.reload();await review.click()
  await expect(details).toContainText('selected');await expect(details).toContainText(input.reason)
  assert.equal(writes,1)
  await page.screenshot({path:join(output,`native-selection-${phase.sequence}.png`),fullPage:true})
  return {application_id:application.id,actor:input.actor,audience:application.audience,state:value.state,review_without_write:true,post_attempts:writes,persisted_after_reload:true}
 }finally{page.off('request',count)}
}

export async function competitionGoInFleet({page,expect,phase,output}){
 assert.equal(phase.action,'competition-go')
 await page.clock.setFixedTime(new Date(phase.domain_now))
 await page.goto(`http://admin.localhost:8080/events/${phase.event_id}`)
 await expect(page.getByRole('heading',{name:'Organiser decision',exact:true})).toBeVisible()
 const response=page.waitForResponse(r=>new URL(r.url()).pathname===`/api/v1/events/${phase.event_id}/transition`&&r.request().method()==='POST')
 await page.getByRole('button',{name:'Advance to preparation',exact:true}).click()
 const saved=await response;assert.equal(saved.status(),200);const value=await saved.json()
 assert.equal(value.stage,'preparation');assert.equal(value.confirmed_attendees,35)
 await page.reload();await expect(page.getByRole('button',{name:'Advance to live',exact:true})).toBeVisible()
 await page.screenshot({path:join(output,`native-go-${phase.sequence}.png`),fullPage:true})
 return {event_id:phase.event_id,action:phase.action,stage:value.stage,confirmed_attendees:value.confirmed_attendees,persisted_after_reload:true}
}

export async function respondToAdmissionInFleet({browser,expect,phase,output,errors}){
 assert.equal(phase.action,'respond-registration')
 const input=phase.response,application=input.application
 const labels={withdraw:['Withdraw application','Review application withdrawal','Confirm withdrawal','withdrawn'],accept:['Accept place','Review offer acceptance','Confirm acceptance','confirmed']}[input.action]
 assert.ok(labels)
 const context=await browser.newContext({viewport:{width:1280,height:1000},proxy:{server:'http://127.0.0.1:18088',bypass:'<-loopback>'}})
 const page=await context.newPage();page.on('pageerror',error=>errors.push(error.message))
 try{
  await page.goto('http://public.localhost:8080/auth/login')
  await expect(page.getByRole('heading',{name:'Synthetic identity provider',exact:true})).toBeVisible()
  await page.getByLabel('Synthetic actor',{exact:true}).selectOption(input.actor)
  await page.getByRole('button',{name:'Continue through normal OIDC',exact:true}).click()
  await expect(page.getByRole('heading',{name:'Your events',exact:true})).toBeVisible()
  await page.clock.setFixedTime(new Date(phase.domain_now))
  await page.goto(`http://public.localhost:8080/register/${phase.event_id}`)
  const status=page.getByRole('region',{name:'attendee application status',exact:true})
  await expect(status).toContainText(application.id)
  const path=`/api/v1/events/${phase.event_id}/registrations/${application.id}/respond`
  let writes=0;page.on('request',request=>{if(new URL(request.url()).pathname===path&&request.method()==='POST')writes++})
  await status.getByRole('button',{name:labels[0],exact:true}).click()
  const dialog=page.getByRole('dialog',{name:labels[1],exact:true})
  await expect(dialog).toContainText(application.id)
  await expect(dialog).toContainText(String(application.revision));assert.equal(writes,0)
  const response=page.waitForResponse(r=>new URL(r.url()).pathname===path&&r.request().method()==='POST')
  await dialog.getByRole('button',{name:labels[2],exact:true}).click()
  const saved=await response;assert.equal(saved.status(),200);const value=await saved.json()
  assert.equal(value.id,application.id);assert.equal(value.event_id,phase.event_id);assert.equal(value.state,labels[3]);assert.equal(value.revision,application.revision+1)
  await expect(dialog).not.toBeVisible()
  await page.reload();await expect(status).toContainText(application.id)
  await expect(status).toContainText(input.action==='withdraw'?'This application has been withdrawn.':'Your competitor place is confirmed.')
  assert.equal(writes,1)
  await page.screenshot({path:join(output,`native-response-${phase.sequence}.png`),fullPage:true})
  return {application_id:application.id,actor:input.actor,action:input.action,state:value.state,review_without_write:true,post_attempts:writes,persisted_after_reload:true}
 }catch(error){await page.screenshot({path:join(output,`response-${phase.sequence}-failure.png`),fullPage:true}).catch(()=>{});throw error}
 finally{await context.close()}
}

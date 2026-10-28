import assert from 'node:assert/strict'
import {join} from 'node:path'

export async function askClarificationInFleet({browser,expect,phase,output,errors}){
 const input=phase.ask
 assert.equal(phase.action,'ask-common-question')
 const context=await browser.newContext({viewport:{width:1280,height:1000},proxy:{server:'http://127.0.0.1:18088',bypass:'<-loopback>'}})
 const page=await context.newPage();page.on('pageerror',error=>errors.push(error.message))
 try{
  await page.goto('http://public.localhost:8080/auth/login')
  await expect(page.getByRole('heading',{name:'Synthetic identity provider',exact:true})).toBeVisible()
  await page.getByLabel('Synthetic actor',{exact:true}).selectOption(input.actor)
  await page.getByRole('button',{name:'Continue through normal OIDC',exact:true}).click()
  await expect(page.getByRole('heading',{name:'Your events',exact:true})).toBeVisible()
  await page.clock.setFixedTime(new Date(phase.domain_now))
  await page.goto(`http://public.localhost:8080/events/${phase.event_id}/clarifications`)
  await page.getByRole('button',{name:"Your team's private questions",exact:true}).click()
  await page.getByRole('combobox',{name:'About',exact:true}).selectOption(input.topic)
  await page.getByLabel('Your question',{exact:true}).fill(input.body)
  const path=`/api/v1/events/${phase.event_id}/questions`,writes=[]
  page.on('request',request=>{if(new URL(request.url()).pathname===path&&request.method()==='POST')writes.push(request)})
  await page.getByRole('button',{name:'Review private question',exact:true}).click()
  const review=page.getByRole('region',{name:'Review private team question',exact:true})
  await expect(review).toContainText(input.body);await expect(review).toContainText(input.team_id)
  assert.equal(writes.length,0,'Reviewing a private question must not send it')
  const response=page.waitForResponse(r=>new URL(r.url()).pathname===path&&r.request().method()==='POST')
  await review.getByRole('button',{name:'Send private question',exact:true}).click()
  const native=await response;assert.equal(native.status(),200)
  const saved=await native.json(),sent=writes[0].postDataJSON()
  assert.equal(writes.length,1);assert.match(sent.command_id,/^[a-f0-9-]{36}$/)
  assert.equal(sent.team_id,input.team_id);assert.equal(sent.topic,input.topic);assert.equal(sent.body,input.body);assert.deepEqual(sent.attachments,[])
  assert.equal(saved.team_id,input.team_id);assert.equal(saved.body,input.body);assert.equal(saved.revision,1)
  await expect(page.getByRole('status').filter({hasText:'Saved as a private team question.'})).toBeVisible()
  await page.reload();await page.getByRole('button',{name:"Your team's private questions",exact:true}).click()
  await page.locator(`a[href="/events/${phase.event_id}/questions/${saved.id}"]`).click()
  await expect(page.getByText(input.body,{exact:true})).toBeVisible()
  assert.equal(writes.length,1)
  return {event_id:phase.event_id,question_id:saved.id,command_id:sent.command_id,action:phase.action,review_without_write:true,one_confirmed_command:true,reloaded:true}
 }finally{await context.close()}
}

export async function answerClarificationInFleet({page,expect,phase,output}){
 const input=phase.answer,question=input.question,before=input.before,document=input.document,action=phase.action.slice('answer-'.length)
 assert.ok(['save','approve','publish'].includes(action))
 await page.clock.setFixedTime(new Date(phase.domain_now))
 await page.goto(`http://admin.localhost:8080/events/${phase.event_id}/questions/${question.id}`)
 await expect(page.getByRole('heading',{name:'Staff official-answer review',exact:true})).toBeVisible()
 await expect(page.getByText(question.body,{exact:true})).toBeVisible()
 const task=page.getByRole('region',{name:'Official answer task',exact:true})
 if(action==='save'){
  await task.getByRole('combobox',{name:'Type of answer',exact:true}).selectOption(document.kind)
  await task.getByLabel('Common answer title',{exact:true}).fill(document.title)
  await task.getByLabel('Common official answer text',{exact:true}).fill(document.body)
 }
 const path=action==='save'?`/api/v1/events/${phase.event_id}/questions/${question.id}/answer`:`/api/v1/events/${phase.event_id}/clarifications/${before.id}/actions`,method=action==='save'?'PUT':'POST',writes=[]
 const count=request=>{if(new URL(request.url()).pathname===path&&request.method()===method)writes.push(request)}
 page.on('request',count)
 try{
  const choose={save:'Review draft version',approve:'Review saved version approval',publish:'Review approved version publication'}[action]
  await task.getByRole('button',{name:choose,exact:true}).click()
  const review=task.getByRole('region',{name:`Review common answer ${action==='save'?'draft':action==='approve'?'approval':'publication'}`,exact:true})
  await expect(review).toContainText(document.title);await expect(review).toContainText(document.body)
  await expect(review).toContainText(`Version ${input.version} · Expected answer revision ${before.revision}`)
  await expect(review).toContainText(`Challenge ${question.source.challenge_version} · Rubric ${question.source.rubric.version}`)
  await expect(review).not.toContainText(question.body)
  assert.equal(writes.length,0,'Reviewing a common answer must not save, approve or publish it')
  const response=page.waitForResponse(r=>new URL(r.url()).pathname===path&&r.request().method()===method)
  const confirm={save:'Save a new draft version',approve:'Approve saved version',publish:'Publish approved version'}[action]
  await review.getByRole('button',{name:confirm,exact:true}).click()
  const native=await response;assert.equal(native.status(),200)
  const saved=await native.json(),sent=writes[0].postDataJSON()
  assert.equal(writes.length,1);assert.equal(writes[0].headers()['if-match'],`"${before.revision}"`)
  assert.equal(saved.question_id,question.id);assert.equal(saved.version,input.version);assert.equal(saved.revision,before.revision+1)
  assert.deepEqual(saved.document,document);assert.deepEqual(saved.source,question.source)
  if(action==='save'){assert.deepEqual(sent,document);assert.equal(saved.approved,false);assert.equal(saved.published,false)}
  else{assert.equal(sent.action,action);assert.equal(sent.version,input.version);assert.equal(saved.approved,true);assert.equal(saved.published,action==='publish')}
  const notice=action==='save'?`Saved draft version ${input.version}.`:action==='approve'?`Version ${input.version} approved.`:`Official version ${input.version} published.`
  await expect(task.getByRole('status').filter({hasText:notice})).toBeVisible()
  await expect(review).toHaveCount(0)
  await page.reload();await expect(page.getByRole('region',{name:'Official answer task',exact:true}).getByRole('status').filter({hasText:`Server draft version ${input.version}`})).toBeVisible()
  await expect(page.getByLabel('Common official answer text',{exact:true})).toHaveValue(document.body)
  assert.equal(writes.length,1)
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
  // Publish only a synthetic common audience preview, never the private source.
  if(action==='publish')await page.getByRole('region',{name:'Actual common audience preview',exact:true}).screenshot({path:join(output,`native-common-answer-${phase.sequence}.png`)})
  return {event_id:phase.event_id,question_id:question.id,answer_id:saved.id,action:phase.action,version:saved.version,revision:saved.revision,review_without_write:true,one_confirmed_command:true,reloaded:true}
 }finally{page.off('request',count)}
}

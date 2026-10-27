// Fail a genuine accepted browser response; never fabricate an application
// response or replay a write. Native Go checks the exact resulting resource.
import assert from 'node:assert/strict'
import {join} from 'node:path'
import {setTimeout as pause} from 'node:timers/promises'

export async function recoverLiveHelp({browser,expect,phase,output,errors}){
 const contexts=[]
 async function actor(alias){
  const context=await browser.newContext({viewport:{width:1280,height:1000},proxy:{server:'http://127.0.0.1:18088',bypass:'<-loopback>'}})
  contexts.push(context)
  const page=await context.newPage();page.on('pageerror',error=>errors.push(error.message))
  await page.goto('http://public:8080/auth/login')
  await expect(page.getByRole('heading',{name:'Synthetic identity provider',exact:true})).toBeVisible()
  await page.getByLabel('Synthetic actor',{exact:true}).selectOption(alias)
  await page.getByRole('button',{name:'Continue through normal OIDC',exact:true}).click()
  await expect(page.getByRole('heading',{name:'Your events',exact:true})).toBeVisible()
  await page.clock.setFixedTime(new Date(phase.domain_now))
  return {context,page}
 }
 try{
  assert.equal(phase.sequence,3);assert.equal(phase.action,'recover-live-help')
  assert.match(phase.actor,/^attendee-[0-9]{2}$/)
  const path=`/api/v1/events/${phase.event_id}/help/`
  const own=await actor(phase.actor),writes=[]
  own.page.on('request',request=>{if(request.method()==='PUT'&&new URL(request.url()).pathname.startsWith(path))writes.push(request.url())})
  await own.page.goto(`http://public:8080/events/${phase.event_id}/help`)
  const form=own.page.getByRole('region',{name:'Request help',exact:true})
  await form.getByLabel('Support category',{exact:true}).selectOption('competition_output')
  await form.getByLabel('What do you need help with?',{exact:true}).fill('PRIVATE synthetic browser recovery help')
  await form.getByLabel('Location (or an approved meeting route required)',{exact:true}).fill('Synthetic recovery desk')
  await form.getByRole('button',{name:'Review help request',exact:true}).click()
  const cdp=await own.context.newCDPSession(own.page)
  let resolveDrop,rejectDrop,requestID
  const dropped=new Promise((resolve,reject)=>{resolveDrop=resolve;rejectDrop=reject})
  cdp.on('Fetch.requestPaused',event=>{
   void(async()=>{
    if(event.request.method==='PUT'&&new URL(event.request.url).pathname.startsWith(path)&&event.responseStatusCode===200&&!requestID){
     requestID=new URL(event.request.url).pathname.slice(path.length)
     assert.match(requestID,/^[a-f0-9-]{36}$/)
     await cdp.send('Fetch.failRequest',{requestId:event.requestId,errorReason:'InternetDisconnected'})
     await own.context.setOffline(true)
     resolveDrop()
    }else await cdp.send('Fetch.continueRequest',{requestId:event.requestId})
   })().catch(rejectDrop)
  })
  await cdp.send('Fetch.enable',{patterns:[{urlPattern:`http://public:8080${path}*`,requestStage:'Response'}]})
  await own.page.getByRole('alertdialog').getByRole('button',{name:'Submit help request',exact:true}).click()
  await Promise.race([dropped,pause(15000).then(()=>{throw new Error('No actual accepted help response was intercepted')})])
  assert.equal(await own.page.evaluate(()=>navigator.onLine),false)
  await expect(form.getByRole('status',{name:'Help request save state',exact:true})).toContainText('unconfirmed')
  await own.page.screenshot({path:join(output,'live-help-offline-unconfirmed.png'),fullPage:true})
  assert.equal(writes.length,1)
  await own.context.setOffline(false)
  await cdp.send('Fetch.disable')
  const recovered=own.page.waitForResponse(response=>new URL(response.url()).pathname===path+requestID&&response.request().method()==='GET')
  await form.getByRole('button',{name:'Check whether request was created',exact:true}).click()
  assert.equal((await recovered).status(),200)
  await expect(form.getByRole('status',{name:'Help request save state',exact:true})).toContainText('saved on the server')
  const saved=form.getByRole('link',{name:'Open saved help request',exact:true})
  await expect(saved).toHaveAttribute('href',`/events/${phase.event_id}/help/${requestID}`)
  assert.equal(writes.length,1,'Recovery must not replay the original PUT')
  await saved.click()
  const mentor=await actor('mentor-a')
  await mentor.page.goto(`http://public:8080/events/${phase.event_id}/help/${requestID}`)
  for(const [label,status] of [['Claim request','claimed'],['Resolve request','resolved']]){
   await mentor.page.getByRole('region',{name:'Current request actions',exact:true}).getByRole('button',{name:label,exact:true}).click()
   const accepted=mentor.page.waitForResponse(response=>new URL(response.url()).pathname===path+requestID+'/actions'&&response.request().method()==='POST')
   await mentor.page.getByRole('alertdialog').getByRole('button',{name:'Confirm reviewed change',exact:true}).click()
   const response=await accepted;assert.equal(response.status(),200)
   assert.equal((await response.json()).request.status,status)
  }
  await mentor.page.screenshot({path:join(output,'live-help-mentor-resolved.png'),fullPage:true})
  assert.equal(writes.length,1)
  return {request_id:requestID,accepted_response_dropped:true,offline_observed:true,put_attempts:writes.length}
 }finally{for(const context of contexts)await context.close()}
}

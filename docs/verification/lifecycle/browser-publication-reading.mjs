import assert from 'node:assert/strict'
import {join} from 'node:path'

export async function readPublicationInFleet({browser,expect,phase,output,errors}){
 assert.equal(phase.action,'acknowledge-common-publication')
 const input=phase.reading,resource=input.resource,published=input.public_rubric
 const context=await browser.newContext({viewport:{width:1280,height:1000},proxy:{server:'http://127.0.0.1:18088',bypass:'<-loopback>'}})
 const page=await context.newPage();page.on('pageerror',error=>errors.push(error.message))
 try{
  await page.goto('http://public.localhost:8080/auth/login')
  await expect(page.getByRole('heading',{name:'Synthetic identity provider',exact:true})).toBeVisible()
  await page.getByLabel('Synthetic actor',{exact:true}).selectOption(input.actor)
  await page.getByRole('button',{name:'Continue through normal OIDC',exact:true}).click()
  await expect(page.getByRole('heading',{name:'Your events',exact:true})).toBeVisible()
  await page.clock.setFixedTime(new Date(phase.domain_now))
  await page.goto(`http://public.localhost:8080/events/${phase.event_id}/resources/${resource.id}`)
  await expect(page.getByRole('heading',{name:resource.document.title,exact:true})).toBeVisible()
  await expect(page.getByRole('article',{name:'Released resource text',exact:true})).toContainText(resource.document.markdown)
  const path=`/api/v1/events/${phase.event_id}/content/${resource.id}/acknowledgements`,writes=[]
  page.on('request',request=>{if(new URL(request.url()).pathname===path&&request.method()==='POST')writes.push(request)})
  const opener=page.getByRole('button',{name:`I have read version ${resource.version}`,exact:true})
  await opener.focus();await opener.press('Enter')
  const dialog=page.getByRole('alertdialog',{name:'Review reading acknowledgement',exact:true})
  await expect(dialog).toContainText(`${resource.document.title} · Released version ${resource.version}`)
  assert.equal(writes.length,0)
  await dialog.getByRole('button',{name:'Cancel',exact:true}).click();await expect(opener).toBeFocused()
  assert.equal(writes.length,0,'Cancelling acknowledgement must not record reading')
  await opener.press('Enter')
  const response=page.waitForResponse(r=>new URL(r.url()).pathname===path&&r.request().method()==='POST')
  await dialog.getByRole('button',{name:'Confirm acknowledgement',exact:true}).click()
  const native=await response;assert.equal(native.status(),200)
  const receipt=await native.json();assert.ok(receipt.acknowledged_at);assert.equal(receipt.id,resource.id);assert.equal(receipt.version,resource.version)
  assert.equal(writes.length,1);assert.equal(writes[0].headers()['if-match'],`"${resource.version}"`)
  await expect(dialog).toHaveCount(0)
  const acknowledged=page.getByRole('region',{name:'Required acknowledgement',exact:true}).getByRole('status')
  await expect(acknowledged).toContainText(`You acknowledged version ${resource.version}`)
  await page.reload();await expect(acknowledged).toContainText(`You acknowledged version ${resource.version}`)
  await expect(page.getByRole('button',{name:`I have read version ${resource.version}`,exact:true})).toHaveCount(0)
  await page.goto(`http://public.localhost:8080/events/${phase.event_id}/clarifications`)
  await page.getByText('Read the current published rubric',{exact:true}).click()
  await expect(page.getByRole('heading',{name:`${published.definition.name} · version ${published.reference.version}`,exact:true})).toBeVisible()
  const criteria=page.locator('#published-rubric')
  for(const category of published.definition.categories){
   await expect(criteria.getByRole('heading',{name:category.label,exact:true})).toBeVisible()
   await expect(criteria).toContainText(`Range ${category.min_score}–${category.max_score} · Weight ${category.weight}`)
   await expect(criteria).toContainText(category.guidance)
   for(const anchor of category.anchors)await expect(criteria).toContainText(`${anchor.score}: ${anchor.description}`)
  }
  for(const prize of published.definition.prizes)await expect(criteria).toContainText(prize.name)
  await expect(page.locator('body')).not.toContainText('PRIVATE synthetic judge guidance must never reach attendee responses')
  await expect(page.locator('body')).not.toContainText(input.private_objective_canary)
  assert.equal(writes.length,1)
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
  await page.screenshot({path:join(output,`native-published-reading-${phase.sequence}.png`),fullPage:true})
  return {event_id:phase.event_id,content_id:resource.id,actor:input.actor,version:resource.version,rubric_version:published.reference.version,cancel_without_write:true,one_confirmed_command:true,reloaded:true,public_criteria_visible:true,private_guidance_and_objective_absent:true}
 }finally{await context.close()}
}

import assert from 'node:assert/strict'
import {join} from 'node:path'

export async function rehearseJudgeInFleet({browser,expect,phase,output,errors}){
 assert.equal(phase.action,'submit-fictional-practice')
 const input=phase.practice,before=input.before
 const context=await browser.newContext({viewport:{width:1280,height:1000},proxy:{server:'http://127.0.0.1:18088',bypass:'<-loopback>'}})
 const page=await context.newPage();page.on('pageerror',error=>errors.push(error.message))
 try{
  await page.goto('http://public.localhost:8080/auth/login')
  await expect(page.getByRole('heading',{name:'Synthetic identity provider',exact:true})).toBeVisible()
  await page.getByLabel('Synthetic actor',{exact:true}).selectOption(input.actor)
  await page.getByRole('button',{name:'Continue through normal OIDC',exact:true}).click()
  await expect(page.getByRole('heading',{name:'Your events',exact:true})).toBeVisible()
  await page.clock.setFixedTime(new Date(phase.domain_now))
  await page.goto(`http://public.localhost:8080/events/${phase.event_id}/judging/practice`)
  await expect(page.getByRole('heading',{name:'Synthetic judge practice',exact:true})).toBeVisible()
  const editor=page.getByRole('region',{name:'Synthetic practice scoring',exact:true})
  await expect(editor.getByRole('heading',{name:before.name,exact:true})).toBeVisible()
  await expect(page.locator('body')).not.toContainText('PRIVATE synthetic judge guidance')
  for(const category of before.criteria.categories){
   const score=input.scores.find(value=>value.category_id===category.id)
   assert.ok(score);assert.equal(score.evidence.length,1);assert.equal(score.evidence[0].deliverable_id,null)
   await editor.getByRole('spinbutton',{name:`Score for ${category.label}`,exact:true}).fill(String(score.value))
   await editor.getByRole('textbox',{name:`Your evidence for ${category.label}`,exact:true}).fill(score.evidence[0].note)
  }
  const path=`/api/v1/events/${phase.event_id}/judge-practice`,writes=[]
  page.on('request',request=>{if(new URL(request.url()).pathname===path&&request.method()==='POST')writes.push(request)})
  const opener=editor.getByRole('button',{name:'Review practice submission',exact:true})
  await opener.focus();await opener.press('Enter')
  const dialog=page.getByRole('alertdialog',{name:'Review locked submission',exact:true})
  await expect(dialog).toContainText('Fictional synthetic entry; never counted towards competition. Saved card revision 0.')
  for(const score of input.scores)await expect(dialog).toContainText(score.evidence[0].note)
  assert.equal(writes.length,0)
  await dialog.getByRole('button',{name:'Cancel',exact:true}).click();await expect(opener).toBeFocused()
  assert.equal(writes.length,0)
  await opener.press('Enter')
  const response=page.waitForResponse(r=>new URL(r.url()).pathname===path&&r.request().method()==='POST')
  await dialog.getByRole('button',{name:'Confirm reviewed scores',exact:true}).click()
  const native=await response;assert.equal(native.status(),200)
  assert.equal(writes.length,1);assert.equal(writes[0].headers()['if-match'],'"0"')
  const sent=writes[0].postDataJSON();assert.equal(sent.action,'submit');assert.deepEqual(sent.scores,input.scores)
  assert.deepEqual(Object.keys(sent).sort(),['action','request_id','scores'])
  assert.match(sent.request_id,/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/)
  const receipt=await native.json()
  assert.equal(receipt.request_id,sent.request_id);assert.equal(receipt.team_id,before.fictional_entry_id)
  assert.equal(receipt.action,'practice_submit');assert.equal(receipt.revision,1);assert.equal(receipt.state,'submitted')
  assert.equal(receipt.run_id,'00000000-0000-0000-0000-000000000000')
  await expect(dialog).toHaveCount(0)
  await expect(editor.getByRole('status').filter({hasText:'Practice original acknowledgement: submitted'})).toBeVisible()
  await page.reload();await expect(editor).toContainText('Synthetic practice submitted')
  for(const category of before.criteria.categories){
   const score=input.scores.find(value=>value.category_id===category.id)
   await expect(editor.getByRole('spinbutton',{name:`Score for ${category.label}`,exact:true})).toHaveValue(String(score.value))
   await expect(editor.getByRole('spinbutton',{name:`Score for ${category.label}`,exact:true})).toBeDisabled()
   await expect(editor.getByRole('textbox',{name:`Your evidence for ${category.label}`,exact:true})).toHaveValue(score.evidence[0].note)
  }
  assert.equal(writes.length,1)
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
  // Fictional training scores only; never screenshot real private assessments.
  await editor.screenshot({path:join(output,`native-fictional-practice-${phase.sequence}.png`)})
  return {event_id:phase.event_id,actor:input.actor,fictional_entry_id:before.fictional_entry_id,command_id:sent.request_id,version:1,cancel_without_write:true,one_confirmed_command:true,reloaded:true,competition_namespace_absent:true}
 }finally{await context.close()}
}

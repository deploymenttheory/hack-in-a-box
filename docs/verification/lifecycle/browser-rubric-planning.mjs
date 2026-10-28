import assert from 'node:assert/strict'
import {join} from 'node:path'

export async function configureRubricInFleet({page,expect,phase,output}){
 assert.ok(['save-rubric','approve-rubric'].includes(phase.action))
 const input=phase.rubric,definition=input.definition,before=input.configuration,path=`/api/v1/events/${phase.event_id}/rubric/configuration`
 await page.clock.setFixedTime(new Date(phase.domain_now))
 await page.goto(`http://admin.localhost:8080/events/${phase.event_id}?section=judging%20criteria`)
 await expect(page.getByRole('heading',{name:'Judging criteria',exact:true})).toBeVisible()
 const writes=[],count=request=>{if(request.method()==='POST'&&new URL(request.url()).pathname===path)writes.push(request)}
 page.on('request',count)
 try{
  if(phase.action==='save-rubric'){
   await page.getByLabel('Rubric name',{exact:true}).fill(definition.name)
   await page.getByLabel('Judging mode',{exact:true}).selectOption(definition.mode)
   await page.getByRole('spinbutton',{name:'Required judges',exact:true}).fill(String(definition.required_judge_count))
   if(definition.mode==='mentor_qualification'){
    await page.getByRole('spinbutton',{name:'Finalist count',exact:true}).fill(String(definition.finalist_count))
    await page.getByRole('spinbutton',{name:'Reserve count',exact:true}).fill(String(definition.reserve_count))
   }
   const sections=page.getByRole('navigation',{name:'Rubric definition sections',exact:true})
   await sections.getByRole('button',{name:'Awards',exact:true}).click()
   for(const [index,prize] of definition.prizes.entries()){
    await page.getByRole('button',{name:'Add declared award',exact:true}).click()
    await page.getByLabel(`Award ${index+1} name`,{exact:true}).fill(prize.name)
    await page.getByLabel(`Award ${index+1} description`,{exact:true}).fill(prize.description)
    await page.getByLabel(`Award ${index+1} category`,{exact:true}).selectOption(prize.category_id??'')
   }
   await sections.getByRole('button',{name:'Private guidance',exact:true}).click()
   await page.getByLabel('Private judge guidance',{exact:true}).fill(definition.private_judge_guidance)
  }
  await page.getByLabel('Rubric decision reason',{exact:true}).fill(input.reason)
  await page.getByRole('button',{name:phase.action==='save-rubric'?'Review draft save':'Review version approval',exact:true}).click()
  const dialog=page.getByRole('alertdialog',{name:phase.action==='save-rubric'?'Review private rubric draft':'Approve exact rubric version',exact:true})
  await expect(dialog).toBeVisible();await expect(dialog).toContainText(`Starting revision ${before.revision} · Version ${before.reference.version}`)
  await expect(dialog).toContainText(input.reason);await expect(dialog).toContainText(definition.private_judge_guidance)
  for(const category of definition.categories)await expect(dialog.getByRole('region',{name:'Captured rubric decision',exact:true})).toContainText(category.label)
  assert.equal(writes.length,0,'Opening and reviewing criteria must not save or approve them')
  const response=page.waitForResponse(r=>r.request().method()==='POST'&&new URL(r.url()).pathname===path)
  await dialog.getByRole('button',{name:phase.action==='save-rubric'?'Save reviewed draft':'Approve reviewed version',exact:true}).click()
  const saved=await response;assert.equal(saved.status(),200)
  const value=await saved.json();assert.equal(value.revision,before.revision+1)
  assert.equal(writes.length,1);assert.equal(writes[0].headers()['if-match'],`"${before.revision}"`)
  const sent=writes[0].postDataJSON();assert.equal(sent.action,phase.action==='save-rubric'?'save':'approve');assert.equal(sent.reason,input.reason)
  if(phase.action==='approve-rubric'){assert.equal(sent.version,before.reference.version);assert.deepEqual(value.reference,before.reference);assert.equal(value.approved,true)}
  else{assert.equal(value.reference.version,before.reference.version+1);assert.equal(value.approved,false)}
  assert.equal(value.published_version,null);assert.deepEqual(value.runs,[])
  await expect(dialog).toHaveCount(0)
  if(phase.action==='save-rubric')await expect(page.getByLabel('Rubric decision reason',{exact:true})).toBeFocused()
  else await expect(page.getByText(`Saved version ${value.reference.version} · Revision ${value.revision} · Approved`,{exact:true})).toBeFocused()
  await page.reload();await expect(page.getByLabel('Rubric name',{exact:true})).toHaveValue(definition.name)
  await expect(page.getByRole('spinbutton',{name:'Required judges',exact:true})).toHaveValue(String(definition.required_judge_count))
  if(phase.action==='approve-rubric')await expect(page.getByRole('button',{name:'Review version approval',exact:true})).toBeDisabled()
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
  if(phase.action==='approve-rubric')await page.screenshot({path:join(output,`native-rubric-approved-${phase.sequence}.png`),fullPage:true})
  return {event_id:phase.event_id,action:phase.action,mode:definition.mode,version:value.reference.version,revision:value.revision,review_without_write:true,one_confirmed_command:true,reloaded:true}
 }finally{page.off('request',count)}
}

import assert from 'node:assert/strict'
import {join} from 'node:path'

export async function publishRegistrationFormInFleet({page,expect,phase,output}){
 assert.equal(phase.action,'publish-registration-form')
 const input=phase.form,prefix=`/api/v1/events/${phase.event_id}/registration/forms/`
 await page.clock.setFixedTime(new Date(phase.domain_now))
 await page.goto(`http://admin.localhost:8080/events/${phase.event_id}?section=forms`)
 await expect(page.getByRole('heading',{name:'Registration forms',exact:true})).toBeVisible()
 await page.getByRole('button',{name:'New registration form',exact:true}).click()
 await page.getByLabel('Form name',{exact:true}).fill(input.name)
 await page.getByLabel('Form audience',{exact:true}).selectOption(input.audience)
 await page.getByRole('button',{name:'Add question',exact:true}).click()
 await page.getByLabel('Question',{exact:true}).fill(input.label)
 await page.getByLabel('Answer type',{exact:true}).selectOption('text')
 await page.getByLabel('Required',{exact:true}).check()
 await page.getByLabel(input.label+' (required)',{exact:true}).fill('Synthetic preview answer')
 let saves=0,publications=0
 const count=request=>{
  const path=new URL(request.url()).pathname
  if(!path.startsWith(prefix))return
  if(request.method()==='PUT')saves++
  if(request.method()==='POST'&&path.endsWith('/publish'))publications++
 }
 page.on('request',count)
 try{
  await page.getByRole('button',{name:'Validate preview answers',exact:true}).click()
  await expect(page.getByRole('status').filter({hasText:'Preview answers are valid. Nothing was submitted.'})).toBeVisible()
  assert.equal(saves,0);assert.equal(publications,0)
  const savedResponse=page.waitForResponse(r=>new URL(r.url()).pathname.startsWith(prefix)&&r.request().method()==='PUT')
  await page.getByRole('button',{name:'Save form draft',exact:true}).click()
  const saved=await savedResponse;assert.equal(saved.status(),200);const draft=await saved.json()
  assert.equal(draft.definition.name,input.name);assert.equal(draft.definition.audience,input.audience)
  assert.equal(draft.version,1);assert.equal(draft.revision,1);assert.equal(draft.published_version,null)
  const publish=page.getByRole('button',{name:'Publish form version 1',exact:true})
  await expect(publish).toBeDisabled();assert.equal(saves,1);assert.equal(publications,0)
  await page.getByLabel('I acknowledge the effect on existing and new responses.',{exact:true}).check()
  const publication=page.waitForResponse(r=>new URL(r.url()).pathname===prefix+draft.definition.id+'/publish'&&r.request().method()==='POST')
  await publish.click()
  const published=await publication;assert.equal(published.status(),200);const value=await published.json()
  assert.deepEqual(value.definition,draft.definition);assert.equal(value.published_version,draft.version);assert.equal(value.revision,2)
  await expect(page.getByRole('status').filter({hasText:'Version 1 published for new responses.'})).toBeVisible()
  await page.reload()
  await page.getByRole('button',{name:input.name+' · Published 1',exact:true}).click()
  await expect(page.getByLabel('Form name',{exact:true})).toHaveValue(input.name)
  await expect(page.getByLabel('Form audience',{exact:true})).toHaveValue(input.audience)
  await page.getByRole('button',{name:'Edit '+input.label,exact:true}).click()
  await expect(page.getByLabel('Required',{exact:true})).toBeChecked()
  assert.equal(saves,1);assert.equal(publications,1)
  await page.screenshot({path:join(output,`native-form-${phase.sequence}.png`),fullPage:true})
  return {form_id:value.definition.id,audience:input.audience,version:value.version,published_version:value.published_version,preview_without_write:true,save_attempts:saves,publish_attempts:publications,persisted_after_reload:true}
 }catch(error){await page.screenshot({path:join(output,`form-${phase.sequence}-failure.png`),fullPage:true}).catch(()=>{});throw error}
 finally{page.off('request',count)}
}

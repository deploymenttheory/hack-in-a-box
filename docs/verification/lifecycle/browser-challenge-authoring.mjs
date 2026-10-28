import assert from 'node:assert/strict'
import {join} from 'node:path'

// Own the normal production controls; IPC carries only synthetic desired input.
// No route fulfilments, cookie injection or controller-side application writes.
export async function authorChallengeInFleet({page,expect,phase,output}){
 const step=phase.action,input=phase.challenge.input,before=phase.challenge.saved
 assert.ok(['save-challenge','review-challenge','approve-challenge'].includes(step))
 await page.clock.setFixedTime(new Date(phase.domain_now))
 await page.goto(`http://admin.localhost:8080/events/${phase.event_id}?section=content`)
 await expect(page.getByRole('heading',{name:'Content',exact:true})).toBeVisible()
 const base=`/api/v1/events/${phase.event_id}/content/`,writes=[]
 const count=request=>{if(new URL(request.url()).pathname.startsWith(base)&&!['GET','HEAD'].includes(request.method()))writes.push(request)}
 page.on('request',count)
 try{
  if(step==='save-challenge'){
   await page.getByLabel('Starting template',{exact:true}).selectOption('challenge')
   await page.getByRole('button',{name:'Start from template',exact:true}).click()
   await page.getByLabel('Page title',{exact:true}).fill(input.document.title)
   await page.getByLabel('Markdown',{exact:true}).fill(input.document.markdown)
   await page.getByLabel('Require readers to acknowledge this version',{exact:true}).check()
   await page.getByRole('navigation',{name:'Content editor sections',exact:true}).getByRole('button',{name:'Judging criteria',exact:true}).click()
   const binding=page.getByRole('region',{name:'Challenge judging criteria',exact:true})
   await expect(binding).toContainText('No rubric selected.')
   await binding.getByRole('button',{name:`Use approved version ${input.document.rubric.version}`,exact:true}).click()
   await expect(binding).toContainText(`Selected version ${input.document.rubric.version} is currently approved.`)
  }else{
   await page.getByRole('button',{name:`Edit ${input.document.title}`,exact:true}).click()
   await expect(page.getByLabel('Page title',{exact:true})).toHaveValue(input.document.title)
   await expect(page.getByLabel('Markdown',{exact:true})).toHaveValue(input.document.markdown)
  }
  await expect(page.getByRole('region',{name:'Audience preview',exact:true})).toContainText(input.document.markdown)
  await expect(page.locator('body')).not.toContainText('PRIVATE synthetic judge guidance must never reach attendee responses')
  assert.equal(writes.length,0,'Viewing, editing and selecting an approved rubric must not write content')
  const label=step==='save-challenge'?'Save new draft':step==='review-challenge'?'Mark ready for review':`Approve version ${before.version}`
  const response=page.waitForResponse(r=>new URL(r.url()).pathname.startsWith(base)&&r.request().method()===(step==='save-challenge'?'PUT':'POST'))
  await page.getByRole('button',{name:label,exact:true}).click()
  const native=await response;assert.equal(native.status(),200)
  const saved=await native.json()
  assert.equal(writes.length,1);assert.equal(writes[0].headers()['if-match'],`"${before?.revision??0}"`)
  assert.deepEqual(saved.document.rubric,input.document.rubric)
  assert.equal(saved.version,1);assert.equal(saved.revision,(before?.revision??0)+1)
  assert.equal(saved.state,step==='save-challenge'?'draft':step==='review-challenge'?'review':'approved')
  const sent=writes[0].postDataJSON()
  if(step==='save-challenge'){
   assert.deepEqual(sent.document.rubric,input.document.rubric)
   assert.equal(sent.document.title,input.document.title);assert.equal(sent.document.markdown,input.document.markdown)
   assert.equal(JSON.stringify(sent).includes('private_judge_guidance'),false)
  }else assert.equal(sent.action,step==='review-challenge'?'review':'approve')
  await expect(page.getByRole('status').filter({hasText:step==='save-challenge'?'Draft version 1 saved.':step==='review-challenge'?'Version ready for approval review.':'Version approved.'})).toBeVisible()
  assert.equal(saved.released_version,null)
  await page.reload();await page.getByRole('button',{name:`Edit ${input.document.title}`,exact:true}).click()
  await expect(page.getByLabel('Page title',{exact:true})).toHaveValue(input.document.title)
  await page.getByRole('navigation',{name:'Content editor sections',exact:true}).getByRole('button',{name:'Judging criteria',exact:true}).click()
  const binding=page.getByRole('region',{name:'Challenge judging criteria',exact:true})
  await expect(binding).toContainText(`Selected version ${input.document.rubric.version} is currently approved.`)
  await expect(page.getByRole('button',{name:'Release approved version',exact:true})).toHaveCount(0)
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
  if(step==='approve-challenge')await page.screenshot({path:join(output,`native-challenge-approved-${phase.sequence}.png`),fullPage:true})
  return {event_id:phase.event_id,content_id:saved.id,action:step,version:saved.version,revision:saved.revision,rubric:saved.document.rubric,preview_without_write:true,one_confirmed_command:true,reloaded:true}
 }finally{page.off('request',count)}
}

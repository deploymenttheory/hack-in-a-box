import assert from 'node:assert/strict'
import {join} from 'node:path'

export async function openDeliberationInFleet({page,expect,phase,output}){
 const input=phase.deliberation,before=input.round,path=`/api/v1/events/${phase.event_id}/judging/${before.run_id}`,writes=[]
 const record=request=>{if(new URL(request.url()).pathname===path&&request.method()==='POST')writes.push(request)}
 page.on('request',record)
 try{
  await page.clock.setFixedTime(new Date(phase.domain_now))
  await page.goto(`http://admin.localhost:8080/events/${phase.event_id}/judging`)
  await expect(page.getByRole('heading',{name:'Judging coordination',exact:true})).toBeVisible()
  const task=page.getByRole('region',{name:'Organiser final judging setup',exact:true})
  await task.getByRole('combobox',{name:'Judging task',exact:true}).selectOption('open_deliberation')
  await expect(task).toContainText('Coverage alone does not prove that scoring is complete')
  await task.getByRole('textbox',{name:'Deliberation reason',exact:true}).fill(input.reason)
  const opener=task.getByRole('button',{name:'Review opening deliberation',exact:true})
  await opener.focus();await opener.press('Enter')
  const review=page.getByRole('alertdialog',{name:'Review final deliberation',exact:true})
  await expect(review).toContainText(before.run_id);await expect(review).toContainText(`Round revision ${before.revision}`)
  await expect(review).toContainText(input.reason)
  assert.equal(writes.length,0)
  await review.getByRole('button',{name:'Cancel',exact:true}).click();await expect(opener).toBeFocused();assert.equal(writes.length,0)
  await opener.press('Enter')
  const pending=page.waitForResponse(response=>new URL(response.url()).pathname===path&&response.request().method()==='POST')
  await review.getByRole('button',{name:'Open reviewed deliberation',exact:true}).click()
  const response=await pending;assert.equal(response.status(),input.expected_status);assert.equal(writes.length,1)
  assert.equal(writes[0].headers()['if-match'],`"${before.revision}"`)
  const sent=writes[0].postDataJSON()
  assert.match(sent.request_id,/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/)
  assert.deepEqual(sent,{request_id:sent.request_id,action:'open_deliberation',reason:input.reason})
  await expect(review).toHaveCount(0)
  const accepted=input.expected_status===200
  if(accepted){
   const receipt=await response.json()
   assert.equal(receipt.request_id,sent.request_id);assert.equal(receipt.actor_id,input.actor_id);assert.equal(receipt.run_id,before.run_id)
   assert.equal(receipt.team_id,'00000000-0000-0000-0000-000000000000');assert.equal(receipt.action,'open_deliberation');assert.equal(receipt.revision,before.revision+1);assert.equal(receipt.state,'recorded');assert.match(receipt.fingerprint,/^[0-9a-f]{64}$/)
   await expect(task.getByRole('status',{name:'Last acknowledged setup change',exact:true})).toContainText('original opening of deliberation was acknowledged')
   await page.reload();await task.getByRole('combobox',{name:'Judging task',exact:true}).selectOption('open_deliberation')
   await expect(task).toContainText('Deliberation is already open for this round')
   await task.screenshot({path:join(output,'native-deliberation.png')})
  }else{
   assert.equal(input.expected_status,409)
   await expect(task).toContainText('starting point changed')
   await expect(task.getByRole('status',{name:'Last acknowledged setup change',exact:true})).not.toContainText('was acknowledged')
   await task.getByRole('button',{name:'Read current judging setup',exact:true}).click()
   await expect(task.getByRole('textbox',{name:'Deliberation reason',exact:true})).toHaveValue(input.reason)
  }
  await expect(page.getByRole('region',{name:'Independent final scoring',exact:true})).toHaveCount(0)
  assert.equal(writes.length,1);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
  return {event_id:phase.event_id,run_id:before.run_id,command_id:sent.request_id,expected_revision:before.revision,accepted,expected_status:input.expected_status,cancel_without_write:true,one_explicit_attempt:true,current_read_checked:true}
 }finally{page.off('request',record)}
}

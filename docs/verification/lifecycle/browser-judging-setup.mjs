import assert from 'node:assert/strict'
import {join} from 'node:path'

// Browser writes through production controls; native Go independently verifies
// exact configuration identity and immutable assignment fingerprints afterward.
export async function planFinalJudgingInFleet({page,expect,phase,output}){
 const input=phase.judging_setup,creating=phase.action==='create-final-round'
 assert.ok(creating||phase.action==='assign-final-judges')
 await page.clock.setFixedTime(new Date(phase.domain_now))
 await page.goto(`http://admin.localhost:8080/events/${phase.event_id}/judging`)
 const region=page.getByRole('region',{name:'Organiser final judging setup',exact:true})
 await expect(region).toBeVisible()
 await expect(page.locator('body')).not.toContainText('PRIVATE synthetic judge guidance')
 const path=creating?`/api/v1/events/${phase.event_id}/rubric/configuration`:`/api/v1/events/${phase.event_id}/judging/${input.round.run_id}`
 const writes=[]
 const record=request=>{if(new URL(request.url()).pathname===path&&request.method()==='POST')writes.push(request)}
 page.on('request',record)
 try{
  if(creating){
   const ref=input.configuration.reference
   await region.getByRole('button',{name:'Review final round creation',exact:true}).press('Enter')
   await expect(region.getByRole('combobox',{name:'Approved rubric version',exact:true})).toBeFocused()
   assert.equal(writes.length,0)
   await region.getByRole('combobox',{name:'Approved rubric version',exact:true}).selectOption(JSON.stringify([ref.id,ref.version,ref.digest]))
   await region.getByRole('textbox',{name:'Round creation reason',exact:true}).fill(input.reason)
  }else{
   await region.getByRole('combobox',{name:'Team to assign',exact:true}).selectOption({label:input.team_name})
   await expect(region.getByRole('combobox',{name:'Team to assign',exact:true})).toHaveValue(input.team_id)
   for(const judge of input.judges)await region.getByRole('checkbox',{name:judge.name,exact:true}).check()
   await expect(region.getByRole('status',{name:'Selected judge coverage',exact:true})).toContainText('3 selected · 3 required')
   await region.getByRole('textbox',{name:'Assignment reason',exact:true}).fill(input.reason)
  }
  const opener=region.getByRole('button',{name:creating?'Review final round creation':'Review judge assignment',exact:true})
  await opener.press('Enter')
  const dialog=page.getByRole('alertdialog',{name:creating?'Create final judging round':'Review final judge assignment',exact:true})
  await expect(dialog).toContainText(input.reason)
  if(creating)await expect(dialog).toContainText(`Approved version ${input.configuration.reference.version}`)
  else{await expect(dialog).toContainText(input.team_name);for(const judge of input.judges)await expect(dialog).toContainText(judge.name)}
  assert.equal(writes.length,0)
  await dialog.getByRole('button',{name:'Cancel',exact:true}).click();await expect(opener).toBeFocused()
  assert.equal(writes.length,0)
  await opener.press('Enter')
  const pending=page.waitForResponse(response=>new URL(response.url()).pathname===path&&response.request().method()==='POST')
  await dialog.getByRole('button',{name:creating?'Create reviewed final round':'Save reviewed judge assignment',exact:true}).click()
  const response=await pending;assert.equal(response.status(),200);assert.equal(writes.length,1)
  const sent=writes[0].postDataJSON(),received=await response.json()
  assert.equal(writes[0].headers()['if-match'],`"${creating?input.configuration.revision:input.round.revision}"`)
  let result
  if(creating){
   assert.deepEqual(sent,{action:'create_run',purpose:'judge_final',version:input.configuration.reference.version,reason:input.reason})
   assert.equal(received.revision,input.configuration.revision+1)
   const added=received.runs.filter(run=>!(input.configuration.runs??[]).some(old=>old.id===run.id));assert.equal(added.length,1)
   const run=added[0];assert.equal(run.purpose,'judge_final');assert.equal(run.started_at,null);assert.deepEqual(run.reference,input.configuration.reference)
   result={run_id:run.id}
  }else{
   assert.deepEqual(sent,{request_id:sent.request_id,action:'assign',team_id:input.team_id,judge_ids:input.judges.map(judge=>judge.id),reason:input.reason})
   assert.match(sent.request_id,/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/)
   assert.equal(received.request_id,sent.request_id);assert.equal(received.run_id,input.round.run_id);assert.equal(received.team_id,input.team_id)
   assert.equal(received.action,'assign');assert.equal(received.state,'recorded');assert.equal(received.revision,input.round.revision+1)
   result={command_id:sent.request_id}
  }
  await expect(dialog).toHaveCount(0)
  await expect(region.getByRole('status',{name:'Last acknowledged setup change',exact:true})).toContainText(creating?'Final round created from the reviewed rubric':'assignment was acknowledged')
  await page.reload()
  await expect(region.getByRole('combobox',{name:'Team to assign',exact:true})).toBeVisible()
  if(!creating){
   const coverage=region.getByRole('region',{name:'Current final judge assignments',exact:true})
   const row=coverage.getByRole('row').filter({has:page.getByText(input.team_name,{exact:true})})
   for(const judge of input.judges)await expect(row).toContainText(judge.name)
   await expect(row).toContainText('0')
  }
  assert.equal(writes.length,1)
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
  if(!creating&&phase.sequence===1607)await region.screenshot({path:join(output,'native-final-judging-setup.png')})
  return {event_id:phase.event_id,action:phase.action,...result,cancel_without_write:true,one_confirmed_write:true,reloaded:true}
 }finally{page.off('request',record)}
}

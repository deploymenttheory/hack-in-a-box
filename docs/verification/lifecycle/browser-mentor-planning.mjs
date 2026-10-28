import assert from 'node:assert/strict'
import {join} from 'node:path'

export async function planMentorAssessmentInFleet({page,expect,phase,output}){
 const input=phase.mentor_planning,creating=phase.action==='create-mentor-assessment'
 assert.ok(creating||phase.action==='configure-mentor-checkpoint')
 await page.clock.setFixedTime(new Date(phase.domain_now))
 await page.goto(`http://admin.localhost:8080/events/${phase.event_id}?section=mentors`)
 const planner=page.getByRole('region',{name:'Organiser mentor checkpoint planning',exact:true})
 await expect(planner.getByRole('combobox',{name:'Planning task',exact:true})).toBeVisible()
 await expect(page.locator('body')).not.toContainText('PRIVATE synthetic judge guidance')
 await planner.getByRole('combobox',{name:'Planning task',exact:true}).selectOption(creating?'run':'checkpoint')
 const path=creating?`/api/v1/events/${phase.event_id}/rubric/configuration`:`/api/v1/events/${phase.event_id}/scorecard-checkpoints`
 const writes=[],record=request=>{if(new URL(request.url()).pathname===path&&request.method()==='POST')writes.push(request)}
 page.on('request',record)
 try{
  if(creating){
   const ref=input.configuration.reference
   if(input.configuration.definition.mode==='all_teams')await expect(planner.getByRole('combobox',{name:'Assessment purpose',exact:true}).locator('option')).toHaveCount(1)
   await planner.getByRole('combobox',{name:'Assessment purpose',exact:true}).selectOption(input.purpose)
   await planner.getByRole('combobox',{name:'Approved mentor rubric',exact:true}).selectOption(JSON.stringify([ref.id,ref.version,ref.digest]))
   await planner.getByRole('textbox',{name:'Assessment planning reason',exact:true}).fill(input.reason)
  }else{
   const definition=input.command.definition
   await planner.getByRole('combobox',{name:'Saved checkpoint',exact:true}).selectOption('')
   await planner.getByRole('combobox',{name:'Mentor assessment run',exact:true}).selectOption(definition.run_id)
   await planner.getByRole('textbox',{name:'Checkpoint name',exact:true}).fill(definition.name)
   // Chromium normalizes zero seconds away in datetime-local inputs. The
   // production owner still sends the exact second-resolution native command.
   const fillWall=value=>value.length===19&&value.endsWith(':00')?value.slice(0,16):value
   await planner.getByLabel('Checkpoint opens',{exact:true}).fill(fillWall(definition.opens_wall_time))
   await planner.getByLabel('Checkpoint closes',{exact:true}).fill(fillWall(definition.closes_wall_time))
   await planner.getByRole('combobox',{name:'Repeated local clock times',exact:true}).selectOption(definition.dst_choice)
   await planner.getByRole('textbox',{name:'Checkpoint configuration reason',exact:true}).fill(input.command.reason)
  }
  const opener=planner.getByRole('button',{name:creating?'Review mentor assessment':'Review checkpoint window',exact:true})
  await opener.focus();await opener.press('Enter')
  const review=page.getByRole('alertdialog',{name:creating?'Review mentor assessment run':'Review mentor checkpoint window',exact:true})
  await expect(review).toBeVisible();assert.equal(writes.length,0)
  await expect(review).toContainText(creating?input.reason:input.command.reason)
  if(creating)await expect(review).toContainText(`Approved version ${input.configuration.reference.version}`)
  else{await expect(review).toContainText(input.command.definition.name);await expect(review).toContainText(input.window.timezone);await expect(review).toContainText(`Event revision ${input.revision}`)}
  await review.getByRole('button',{name:'Cancel',exact:true}).click();await expect(opener).toBeFocused();assert.equal(writes.length,0)
  await opener.press('Enter');await expect(review).toBeVisible()
  const pending=page.waitForResponse(response=>new URL(response.url()).pathname===path&&response.request().method()==='POST')
  await review.getByRole('button',{name:creating?'Create reviewed mentor assessment':'Save reviewed checkpoint window',exact:true}).click()
  const response=await pending;assert.equal(response.status(),200);assert.equal(writes.length,1)
  const sent=writes[0].postDataJSON(),receipt=await response.json()
  assert.equal(writes[0].headers()['if-match'],`"${creating?input.configuration.revision:input.revision}"`)
  let result
  if(creating){
   assert.deepEqual(sent,{action:'create_run',purpose:input.purpose,version:input.configuration.reference.version,reason:input.reason})
   const added=receipt.runs.filter(run=>!(input.configuration.runs??[]).some(old=>old.id===run.id));assert.equal(added.length,1)
   assert.equal(receipt.revision,input.configuration.revision+1)
   assert.equal(added[0].purpose,input.purpose);assert.equal(added[0].started_at,null);assert.equal(added[0].previous_id,null);assert.deepEqual(added[0].reference,input.configuration.reference)
   result={run_id:added[0].id}
  }else{
   assert.match(sent.definition.id,/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/)
   assert.deepEqual(sent,{definition:{...input.command.definition,id:sent.definition.id},version:input.command.version,reason:input.command.reason})
   assert.deepEqual(receipt.definition,sent.definition);assert.equal(receipt.version,input.command.version+1);assert.equal(receipt.purpose,input.purpose)
   assert.equal(Date.parse(receipt.opens_at),Date.parse(input.window.opens_at));assert.equal(Date.parse(receipt.closes_at),Date.parse(input.window.closes_at));assert.equal(receipt.timezone,input.window.timezone)
   result={checkpoint_id:receipt.definition.id,run_id:receipt.definition.run_id,version:receipt.version}
  }
  await expect(review).toHaveCount(0)
  await expect(planner.getByRole('status',{name:'Last acknowledged mentor planning change',exact:true})).toContainText(creating?'Mentor assessment created':'checkpoint window was acknowledged')
  await page.reload();await expect(planner.getByRole('combobox',{name:'Planning task',exact:true})).toBeVisible()
  if(!creating)await expect(planner.getByRole('region',{name:'Current mentor checkpoints',exact:true})).toContainText(input.command.definition.name)
  assert.equal(writes.length,1);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
  if(phase.sequence===1942)await planner.screenshot({path:join(output,'native-mentor-checkpoint-planning.png')})
  return {event_id:phase.event_id,action:phase.action,purpose:input.purpose,...result,cancel_without_write:true,one_confirmed_write:true,reloaded:true}
 }finally{page.off('request',record)}
}

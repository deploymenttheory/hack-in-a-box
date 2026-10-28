import assert from 'node:assert/strict'
import {join} from 'node:path'

export async function reviewSubmissionFreezeInFleet({page,expect,phase,output}){
 const input=phase.freeze,command=input.command,configure=command.action==='configure'
 assert.equal(phase.action,`freeze-${command.action}`)
 assert.ok(['configure','reconcile'].includes(command.action))
 await page.clock.setFixedTime(new Date(phase.domain_now))
 await page.goto(`http://admin.localhost:8081/events/${phase.event_id}/teams/${input.team_id}/build`)
 await expect(page.getByRole('heading',{name:'Submitted build',exact:true})).toBeVisible()
 const workspace=page.getByRole('region',{name:'Submission code workspace',exact:true})
 const editor=workspace.getByRole('region',{name:'Submission code controls',exact:true})
 await expect(editor).toBeVisible()
 if(configure)await editor.getByRole('textbox',{name:'Submission branch',exact:true}).fill(command.branch)
 else{
  await expect(workspace.getByRole('region',{name:'Code cutoff evidence',exact:true})).toContainText('Observed branch heads are candidates, not verified cutoff code.')
  await expect(workspace.getByRole('link',{name:'Open immutable code revision',exact:true})).toHaveCount(0)
  await editor.getByRole('textbox',{name:'Reviewed commit SHA',exact:true}).fill(command.sha)
  await editor.getByRole('textbox',{name:'Decision reason',exact:true}).fill(command.reason)
  await editor.getByRole('textbox',{name:'Independent supporting evidence',exact:true}).fill(command.external_evidence)
 }
 const path=`/api/v1/events/${phase.event_id}/teams/${input.team_id}/submission-freeze`,writes=[]
 page.on('request',request=>{if(new URL(request.url()).pathname===path&&request.method()==='POST')writes.push(request)})
 const opener=editor.getByRole('button',{name:configure?'Review submission branch':'Review code reconciliation',exact:true})
 await opener.focus();await opener.press('Enter')
 const dialog=page.getByRole('alertdialog',{name:configure?'Review submission branch':'Review organiser code decision',exact:true})
 await expect(dialog).toContainText(`Team ${input.team_id} · Freeze revision ${input.before.revision}`)
 if(configure)await expect(dialog).toContainText(`Configured branch: ${command.branch}`)
 else{
  await expect(dialog).toContainText(`Source ${command.source_id} · Submission state revision ${command.native_revision} · Current formal version ${command.submission_revision}`)
  await expect(dialog).toContainText(command.sha)
  await expect(dialog).toContainText(command.reason)
  await expect(dialog).toContainText(command.external_evidence)
  await expect(dialog).toContainText('Observations: None')
  await expect(dialog).toContainText('Pushes: None')
 }
 assert.equal(writes.length,0)
 await dialog.getByRole('button',{name:'Cancel',exact:true}).click()
 await expect(dialog).toHaveCount(0);await expect(opener).toBeFocused()
 assert.equal(writes.length,0,'Cancelled code review must not change the native decision')
 await opener.press('Enter')
 const response=page.waitForResponse(r=>new URL(r.url()).pathname===path&&r.request().method()==='POST')
 await dialog.getByRole('button',{name:'Confirm reviewed code change',exact:true}).click()
 const native=await response;assert.equal(native.status(),200)
 assert.equal(writes.length,1)
 assert.equal(writes[0].headers()['if-match'],`"${input.before.revision}"`)
 const sent=writes[0].postDataJSON()
 assert.match(sent.request_id,/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/)
 assert.equal(sent.action,command.action)
 if(configure){assert.deepEqual(Object.keys(sent).sort(),['action','branch','request_id']);assert.equal(sent.branch,command.branch)}
 else{
  for(const key of ['source_id','native_revision','submission_revision','sha','reason','external_evidence','observation_ids','push_ids','prior_decision_id'])assert.deepEqual(sent[key],command[key])
 }
 const receipt=await native.json()
 assert.equal(receipt.request_id,sent.request_id);assert.equal(receipt.action,command.action)
 assert.equal(receipt.expected_revision,input.before.revision);assert.equal(receipt.revision,input.before.revision+1)
 if(configure)assert.equal(receipt.branch,command.branch)
 else{
  assert.ok(receipt.decision)
  for(const key of ['source_id','native_revision','submission_revision','sha','reason','external_evidence','observation_ids','push_ids','prior_decision_id'])assert.deepEqual(receipt.decision[key],command[key])
 }
 await expect(dialog).toHaveCount(0)
 await expect(workspace.getByRole('status')).toContainText(`Original code change acknowledged: ${command.action}`)
 await page.reload();await expect(editor).toBeVisible()
 if(configure)await expect(editor.getByRole('textbox',{name:'Submission branch',exact:true})).toHaveValue(command.branch)
 else{
  await expect(workspace).toContainText(`Reviewed SHA ${command.sha}`)
  await expect(workspace.getByRole('link',{name:'Open immutable code revision',exact:true})).toHaveAttribute('href',`${input.before.repository_url}/commit/${command.sha}`)
  await expect(editor.getByRole('button',{name:'Review code reconciliation',exact:true})).toHaveCount(0)
 }
 assert.equal(writes.length,1)
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
 await workspace.screenshot({path:join(output,`native-freeze-${phase.sequence}.png`)})
 return {event_id:phase.event_id,team_id:input.team_id,command_id:sent.request_id,action:command.action,revision:receipt.revision,cancel_without_write:true,one_confirmed_command:true,reloaded:true,source_id:configure?undefined:command.source_id,selected_sha:configure?undefined:command.sha}
}

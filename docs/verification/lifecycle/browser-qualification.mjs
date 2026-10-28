import assert from 'node:assert/strict'

// All assessment values stay in protected IPC. Only original request identity,
// actor alias, action and versions enter the published verification metadata.
export async function recordQualificationInFleet({browser,owner,expect,phase,errors}){
 const input=phase.qualification,command=input.command,before=input.page
 let context,page=owner,origin='http://admin.localhost:8080'
 if(input.actor!=='owner'){
  assert.ok(['mentor-a','mentor-b'].includes(input.actor));origin='http://public.localhost:8080'
  context=await browser.newContext({viewport:{width:1280,height:1000},proxy:{server:'http://127.0.0.1:18088',bypass:'<-loopback>'}})
  page=await context.newPage();page.on('pageerror',error=>errors.push(error.message))
  await page.goto(origin+'/auth/login')
  await expect(page.getByRole('heading',{name:'Synthetic identity provider',exact:true})).toBeVisible()
  await page.getByLabel('Synthetic actor',{exact:true}).selectOption(input.actor)
  await page.getByRole('button',{name:'Continue through normal OIDC',exact:true}).click()
  await expect(page.getByRole('heading',{name:'Your events',exact:true})).toBeVisible()
 }
 const path=`/api/v1/events/${phase.event_id}/mentor-qualification/${before.round.run_id}`,writes=[]
 const record=request=>{if(new URL(request.url()).pathname===path&&request.method()==='POST')writes.push(request)}
 page.on('request',record)
 try{
  await page.clock.setFixedTime(new Date(phase.domain_now))
  await page.goto(`${origin}/events/${phase.event_id}/panel/${before.round.run_id}`)
  await expect(page.getByRole('heading',{name:'Moderation and selections',exact:true})).toBeVisible()
  const teamID=command.card?.team_id||(command.action==='sign_card'?command.team_id:undefined)
  if(teamID)await page.locator(`[data-record-focus="${teamID}"]`).click()
  const actions=page.getByRole('region',{name:'Current panel actions',exact:true})
  const labels={configure_panel:'Configure intended panel',save_card:'Edit reasoned final card',sign_card:'Review saved final card confirmation',propose:'Review current selection proposal',sign_off:'Review exact panel signoff'}
  let label=labels[command.action]
  if(command.action==='vote_tie'||command.action==='resolve_tie'){
   const tie=before.preview.ties.find(tie=>tie.id===command.tie_id);assert.ok(tie)
   const name=tie.category_id?(before.criteria.categories.find(category=>category.id===tie.category_id)?.label??tie.category_id):tie.kind==='total'?'Finalist and reserve':`Selection ${command.tie_id}`
   label=`${command.action==='vote_tie'?'Record ballot':'Review tie resolution'}: ${name} · Seat ${command.seat}`
  }
  assert.ok(label)
  const staffPending=command.action==='configure_panel'?page.waitForResponse(response=>new URL(response.url()).pathname===`/api/v1/events/${phase.event_id}/staff`&&response.request().method()==='GET'):undefined
  await actions.getByRole('button',{name:label,exact:true}).click()
  if(command.action==='configure_panel'){
   const staffResponse=await staffPending;assert.equal(staffResponse.status(),200)
   const staff=(await staffResponse.json()).items.filter(person=>person.active&&person.role==='mentor')
   await actions.getByRole('combobox',{name:'Organising lead',exact:true}).selectOption(command.panel.organiser_id)
   for(const person of staff)await actions.getByRole('checkbox',{name:person.display_name||person.person_id,exact:true}).uncheck()
   for(const id of command.panel.mentor_ids){
    const person=staff.find(person=>person.person_id===id);assert.ok(person)
    await actions.getByRole('checkbox',{name:person.display_name||person.person_id,exact:true}).check()
   }
  }
  if(command.action==='save_card')for(const category of before.criteria.categories){
   const score=command.card.scores.find(score=>score.category_id===category.id);assert.ok(score)
   assert.equal(score.pending,false);assert.equal(score.evidence.length,1)
   await expect(actions.getByRole('checkbox',{name:`Pending/unobserved for ${category.label}`,exact:true})).toBeChecked()
   await expect(actions.getByRole('spinbutton',{name:`Moderated score for ${category.label}`,exact:true})).toHaveValue('')
   await expect(actions.getByRole('textbox',{name:`Moderated evidence for ${category.label}`,exact:true})).toHaveValue('')
   await actions.getByRole('checkbox',{name:`Pending/unobserved for ${category.label}`,exact:true}).uncheck()
   await actions.getByRole('spinbutton',{name:`Moderated score for ${category.label}`,exact:true}).fill(String(score.value))
   await actions.getByRole('textbox',{name:`Moderated evidence for ${category.label}`,exact:true}).fill(score.evidence[0].note)
  }
  if(command.action==='vote_tie'||command.action==='resolve_tie'){
   await expect(actions.getByRole('combobox',{name:'Candidate for affected tie seat',exact:true})).toHaveValue('')
   await actions.getByRole('combobox',{name:'Candidate for affected tie seat',exact:true}).selectOption(command.team_id)
   await actions.getByRole('textbox',{name:'Supporting observed evidence',exact:true}).fill(command.evidence[0].note)
  }
  await actions.getByRole('textbox',{name:'Recorded panel reason',exact:true}).fill(command.reason)
  const opener=actions.getByRole('button',{name:'Review entered panel decision',exact:true})
  await opener.focus();await opener.press('Enter')
  const review=page.getByRole('alertdialog',{name:'Review private panel command',exact:true})
  await expect(review.getByRole('button',{name:'Cancel',exact:true})).toBeFocused()
  await expect(review).toContainText(command.reason)
  await expect(review).toContainText(`Panel version ${before.round.version} · Event revision ${before.revision}`)
  if(command.card)for(const score of command.card.scores)await expect(review).toContainText(score.evidence[0].note)
  if(command.evidence)for(const evidence of command.evidence)await expect(review).toContainText(evidence.note)
  assert.equal(writes.length,0)
  await review.getByRole('button',{name:'Cancel',exact:true}).click();await expect(opener).toBeFocused();assert.equal(writes.length,0)
  await opener.press('Enter')
  const pending=page.waitForResponse(response=>new URL(response.url()).pathname===path&&response.request().method()==='POST')
  await review.getByRole('button',{name:'Confirm reviewed panel command',exact:true}).click()
  const response=await pending;assert.equal(response.status(),200);assert.equal(writes.length,1)
  assert.equal(writes[0].headers()['if-match'],`"${before.revision}"`)
  const sent=writes[0].postDataJSON(),next=await response.json()
  assert.match(sent.request_id,/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/)
  const expected={request_id:sent.request_id,version:before.round.version,action:command.action,reason:command.reason}
  const fields={configure_panel:['panel'],save_card:['card','freeze_binding','repository_decision_id'],sign_card:['team_id','card_version','freeze_binding','repository_decision_id'],vote_tie:['tie_id','seat','team_id','evidence'],resolve_tie:['tie_id','seat','team_id','evidence']}
  for(const field of fields[command.action]??[])expected[field]=command[field]
  assert.deepEqual(sent,expected)
  const receipt=next.receipt;assert.ok(receipt)
  assert.equal(receipt.request_id,sent.request_id);assert.equal(receipt.run_id,before.round.run_id);assert.equal(receipt.action,command.action)
  assert.equal(receipt.expected_revision,before.revision);assert.equal(receipt.expected_version,before.round.version);assert.equal(receipt.version,before.round.version+1);assert.match(receipt.fingerprint,/^[0-9a-f]{64}$/)
  assert.equal(next.round.actor_id,input.actor_id);assert.equal(next.round.action,command.action);assert.equal(next.round.reason,command.reason);assert.equal(next.round.version,receipt.version)
  await expect(review).toHaveCount(0)
  await page.reload();await expect(page.getByRole('heading',{name:'Moderation and selections',exact:true})).toBeVisible()
  if(command.action==='sign_off')await expect(page.getByText('Current private panel signoff',{exact:true})).toBeVisible()
  assert.equal(writes.length,1);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
  return {event_id:phase.event_id,run_id:before.round.run_id,actor:input.actor,action:command.action,command_id:sent.request_id,version:receipt.version,expected_revision:before.revision,expected_version:before.round.version,cancel_without_write:true,one_confirmed_write:true,reloaded:true}
 }finally{page.off('request',record);if(context)await context.close()}
}

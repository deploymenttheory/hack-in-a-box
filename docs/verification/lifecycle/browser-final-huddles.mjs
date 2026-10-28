import assert from 'node:assert/strict'

// Only resource/action/version metadata leaves this private assessment flow.
export async function reviewFinalHuddleInFleet({browser,owner,expect,phase,errors}){
 const input=phase.huddle,creating=phase.action==='configure-final-huddle'
 assert.ok(creating||phase.action==='decide-final-huddle')
 let context,page=owner,origin='http://admin.localhost:8080'
 if(input.actor!=='owner'){
  assert.equal(input.actor,'mentor-b');origin='http://public.localhost:8080'
  context=await browser.newContext({viewport:{width:1280,height:1000},proxy:{server:'http://127.0.0.1:18088',bypass:'<-loopback>'}})
  page=await context.newPage();page.on('pageerror',error=>errors.push(error.message))
  await page.goto(origin+'/auth/login')
  await expect(page.getByRole('heading',{name:'Synthetic identity provider',exact:true})).toBeVisible()
  await page.getByLabel('Synthetic actor',{exact:true}).selectOption(input.actor)
  await page.getByRole('button',{name:'Continue through normal OIDC',exact:true}).click()
  await expect(page.getByRole('heading',{name:'Your events',exact:true})).toBeVisible()
 }
 const prefix=`/api/v1/events/${phase.event_id}/teams/${input.team_id}/moderation-huddles/`,writes=[]
 const record=request=>{if(new URL(request.url()).pathname.startsWith(prefix)&&request.method()==='PUT')writes.push(request)}
 page.on('request',record)
 try{
  await page.clock.setFixedTime(new Date(phase.domain_now))
  await page.goto(`${origin}/events/${phase.event_id}/review/${input.team_id}`)
  await expect(page.getByRole('heading',{name:'Team review and huddle agenda',exact:true})).toBeVisible()
  await expect(page.locator('body')).not.toContainText('Independent observation of synthetic live work; this checkpoint never supplies final judge scores')
  if(creating){
   await expect(page.locator('body')).not.toContainText('Fresh independent review of this exact frozen synthetic final; live checkpoint excluded')
   await page.getByRole('button',{name:'Configure a new huddle',exact:true}).click()
   await page.getByRole('combobox',{name:'Checkpoint',exact:true}).selectOption(input.definition.checkpoint_id)
   await page.getByRole('combobox',{name:'Named organiser moderator',exact:true}).selectOption(input.definition.moderator_id)
   await page.getByRole('combobox',{name:'Current published second reviewer',exact:true}).selectOption(input.definition.second_reviewer_id)
   const due=new Date(input.definition.review_due_at);assert.equal(due.getUTCSeconds(),0)
   await page.getByLabel('Second-review deadline (UTC)',{exact:true}).fill(due.toISOString().slice(0,16))
   await page.getByRole('textbox',{name:'Configuration review reason',exact:true}).fill(input.reason)
  }else{
   if(input.actor!=='owner')await expect(page.getByRole('button',{name:'Configure a new huddle',exact:true})).toHaveCount(0)
   await page.getByRole('button',{name:'Open intended-panel context',exact:true}).click()
   const panel=page.getByRole('region',{name:'Intended-panel context',exact:true})
   await expect(panel).toBeVisible()
   await panel.getByRole('button',{name:'Record formal decision',exact:true}).click()
   await panel.getByRole('textbox',{name:'Explicit decision reason',exact:true}).fill(input.reason)
  }
  const labels={complete_second_review:'Record my submitted independent second review',open:'Open huddle against both exact submitted cards',sign_off:'Sign off exact complete frozen final evidence'}
  const opener=page.getByRole('button',{name:creating?'Review configuration':labels[input.action],exact:true})
  await opener.focus();await opener.press('Enter')
  const review=page.getByRole('region',{name:creating?'Review huddle configuration':'Review exact huddle decision',exact:true})
  await expect(review.getByRole('heading',{name:creating?'Review huddle configuration':'Review exact huddle decision',exact:true})).toBeFocused()
  await expect(review).toContainText(input.reason);await expect(review).toContainText(`Event revision ${input.revision}`)
  assert.equal(writes.length,0)
  await review.getByRole('button',{name:'Correct details',exact:true}).click();await expect(opener).toBeFocused();assert.equal(writes.length,0)
  await opener.press('Enter');await expect(review).toBeVisible()
  const pending=page.waitForResponse(response=>new URL(response.url()).pathname.startsWith(prefix)&&response.request().method()==='PUT')
  await review.getByRole('button',{name:creating?'Record reviewed configuration':'Confirm reviewed huddle decision',exact:true}).click()
  const response=await pending;assert.equal(response.status(),200);assert.equal(writes.length,1)
  assert.equal(writes[0].headers()['if-match'],`"${input.revision}"`)
  const sent=writes[0].postDataJSON(),huddle=await response.json(),id=new URL(writes[0].url()).pathname.slice(prefix.length)
  assert.match(id,/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/)
  assert.equal(huddle.definition.id,id);assert.equal(huddle.team_id,input.team_id);assert.equal(huddle.actor_id,input.actor_id);assert.equal(huddle.action,input.action);assert.equal(huddle.reason,input.reason);assert.equal(huddle.version,input.version+1)
  assert.equal(huddle.checkpoint.purpose,'mentor_final');assert.equal(huddle.definition.checkpoint_id,input.definition.checkpoint_id);assert.equal(huddle.definition.checkpoint_version,input.definition.checkpoint_version)
  if(creating){
   assert.deepEqual(Object.keys(sent).sort(),['action','definition','reason','version'])
   assert.deepEqual(sent,{action:'configure',version:0,reason:input.reason,definition:{...input.definition,id,review_due_at:sent.definition.review_due_at}})
   assert.equal(Date.parse(sent.definition.review_due_at),Date.parse(input.definition.review_due_at));assert.deepEqual(huddle.definition,sent.definition);assert.equal(huddle.state,'planned')
  }else{
   assert.equal(id,input.huddle_id);assert.deepEqual(sent,{action:input.action,version:input.version,reason:input.reason})
   if(input.action==='complete_second_review'){
    assert.equal(huddle.state,'planned');assert.ok(huddle.second_review.completed_at);assert.equal(huddle.second_review.reviewer_id,input.actor_id);assert.deepEqual(huddle.second_review.card,input.cards[1])
   }else{
    assert.equal(huddle.state,input.action==='open'?'open':'signed');assert.equal(huddle.submission_revision,input.frozen_revision)
    const keys=cards=>cards.map(card=>`${card.id}:${card.version}`).sort()
    assert.deepEqual(keys(huddle.cards),keys(input.cards))
    if(input.action==='sign_off'){assert.ok(huddle.signoff_id);assert.ok(huddle.signoff_context?.binding)}
   }
  }
  await expect(review).toHaveCount(0)
  await page.reload();await expect(page.getByRole('heading',{name:'Team review and huddle agenda',exact:true})).toBeVisible()
  await expect(page.getByRole('table',{name:'Huddle agenda',exact:true}).or(page.getByRole('list',{name:'Huddle agenda',exact:true}))).toContainText(huddle.state)
  assert.equal(writes.length,1);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
  return {event_id:phase.event_id,team_id:input.team_id,huddle_id:id,actor:input.actor,action:input.action,version:huddle.version,state:huddle.state,correction_without_write:true,one_confirmed_write:true,reloaded:true}
 }finally{page.off('request',record);if(context)await context.close()}
}

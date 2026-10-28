import assert from 'node:assert/strict'

// Private assessment bodies remain in protected native artifacts only. This
// helper returns identity/version metadata and never captures score screenshots.
export async function submitLiveMentorCardInFleet({browser,expect,phase,errors}){
 assert.equal(phase.action,'submit-live-mentor-card')
 const input=phase.mentor_card,checkpoint=input.checkpoint
 assert.equal(checkpoint.purpose,'checkpoint')
 const context=await browser.newContext({viewport:{width:1280,height:1000},proxy:{server:'http://127.0.0.1:18088',bypass:'<-loopback>'}})
 const page=await context.newPage();page.on('pageerror',error=>errors.push(error.message))
 try{
  await page.goto('http://public.localhost:8080/auth/login')
  await expect(page.getByRole('heading',{name:'Synthetic identity provider',exact:true})).toBeVisible()
  await page.getByLabel('Synthetic actor',{exact:true}).selectOption(input.actor)
  await page.getByRole('button',{name:'Continue through normal OIDC',exact:true}).click()
  await expect(page.getByRole('heading',{name:'Your events',exact:true})).toBeVisible()
  await page.clock.setFixedTime(new Date(phase.domain_now))
  await page.goto(`http://public.localhost:8080/events/${phase.event_id}/mentor/${input.team_id}/scorecards`)
  await expect(page.getByRole('heading',{name:'Your independent scorecards',exact:true})).toBeVisible()
  const open=page.getByRole('button',{name:`Open ${checkpoint.definition.name}`,exact:true})
  await open.click()
  const editor=page.getByRole('region',{name:'Your private scorecard',exact:true})
  await expect(editor.getByRole('heading',{name:checkpoint.definition.name,exact:true})).toBeVisible()
  await expect(editor).toContainText('PROVISIONAL live checkpoint. This is not a final score or a running total.')
  await expect(editor.getByRole('status',{name:'Scorecard save state',exact:true})).toContainText('No server save exists yet')
  const path=`/api/v1/events/${phase.event_id}/teams/${input.team_id}/scorecards`,writes=[]
  page.on('request',request=>{if(new URL(request.url()).pathname===path&&request.method()==='POST')writes.push(request)})
  for(const category of checkpoint.criteria.categories){
   const score=input.command.scores.find(value=>value.category_id===category.id)
   assert.ok(score);assert.equal(score.pending,false);assert.equal(score.evidence.length,1)
   const group=editor.getByRole('group',{name:category.label,exact:true})
   await group.getByRole('checkbox',{name:'Pending/unobserved — no numeric score',exact:true}).uncheck()
   await group.getByRole('spinbutton',{name:`${category.label} score (explicit zero is allowed when within range)`,exact:true}).fill(String(score.value))
   await group.getByRole('textbox',{name:`Private evidence note for ${category.label}`,exact:true}).fill(score.evidence[0].note)
  }
  const opener=editor.getByRole('button',{name:'Submit independent card',exact:true})
  await opener.focus();await opener.press('Enter')
  const review=editor.getByRole('region',{name:'Review independent scorecard',exact:true})
  await expect(review.getByRole('heading',{name:'Review independent scorecard',exact:true})).toBeFocused()
  await expect(review).toContainText(`Checkpoint version ${checkpoint.version} · Rubric version ${checkpoint.rubric.version}`)
  for(const score of input.command.scores)await expect(review).toContainText(score.evidence[0].note)
  assert.equal(writes.length,0)
  await review.getByRole('button',{name:'Correct details',exact:true}).click();await expect(opener).toBeFocused();assert.equal(writes.length,0)
  await opener.press('Enter')
  const pending=page.waitForResponse(response=>new URL(response.url()).pathname===path&&response.request().method()==='POST')
  await review.getByRole('button',{name:'Confirm independent submission',exact:true}).click()
  const response=await pending;assert.equal(response.status(),200);assert.equal(writes.length,1)
  assert.equal(writes[0].headers()['if-match'],`"${input.revision}"`)
  assert.deepEqual(writes[0].postDataJSON(),input.command)
  const card=await response.json()
  assert.equal(card.author_id,input.author_id);assert.equal(card.team_id,input.team_id);assert.equal(card.checkpoint_id,checkpoint.definition.id)
  assert.equal(card.checkpoint_version,checkpoint.version);assert.deepEqual(card.rubric,checkpoint.rubric);assert.equal(card.run_id,checkpoint.definition.run_id)
  assert.equal(card.reference.version,1);assert.equal(card.revision,input.revision+1);assert.equal(card.purpose,'checkpoint');assert.equal(card.state,'submitted');assert.equal(card.submission_revision,null)
  assert.deepEqual(card.scores,input.command.scores)
  await expect(editor.getByRole('status',{name:'Scorecard save state',exact:true})).toContainText('Confirmed your exact submitted version 1 is saved on the server')
  await page.reload();await open.click()
  await expect(editor.getByRole('status',{name:'Scorecard save state',exact:true})).toContainText('Loaded your saved submitted version 1')
  for(const category of checkpoint.criteria.categories){
   const score=input.command.scores.find(value=>value.category_id===category.id)
   await expect(editor.getByRole('spinbutton',{name:`${category.label} score (explicit zero is allowed when within range)`,exact:true})).toHaveValue(String(score.value))
   await expect(editor.getByRole('textbox',{name:`Private evidence note for ${category.label}`,exact:true})).toHaveValue(score.evidence[0].note)
  }
  assert.equal(writes.length,1)
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
  return {event_id:phase.event_id,actor:input.actor,team_id:input.team_id,card_id:card.reference.id,version:1,correct_without_write:true,one_confirmed_write:true,reloaded:true,provisional_checkpoint:true}
 }finally{await context.close()}
}

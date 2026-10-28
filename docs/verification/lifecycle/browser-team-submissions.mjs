import assert from 'node:assert/strict'
import {join} from 'node:path'

// A real team member authors the evidence through production controls. The
// native observer separately checks identities, timing, privacy and cutoff.
export async function submitTeamEvidenceInFleet({browser,expect,phase,output,errors}){
 const input=phase.submission,isDraft=input.kind==='draft'
 assert.equal(phase.action,`submit-${input.kind}-evidence`)
 assert.ok(['draft','formal'].includes(input.kind))
 assert.equal(input.evidence.length,1)
 const evidence=input.evidence[0]
 const context=await browser.newContext({viewport:{width:1280,height:1000},proxy:{server:'http://127.0.0.1:18088',bypass:'<-loopback>'}})
 const page=await context.newPage();page.on('pageerror',error=>errors.push(error.message))
 try{
  await page.goto('http://public.localhost:8080/auth/login')
  await expect(page.getByRole('heading',{name:'Synthetic identity provider',exact:true})).toBeVisible()
  await page.getByLabel('Synthetic actor',{exact:true}).selectOption(input.actor)
  await page.getByRole('button',{name:'Continue through normal OIDC',exact:true}).click()
  await expect(page.getByRole('heading',{name:'Your events',exact:true})).toBeVisible()
  await page.clock.setFixedTime(new Date(phase.domain_now))
  await page.goto(`http://public.localhost:8080/events/${phase.event_id}?section=deliverables`)
  const editor=page.getByRole('region',{name:'Team evidence editor',exact:true})
  await expect(editor).toBeVisible()
  const title='Reviewed competition output'
  await editor.getByRole('combobox',{name:`Evidence type for ${title}`,exact:true}).selectOption(evidence.kind)
  await editor.getByRole('textbox',{name:`Evidence label for ${title}`,exact:true}).fill(evidence.label)
  await editor.getByRole('textbox',{name:`Reference URL for ${title}`,exact:true}).fill(evidence.url)
  const path=`/api/v1/events/${phase.event_id}/teams/${input.team_id}/submission${isDraft?'/draft':''}`,writes=[]
  page.on('request',request=>{if(new URL(request.url()).pathname===path&&request.method()==='PUT')writes.push(request)})
  const opener=editor.getByRole('button',{name:isDraft?'Save private draft':'Submit formal version',exact:true})
  if(isDraft)await expect(editor.getByRole('button',{name:'Submit formal version',exact:true})).toBeDisabled()
  await opener.focus();await opener.press('Enter')
  const dialog=page.getByRole('alertdialog',{name:isDraft?'Review private evidence draft':'Review formal submission',exact:true})
  await expect(dialog).toContainText(`Expected revision ${input.revision}`)
  await expect(dialog).toContainText(evidence.label)
  await expect(dialog.getByRole('link')).toHaveAttribute('href',evidence.url)
  assert.equal(writes.length,0)
  await dialog.getByRole('button',{name:'Cancel',exact:true}).click()
  await expect(dialog).toHaveCount(0)
  await expect(opener).toBeFocused()
  assert.equal(writes.length,0,'Cancelled review must not save evidence')
  await opener.focus();await opener.press('Enter')
  const response=page.waitForResponse(r=>new URL(r.url()).pathname===path&&r.request().method()==='PUT')
  await dialog.getByRole('button',{name:isDraft?'Confirm private draft':'Confirm formal submission',exact:true}).click()
  const native=await response;assert.equal(native.status(),200)
  assert.equal(writes.length,1)
  assert.equal(writes[0].headers()['if-match'],`"${input.revision}"`)
  const sent=writes[0].postDataJSON();assert.deepEqual(sent.evidence,input.evidence)
  if(!isDraft)assert.equal(sent.reason,'')
  const receipt=await native.json(),saved=isDraft?receipt:receipt.final
  assert.equal(saved.revision,input.revision+1)
  assert.deepEqual(saved.evidence,input.evidence)
  if(!isDraft){assert.equal(receipt.team_id,input.team_id);assert.equal(saved.late,false)}
  await expect(dialog).toHaveCount(0)
  await expect(editor).toContainText(isDraft?'Private draft saved. No formal submission was accepted or reviewed.':'Formal version 1 submitted. Completion review is pending.')
  await page.reload();await expect(editor).toBeVisible()
  await expect(editor.getByRole('combobox',{name:`Evidence type for ${title}`,exact:true})).toHaveValue(evidence.kind)
  await expect(editor.getByRole('textbox',{name:`Evidence label for ${title}`,exact:true})).toHaveValue(evidence.label)
  await expect(editor.getByRole('textbox',{name:`Reference URL for ${title}`,exact:true})).toHaveValue(evidence.url)
  if(isDraft)await expect(editor.getByRole('button',{name:'Submit formal version',exact:true})).toBeDisabled()
  assert.equal(writes.length,1)
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
  // Synthetic team evidence only; no private question or assessment workspace.
  await editor.screenshot({path:join(output,`native-team-${input.kind}-${phase.sequence}.png`)})
  return {event_id:phase.event_id,team_id:input.team_id,actor:input.actor,kind:input.kind,revision:saved.revision,evidence_kind:evidence.kind,cancel_without_write:true,one_confirmed_command:true,reloaded:true,preweek_formal_disabled:isDraft}
 }finally{await context.close()}
}

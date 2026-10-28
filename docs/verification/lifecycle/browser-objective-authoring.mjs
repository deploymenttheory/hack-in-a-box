import assert from 'node:assert/strict'
import {join} from 'node:path'

export async function createObjectiveInFleet({page,expect,phase,output}){
 assert.equal(phase.action,'create-objective')
 const input=phase.objective,d=input.definition,base=`/api/v1/events/${phase.event_id}/`,writes=[]
 await page.clock.setFixedTime(new Date(phase.domain_now))
 await page.goto(`http://admin.localhost:8080/events/${phase.event_id}?section=objectives`)
 await expect(page.getByRole('heading',{name:'Objectives and success measures',exact:true})).toBeVisible()
 const count=request=>{if(new URL(request.url()).pathname.startsWith(base)&&!['GET','HEAD'].includes(request.method()))writes.push(request)}
 page.on('request',count)
 try{
  await page.getByRole('button',{name:'Add objective',exact:true}).click()
  await page.getByLabel('Objective description',{exact:true}).fill(d.description)
  await page.getByLabel('Objective owner (optional)',{exact:true}).selectOption(d.owner_id)
  await page.getByLabel('This objective reveals the hackathon theme',{exact:true}).setChecked(d.theme_sensitive)
  assert.equal(d.measures.length,1)
  await page.getByRole('button',{name:'Add success measure',exact:true}).click()
  const measure=d.measures[0]
  await page.getByLabel('Measure description',{exact:true}).fill(measure.description)
  await page.getByLabel('Target (optional; blank means unknown)',{exact:true}).fill(measure.target??'')
  await page.getByLabel('Unit',{exact:true}).fill(measure.unit)
  assert.ok(measure.baseline)
  await page.getByLabel('Record a planning baseline',{exact:true}).check()
  await page.getByLabel('Baseline value',{exact:true}).fill(measure.baseline.value)
  await page.getByRole('combobox',{name:'Baseline basis',exact:true}).selectOption(measure.baseline.basis)
  await page.getByLabel('Measurement method',{exact:true}).fill(measure.method)
  await page.getByLabel('Where the review should obtain evidence',{exact:true}).fill(measure.evidence_source)
  await page.getByLabel('Review horizon',{exact:true}).fill(measure.review_horizon)
  await page.getByLabel('Reason for this definition version',{exact:true}).fill(input.reason)
  assert.equal(writes.length,0,'Editing planning inputs must not save a definition or achieved results')
  const response=page.waitForResponse(r=>new URL(r.url()).pathname.startsWith(base+'objectives/')&&r.request().method()==='PUT')
  await page.getByRole('button',{name:'Save definition version',exact:true}).click()
  const native=await response;assert.equal(native.status(),200)
  const saved=await native.json()
  assert.equal(writes.length,1);assert.equal(writes[0].headers()['if-match'],'"0"')
  assert.equal(saved.reference.version,1);assert.equal(saved.current_version,1);assert.equal(saved.retired,false)
  const sent=writes[0].postDataJSON();assert.equal(sent.action,'save');assert.equal(sent.reason,input.reason)
  assert.equal(sent.definition.description,d.description);assert.equal(sent.definition.owner_id,d.owner_id)
  assert.equal(sent.definition.measures[0].baseline.basis,'estimate');assert.equal(saved.definition.measures[0].id,sent.definition.measures[0].id)
  assert.equal(new URL(writes[0].url()).pathname,base+'objectives/'+saved.reference.id)
  await expect(page.getByRole('status',{name:'Objective save state',exact:true})).toContainText('Definition version 1 saved. No achieved results were recorded.')
  await page.reload();await page.locator(`[data-record-focus="${saved.reference.id}"]`).click()
  await expect(page.getByLabel('Objective description',{exact:true})).toHaveValue(d.description)
  await expect(page.getByLabel('Objective owner (optional)',{exact:true})).toHaveValue(d.owner_id)
  await expect(page.getByLabel('Baseline value',{exact:true})).toHaveValue(measure.baseline.value)
  await expect(page.getByRole('combobox',{name:'Baseline basis',exact:true})).toHaveValue('estimate')
  await expect(page.getByLabel('Target (optional; blank means unknown)',{exact:true})).toHaveValue(measure.target)
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
  await page.screenshot({path:join(output,`native-objective-${phase.sequence}.png`),fullPage:true})
  return {event_id:phase.event_id,objective_id:saved.reference.id,version:saved.reference.version,measure_count:saved.definition.measures.length,baseline_basis:saved.definition.measures[0].baseline.basis,editing_without_write:true,one_confirmed_command:true,reloaded:true}
 }finally{page.off('request',count)}
}

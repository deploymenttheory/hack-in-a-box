import assert from 'node:assert/strict'
import {join} from 'node:path'

// Normal Fleet controls own every write. The native journey separately reads
// the result before it releases the next command barrier.
export async function changeTeamInFleet({page,expect,phase,output}){
 assert.equal(phase.action,'team-change')
 const input=phase.team_change,command=input.command,path=`/api/v1/events/${phase.event_id}/teams`
 await page.clock.setFixedTime(new Date(phase.domain_now))
 await page.goto(`http://admin.localhost:8080/events/${phase.event_id}?section=teams`)
 await expect(page.getByRole('heading',{name:'Team allocation',exact:true})).toBeVisible()
 const writes=[]
 const count=request=>{if(new URL(request.url()).pathname===path&&request.method()==='POST')writes.push(request)}
 page.on('request',count)
 try{
  let confirm
  switch(command.action){
   case 'create':
    await page.getByRole('button',{name:'Create draft team',exact:true}).click()
    await page.getByLabel('Allocation decision reason',{exact:true}).fill(command.reason)
    await page.getByLabel('Team name',{exact:true}).fill(command.name)
    await page.getByLabel('Confirmed captain',{exact:true}).selectOption(command.person_id)
    await page.getByRole('button',{name:'Review draft team',exact:true}).click()
    confirm=page.getByRole('button',{name:'Confirm team change',exact:true})
    break
   case 'move':
    await page.getByRole('button',{name:/^Placement pool/}).click()
    await page.getByLabel('Allocation decision reason',{exact:true}).fill(command.reason)
    await page.getByLabel('Confirmed competitor',{exact:true}).selectOption(command.person_id)
    await page.getByLabel('Destination team',{exact:true}).selectOption(command.team_id)
    await page.getByRole('button',{name:'Assign draft member',exact:true}).click()
    confirm=page.getByRole('button',{name:'Confirm team change',exact:true})
    break
   case 'unlock':
    await page.locator(`[data-record-focus="${command.team_id}"]`).click()
    await page.getByLabel('Allocation decision reason',{exact:true}).fill(command.reason)
    await page.getByRole('button',{name:'Unlock team',exact:true}).click()
    confirm=page.getByRole('button',{name:'Confirm team change',exact:true})
    break
   case 'publish':
   case 'freeze':
    await page.getByRole('button',{name:'Review allocation',exact:true}).click()
    await page.getByLabel('Allocation decision reason',{exact:true}).fill(command.reason)
    await page.getByRole('button',{name:command.action==='freeze'?'Review publication and freeze':'Review publication',exact:true}).click()
    const preview=page.getByRole('region',{name:'Publication preview',exact:true})
    await expect(preview).toBeVisible()
    await expect(preview.getByRole('heading',{name:`Publication preview · revision ${input.revision}`,exact:true})).toBeVisible()
    await expect(preview).toContainText(command.reason)
    confirm=preview.getByRole('button',{name:command.action==='freeze'?'Publish and freeze teams':'Publish assignments',exact:true})
    break
   default:throw new Error('Unsupported reviewed team command')
  }
  if(!['publish','freeze'].includes(command.action)){
   const review=page.getByRole('region',{name:'Review team change',exact:true})
   await expect(review.getByRole('heading',{name:'Review team change',exact:true})).toBeFocused()
   await expect(review).toContainText(`reviewed revision ${input.revision}`)
   await expect(review).toContainText(command.reason)
   assert.ok(input.team_name)
   await expect(review).toContainText(input.team_name)
   if(command.action==='create'||command.action==='move'){
    assert.ok(input.person_name)
    await expect(review).toContainText(input.person_name)
   }
  }
  await expect(confirm).toBeEnabled()
  assert.equal(writes.length,0,'Review must not perform a team write')
  const response=page.waitForResponse(r=>new URL(r.url()).pathname===path&&r.request().method()==='POST')
  await confirm.click()
  const result=await response
  assert.equal(result.status(),200)
  const saved=await result.json()
  assert.equal(saved.revision,input.revision+1)
  assert.equal(writes.length,1,'One reviewed confirmation must send exactly one command')
  const sent=writes[0].postDataJSON()
  for(const key of ['action','reason',...(command.action==='create'?['name','person_id']:command.action==='move'?['team_id','person_id']:command.action==='unlock'?['team_id']:[])])assert.equal(sent[key],command[key])
  assert.equal(writes[0].headers()['if-match'],`"${input.revision}"`)
  // Wait for the owner acknowledgement before reloading; a pending command
  // must never be abandoned merely because its HTTP response arrived.
  await expect(confirm).toHaveCount(0)
  await page.reload()
  await expect(page.getByRole('heading',{name:'Team allocation',exact:true})).toBeVisible()
  if(['publish','freeze'].includes(command.action))await page.screenshot({path:join(output,`native-teams-${phase.sequence}.png`),fullPage:true})
  assert.equal(writes.length,1)
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
  return {event_id:phase.event_id,sequence:phase.sequence,action:command.action,review_without_write:true,one_confirmed_command:true,reloaded:true}
 }finally{page.off('request',count)}
}

// Real Fleet staff actions and current-authority reads on both native events.
// The browser receives fixture identities, never cookies, CSRF or API responses.
import assert from 'node:assert/strict'
import {join} from 'node:path'

const admin='http://admin.localhost:8080',publicOrigin='http://public.localhost:8080'
const privateHelp="PRIVATE synthetic request for help demonstrating the current team's work."

export async function checkLiveRoles({browser,expect,owner,readPhase,acknowledge,output,errors}){
 const receipts=[]
 async function actor(alias){
  const context=await browser.newContext({viewport:{width:1280,height:1000},proxy:{server:'http://127.0.0.1:18088',bypass:'<-loopback>'}})
  async function page(origin){
   const page=await context.newPage();page.on('pageerror',error=>errors.push(error.message))
   if(alias){
    await page.goto(origin+'/auth/login')
    await expect(page.getByRole('heading',{name:'Synthetic identity provider',exact:true})).toBeVisible()
    await page.getByLabel('Synthetic actor',{exact:true}).selectOption(alias)
    await page.getByRole('button',{name:'Continue through normal OIDC',exact:true}).click()
    await expect(page.getByRole('heading',{name:'Your events',exact:true})).toBeVisible()
   }
   return page
  }
  return {context,page}
 }
 async function read(page,path){
  return page.evaluate(async path=>{
   const response=await fetch(path,{credentials:'same-origin',cache:'no-store'})
   const body=await response.text()
   return {status:response.status,private_help:body.includes("PRIVATE synthetic request for help demonstrating the current team's work."),private_staff:body.includes('Synthetic mentor-a')}
  },path)
 }
 async function openStaff(eventID){
  await owner.goto(`${admin}/events/${eventID}?section=staff`)
  await expect(owner.getByRole('heading',{name:'Assign event staff',exact:true})).toBeVisible()
 }
 async function assign(eventID,person,role){
  await owner.getByRole('navigation',{name:'Staff administration sections',exact:true}).getByRole('button',{name:'Assign staff',exact:true}).click()
  await owner.getByLabel('Person account ID',{exact:true}).fill(person)
  await owner.getByLabel('Role',{exact:true}).selectOption(role)
  if(role==='organiser')await owner.getByRole('checkbox',{name:'Acknowledge the lasting competition restriction for organiser or early brief access. Active and waitlisted attendees must withdraw first.',exact:true}).check()
  const response=owner.waitForResponse(r=>new URL(r.url()).pathname===`/api/v1/events/${eventID}/staff`&&r.request().method()==='PUT')
  await owner.getByRole('button',{name:'Assign role',exact:true}).click()
  assert.equal((await response).status(),200)
  await expect(owner.getByLabel('Person account ID',{exact:true})).toBeEnabled()
  await expect(owner.getByRole('status').filter({hasText:'Role assigned.'})).toBeVisible()
 }
 for(let sequence=4;sequence<=5;sequence++){
  const phase=await readPhase(sequence,180000)
  assert.equal(phase.action,'assign-role-staff');assert.equal(phase.roles.grants.length,3)
  await owner.clock.setFixedTime(new Date(phase.domain_now));await openStaff(phase.event_id)
  for(const grant of phase.roles.grants)await assign(phase.event_id,grant.person_id,grant.role)
  await owner.screenshot({path:join(output,`live-staff-grants-${sequence}.png`),fullPage:true})
  await acknowledge({sequence,action:phase.action,event_id:phase.event_id,saved_by_ui:true})
 }
 for(let sequence=6;sequence<=17;sequence++){
  const phase=await readPhase(sequence,180000),entry=phase.roles.case
  assert.equal(phase.action,'check-live-role');assert.equal(entry.checks.length,2)
  const current=await actor(entry.actor),adminPage=await current.page(admin),publicPage=await current.page(publicOrigin)
  const writes=[]
  for(const page of [adminPage,publicPage])page.on('request',request=>{if(new URL(request.url()).pathname.startsWith('/api/v1/events/')&&!['GET','HEAD'].includes(request.method()))writes.push(request.method())})
  try{
   for(const check of entry.checks){
    await adminPage.clock.setFixedTime(new Date(phase.domain_now));await publicPage.clock.setFixedTime(new Date(phase.domain_now))
    await adminPage.goto(`${admin}/events/${check.event_id}?section=staff`)
    if(check.event_read){
     await expect(adminPage.getByRole('heading',{name:check.event_name,exact:true})).toBeVisible()
     const navigation=adminPage.getByRole('navigation',{name:'Event sections',exact:true})
     await expect(navigation.getByRole('button',{name:'Staff',exact:true})).toHaveCount(check.staff_read?1:0)
     await expect(navigation.getByRole('button',{name:'MC console',exact:true})).toHaveCount(check.present?1:0)
     await expect(navigation.getByRole('button',{name:'Logistics',exact:true})).toHaveCount(check.logistics?1:0)
     if(check.staff_read){
      await expect(adminPage.getByRole('heading',{name:'Assign event staff',exact:true})).toBeVisible()
      await expect(adminPage.getByRole('button',{name:'Transfer ownership',exact:true})).toHaveCount(check.owns_event?1:0)
      await adminPage.getByRole('navigation',{name:'Staff administration sections',exact:true}).getByRole('button',{name:'Assignments',exact:true}).click()
      await expect(adminPage.getByRole('heading',{name:'Current and previous assignments',exact:true})).toBeVisible()
      await expect(adminPage.getByText('Synthetic mentor-a',{exact:true}).first()).toBeVisible()
     }else await expect(adminPage.getByRole('heading',{name:'Assign event staff',exact:true})).toHaveCount(0)
    }else if(entry.actor){
     await expect(adminPage.getByRole('heading',{name:'Something needs attention',exact:true}).first()).toBeVisible()
     await expect(adminPage.getByRole('navigation',{name:'Event sections',exact:true})).toHaveCount(0)
    }else await expect(adminPage.getByRole('link',{name:'Sign in with your organisation',exact:true})).toBeVisible()
    const staff=await read(adminPage,`/api/v1/events/${check.event_id}/staff`)
    assert.equal(staff.status,entry.actor?(check.staff_read?200:403):401)
    if(!check.staff_read)assert.equal(staff.private_staff,false)
    if(check.present){
     await adminPage.getByRole('navigation',{name:'Event sections',exact:true}).getByRole('button',{name:'MC console',exact:true}).click()
     await expect(adminPage.getByRole('heading',{name:'MC console',exact:true})).toBeVisible()
    }
    if(check.logistics){
     await adminPage.getByRole('navigation',{name:'Event sections',exact:true}).getByRole('button',{name:'Logistics',exact:true}).click()
     await expect(adminPage.getByRole('heading',{name:'Restricted logistics',exact:true})).toBeVisible()
    }
    for(const [path,allowed] of [['presentation',check.present],['logistics',check.logistics]]){
     const response=await read(adminPage,`/api/v1/events/${check.event_id}/${path}`)
     assert.equal(response.status,entry.actor?(allowed?200:403):401)
    }
    await publicPage.goto(`${publicOrigin}/events/${check.event_id}/help/${check.help_id}`)
    if(check.help_read){
     await expect(publicPage.getByRole('region',{name:'Private request context',exact:true}).getByText(privateHelp,{exact:true})).toBeVisible()
    }else{
     if(entry.actor)await expect(publicPage.getByText('Current access to this private request is unavailable.',{exact:true})).toBeVisible()
     else await expect(publicPage.getByRole('link',{name:'Sign in with your organisation',exact:true})).toBeVisible()
     await expect(publicPage.getByRole('region',{name:'Private request context',exact:true})).toHaveCount(0)
     await expect(publicPage.getByText(privateHelp,{exact:true})).toHaveCount(0)
    }
    const help=await read(publicPage,`/api/v1/events/${check.event_id}/help/${check.help_id}`)
    assert.equal(help.status,entry.actor?(check.help_read?200:404):401)
    if(!check.help_read)assert.equal(help.private_help,false)
   }
   assert.equal(writes.length,0,'Role discovery and reads must not submit application writes')
   receipts.push({role:entry.name,events_checked:2,unexpected_writes:0})
   await publicPage.screenshot({path:join(output,`live-role-${entry.name}.png`),fullPage:true})
   await acknowledge({sequence,action:phase.action,event_id:phase.event_id,saved_by_ui:false})
  }finally{await current.context.close()}
 }
 const phase=await readPhase(18,180000),mentor=await actor('mentor-a')
 assert.equal(phase.action,'revoke-mentor-event')
 try{
  const held=await mentor.page(publicOrigin),peer=await mentor.page(publicOrigin)
  const resource=`/api/v1/events/${phase.event_id}/help/${phase.roles.help_id}`,otherResource=`/api/v1/events/${phase.roles.peer_event_id}/help/${phase.roles.peer_help_id}`
  await held.goto(publicOrigin+`/events/${phase.event_id}/help/${phase.roles.help_id}`)
  await peer.goto(publicOrigin+`/events/${phase.roles.peer_event_id}/help/${phase.roles.peer_help_id}`)
  for(const page of [held,peer])await expect(page.getByRole('region',{name:'Private request context',exact:true}).getByText(privateHelp,{exact:true})).toBeVisible()
  await openStaff(phase.event_id)
  await owner.getByRole('navigation',{name:'Staff administration sections',exact:true}).getByRole('button',{name:'Assignments',exact:true}).click()
  const revoked=owner.waitForResponse(r=>new URL(r.url()).pathname===`/api/v1/events/${phase.event_id}/staff`&&r.request().method()==='PUT')
  await owner.getByRole('button',{name:`Revoke mentor from ${phase.roles.mentor_name}`,exact:true}).click();assert.equal((await revoked).status(),200)
  await held.getByRole('button',{name:'Refresh request',exact:true}).click()
  await expect(held.getByText('Current access to this private request is unavailable.',{exact:true})).toBeVisible()
  await expect(held.getByRole('region',{name:'Private request context',exact:true})).toHaveCount(0)
  await expect(held.getByText(privateHelp,{exact:true})).toHaveCount(0)
  assert.equal((await read(held,resource)).status,404)
  await peer.getByRole('button',{name:'Refresh request',exact:true}).click()
  await expect(peer.getByRole('region',{name:'Private request context',exact:true}).getByText(privateHelp,{exact:true})).toBeVisible()
  assert.equal((await read(peer,otherResource)).status,200)
  await held.screenshot({path:join(output,'live-revoked-mentor-private-cleared.png'),fullPage:true})
  await peer.screenshot({path:join(output,'live-peer-event-mentor-retained.png'),fullPage:true})
  await assign(phase.event_id,phase.roles.mentor_id,'mentor')
  await held.getByRole('button',{name:'Refresh request',exact:true}).click()
  await expect(held.getByRole('region',{name:'Private request context',exact:true}).getByText(privateHelp,{exact:true})).toBeVisible()
  await acknowledge({sequence:18,action:phase.action,event_id:phase.event_id,saved_by_ui:true})
  return {stage:'live',staff_grants:6,cases:receipts,mentor_revocation_cleared:true,other_event_retained:true,mentor_restored:true}
 }finally{await mentor.context.close()}
}

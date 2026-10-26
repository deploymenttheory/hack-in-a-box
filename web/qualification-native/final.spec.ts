import {writeFileSync} from 'node:fs'
import {expect,test,type Page} from '@playwright/test'
import {native,owner,control,teamLabel} from './native'
import {activate,typeInto,selectNative,reach,inspectCurrent,openLocal} from './keyboard'
import {confirmReviewedDialog} from './confirmation'
import type {QualificationPage} from '../src/qualification-model'
async function accept(page:Page,reason:string){
 await typeInto(page.getByLabel('Recorded panel reason',{exact:true}),reason)
 const submit=page.getByRole('button',{name:'Review entered panel decision',exact:true});await openLocal(submit,page.getByRole('alertdialog',{name:'Review private panel command',exact:true}),'visible')
 const dialog=page.getByRole('alertdialog',{name:'Review private panel command',exact:true})
 await expect(dialog).toContainText(reason)
 await confirmReviewedDialog(dialog,`/api/v1/events/${native.event_id}/mentor-qualification/${native.run_id}`)
}
test('real five-team evidence, final cards, split tie ballots, casting signoff and separate preparation',async({browser})=>{
 test.skip(native.scenario!=='final','Separate initial no-final journey')
 test.setTimeout(180_000)
 const context=await owner(browser),page=await context.newPage()
 page.on('request',request=>{
  if(request.method()!=='POST'||!request.url().endsWith(`/mentor-qualification/${native.run_id}`))return
  const body=request.postDataJSON() as {request_id:string;action:string}
  writeFileSync(native.command_metadata_path,JSON.stringify({request_id:body.request_id,action:body.action}),{mode:0o600})
 })
 await page.goto(`/events/${native.event_id}/panel/${native.run_id}`);await page.bringToFront()
 await openLocal(page.getByRole('button',{name:'Configure intended panel',exact:true}),page.getByLabel('Recorded panel reason',{exact:true}),'visible')
 await selectNative(page.getByLabel('Organising lead',{exact:true}),native.owner_id)
 for(const name of ['Native primary mentor','Native independent second mentor'])await activate(page.getByRole('checkbox',{name,exact:true}),'Space')
 await accept(page,'Real native intended five-team panel')
 const values=[[10,9,10,9,9],[9,8,9,8,8],[8,7,8,7,7],[7,6,7,6,6],[1,10,10,1,1]]
 for(const [index,team] of native.team_ids.entries()){
  await inspectCurrent(page.getByRole('button',{name:`Inspect ${teamLabel(team)}`,exact:true}),page.getByRole('heading',{name:teamLabel(team),exact:true,level:2}))
  await expect(page.getByRole('heading',{name:teamLabel(team),exact:true,level:2})).toBeFocused()
  const source=page.getByRole('region',{name:'Independent original final evidence',exact:true})
  await expect(source).toBeVisible();await expect(source.getByText('Native independent final review evidence',{exact:true})).toHaveCount(10)
  await openLocal(page.getByRole('button',{name:'Edit reasoned final card',exact:true}),page.getByLabel('Recorded panel reason',{exact:true}),'visible')
  for(const [position,category] of native.categories.entries()){
   await activate(page.getByRole('checkbox',{name:`Pending/unobserved for ${category.label}`,exact:true}),'Space')
   await typeInto(page.getByLabel(`Moderated score for ${category.label}`,{exact:true}),String(values[index][position]))
   await typeInto(page.getByLabel(`Moderated evidence for ${category.label}`,{exact:true}),`Team ${index+1}: actual independent ${category.label} evidence <trace> & observation`)
  }
  await accept(page,`Actual final card ${index+1}, preserving native evidence trace`)
  await expect(page.getByRole('region',{name:'Moderated final card',exact:true})).toContainText('Version 1 · draft')
  const confirm=page.getByRole('button',{name:'Review saved final card confirmation',exact:true});await expect(confirm).toBeFocused()
  await expect(page.getByRole('status').filter({hasText:'Original save card acknowledged for request'})).toBeVisible()
  if(index===0){await expect(source.getByText('Native independent final review evidence',{exact:true})).toHaveCount(10);await page.locator('[data-private-qualification]').screenshot({path:test.info().outputPath('current-independent-and-moderated-card.png'),animations:'disabled'})}
  await openLocal(confirm,page.getByLabel('Recorded panel reason',{exact:true}));await accept(page,`Confirm exact reviewed final card ${index+1}`)
  await expect(page.getByRole('region',{name:'Moderated final card',exact:true})).toContainText('Version 2 · signed')
  await expect(page.getByRole('button',{name:'Edit reasoned final card',exact:true})).toHaveCount(0)
  const nextTeam=page.getByRole('button',{name:'Review another team',exact:true});await expect(nextTeam).toBeFocused();await openLocal(nextTeam,page.getByRole('button',{name:`Inspect ${teamLabel(team)}`,exact:true}))
  await expect(page.getByRole('button',{name:`Inspect ${teamLabel(team)}`,exact:true})).toBeFocused()
  if(index===0){await inspectCurrent(page.getByRole('button',{name:`Inspect ${teamLabel(team)}`,exact:true}),page.getByRole('heading',{name:teamLabel(team),exact:true,level:2}));await expect(page.getByRole('heading',{name:teamLabel(team),exact:true,level:2})).toBeFocused();await activate(page.getByRole('link',{name:'Back to panel',exact:true}));await expect(page.getByRole('button',{name:`Inspect ${teamLabel(team)}`,exact:true})).toBeFocused()}
 }
 let proof=await control<{cards:number;current:boolean;signoff:boolean;page:QualificationPage}>('evidence');expect(proof).toMatchObject({cards:5,current:false,signoff:false})
 expect(proof.page.round.cards.every(card=>card.state==='signed')).toBe(true)
 await openLocal(page.getByRole('button',{name:'Review current selection proposal',exact:true}),page.getByLabel('Recorded panel reason',{exact:true}),'visible');await accept(page,'Review all five signed native finals, including non-finalist category evidence')
 await expect(page.getByRole('region',{name:'Affected tie: UX · Seat 1',exact:true})).toContainText('Required current ballots incomplete')
 await expect(page.getByRole('button',{name:'Review exact panel signoff',exact:true})).toHaveCount(0)
 for(const [actor,candidate] of [['primary',native.team_ids[0]],['second',native.team_ids[4]]] as const){
  const voterContext=await owner(browser,actor),voter=await voterContext.newPage();await voter.goto(`/events/${native.event_id}/panel/${native.run_id}`);await voter.bringToFront()
  await openLocal(voter.getByRole('button',{name:'Record ballot: UX · Seat 1',exact:true}),voter.getByLabel('Candidate for affected tie seat',{exact:true}))
  await selectNative(voter.getByLabel('Candidate for affected tie seat',{exact:true}),candidate)
  await typeInto(voter.getByLabel('Supporting observed evidence',{exact:true}),`Independent ${actor} ballot grounded in the actual UX comparison`)
  await accept(voter,`Independent ${actor} reasoned UX ballot`);await voterContext.close()
 }
 await page.bringToFront();await openLocal(page.getByRole('button',{name:'Read current panel source',exact:true}),page.getByRole('heading',{name:'Moderation and selections',exact:true}),'visible')
 const tie=page.getByRole('region',{name:'Affected tie: UX · Seat 1',exact:true});await expect(tie).toContainText('Current ballots complete');await expect(tie).toContainText('Casting decision required')
 await openLocal(page.getByRole('button',{name:'Review casting tie decision: UX · Seat 1',exact:true}),page.getByLabel('Recorded panel reason',{exact:true}),'visible')
 await selectNative(page.getByLabel('Candidate for affected tie seat',{exact:true}),native.team_ids[4])
 await typeInto(page.getByLabel('Supporting observed evidence',{exact:true}),'Compare both independent UX observations; retain the non-finalist category candidate')
 await accept(page,'Recorded organiser casting reason after both independent mentor ballots')
 await openLocal(page.getByRole('button',{name:'Review exact panel signoff',exact:true}),page.getByLabel('Recorded panel reason',{exact:true}),'visible')
 await typeInto(page.getByLabel('Recorded panel reason',{exact:true}),'Exact versioned private panel approval after all native blockers are resolved')
 const submitSignoff=page.getByRole('button',{name:'Review entered panel decision',exact:true});await openLocal(submitSignoff,page.getByRole('alertdialog',{name:'Review private panel command',exact:true}),'visible')
 const review=page.getByRole('alertdialog',{name:'Review private panel command',exact:true});await expect(review).toContainText('Proposed finalists');await expect(review).toContainText('Reserve');await expect(review).toContainText('All eligible teams remain category candidates')
 await confirmReviewedDialog(review,`/api/v1/events/${native.event_id}/mentor-qualification/${native.run_id}`)
 await expect(page.getByText('Current private panel signoff',{exact:true})).toBeVisible()
 await expect(page.getByText(/ties require evidence review and recorded panel decisions/)).toHaveCount(0)
 proof=await control<typeof proof>('evidence');expect(proof).toMatchObject({cards:5,current:true,signoff:true})
 expect(proof.page.preview!.finalist_ids).toEqual(native.team_ids.slice(0,3));expect(proof.page.preview!.reserve_ids).toEqual([native.team_ids[3]])
 for(const label of ['Technical approach','UX'])expect(proof.page.preview!.categories.find(value=>value.category_id===native.categories.find(category=>category.label===label)!.id)!.team_ids).toEqual([native.team_ids[4]])
 await openLocal(page.getByRole('button',{name:'Preview finalist preparation',exact:true}),page.getByRole('region',{name:'Private finalist preparation',exact:true}),'visible')
 const finalists=page.getByRole('region',{name:'Private finalist preparation',exact:true});await expect(finalists).toBeVisible();await expect(finalists).not.toContainText('Technical approach');await expect(finalists).not.toContainText(teamLabel(native.team_ids[4]))
 await expect(page.getByRole('region',{name:'Private category preparation',exact:true})).toHaveCount(0)
 await openLocal(page.getByRole('button',{name:'Preview category preparation',exact:true}),page.getByRole('region',{name:'Private category preparation',exact:true}),'visible')
 await expect(page.getByRole('region',{name:'Private finalist preparation',exact:true})).toHaveCount(0);await expect(page.getByRole('region',{name:'Private category preparation',exact:true})).toContainText(teamLabel(native.team_ids[4]))
 await expect(page.getByRole('button',{name:/^(Publish|Send|Release) /})).toHaveCount(0)
 await page.emulateMedia({colorScheme:'light'});const light=await page.locator('.theme').first().evaluate(element=>getComputedStyle(element).getPropertyValue('--background'))
 await page.screenshot({path:test.info().outputPath('signed-private-preparation-light.png'),fullPage:true,animations:'disabled'})
 await page.emulateMedia({colorScheme:'dark'});await expect.poll(()=>page.locator('.theme').first().evaluate(element=>getComputedStyle(element).getPropertyValue('--background'))).not.toBe(light)
 await page.screenshot({path:test.info().outputPath('signed-private-preparation-dark.png'),fullPage:true,animations:'disabled'})
 await page.emulateMedia({colorScheme:'light'})


 await openLocal(page.getByRole('button',{name:'Read recorded panel history',exact:true}),page.getByRole('region',{name:'Currently authorised panel history',exact:true}),'visible')
 const history=page.getByRole('region',{name:'Currently authorised panel history',exact:true})
 await openLocal(history.getByText(new RegExp(`Panel version ${proof.page.round.version} · sign off`)),history.getByText('Recorded private signoff reason: Exact versioned private panel approval after all native blockers are resolved. Current authority and source freshness must be read independently.',{exact:true}),'visible')
 await expect(history.getByText('Recorded private signoff reason: Exact versioned private panel approval after all native blockers are resolved. Current authority and source freshness must be read independently.',{exact:true})).toBeVisible()
 await control('retire');await openLocal(page.getByRole('button',{name:'Read current panel source',exact:true}),page.getByRole('heading',{name:'Moderation and selections',exact:true}),'visible')
 await expect(history).toHaveCount(0)
 await expect(page.getByRole('region',{name:'Private category preparation',exact:true})).toHaveCount(0)
 await expect(page.getByText('Withdrawn entry · excluded from scoring',{exact:true})).toBeVisible();await expect(page.getByText('Disbanded team · repository unused; excluded from scoring',{exact:true})).toBeVisible()
 for(const team of native.team_ids.slice(0,2)){
  await inspectCurrent(page.getByRole('button',{name:`Inspect ${teamLabel(team)}`,exact:true}),page.getByRole('heading',{name:teamLabel(team),exact:true,level:2}))
  await expect(page.getByRole('region',{name:'Moderated final card',exact:true})).toContainText('No current moderated card is disclosed. Missing evidence is not a zero score.')
  await expect(page.getByRole('button',{name:'Edit reasoned final card',exact:true})).toHaveCount(0);await expect(page.getByRole('button',{name:'Review saved final card confirmation',exact:true})).toHaveCount(0)
  await expect(page.getByText('UNUSED-DISBANDED-REPOSITORY-CANARY',{exact:false})).toHaveCount(0)
  await activate(page.getByRole('link',{name:'Back to panel',exact:true}));await expect(page.getByRole('button',{name:`Inspect ${teamLabel(team)}`,exact:true})).toBeFocused()
 }
 proof=await control<typeof proof>('evidence');expect(proof.current).toBe(false);for(const team of proof.page.teams.filter(team=>native.team_ids.slice(0,2).includes(team.team_id))){expect(team.card_source_current).toBe(false);expect(team.can_save_card).toBe(false);expect(team.can_sign_card).toBe(false)}


 await openLocal(page.getByRole('button',{name:'Read recorded panel history',exact:true}),page.getByRole('region',{name:'Currently authorised panel history',exact:true}),'visible')
 await openLocal(history.getByText(new RegExp(`Panel version ${proof.page.round.version} · sign off`)),history.getByText('Recorded private signoff reason: Exact versioned private panel approval after all native blockers are resolved. Current authority and source freshness must be read independently.',{exact:true}),'visible')
 await expect(history.getByText(new RegExp(`Panel version ${proof.page.round.version} · sign off`)).locator('..').getByText('Recorded reason: Confirm exact reviewed final card 3',{exact:true})).toBeVisible()

 // Start from a real available keyboard action, then let the actual native
 // request observe a revoked synthetic session. A periodic request may win the
 // same race; both must dispose the private history on the real denial.
 const currentSource=page.getByRole('button',{name:'Read current panel source',exact:true});await reach(currentSource)
 const denied=page.waitForResponse(response=>{const path=new URL(response.url()).pathname;return response.status()===401&&path.startsWith(`/api/v1/events/${native.event_id}`)||response.status()===403&&path===`/api/v1/events/${native.event_id}/mentor-qualification/${native.run_id}`})
 let revoked=false
 await page.route(`**/api/v1/events/${native.event_id}/mentor-qualification/${native.run_id}`,async route=>{
  if(route.request().method()==='GET'&&!revoked){revoked=true;await control('expire')}
  await route.continue()
 })
 await page.keyboard.press('Enter');await denied

 await expect(history).toHaveCount(0);await expect(page.getByRole('heading',{name:'Moderation and selections',exact:true})).toHaveCount(0)
 expect(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth)).toBe(false)
 await context.close()
})

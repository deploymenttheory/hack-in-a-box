import {writeFileSync} from 'node:fs'
import {expect,test,type Page,type Locator} from '@playwright/test'
import {native,owner,control,teamLabel} from './native'
import type {QualificationPage} from '../src/qualification-model'
async function activate(control:Locator){await expect(control).toBeEnabled();await control.press('Enter')}
async function accept(page:Page,reason:string){
 await page.getByLabel('Recorded panel reason',{exact:true}).fill(reason)
 const submit=page.getByRole('button',{name:'Review entered panel decision',exact:true});await expect(submit).toBeEnabled();await submit.press('Enter')
 const dialog=page.getByRole('alertdialog',{name:'Review private panel command',exact:true})
 await expect(dialog).toContainText(reason)
 const confirm=dialog.getByRole('button',{name:'Confirm reviewed panel command',exact:true});await expect(confirm).toBeEnabled();await confirm.press('Enter')
 await expect(dialog).toHaveCount(0)
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
 await page.goto(`/events/${native.event_id}/panel/${native.run_id}`)
 await activate(page.getByRole('button',{name:'Configure intended panel',exact:true}))
 await page.getByLabel('Organising lead',{exact:true}).selectOption(native.owner_id)
 for(const name of ['Native primary mentor','Native independent second mentor'])await page.getByRole('checkbox',{name,exact:true}).press('Space')
 await accept(page,'Real native intended five-team panel')
 const values=[[10,9,10,9,9],[9,8,9,8,8],[8,7,8,7,7],[7,6,7,6,6],[1,10,10,1,1]]
 for(const [index,team] of native.team_ids.entries()){
  await activate(page.getByRole('button',{name:`Inspect ${teamLabel(team)}`,exact:true}))
  const source=page.getByRole('region',{name:'Independent original final evidence',exact:true})
  await expect(source).toBeVisible();await expect(source.getByText('Native independent final review evidence',{exact:true})).toHaveCount(10)
  await activate(page.getByRole('button',{name:'Edit reasoned final card',exact:true}))
  for(const [position,category] of native.categories.entries()){
   await page.getByRole('checkbox',{name:`Pending/unobserved for ${category.label}`,exact:true}).press('Space')
   await page.getByLabel(`Moderated score for ${category.label}`,{exact:true}).fill(String(values[index][position]))
   await page.getByLabel(`Moderated evidence for ${category.label}`,{exact:true}).fill(`Team ${index+1}: actual independent ${category.label} evidence <trace> & observation`)
  }
  await accept(page,`Actual final card ${index+1}, preserving native evidence trace`)
  await expect(page.getByRole('region',{name:'Moderated final card',exact:true})).toContainText('Version 1 · draft')
  const confirm=page.getByRole('button',{name:'Review saved final card confirmation',exact:true});await expect(confirm).toBeFocused()
  await expect(page.getByRole('status').filter({hasText:'Original save card acknowledged for request'})).toBeVisible()
  if(index===0)await page.locator('[data-private-qualification]').screenshot({path:test.info().outputPath('current-independent-and-moderated-card.png'),animations:'disabled'})
  await expect(confirm).toBeEnabled();await confirm.press('Enter');await accept(page,`Confirm exact reviewed final card ${index+1}`)
  await expect(page.getByRole('region',{name:'Moderated final card',exact:true})).toContainText('Version 2 · signed')
  await expect(page.getByRole('button',{name:'Edit reasoned final card',exact:true})).toHaveCount(0)
  await page.getByRole('link',{name:'Back to panel',exact:true}).press('Enter')
  await expect(page.getByRole('button',{name:`Inspect ${teamLabel(team)}`,exact:true})).toBeFocused()
 }
 let proof=await control<{cards:number;current:boolean;signoff:boolean;page:QualificationPage}>('evidence');expect(proof).toMatchObject({cards:5,current:false,signoff:false})
 expect(proof.page.round.cards.every(card=>card.state==='signed')).toBe(true)
 await activate(page.getByRole('button',{name:'Review current selection proposal',exact:true}));await accept(page,'Review all five signed native finals, including non-finalist category evidence')
 await expect(page.getByRole('region',{name:'Affected tie: UX · Seat 1',exact:true})).toContainText('Required current ballots incomplete')
 await expect(page.getByRole('button',{name:'Review exact panel signoff',exact:true})).toHaveCount(0)
 for(const [actor,candidate] of [['primary',native.team_ids[0]],['second',native.team_ids[4]]] as const){
  const voterContext=await owner(browser,actor),voter=await voterContext.newPage();await voter.goto(`/events/${native.event_id}/panel/${native.run_id}`)
  await activate(voter.getByRole('button',{name:'Record ballot: UX · Seat 1',exact:true}))
  await voter.getByLabel('Candidate for affected tie seat',{exact:true}).selectOption(candidate)
  await voter.getByLabel('Supporting observed evidence',{exact:true}).fill(`Independent ${actor} ballot grounded in the actual UX comparison`)
  await accept(voter,`Independent ${actor} reasoned UX ballot`);await voterContext.close()
 }
 await activate(page.getByRole('button',{name:'Read current panel source',exact:true}))
 const tie=page.getByRole('region',{name:'Affected tie: UX · Seat 1',exact:true});await expect(tie).toContainText('Current ballots complete');await expect(tie).toContainText('Casting decision required')
 await activate(page.getByRole('button',{name:'Review casting tie decision: UX · Seat 1',exact:true}))
 await page.getByLabel('Candidate for affected tie seat',{exact:true}).selectOption(native.team_ids[4])
 await page.getByLabel('Supporting observed evidence',{exact:true}).fill('Compare both independent UX observations; retain the non-finalist category candidate')
 await accept(page,'Recorded organiser casting reason after both independent mentor ballots')
 await activate(page.getByRole('button',{name:'Review exact panel signoff',exact:true}))
 await page.getByLabel('Recorded panel reason',{exact:true}).fill('Exact versioned private panel approval after all native blockers are resolved')
 const submitSignoff=page.getByRole('button',{name:'Review entered panel decision',exact:true});await expect(submitSignoff).toBeEnabled();await submitSignoff.press('Enter')
 const review=page.getByRole('alertdialog',{name:'Review private panel command',exact:true});await expect(review).toContainText('Proposed finalists');await expect(review).toContainText('Reserve');await expect(review).toContainText('All eligible teams remain category candidates')
 await activate(review.getByRole('button',{name:'Confirm reviewed panel command',exact:true}))
 await expect(page.getByText('Current private panel signoff',{exact:true})).toBeVisible()
 proof=await control<typeof proof>('evidence');expect(proof).toMatchObject({cards:5,current:true,signoff:true})
 expect(proof.page.preview!.finalist_ids).toEqual(native.team_ids.slice(0,3));expect(proof.page.preview!.reserve_ids).toEqual([native.team_ids[3]])
 for(const label of ['Technical approach','UX'])expect(proof.page.preview!.categories.find(value=>value.category_id===native.categories.find(category=>category.label===label)!.id)!.team_ids).toEqual([native.team_ids[4]])
 await activate(page.getByRole('button',{name:'Preview finalist preparation',exact:true}))
 const finalists=page.getByRole('region',{name:'Private finalist preparation',exact:true});await expect(finalists).toBeVisible();await expect(finalists).not.toContainText('Technical approach');await expect(finalists).not.toContainText(teamLabel(native.team_ids[4]))
 await expect(page.getByRole('region',{name:'Private category preparation',exact:true})).toHaveCount(0)
 await activate(page.getByRole('button',{name:'Preview category preparation',exact:true}))
 await expect(page.getByRole('region',{name:'Private finalist preparation',exact:true})).toHaveCount(0);await expect(page.getByRole('region',{name:'Private category preparation',exact:true})).toContainText(teamLabel(native.team_ids[4]))
 await expect(page.getByRole('button',{name:/^(Publish|Send|Release) /})).toHaveCount(0)
 await page.emulateMedia({colorScheme:'light'});const light=await page.locator('.theme').first().evaluate(element=>getComputedStyle(element).getPropertyValue('--background'))
 await page.screenshot({path:test.info().outputPath('signed-private-preparation-light.png'),fullPage:true,animations:'disabled'})
 await page.emulateMedia({colorScheme:'dark'});await expect.poll(()=>page.locator('.theme').first().evaluate(element=>getComputedStyle(element).getPropertyValue('--background'))).not.toBe(light)
 await page.screenshot({path:test.info().outputPath('signed-private-preparation-dark.png'),fullPage:true,animations:'disabled'})
 await page.emulateMedia({colorScheme:'light'})


 await activate(page.getByRole('button',{name:'Read recorded panel history',exact:true}))
 const history=page.getByRole('region',{name:'Currently authorised panel history',exact:true})
 await history.getByText(new RegExp(`Panel version ${proof.page.round.version} · sign off`)).press('Enter')
 await expect(history.getByText('Recorded private signoff reason: Exact versioned private panel approval after all native blockers are resolved. Current authority and source freshness must be read independently.',{exact:true})).toBeVisible()
 await control('retire');await activate(page.getByRole('button',{name:'Read current panel source',exact:true}))
 await expect(history).toHaveCount(0)
 await expect(page.getByRole('region',{name:'Private category preparation',exact:true})).toHaveCount(0)
 await expect(page.getByText('Withdrawn entry · excluded from scoring',{exact:true})).toBeVisible();await expect(page.getByText('Disbanded team · repository unused; excluded from scoring',{exact:true})).toBeVisible()
 for(const team of native.team_ids.slice(0,2)){
  await activate(page.getByRole('button',{name:`Inspect ${teamLabel(team)}`,exact:true}))
  await expect(page.getByRole('region',{name:'Moderated final card',exact:true})).toContainText('No current moderated card is disclosed. Missing evidence is not a zero score.')
  await expect(page.getByRole('button',{name:'Edit reasoned final card',exact:true})).toHaveCount(0);await expect(page.getByRole('button',{name:'Review saved final card confirmation',exact:true})).toHaveCount(0)
  await expect(page.getByText('UNUSED-DISBANDED-REPOSITORY-CANARY',{exact:false})).toHaveCount(0)
  await page.getByRole('link',{name:'Back to panel',exact:true}).press('Enter');await expect(page.getByRole('button',{name:`Inspect ${teamLabel(team)}`,exact:true})).toBeFocused()
 }
 proof=await control<typeof proof>('evidence');expect(proof.current).toBe(false);for(const team of proof.page.teams.filter(team=>native.team_ids.slice(0,2).includes(team.team_id))){expect(team.card_source_current).toBe(false);expect(team.can_save_card).toBe(false);expect(team.can_sign_card).toBe(false)}


 await activate(page.getByRole('button',{name:'Read recorded panel history',exact:true}))
 await history.getByText(new RegExp(`Panel version ${proof.page.round.version} · sign off`)).press('Enter')
 await expect(history.getByText('Recorded reason: Confirm exact reviewed final card 3',{exact:true})).toBeVisible()
 await control('expire');await activate(page.getByRole('button',{name:'Read current panel source',exact:true}))
 await expect(history).toHaveCount(0);await expect(page.getByRole('heading',{name:'Moderation and selections',exact:true})).toHaveCount(0)
 expect(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth)).toBe(false)
 await context.close()
})

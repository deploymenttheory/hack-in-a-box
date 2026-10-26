import {expect,test} from '@playwright/test'
import {native,owner,control,teamLabel} from './native'
import {openLocal} from './keyboard'
test('actual initial native panel captures exact lost ACK, retains blockers/history and clears on real session expiry',async({browser})=>{
 test.skip(native.scenario!=='initial','Separate real final cohort journey')
 const context=await owner(browser),page=await context.newPage(),errors:string[]=[];page.on('pageerror',error=>errors.push(error.message))
 await page.goto(`/events/${native.event_id}/panel`);await page.getByRole('link',{name:`Open ${native.run_name}`,exact:true}).press('Enter')
 await openLocal(page.getByRole('button',{name:'Configure intended panel',exact:true}),page.getByLabel('Recorded panel reason',{exact:true}),'visible')
 await page.getByLabel('Organising lead',{exact:true}).selectOption(native.owner_id)
 await page.getByRole('checkbox',{name:native.mentor_name,exact:true}).press('Space')
 await page.getByLabel('Recorded panel reason',{exact:true}).fill('Actual native <panel> & reason\u2028\u2029😀')
 const submit=page.getByRole('button',{name:'Review entered panel decision',exact:true});await expect(submit).toBeEnabled();await openLocal(submit,page.getByRole('alertdialog',{name:'Review private panel command',exact:true}),'visible')
 const review=page.getByRole('alertdialog',{name:'Review private panel command',exact:true});await expect(review).toContainText('Panel version 0')
 let lost=false
 await page.route(`**/api/v1/events/${native.event_id}/mentor-qualification/${native.run_id}`,async route=>{if(route.request().method()==='POST'&&!lost){lost=true;const response=await route.fetch();expect(response.ok()).toBe(true);return route.abort('failed')}return route.continue()})
 await review.getByRole('button',{name:'Confirm reviewed panel command',exact:true}).press('Enter')
 await expect(page.getByRole('heading',{name:'Original panel outcome unknown',exact:true})).toBeVisible()
 // Disclosed roster labels hydrate independently of this unknown attempt.
 // Observe them before capturing a body-scoped original-receipt control.
 await expect(page.getByRole('button',{name:`Inspect ${teamLabel(native.team_id)}`,exact:true})).toBeVisible()
 await expect(page.getByLabel('Original panel command history',{exact:true})).toContainText('Outcome unknown')
 await openLocal(page.getByRole('button',{name:'Read original panel acknowledgement',exact:true}),page.getByText('Original configure panel acknowledged at panel version 1. Current panel state must be read separately.',{exact:true}),'visible')
 await expect(page.getByText('Original configure panel acknowledged at panel version 1. Current panel state must be read separately.',{exact:true})).toBeVisible()
 await openLocal(page.getByRole('button',{name:'Read current panel source',exact:true}),page.getByRole('heading',{name:'Moderation and selections',exact:true}),'visible')
 await expect(page.getByRole('button',{name:`Inspect ${teamLabel(native.team_id)}`,exact:true})).toBeVisible()
 await expect(page.getByText('Not submitted · no formal final to score',{exact:true})).toBeVisible()
 await expect(page.getByRole('button',{name:'Edit reasoned final card',exact:true})).toHaveCount(0)
 await expect(page.getByRole('button',{name:'Review exact panel signoff',exact:true})).toHaveCount(0)
 await openLocal(page.getByRole('button',{name:'Read recorded panel history',exact:true}),page.getByRole('region',{name:'Currently authorised panel history',exact:true}),'visible')
 await expect(page.getByRole('region',{name:'Currently authorised panel history',exact:true})).toContainText('Panel version 1')
 await openLocal(page.getByText(/Panel version 1 · configure panel/),page.getByText('Recorded reason: Actual native <panel> & reason\u2028\u2029😀',{exact:true}),'visible')
 await expect(page.getByText('Recorded reason: Actual native <panel> & reason\u2028\u2029😀',{exact:true})).toBeVisible()
 const proof=await control<{version:number;cards:number;current:boolean;signoff:boolean}>('evidence');expect(proof).toMatchObject({version:1,cards:0,current:false,signoff:false})
 expect(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth)).toBe(false)
 await control('expire');await page.getByRole('button',{name:'Read current panel source',exact:true}).press('Enter')
 await expect(page.getByText('Recorded reason: Actual native <panel> & reason\u2028\u2029😀',{exact:true})).toHaveCount(0)
 await expect(page.getByRole('heading',{name:'Moderation and selections',exact:true})).toHaveCount(0)
 expect(errors).toEqual([]);await context.close()
})

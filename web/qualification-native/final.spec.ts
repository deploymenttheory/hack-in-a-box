import {expect,test,type Page} from '@playwright/test'
import {native,owner,control} from './native'
async function accept(page:Page,reason:string){
 await page.getByLabel('Recorded panel reason',{exact:true}).fill(reason)
 await page.getByRole('button',{name:'Review entered panel decision',exact:true}).press('Enter')
 const dialog=page.getByRole('alertdialog',{name:'Review private panel command',exact:true})
 await expect(dialog).toContainText(reason)
 await dialog.getByRole('button',{name:'Confirm reviewed panel command',exact:true}).press('Enter')
 await expect(dialog).toHaveCount(0)
}
test('real five-team final cards retain exact outcome and next confirmation action',async({browser})=>{
 test.skip(native.scenario!=='final','Separate initial no-final journey')
 const context=await owner(browser),page=await context.newPage()
 await page.goto(`/events/${native.event_id}/panel/${native.run_id}`)
 await page.getByRole('button',{name:'Configure intended panel',exact:true}).press('Enter')
 await page.getByLabel('Organising lead',{exact:true}).selectOption(native.owner_id)
 for(const name of ['Native primary mentor','Native independent second mentor'])await page.getByRole('checkbox',{name,exact:true}).press('Space')
 await accept(page,'Real native intended five-team panel')
 const team=native.team_ids[0]
 await page.getByRole('button',{name:`Inspect Team ${team}`,exact:true}).press('Enter')
 await page.getByRole('button',{name:'Edit reasoned final card',exact:true}).press('Enter')
 for(const category of native.categories){
  await page.getByRole('checkbox',{name:`Pending/unobserved for ${category.label}`,exact:true}).press('Space')
  await page.getByLabel(`Moderated score for ${category.label}`,{exact:true}).fill('6')
  await page.getByLabel(`Moderated evidence for ${category.label}`,{exact:true}).fill('Actual reviewed independent source, final observation <trace> & evidence')
 }
 await accept(page,'Actual first final card, preserving native evidence trace')
 await expect(page.getByRole('region',{name:'Moderated final card',exact:true})).toContainText('Version 1 · draft')
 await expect(page.getByRole('button',{name:'Review saved final card confirmation',exact:true})).toBeVisible()
 await expect(page.getByRole('status').filter({hasText:'Original save card acknowledged for request'})).toBeVisible()
 const proof=await control<{cards:number;current:boolean;signoff:boolean}>('evidence');expect(proof).toMatchObject({cards:1,current:false,signoff:false})
 expect(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth)).toBe(false)
 await context.close()
})

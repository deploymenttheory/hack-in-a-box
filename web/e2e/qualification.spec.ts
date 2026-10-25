import {test,expect} from '@playwright/test'
import {mockWorkspace,firstID} from './fixtures'
import {qualificationGuidePage} from '../src/styleguide/qualification-fixture'
const initial=qualificationGuidePage(),run=initial.round.run_id,path=`/events/${firstID}/panel/${run}`
async function fixture(page:import('@playwright/test').Page){
 await mockWorkspace(page)
 const state={page:structuredClone(initial),denied:false}
 await page.route(`**/api/v1/events/${firstID}/mentor-qualification/${run}`,route=>state.denied?route.fulfill({status:403,json:{error:{code:'forbidden',message:'Synthetic current scope denied',request_id:'synthetic'}}}):route.fulfill({json:state.page}))
 await page.route('**/moderation-huddles/*',route=>route.fulfill({status:403,json:{error:{code:'forbidden',message:'Synthetic source-only denial',request_id:'synthetic'}}}))
 return state
}
test('native panel reader separates source-specific authority and uses organiser build destination',async({page})=>{
 await fixture(page);await page.goto(path);await expect(page.getByRole('heading',{name:'Moderation and selections',exact:true})).toBeVisible()
 const team=initial.teams[0].team_id;await page.getByRole('button',{name:`Inspect Team ${team}`,exact:true}).click()
 await expect(page.getByText('This source is not disclosed under current source-specific authority. A panel selection does not grant private huddle access.',{exact:true})).toBeVisible()
 await expect(page.getByRole('link',{name:'Inspect formal build and frozen code',exact:true})).toHaveAttribute('href',`/events/${firstID}/teams/${team}/build`)
 await expect(page.getByRole('region',{name:'Moderated final card',exact:true})).toContainText('Synthetic recorded moderation reason')
 expect(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth)).toBe(false)
})
test('finalist preparation is private and independent from category preview',async({page})=>{
 await fixture(page);await page.goto(path)
 await page.getByRole('button',{name:'Preview finalist preparation',exact:true}).click()
 const preview=page.getByRole('region',{name:'Private finalist preparation',exact:true});await expect(preview).toBeVisible();await expect(preview).not.toContainText('User experience');await expect(page.getByRole('region',{name:'Private category preparation',exact:true})).toHaveCount(0)
 await page.getByRole('button',{name:'Preview category preparation',exact:true}).click();await expect(page.getByRole('region',{name:'Private finalist preparation',exact:true})).toHaveCount(0);await expect(page.getByRole('region',{name:'Private category preparation',exact:true})).toContainText('User experience')
 await expect(page.getByRole('button',{name:/^(Publish|Send|Release) /})).toHaveCount(0)
})
test('changed signed source removes old private preparation and selected evidence',async({page})=>{
 const state=await fixture(page);await page.goto(path);await page.getByRole('button',{name:`Inspect Team ${initial.teams[0].team_id}`,exact:true}).click();await page.getByRole('button',{name:'Preview finalist preparation',exact:true}).click()
 state.page.current=false;state.page.round.signoff=null;state.page.preview!.blockers=['unresolved_tie:synthetic-finalist-seat'];await page.evaluate(()=>dispatchEvent(new Event('focus')))
 await expect(page.getByRole('region',{name:'Private finalist preparation',exact:true})).toHaveCount(0);await expect(page.getByRole('region',{name:'Moderated final card',exact:true})).toHaveCount(0);await expect(page.getByText(/unresolved tie:synthetic-finalist-seat/).first()).toBeVisible()
})
test('current native denial clears private panel data and requires an explicit fresh read',async({page})=>{
 const state=await fixture(page);await page.goto(path);await page.getByRole('button',{name:'Preview category preparation',exact:true}).click();state.denied=true;await page.evaluate(()=>dispatchEvent(new Event('focus')))
 await expect(page.getByText('Current panel access is unavailable. Private selections, evidence and earlier review context have been cleared.',{exact:true})).toBeVisible();await expect(page.getByRole('heading',{name:'Moderation and selections',exact:true})).toHaveCount(0);await expect(page.getByRole('region',{name:'Private category preparation',exact:true})).toHaveCount(0)
 state.denied=false;state.page.round.cards=[];state.page.round.signoff=null;state.page.current=false;await page.getByRole('button',{name:'Check current panel access',exact:true}).click();await expect(page.getByRole('heading',{name:'Moderation and selections',exact:true})).toBeVisible();await expect(page.getByText('Synthetic recorded moderation reason',{exact:false})).toHaveCount(0)
})

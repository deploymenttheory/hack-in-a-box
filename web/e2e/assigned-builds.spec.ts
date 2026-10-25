import {test,expect} from '@playwright/test'
import {freezeFixture,buildTeam} from './submission-freeze-fixture'
import {firstID,person} from './fixtures'
const run='88888888-8888-4888-8888-888888888888',path=`/events/${firstID}/teams/${buildTeam}/build`
test('only current own assignments produce links and removal clears them',async({page})=>{
 await freezeFixture(page);let assigned=true
 await page.route(`**/api/v1/events/${firstID}/judging`,r=>r.fulfill({json:{items:[{run_id:run,revision:3,deliberation:false,required_judges:2,assignments:[{team_id:buildTeam,judge_ids:assigned?[person.id]:[],missing:0},{team_id:'99999999-9999-4999-8999-999999999999',judge_ids:['aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa'],missing:0}]}]}}))
 await page.goto(`/events/${firstID}/builds`)
 await expect(page.getByRole('link',{name:'Open assigned build',exact:true})).toHaveCount(1)
 await expect(page.getByRole('link',{name:'Open assigned build',exact:true})).toHaveAttribute('href',`${path}?judge_run=${run}`)
 assigned=false;await page.getByRole('button',{name:'Read current judge assignments',exact:true}).click()
 await expect(page.getByRole('link',{name:'Open assigned build',exact:true})).toHaveCount(0)
 await expect(page.getByText(/No current conflict-free final judging assignment/)).toBeVisible()
})
test('denied exact context prevents artifact requests',async({page})=>{
 await freezeFixture(page);const artifacts:string[]=[]
 page.on('request',r=>{if(new URL(r.url()).pathname.includes(`/teams/${buildTeam}/submission`))artifacts.push(r.url())})
 await page.route(`**/api/v1/events/${firstID}/judging/${run}/teams/${buildTeam}`,r=>r.fulfill({status:403,json:{error:{code:'forbidden',message:'Synthetic current conflict',request_id:'synthetic'}}}))
 await page.goto(`${path}?judge_run=${run}`)
 await expect(page.getByText('Current submission access is unavailable. Private build evidence has been cleared.',{exact:true})).toBeVisible()
 expect(artifacts).toEqual([])
 await expect(page.getByRole('link',{name:'Synthetic live demo'})).toHaveCount(0)
})
test('formal mismatch clears artifacts without presenting private briefing fields',async({page})=>{
 await freezeFixture(page);let revision=1
 await page.route(`**/api/v1/events/${firstID}/judging/${run}/teams/${buildTeam}`,r=>r.fulfill({json:{team_id:buildTeam,team_name:'Synthetic assigned team',run_id:run,submission_revision:revision,private_judge_guidance:'PRIVATE-JUDGE-GUIDANCE',own_card:{scores:[{note:'PRIVATE-SCORE'}]},submitted_cards:[{note:'PRIVATE-OTHER-CARD'}]}}))
 await page.goto(`${path}?judge_run=${run}`)
 await expect(page.getByRole('link',{name:'Synthetic live demo'})).toBeVisible()
 await expect(page.locator('body')).not.toContainText('PRIVATE-')
 expect(await page.evaluate(()=>JSON.stringify({...localStorage,...sessionStorage}))).not.toContain('PRIVATE-')
 revision=2;await page.evaluate(()=>dispatchEvent(new Event('focus')))
 await expect(page.getByText(/current formal version changed while checking the assigned judge context/)).toBeVisible()
 await expect(page.getByRole('link',{name:'Synthetic live demo'})).toHaveCount(0)
 await expect(page.getByRole('heading',{name:'Code cutoff evidence'})).toHaveCount(0)
})

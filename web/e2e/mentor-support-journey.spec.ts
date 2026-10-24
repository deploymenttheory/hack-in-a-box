import { expect, test } from '@playwright/test'
import { firstID, person } from './fixtures'
import { fixture, teamPath, teamID } from './review-dashboard-fixtures'
import { helpState, mockHelp, organiser, secondHelper, requestID } from './help-workspace-fixtures'

test('permitted support drilldown/ownership history stays separate from private independent review and panel decisions',async({page})=>{
 const support=helpState()
 support.records[0].team_id=teamID;support.records[0].category='competition_output'
 await mockHelp(page,support,{...organiser,id:person.id})
 const review=await fixture(page,false)
 await page.route('**/api/v1/session',route=>route.fulfill({json:{person:{...person,platform_admin:false},draft_scope:'88888888-8888-4888-8888-888888888888',csrf_token:'synthetic-csrf-token-32-characters',can_create_events:false}}))
 review.team.support.help.requests=[{...review.team.support.help.requests[0],id:requestID,team_id:teamID}]
 const independent=JSON.stringify(review.team.assessment)
 await page.goto(`/events/${firstID}/review`)
 await expect(page.getByText('PRIVATE-ORIGINAL-EVIDENCE')).toHaveCount(0);await expect(page.getByText('PRIVATE HELP',{exact:false})).toHaveCount(0)
 await page.getByRole('link',{name:/competition output · waiting/}).click()
 await expect(page.getByText('PRIVATE HELP <script>window.badHelp=true</script>',{exact:true})).toBeVisible()
 await expect(page.getByText('PRIVATE-ORIGINAL-EVIDENCE')).toHaveCount(0)
 await page.getByRole('button',{name:'Claim request',exact:true}).click(); await page.getByRole('button',{name:'Confirm reviewed change',exact:true}).click()
 await page.getByLabel('Transfer to a currently eligible helper').selectOption(secondHelper.id)
 await page.getByRole('button',{name:'Transfer request',exact:true}).click(); await page.getByRole('button',{name:'Confirm reviewed change',exact:true}).click()
 await expect(page.getByText('Current helper: Second Helper', {exact:true})).toBeVisible()
 await page.getByRole('button',{name:'Resolve request',exact:true}).click(); await page.getByRole('button',{name:'Confirm reviewed change',exact:true}).click()
 await page.getByRole('button',{name:'Reopen request',exact:true}).click(); await page.getByRole('button',{name:'Confirm reviewed change',exact:true}).click()
 await expect(page.getByText('reopen → waiting',{exact:false})).toBeVisible()
 expect(support.commands.map(c=>c.action)).toEqual(['claim','transfer','resolve','reopen'])
 expect(review.writes).toHaveLength(0);expect(JSON.stringify(review.team.assessment)).toBe(independent)
 await page.goto(teamPath)
 await expect(page.getByText('1 pending categories · 1 categories need evidence',{exact:false}).first()).toBeVisible()
 await page.getByRole('button',{name:'Open intended-panel context'}).click()
 await expect(page.getByText('PRIVATE-ORIGINAL-EVIDENCE',{exact:true})).toBeVisible()
 await expect(page.getByText('PRIVATE HELP',{exact:false})).toHaveCount(0)
 await page.getByLabel('Explicit decision reason').fill('PRIVATE-UNSENT-MODERATION-REASON')
 await page.getByRole('link',{name:'Open help workspace'}).click()
 await expect(page.getByText('PRIVATE-ORIGINAL-EVIDENCE')).toHaveCount(0)
 expect(await page.evaluate(()=>JSON.stringify({local:{...localStorage},session:{...sessionStorage}}))).not.toContain('PRIVATE-UNSENT-MODERATION-REASON')
 support.denied=true
 await page.goto(`/events/${firstID}/help/${requestID}`)
 await expect(page.getByRole('alert')).toBeVisible();await expect(page.getByText('PRIVATE HELP',{exact:false})).toHaveCount(0)
 review.denied=true
 await page.goto(teamPath)
 await expect(page.getByRole('alert')).toBeVisible();await expect(page.getByText('PRIVATE-ORIGINAL-EVIDENCE')).toHaveCount(0)
 expect(review.writes).toHaveLength(0)
})

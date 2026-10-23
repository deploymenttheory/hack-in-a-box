import { expect, test } from '@playwright/test'
import { firstID } from './fixtures'
import { fixture, teamPath, primaryID, secondID, categoryID, cpID, summary } from './review-dashboard-fixtures'
test('support risk and provisional/final evidence completeness stay separate with permitted help drilldown',async({page},info)=>{
 await fixture(page);await page.goto(`/events/${firstID}/review`)
 await expect(page.getByRole('heading',{name:'Support and review coverage'})).toBeVisible()
 await expect(page.getByText('Support risk:',{exact:false})).toContainText('blocked')
 await expect(page.getByText('2 waiting · 1 claimed · 3 unresolved', {exact:false})).toBeVisible()
 await expect(page.getByText('Live checkpoints · provisional only')).toBeVisible();await expect(page.getByText('Final qualification · post-cutoff frozen output')).toBeVisible()
 await expect(page.getByText('First-response mean',{exact:false})).toContainText('2 recorded samples')
 await expect(page.getByRole('link',{name:/competition output · waiting/})).toHaveAttribute('href',`/events/${firstID}/help/${cpID}`)
 await expect(page.getByText('PRIVATE-ORIGINAL-EVIDENCE')).toHaveCount(0)
 await page.screenshot({path:info.outputPath('support-and-completeness.png'),fullPage:true})
})
test('not-submitted output has no awaiting-score agenda and judge preview is factual and printable',async({page})=>{
 const state=await fixture(page);state.team.assessment.availability='not_applicable';state.team.assessment.output_state='not_submitted';state.team.output!.outcome='not_submitted';state.team.output!.frozen_revision=null;state.team.output!.artifacts=[]
 await page.goto(teamPath);await expect(page.getByText('not submitted output is not awaiting scores.')).toBeVisible();await expect(page.getByRole('button',{name:'Configure a new huddle'})).toHaveCount(0)
 await page.getByRole('link',{name:'Preview factual judge handoff'}).click();await expect(page.getByRole('heading',{name:'Factual judge handoff preview'})).toBeVisible();await expect(page.getByText('No frozen formal output is available',{exact:false})).toBeVisible()
 await expect(page.locator('.factual-judge-preview')).not.toContainText('PRIVATE-ORIGINAL-EVIDENCE');await expect(page.locator('.factual-judge-preview')).not.toContainText('Original score')
 await page.emulateMedia({media:'print'});await expect(page.locator('.factual-judge-preview')).toBeVisible()
})
test('current intended panel sees pending separate from numeric original and private history is hidden in print',async({page},info)=>{
 await fixture(page);await page.goto(teamPath);await page.getByRole('button',{name:'Open intended-panel context'}).click()
 await expect(page.getByText('PRIVATE-ORIGINAL-EVIDENCE',{exact:true})).toBeVisible();await expect(page.getByText('Pending/unobserved — no numeric score')).toBeVisible();await expect(page.getByText('Original score 2')).toBeVisible()
 await page.getByRole('button',{name:'Show authorised huddle decision history'}).click()
 expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(page.viewportSize()!.width)
 await page.screenshot({path:info.outputPath('private-panel-evidence.png'),fullPage:true})
 await page.emulateMedia({media:'print'});await expect(page.locator('.private-review').first()).toBeHidden()
})
test('accepted proposal response lost reconciles exact history once without replay or overwriting originals',async({page})=>{
 const state=await fixture(page);state.lost=true;await page.goto(teamPath);await page.getByRole('button',{name:'Open intended-panel context'}).click()
 await page.getByLabel('Explicit decision reason').fill('Observed frozen output warrants review');await page.getByLabel('Formal evidence note',{exact:true}).fill('PRIVATE-PROPOSED-EVIDENCE')
 await page.getByLabel('Original independent card').selectOption(primaryID);await page.getByLabel('Category',{exact:true}).selectOption(categoryID);await page.getByLabel('Proposed score',{exact:true}).fill('4');await page.getByRole('button',{name:'Record proposed change and evidence'}).click()
 await expect(page.getByRole('status',{name:'Huddle decision state'})).toContainText('Outcome uncertain');expect(state.writes).toHaveLength(1)
 await page.getByRole('button',{name:'Check exact recorded decision'}).click();await expect(page.getByText('Original 2 → Proposed 4 · proposed')).toBeVisible();expect(state.writes).toHaveLength(1);await expect(page.getByText('Original score 2')).toBeVisible()
 expect(await page.evaluate(()=>JSON.stringify({local:{...localStorage},session:{...sessionStorage}}))).not.toContain('PRIVATE-PROPOSED-EVIDENCE')
})
test('planned replacement offers explicit repair without requesting obsolete private context',async({page})=>{
 const state=await fixture(page);state.h.state='planned';state.item={...summary(state.h),source_state:'review_required',can_open:false,can_manage:false,can_record:false,can_reconfigure:true}
 await page.goto(teamPath);await expect(page.getByRole('button',{name:'Open intended-panel context'})).toHaveCount(0);await page.getByRole('button',{name:'Review planned configuration with current coverage'}).click()
 await expect(page.getByRole('heading',{name:'Explicit planned-context repair'})).toBeVisible();await page.getByLabel('Configuration review reason').fill('Review replacement published second coverage');await page.getByRole('button',{name:'Record reviewed configuration'}).click();await expect(page.getByRole('heading',{name:'Explicit planned-context repair'})).toHaveCount(0)
 expect(state.writes).toHaveLength(1);expect(state.writes[0]).toMatchObject({action:'configure',version:3,definition:{second_reviewer_id:secondID}});expect(state.panelReads).toBe(0)
})
test('non-organiser second reviewer sees own metadata but denied judge preview and revoked panel clears',async({page})=>{
 const state=await fixture(page);state.actor=secondID;state.organiser=false;state.item.can_manage=false;state.item.can_reconfigure=false;state.previewDenied=true
 await page.goto(teamPath);await expect(page.getByRole('link',{name:'Preview factual judge handoff'})).toHaveCount(0);await expect(page.getByRole('button',{name:'Configure a new huddle'})).toHaveCount(0);await page.getByRole('button',{name:'Open intended-panel context'}).click();await expect(page.getByText('PRIVATE-ORIGINAL-EVIDENCE',{exact:true})).toBeVisible()
 state.cardDenied=true;await page.getByRole('button',{name:'Recheck current panel authority'}).click();await expect(page.getByText('PRIVATE-ORIGINAL-EVIDENCE',{exact:true})).toHaveCount(0)
 await page.goto(`${teamPath}/summary-preview`);await expect(page.getByRole('alert')).toBeVisible();await expect(page.getByText('Frozen submission revision 7',{exact:false})).toHaveCount(0)
})

test('definite panel denial invalidates a deferred write preflight before any decision is sent',async({page})=>{
 const state=await fixture(page);await page.goto(teamPath);await page.getByRole('button',{name:'Open intended-panel context'}).click()
 await page.getByLabel('Explicit decision reason').fill('Must not escape revoked authority')
 let release!:()=>void;state.preflightGate=new Promise<void>(resolve=>{release=resolve})
 await page.getByRole('button',{name:'Sign off exact complete frozen final evidence'}).click()
 state.cardDenied=true
 await page.getByRole('button',{name:'Recheck current panel authority'}).click()
 await expect(page.getByRole('status',{name:'Huddle decision state'})).toContainText('Current authority was denied')
 release();state.preflightGate=undefined
 await expect(page.getByText('PRIVATE-ORIGINAL-EVIDENCE',{exact:true})).toHaveCount(0)
 await expect.poll(()=>state.writes.length).toBe(0)
})
test('primary mentor can propose but cannot moderate or sign; changed event revision requires explicit reviewed base',async({page})=>{
 const state=await fixture(page);state.actor=primaryID;state.organiser=false;state.item.can_manage=false;state.item.can_reconfigure=false
 await page.goto(teamPath);await page.getByRole('button',{name:'Open intended-panel context'}).click()
 await expect(page.getByRole('button',{name:'Sign off exact complete frozen final evidence'})).toHaveCount(0)
 await page.getByLabel('Explicit decision reason').fill('Own independent review proposal');await page.getByLabel('Formal evidence note',{exact:true}).fill('Private current evidence')
 await page.getByLabel('Original independent card').selectOption(primaryID);await page.getByLabel('Category',{exact:true}).selectOption(categoryID);await page.getByLabel('Proposed score',{exact:true}).fill('3')
 state.revision++
 await page.getByRole('button',{name:'Record proposed change and evidence'}).click()
 await expect(page.getByRole('status',{name:'Huddle decision state'})).toContainText('Review fresh current sources');expect(state.writes).toHaveLength(0)
 await page.getByRole('button',{name:'Use reviewed current context and clear unsent decision'}).click()
 await expect(page.getByLabel('Formal evidence note',{exact:true})).toHaveValue('')
 await page.getByLabel('Explicit decision reason').fill('Explicitly reviewed current context');await page.getByLabel('Formal evidence note',{exact:true}).fill('Current source reviewed')
 await page.getByLabel('Original independent card').selectOption(primaryID);await page.getByLabel('Category',{exact:true}).selectOption(categoryID);await page.getByLabel('Proposed score',{exact:true}).fill('3');await page.getByRole('button',{name:'Record proposed change and evidence'}).click()
 await expect(page.getByText('Original 2 → Proposed 3 · proposed')).toBeVisible();expect(state.writes).toHaveLength(1)
})

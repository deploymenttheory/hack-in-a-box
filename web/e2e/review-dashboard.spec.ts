import { expect, test } from '@playwright/test'
import { firstID } from './fixtures'
import { fixture, teamPath, primaryID, secondID, categoryID, cpID, summary } from './review-dashboard-fixtures'
test('support risk and provisional/final evidence completeness stay separate with permitted help drilldown',async({page},info)=>{
 await fixture(page);await page.goto(`/events/${firstID}/review`)
 await expect(page.getByRole('heading',{name:'Support and review coverage'})).toBeVisible()
 await page.getByRole('button',{name:'Inspect Factual output team'}).click()
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
 await page.emulateMedia({media:'print'});await expect(page.locator('[data-private-review]').first()).toBeHidden()
})
test('accepted proposal response lost reconciles exact history once without replay or overwriting originals',async({page})=>{
 const state=await fixture(page);state.lost=true;await page.goto(teamPath);await page.getByRole('button',{name:'Open intended-panel context'}).click()
 await page.getByRole('button',{name:'Propose category change',exact:true}).click()
 await page.getByLabel('Explicit decision reason').fill('Observed frozen output warrants review');await page.getByLabel('Formal evidence note',{exact:true}).fill('PRIVATE-PROPOSED-EVIDENCE')
 await page.getByLabel('Original independent card').selectOption(primaryID);await page.getByLabel('Category',{exact:true}).selectOption(categoryID);await page.getByLabel('Proposed score',{exact:true}).fill('4');await page.getByRole('button',{name:'Record proposed change and evidence'}).click();await page.getByRole('button',{name:'Confirm reviewed huddle decision'}).click()
 await expect(page.getByRole('status',{name:'Huddle decision state'})).toContainText('Outcome uncertain');expect(state.writes).toHaveLength(1)
 await page.getByRole('button',{name:'Check exact recorded decision'}).click();await page.getByRole('button',{name:'Open intended-panel context'}).click();await page.getByRole('button',{name:'Propose category change',exact:true}).click();await expect(page.getByText('Original 2 → Proposed 4 · proposed')).toBeVisible();expect(state.writes).toHaveLength(1);await page.getByRole('button',{name:'Inspect independent evidence',exact:true}).click();await expect(page.getByText('Original score 2')).toBeVisible()
 expect(await page.evaluate(()=>JSON.stringify({local:{...localStorage},session:{...sessionStorage}}))).not.toContain('PRIVATE-PROPOSED-EVIDENCE')
})
test('planned replacement offers explicit repair without requesting obsolete private context',async({page})=>{
 const state=await fixture(page);state.h.state='planned';state.item={...summary(state.h),source_state:'review_required',can_open:false,can_manage:false,can_record:false,can_reconfigure:true}
 await page.goto(teamPath);await expect(page.getByRole('button',{name:'Open intended-panel context'})).toHaveCount(0);await page.getByRole('button',{name:'Review planned configuration with current coverage'}).click()
 await expect(page.getByRole('heading',{name:'Explicit planned-context repair'})).toBeVisible();await page.getByLabel('Configuration review reason').fill('Review replacement published second coverage');await page.getByRole('button',{name:'Review configuration',exact:true}).click();await page.getByRole('button',{name:'Record reviewed configuration'}).click();await expect(page.getByRole('heading',{name:'Explicit planned-context repair'})).toHaveCount(0)
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
 await page.getByRole('button',{name:'Record formal decision',exact:true}).click()
 await page.getByLabel('Explicit decision reason').fill('Must not escape revoked authority')
 let release!:()=>void;state.preflightGate=new Promise<void>(resolve=>{release=resolve})
 await page.getByRole('button',{name:'Sign off exact complete frozen final evidence'}).click();await page.getByRole('button',{name:'Confirm reviewed huddle decision'}).click()
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
 await page.getByRole('button',{name:'Propose category change',exact:true}).click()
 await page.getByLabel('Explicit decision reason').fill('Own independent review proposal');await page.getByLabel('Formal evidence note',{exact:true}).fill('Private current evidence')
 await page.getByLabel('Original independent card').selectOption(primaryID);await page.getByLabel('Category',{exact:true}).selectOption(categoryID);await page.getByLabel('Proposed score',{exact:true}).fill('3')
 state.revision++
 await page.getByRole('button',{name:'Record proposed change and evidence'}).click();await page.getByRole('button',{name:'Confirm reviewed huddle decision'}).click()
 await expect(page.getByRole('status',{name:'Huddle decision state'})).toContainText('Review fresh current sources');expect(state.writes).toHaveLength(0)
 page.once('dialog',dialog=>dialog.accept());await page.getByRole('button',{name:'Use reviewed current context and clear unsent decision'}).click();await page.getByRole('button',{name:'Propose category change',exact:true}).click()
 await expect(page.getByLabel('Formal evidence note',{exact:true})).toHaveValue('')
 await page.getByLabel('Explicit decision reason').fill('Explicitly reviewed current context');await page.getByLabel('Formal evidence note',{exact:true}).fill('Current source reviewed')
 await page.getByLabel('Original independent card').selectOption(primaryID);await page.getByLabel('Category',{exact:true}).selectOption(categoryID);await page.getByLabel('Proposed score',{exact:true}).fill('3');await page.getByRole('button',{name:'Record proposed change and evidence'}).click();await page.getByRole('button',{name:'Confirm reviewed huddle decision'}).click()
 await page.getByRole('button',{name:'Open intended-panel context'}).click();await page.getByRole('button',{name:'Propose category change',exact:true}).click();await expect(page.getByText('Original 2 → Proposed 3 · proposed')).toBeVisible();expect(state.writes).toHaveLength(1)
})

test('dirty huddle decisions survive cancelled local and route departures and return focus to the actual opener',async({page})=>{
 await fixture(page);await page.goto(teamPath)
 const open=page.getByRole('button',{name:'Open intended-panel context'});await open.focus();await expect(open).toBeFocused();await page.keyboard.press('Enter')
 await expect(page.getByRole('heading',{name:'Intended-panel context',exact:true})).toBeFocused()
 await page.getByRole('button',{name:'Propose category change',exact:true}).click();await page.getByLabel('Formal evidence note',{exact:true}).fill('PRIVATE DIRTY HUDDLE NOTE')
 page.once('dialog',dialog=>dialog.dismiss());await page.getByRole('button',{name:'Back to roster'}).click();await expect(page.getByLabel('Formal evidence note',{exact:true})).toHaveValue('PRIVATE DIRTY HUDDLE NOTE')
 page.once('dialog',dialog=>dialog.dismiss());await page.getByRole('link',{name:'Support and review coverage',exact:true}).click();await expect(page).toHaveURL(new RegExp('/review/'));await expect(page.getByLabel('Formal evidence note',{exact:true})).toHaveValue('PRIVATE DIRTY HUDDLE NOTE')
 page.once('dialog',dialog=>dialog.accept());await page.getByRole('button',{name:'Back to roster'}).click();await expect(open).toBeFocused();await expect(page.getByLabel('Formal evidence note',{exact:true})).toHaveCount(0)
})
test('configuration changes without a reason are still dirty and cancellation preserves their fields',async({page})=>{
 await fixture(page);await page.goto(teamPath);const opener=page.getByRole('button',{name:'Configure a new huddle'});await opener.focus();await page.keyboard.press('Enter')
 await page.getByLabel('Second-review deadline (UTC)').fill('2027-06-20T16:45')
 page.once('dialog',dialog=>dialog.dismiss());await page.getByRole('button',{name:'Close configuration'}).click();await expect(page.getByLabel('Second-review deadline (UTC)')).toHaveValue('2027-06-20T16:45')
 page.once('dialog',dialog=>dialog.accept());await page.getByRole('button',{name:'Close configuration'}).click();await expect(opener).toBeFocused()
})
test('fresh agenda permission loss during confirmation hides the private decision without a PUT',async({page})=>{
 const state=await fixture(page);await page.goto(teamPath);await page.getByRole('button',{name:'Open intended-panel context'}).click();await page.getByRole('button',{name:'Record formal decision',exact:true}).click();await page.getByLabel('Explicit decision reason').fill('PRIVATE stale decision')
 await page.getByRole('button',{name:'Sign off exact complete frozen final evidence'}).click();state.item.can_manage=false;state.item.can_record=false
 await page.getByRole('button',{name:'Confirm reviewed huddle decision'}).click();await expect(page.getByLabel('Explicit decision reason')).toHaveCount(0);await expect(page.locator('body')).not.toContainText('PRIVATE stale decision');expect(state.writes).toHaveLength(0)
})
test('capability-only agenda downgrade clears a typed private decision and requires an explicit fresh open',async({page})=>{
 const state=await fixture(page);await page.clock.install();await page.goto(teamPath);await page.getByRole('button',{name:'Open intended-panel context'}).click();await page.getByRole('button',{name:'Propose category change',exact:true}).click();await page.getByLabel('Formal evidence note',{exact:true}).fill('PRIVATE capability draft')
 state.item.can_manage=false;state.item.can_record=false;await page.clock.fastForward(16_000)
 await expect(page.getByLabel('Formal evidence note',{exact:true})).toHaveCount(0);await expect(page.getByRole('heading',{name:'Intended-panel context',exact:true})).toHaveCount(0)
 await page.getByRole('button',{name:'Open intended-panel context'}).click();await expect(page.getByText('PRIVATE-ORIGINAL-EVIDENCE',{exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'Propose category change',exact:true})).toHaveCount(0);await expect(page.locator('body')).not.toContainText('PRIVATE capability draft');expect(state.writes).toHaveLength(0)
})
test('held huddle write survives parent denial and partial restoration as minimal metadata without private resurrection',async({page})=>{
 const state=await fixture(page);await page.clock.install();await page.goto(teamPath);await page.getByRole('button',{name:'Open intended-panel context'}).click();await page.getByRole('button',{name:'Record formal decision',exact:true}).click();await page.getByLabel('Explicit decision reason').fill('PRIVATE held reason')
 let release!:()=>void;state.writeGate=new Promise<void>(resolve=>{release=resolve});state.lostBefore=true
 await page.getByRole('button',{name:'Sign off exact complete frozen final evidence'}).click();await page.getByRole('button',{name:'Confirm reviewed huddle decision'}).click();await expect.poll(()=>state.writes.length).toBe(1)
 state.denied=true;await page.clock.fastForward(16_000);await expect(page.getByLabel('Explicit decision reason')).toHaveCount(0);await expect(page.getByLabel('Unconfirmed command history')).toBeVisible();await expect(page.locator('body')).not.toContainText('PRIVATE held reason')
 release();state.writeGate=undefined;state.denied=false;state.cardDenied=true;await page.clock.fastForward(16_000)
 await expect(page.getByRole('heading',{name:'Intended-panel context',exact:true})).toHaveCount(0);await page.getByRole('button',{name:'Open intended-panel context'}).click();await expect(page.getByText('PRIVATE-ORIGINAL-EVIDENCE',{exact:true})).toHaveCount(0);await expect(page.getByLabel('Unconfirmed command history')).toBeVisible();await page.clock.fastForward(20_000);expect(state.writes).toHaveLength(1)
})
test('named second records only their independent submitted review while a planned panel withholds both private cards',async({page})=>{
 const state=await fixture(page);state.actor=secondID;state.organiser=false;state.h.state='planned';state.h.cards=[];state.h.second_review.completed_at=null;state.h.second_review.card=null;state.item={...summary(state.h),can_manage:false,can_reconfigure:false}
 await page.goto(teamPath);await page.getByRole('button',{name:'Open intended-panel context'}).click();await page.getByRole('button',{name:'Record formal decision',exact:true}).click();await page.getByLabel('Explicit decision reason').fill('Reviewed my independent submitted card')
 await expect(page.getByRole('button',{name:'Open huddle against both exact submitted cards'})).toHaveCount(0);expect(state.panelReads).toBe(0)
 await page.getByRole('button',{name:'Record my submitted independent second review'}).click();await expect(page.getByRole('heading',{name:'Review exact huddle decision'})).toBeVisible();expect(state.writes).toHaveLength(0)
 await page.getByRole('button',{name:'Confirm reviewed huddle decision'}).click();await expect.poll(()=>state.writes.length).toBe(1);await expect(page.getByRole('heading',{name:'Intended-panel context',exact:true})).toHaveCount(0);expect(state.writes[0].action).toBe('complete_second_review');expect(state.panelReads).toBe(0)
})

test('temporary parent coordination failure pauses the mounted private draft and explicit recovery preserves it',async({page})=>{
 const state=await fixture(page);await page.goto(teamPath);await page.getByRole('button',{name:'Open intended-panel context'}).click();await page.getByRole('button',{name:'Propose category change',exact:true}).click();await page.getByLabel('Formal evidence note',{exact:true}).fill('PRIVATE same-page correction')
 state.readFailure=true;await page.getByRole('button',{name:'Recheck current panel authority'}).click();await expect(page.getByRole('alert')).toBeVisible();await expect(page.getByLabel('Formal evidence note',{exact:true})).toBeHidden()
 state.readFailure=false;await page.getByRole('button',{name:'Try again',exact:true}).click();await expect(page.getByLabel('Formal evidence note',{exact:true})).toHaveValue('PRIVATE same-page correction');expect(state.writes).toHaveLength(0)
})

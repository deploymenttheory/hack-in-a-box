import { expect, test } from '@playwright/test'
import { firstID, person } from './fixtures'
import { fixture, open, score, reviewCard, leaveCard, status, team, cpID, nextCategory } from './scorecard-workspace-fixtures'

test('keyboard pending is distinct from zero, evidence validation preserves input, own draft and independent submit', async ({ page }, info) => {
 const state = await fixture(page); await open(page)
 await expect(page.getByText('2 of 2 categories pending.', { exact: false })).toBeVisible()
 await page.getByText('Rubric score anchors').first().click(); await expect(page.getByText('Explicitly observed no working output.', { exact: false })).toBeVisible()
 await score(page)
 await page.getByRole('button', { name: 'Submit independent card with 1 pending' }).focus(); await page.keyboard.press('Enter')
 await expect(page.getByRole('alert').filter({ hasText: 'Add evidence before submitting' })).toBeVisible(); expect(state.writes).toHaveLength(0)
 await page.getByLabel('Private evidence note for Technical merit').fill('Observed zero, not unobserved <script>unsafe()</script>')
 await page.getByRole('button', { name: 'Save private draft', exact: true }).focus(); await expect(page.getByRole('button', { name: 'Save private draft', exact: true })).toBeFocused(); await page.keyboard.press('Enter'); await page.getByRole('button', { name: 'Confirm private draft', exact: true }).click()
 await expect(status(page)).toContainText('Confirmed your exact draft version 1')
 expect(state.writes[0].scores[0]).toMatchObject({ pending: false, value: 0 }); expect(state.writes[0].scores[1]).toMatchObject({ pending: true, value: null })
 await reviewCard(page, 'Submit independent card with 1 pending'); await expect(status(page)).toContainText('submitted version 2')
 await page.getByRole('button', { name: 'Show your saved card history' }).click(); await expect(page.getByText('Pending/unobserved (no score)', { exact: false }).first()).toBeVisible()
 const storage = await page.evaluate(() => JSON.stringify({ session: { ...sessionStorage }, local: { ...localStorage } })); expect(storage).not.toContain('Observed zero'); expect(storage).not.toContain('category_id')
 await expect(page.locator('script').filter({ hasText: 'unsafe()' })).toHaveCount(0); await expect(page.getByText('Observed zero, not unobserved <script>unsafe()</script>', { exact: true }).first()).toBeVisible(); await page.screenshot({ path: info.outputPath('private-checkpoint-scorecard.png'), fullPage: true })
})
test('accepted response lost reconciles exact retained version, without another write', async ({ page }) => {
 const state = await fixture(page); state.lost = true; await open(page); await score(page, '3'); await page.getByLabel('Private evidence note for Technical merit').fill('Private unsent assessment')
 await reviewCard(page, 'Save private draft'); await expect(status(page)).toContainText('unconfirmed')
 // A newer server version must not prevent finding the exact original retained receipt.
 const newer = { ...state.cards[0], revision: ++state.revision, reference: { id: cpID, version: 2 }, scores: state.cards[0].scores.map(s => ({ ...s, value: s.pending ? null : 4 })) }; state.cards = [newer]; state.history.push(newer)
 await page.getByRole('button', { name: 'Check exact saved version' }).click(); await expect(status(page)).toContainText('exact draft version 1'); expect(state.writes).toHaveLength(1)
})
test('unseen server revision requires explicit review without losing entered notes', async ({ page }) => {
 const state = await fixture(page); await open(page); await score(page, '2'); await page.getByLabel('Private evidence note for Technical merit').fill('Keep this private evidence'); state.revision++
 await reviewCard(page, 'Save private draft'); await expect(status(page)).toContainText('server changed'); expect(state.writes).toHaveLength(0)
 await page.getByRole('button', { name: 'Use reviewed current version as base', exact: false }).click(); await expect(page.getByLabel('Private evidence note for Technical merit')).toHaveValue('Keep this private evidence')
 await reviewCard(page, 'Save private draft'); await expect(status(page)).toContainText('Confirmed your exact draft version 1')
})
test('native validation rejected differs from unconfirmed transport and preserves memory only', async ({ page }) => {
 const state = await fixture(page); state.reject = 400; await open(page); await score(page, '1'); await page.getByLabel('Private evidence note for Technical merit').fill('Keep invalid native input')
 await reviewCard(page, 'Save private draft'); await expect(status(page)).toContainText('Validation rejected'); await expect(status(page)).not.toContainText('unconfirmed'); await expect(page.getByLabel('Private evidence note for Technical merit')).toHaveValue('Keep invalid native input')
 await leaveCard(page, () => page.reload()); await page.getByRole('button', { name: 'Open Technical checkpoint', exact: true }).click(); await expect(page.getByLabel('Private evidence note for Technical merit')).toHaveValue('')
})
test('final checkpoint requires actual frozen output and binds saved revision without checkpoint averaging', async ({ page }) => {
 const state = await fixture(page); state.cp.purpose = 'mentor_final'; state.frozen = false; await page.goto(`/events/${firstID}/mentor/${team}/scorecards`); await page.getByRole('button', { name: 'Open Technical checkpoint', exact: true }).click()
 await expect(page.getByText('Entry is unavailable until current authority', { exact: false })).toBeVisible(); await expect(page.getByRole('region', { name: 'Your private scorecard' })).toHaveCount(0); expect(state.writes).toHaveLength(0)
 state.frozen = true; await leaveCard(page, () => page.reload()); await page.getByRole('button', { name: 'Open Technical checkpoint', exact: true }).click(); await reviewCard(page, 'Save private draft'); await expect(status(page)).toContainText('draft version 1'); await expect(page.getByText('Saved against frozen submission revision 7.')).toBeVisible(); expect(state.writes[0].scores.every(s => s.pending && s.value === null)).toBeTruthy()
})
test('prelive current denial and future checkpoint windows cannot enable scoring', async ({ page }) => {
 const state = await fixture(page); state.denied = true; await page.goto(`/events/${firstID}/mentor/${team}/scorecards`); await expect(page.getByText('Scorecards require current assignment', { exact: false })).toBeVisible(); await expect(page.getByRole('region', { name: 'Your private scorecard' })).toHaveCount(0)
 state.denied = false; state.cp.opens_at = '2090-01-01T09:00:00Z'; state.cp.closes_at = '2090-01-01T17:00:00Z'; await leaveCard(page, () => page.reload()); await page.getByRole('button', { name: 'Open Technical checkpoint', exact: true }).click(); await expect(page.getByRole('button', { name: 'Save private draft', exact: true })).toBeDisabled(); expect(state.writes).toHaveLength(0)
})
test('active native revocation hides private page without a focus switch', async ({ page }) => {
 const state = await fixture(page); await open(page); await page.getByLabel('Private evidence note for Technical merit').fill('REVOKED PRIVATE INPUT'); state.denied = true
 await expect(page.getByRole('region', { name: 'Your private scorecard' })).toHaveCount(0, { timeout: 20_000 }); await expect(page.locator('body')).not.toContainText('REVOKED PRIVATE INPUT'); expect(state.writes).toHaveLength(0)
 state.denied = false; await page.getByRole('button', { name: 'Read current checkpoints', exact: true }).click(); await expect(page.getByRole('region', { name: 'Your private scorecard' })).toHaveCount(0); await page.getByRole('button', { name: 'Open Technical checkpoint', exact: true }).click(); await expect(page.getByLabel('Private evidence note for Technical merit')).toHaveValue('')
})
test('native own-card cursor finds saved version beyond first page and never renders another author', async ({ page }) => {
 const state = await fixture(page); await open(page); await score(page, '4'); await page.getByLabel('Private evidence note for Technical merit').fill('Saved beyond first page'); await reviewCard(page, 'Save private draft'); await expect(status(page)).toContainText('draft version 1')
 state.secondPage = true; await leaveCard(page, () => page.reload()); await page.getByRole('button', { name: 'Open Technical checkpoint', exact: true }).click(); await expect(page.getByLabel('Private evidence note for Technical merit')).toHaveValue('Saved beyond first page'); expect(state.ownReads).toBeGreaterThan(2)
 state.cards[0] = { ...state.cards[0], author_id: team, scores: [{ ...state.cards[0].scores[0], evidence: [{ kind: 'note', id: null, version: null, note: 'OTHER AUTHOR PRIVATE SCORE' }] }] }
 await leaveCard(page, () => page.reload()); await page.getByRole('button', { name: 'Open Technical checkpoint', exact: true }).click(); await expect(page.getByText('You do not have access to this resource.')).toBeVisible(); await expect(page.locator('body')).not.toContainText('OTHER AUTHOR PRIVATE SCORE')
})
test('leaving and returning drops unsent scores rather than restoring an offline score draft', async ({ page }) => {
 const state = await fixture(page); await open(page); await score(page, '5'); await page.getByLabel('Private evidence note for Technical merit').fill('MEMORY ONLY PRIVATE SCORE')
 await page.goto(`/events/${firstID}/mentor/${team}/scorecards`); await page.getByRole('button', { name: 'Open Technical checkpoint', exact: true }).click(); await expect(page.getByLabel('Private evidence note for Technical merit')).toHaveValue(''); await expect(page.getByRole('checkbox').first()).toBeChecked(); expect(state.writes).toHaveLength(0)
})
test('transport failure cannot imply success and offline preflight cannot submit', async ({ page }) => {
 const state = await fixture(page); state.failed = true; await open(page); await score(page, '3'); await page.getByLabel('Private evidence note for Technical merit').fill('Unconfirmed memory input')
 await reviewCard(page, 'Save private draft'); await expect(status(page)).toContainText('unconfirmed'); await expect(page.getByRole('button', { name: 'Save private draft', exact: true, includeHidden: true })).toBeDisabled(); expect(state.history).toHaveLength(0)
 await page.getByRole('button', { name: 'Check exact saved version' }).click(); await expect(status(page)).toContainText('not confirmed saved'); expect(state.writes).toHaveLength(1)
 page.once('dialog', dialog => void dialog.accept()); await page.getByRole('button', { name: 'Use reviewed current version as base', exact: false }).click(); await page.context().setOffline(true); await reviewCard(page, 'Save private draft'); await expect(status(page)).toContainText('No write was sent'); expect(state.writes).toHaveLength(1); await page.context().setOffline(false)
})
test('saved evidence attaches and opens exact native observation version without exposing a latest-version shortcut', async ({ page }) => {
 const state = await fixture(page); state.observations = [{ id: nextCategory, team_id: team, author_id: person.id, version: 2, observed_at: '2026-10-23T08:00:00Z', changed_at: '2026-10-23T08:00:00Z', evidence: { category: 'Technical demo', evidence: 'EXACT NATIVE VERSION TWO <img src=x onerror=alert(1)>', outcome: 'Demonstrated locally', feedback_reference: '' } }]
 await open(page); await score(page, '4'); await page.getByRole('button', { name: 'Choose saved evidence references', exact: true }).first().click(); await page.getByRole('button', { name: 'Attach observation', exact: false }).click(); await page.getByRole('button', { name: 'Read exact evidence version', exact: true }).click(); await expect(page.getByText('EXACT NATIVE VERSION TWO', { exact: false })).toBeVisible(); expect(state.observationReads[0]).toContain('observation_cursor=3'); await expect(page.locator('img[src="x"]')).toHaveCount(0)
 await reviewCard(page, 'Submit independent card with 1 pending'); await expect(status(page)).toContainText('submitted version 1'); expect(state.writes[0].scores[0].evidence).toEqual([{ kind: 'observation', id: nextCategory, version: 2, note: '' }])
})
for (const source of ['checkpoint', 'card'] as const) test(`transient ${source} poll failure hides stale evidence but preserves same-page input without replay`, async ({ page }) => {
 const state = await fixture(page); await open(page); await score(page, '3'); await page.getByLabel('Private evidence note for Technical merit').fill('PRESERVE IN MEMORY DURING CURRENT READ FAILURE')
 state.readFailure = source
 await expect(page.getByRole('region', { name: 'Your private scorecard' })).toHaveCount(0, { timeout: 20_000 })
 await expect(page.getByRole('button', { name: 'Save private draft', exact: true })).toBeHidden(); expect(state.writes).toHaveLength(0)
 state.readFailure = ''; await page.getByRole('button', { name: source === 'checkpoint' ? 'Read current checkpoints' : 'Try again', exact: true }).first().click()
 await expect(page.getByLabel('Private evidence note for Technical merit')).toHaveValue('PRESERVE IN MEMORY DURING CURRENT READ FAILURE', { timeout: 20_000 }); await expect(page.getByLabel('Technical merit score (explicit zero is allowed when within range)', { exact: true })).toHaveValue('3'); expect(state.writes).toHaveLength(0)
 const storage = await page.evaluate(() => JSON.stringify({ session: { ...sessionStorage }, local: { ...localStorage } })); expect(storage).not.toContain('PRESERVE IN MEMORY')
})


test('a later different successful assessment preserves earlier unresolved command metadata', async ({ page }) => {
 const state = await fixture(page); state.failed = true; await open(page); await score(page, '2')
 await page.getByLabel('Private evidence note for Technical merit').fill('FIRST UNKNOWN PRIVATE CONTENT')
 await reviewCard(page, 'Save private draft'); await expect(status(page)).toContainText('unconfirmed')
 await page.getByRole('button', { name: 'Check exact saved version' }).click()
 page.once('dialog', dialog => void dialog.accept())
 await page.getByRole('button', { name: 'Use reviewed current version as base', exact: false }).click()
 await page.getByLabel('Private evidence note for Technical merit').fill('DIFFERENT DELIBERATE CONTENT')
 state.failed = false; await reviewCard(page, 'Save private draft'); await expect(status(page)).toContainText('Confirmed your exact draft version 1')
 await expect(page.getByRole('complementary', { name: 'Unconfirmed command history' })).toContainText('Original outcome remains unconfirmed')
 expect(state.writes).toHaveLength(2); expect(state.writes[0].scores).not.toEqual(state.writes[1].scores)
})


test('restoring only checkpoint collection authority never resurrects a formerly selected private card', async ({ page }) => {
 const state = await fixture(page); await page.clock.install(); await open(page)
 await score(page, '4'); await page.getByLabel('Private evidence note for Technical merit').fill('PRIVATE-CARD-BEFORE-PARENT-DENIAL')
 await reviewCard(page, 'Save private draft'); await expect(status(page)).toContainText('Confirmed your exact draft')
 state.denied = true; await page.clock.fastForward(16_000)
 await expect(page.getByRole('region', { name: 'Your private scorecard' })).toHaveCount(0)
 state.denied = false; state.denyCard = true
 await page.getByRole('button', { name: 'Read current checkpoints', exact: true }).click()
 await expect(page.getByRole('button', { name: 'Open Technical checkpoint', exact: true })).toBeVisible()
 await expect(page.getByRole('region', { name: 'Your private scorecard' })).toHaveCount(0)
 await page.getByRole('button', { name: 'Open Technical checkpoint', exact: true }).click()
 await expect(page.getByRole('alert')).toBeVisible()
 await expect(page.locator('body')).not.toContainText('PRIVATE-CARD-BEFORE-PARENT-DENIAL')
 await page.clock.fastForward(20_000)
 await expect(page.locator('body')).not.toContainText('PRIVATE-CARD-BEFORE-PARENT-DENIAL')
 await expect(page.getByLabel('Private evidence note for Technical merit')).toHaveCount(0)
 expect(state.writes).toHaveLength(1)
})

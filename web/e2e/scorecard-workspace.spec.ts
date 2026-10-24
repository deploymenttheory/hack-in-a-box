import { expect, test, type Page } from '@playwright/test'
import { firstID, mockWorkspace, person } from './fixtures'
import type { components } from '../src/api/schema.gen'
import type { Card, Checkpoint, Command } from '../src/scorecard-model'
const team = '33333333-3333-4333-8333-333333333333', cpID = '44444444-4444-4444-8444-444444444444', category = '55555555-5555-4555-8555-555555555555', nextCategory = '66666666-6666-4666-8666-666666666666'
function checkpoint(): Checkpoint { return { definition: { id: cpID, run_id: cpID, name: 'Technical checkpoint', opens_wall_time: '2020-01-01T09:00:00', closes_wall_time: '2030-01-01T17:00:00', dst_choice: '' }, version: 1, purpose: 'checkpoint', rubric: { id: cpID, version: 2, digest: 'published-safe-digest' }, criteria: { name: 'Published rubric', mode: 'mentor_qualification', required_judge_count: 1, finalist_count: 1, reserve_count: 0, prizes: [], side_quests: [], categories: [{ id: category, label: 'Technical merit', min_score: 0, max_score: 5, weight: 1, guidance: 'Inspect the demonstrated working output.', anchors: [{ score: 0, description: 'Explicitly observed no working output.' }, { score: 5, description: 'Strong demonstrated output.' }] }, { id: nextCategory, label: 'Teamwork', min_score: 0, max_score: 5, weight: 1, guidance: 'Observe collaboration; leave unobserved work pending.', anchors: [] }] }, timezone: 'Europe/London', opens_at: '2020-01-01T09:00:00Z', closes_at: '2030-01-01T17:00:00Z' } }
async function fixture(page: Page) {
 await mockWorkspace(page)
 const state = { cp: checkpoint(), revision: 10, cards: [] as Card[], history: [] as Card[], writes: [] as Command[], denied: false, readFailure: '' as '' | 'checkpoint' | 'card', denyCard: false, lost: false, failed: false, reject: 0, ownReads: 0, observationReads: [] as string[], observations: [] as components['schemas']['MentorObservation'][], secondPage: false, frozen: true }
 await page.route(`**/events/${firstID}/teams/${team}/**`, async route => {
  const request = route.request(), url = new URL(request.url()), path = url.pathname
  const error = (status: number) => route.fulfill({ status, json: { error: { code: status === 409 ? 'conflict' : 'forbidden', request_id: 'synthetic' } } })
  if (state.denied || (state.denyCard && path.includes('/scorecards'))) return error(403)
  if (request.method() === 'GET' && ((state.readFailure === 'checkpoint' && path.endsWith('/scorecard-checkpoints')) || (state.readFailure === 'card' && path.endsWith('/scorecards')))) return route.abort('failed')
  if (path.endsWith('/scorecard-checkpoints')) return route.fulfill({ json: { revision: state.revision, items: [state.cp], next_cursor: null } })
  if (path.includes('/scorecards/') && path.endsWith('/history')) { const before = Number(url.searchParams.get('cursor') ?? 9999); return route.fulfill({ json: { items: state.history.filter(c => c.reference.version < before).sort((a,b) => b.reference.version-a.reference.version).slice(0, Number(url.searchParams.get('limit') ?? 50)), next_cursor: null } }) }
  if (path.endsWith('/scorecards')) {
   if (request.method() === 'POST') {
    const command = request.postDataJSON() as Command; state.writes.push(command)
    if (state.failed) return route.abort('failed')
    if (state.reject) return error(state.reject)
    if (request.headers()['if-match'] !== `"${state.revision}"` || command.version !== (state.cards[0]?.reference.version ?? 0)) return error(409)
    const card: Card = { revision: ++state.revision, reference: { id: cpID, version: command.version+1 }, team_id: team, checkpoint_id: cpID, checkpoint_version: state.cp.version, author_id: person.id, run_id: state.cp.definition.run_id, rubric: state.cp.rubric, criteria: state.cp.criteria, purpose: state.cp.purpose, submission_revision: state.cp.purpose === 'mentor_final' ? 7 : null, state: command.action === 'submit' ? 'submitted' : 'draft', scores: command.scores, created_at: '2026-10-23T09:00:00Z', changed_at: '2026-10-23T09:00:00Z' }
    state.cards = [card]; state.history.push(card)
    if (state.lost) return route.abort('failed')
    return route.fulfill({ json: card })
   }
   state.ownReads++
   if (state.secondPage && !url.searchParams.has('cursor')) return route.fulfill({ json: { revision: state.revision, items: [], next_cursor: cpID } })
   return route.fulfill({ json: { revision: state.revision, items: state.cards, next_cursor: null } })
  }
  if (path.endsWith('/submission')) return route.fulfill({ json: { outcome: state.frozen ? 'frozen' : 'missing', frozen_at: state.frozen ? '2026-10-23T08:00:00Z' : null, final: state.frozen ? { revision: 7, evidence: [] } : null } })
  if (path.endsWith('/mentor-observations/history')) { state.observationReads.push(url.search); return route.fulfill({ json: { observations: state.observations.filter(n => n.id === url.searchParams.get('observation_id') && n.version < Number(url.searchParams.get('observation_cursor'))), statuses: [], next_observation_cursor: null, next_status_cursor: null } }) }
  if (path.endsWith('/mentor-observations')) return route.fulfill({ json: { observations: state.observations, next_cursor: null } })
  throw new Error(`Unexpected scorecard fixture path ${path}`)
 })
 return state
}
async function leaveCard(page: Page, operation: () => Promise<unknown>) { const accept = (dialog: import('@playwright/test').Dialog) => void dialog.accept(); page.on('dialog', accept); try { await operation() } finally { page.off('dialog', accept) } }
async function open(page: Page) { await page.goto(`/events/${firstID}/mentor/${team}/scorecards`); await page.getByRole('button', { name: 'Open Technical checkpoint', exact: true }).click(); await expect(page.getByRole('region', { name: 'Your private scorecard' })).toBeVisible() }
async function score(page: Page, value = '0') { await page.getByRole('checkbox').first().uncheck(); await page.getByLabel('Technical merit score (explicit zero is allowed when within range)', { exact: true }).fill(value) }
async function reviewCard(page: Page, name: string) {
 const task = page.getByRole('region', { name: 'Your private scorecard' }), source = task.getByRole('button', { name, exact: true })
 await source.click()
 const confirm = page.getByRole('button', { name: name.startsWith('Save') ? 'Confirm private draft' : 'Confirm independent submission', exact: true })
 await expect.poll(async () => await confirm.isVisible() || (await task.getAttribute('aria-busy') === 'false' && await source.count() > 0)).toBe(true)
 if (await confirm.isVisible()) await confirm.click()
}
const status = (page: Page) => page.getByRole('status', { name: 'Scorecard save state' })

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

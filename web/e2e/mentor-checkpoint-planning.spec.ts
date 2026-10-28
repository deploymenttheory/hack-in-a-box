import { test, expect } from '@playwright/test'
import { firstID } from './fixtures'
import { fillCheckpoint, openPlanner, plannerFixture, plannerRegion, reviewCheckpoint } from './mentor-checkpoint-planning-fixtures'

test.describe('organiser checkpoint planner @admin', () => {
test('creates a reviewed assessment then configures and edits its exact named checkpoint with keyboard cancellation and reload', async ({ page }, info) => {
  const state = await plannerFixture(page, false); await openPlanner(page)
  const region = plannerRegion(page), create = region.getByRole('button', { name: 'Review mentor assessment', exact: true })
  await create.click(); await expect(region.getByRole('combobox', { name: 'Approved mentor rubric', exact: true })).toBeFocused(); expect(state.writes).toHaveLength(0)
  await region.getByRole('combobox', { name: 'Approved mentor rubric', exact: true }).selectOption({ index: 1 })
  await region.getByRole('textbox', { name: 'Assessment planning reason', exact: true }).fill('Reviewed independent live assessment')
  await create.press('Enter')
  const runReview = page.getByRole('alertdialog', { name: 'Review mentor assessment run', exact: true })
  await runReview.getByRole('button', { name: 'Cancel', exact: true }).click(); await expect(create).toBeFocused(); expect(state.writes).toHaveLength(0)
  await create.press('Enter'); await runReview.getByRole('button', { name: 'Create reviewed mentor assessment', exact: true }).click()
  await expect(region.getByRole('status', { name: 'Last acknowledged mentor planning change', exact: true })).toContainText('Mentor assessment created')
  expect(state.writes[0].body).toEqual({ action: 'create_run', purpose: 'checkpoint', version: 1, reason: 'Reviewed independent live assessment' })
  await fillCheckpoint(page)
  const review = await reviewCheckpoint(page)
  await expect(review).toContainText('Independent provisional observation'); await expect(review).toContainText('UTC'); await expect(review).toContainText('Checkpoint version 0 → 1')
  await review.getByRole('button', { name: 'Cancel', exact: true }).click(); await expect(region.getByRole('button', { name: 'Review checkpoint window', exact: true })).toBeFocused(); expect(state.writes).toHaveLength(1)
  await reviewCheckpoint(page); await review.getByRole('button', { name: 'Save reviewed checkpoint window', exact: true }).click()
  await expect(region.getByRole('status', { name: 'Last acknowledged mentor planning change', exact: true })).toContainText('checkpoint window was acknowledged')
  expect(state.writes).toHaveLength(2); expect(state.source.checkpoints.items[0].version).toBe(1)
  const id = state.source.checkpoints.items[0].definition.id
  await page.reload(); await expect(region.getByRole('region', { name: 'Current mentor checkpoints', exact: true })).toContainText('Independent provisional observation')
  await region.getByRole('combobox', { name: 'Saved checkpoint', exact: true }).selectOption(id)
  await region.getByRole('textbox', { name: 'Checkpoint name', exact: true }).fill('Reviewed revised checkpoint')
  await region.getByRole('textbox', { name: 'Checkpoint configuration reason', exact: true }).fill('Explicit version-one edit')
  await reviewCheckpoint(page); await expect(review).toContainText('Checkpoint version 1 → 2')
  await review.getByRole('button', { name: 'Save reviewed checkpoint window', exact: true }).click()
  await expect(region.getByRole('status', { name: 'Last acknowledged mentor planning change', exact: true })).toContainText('checkpoint window was acknowledged')
  expect(state.source.checkpoints.items).toHaveLength(1); expect(state.source.checkpoints.items[0].definition.id).toBe(id); expect(state.source.checkpoints.items[0].version).toBe(2)
  expect(state.writes).toHaveLength(3); expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
  await region.screenshot({ path: info.outputPath('synthetic-mentor-checkpoint-planning.png') })
  await expect(page.locator('body')).not.toContainText('PRIVATE PLANNER GUIDANCE CANARY')
})
test('all-team judging offers only live mentor observations while a non-organiser cannot read or configure the planner', async ({ page }) => {
  const state = await plannerFixture(page, false)
  state.configuration.definition.mode = 'all_teams'; state.source.approved!.definition.mode = 'all_teams'
  await openPlanner(page)
  await expect(plannerRegion(page).getByRole('combobox', { name: 'Assessment purpose', exact: true }).locator('option')).toHaveCount(1)
  expect(state.writes).toHaveLength(0)
  state.source.event.can_edit = false; state.source.event.can_manage_owner = false
  const before = state.reads.length
  await page.goto(`/events/${firstID}?section=mentors`)
  await expect(page.getByRole('navigation', { name: 'Event sections', exact: true })).toBeVisible()
  await expect(plannerRegion(page)).toHaveCount(0)
  expect(state.reads).toHaveLength(before); expect(state.writes).toHaveLength(0)
})
test('native local-clock ambiguity focuses the opening field and preview never writes configuration', async ({ page }) => {
  const state = await plannerFixture(page)
  state.source.event.configuration.timezone = 'Europe/London'; state.source.event.configuration.live_start = '2027-10-31T00:00:00Z'; state.source.event.configuration.submission_cutoff = '2027-10-31T03:00:00Z'; state.ambiguous = true
  await openPlanner(page); await fillCheckpoint(page)
  const region = plannerRegion(page)
  await region.getByLabel('Checkpoint opens', { exact: true }).fill('2027-10-31T01:30'); await region.getByLabel('Checkpoint closes', { exact: true }).fill('2027-10-31T02:00')
  await region.getByRole('button', { name: 'Review checkpoint window', exact: true }).click()
  await expect(region.getByLabel('Checkpoint opens', { exact: true })).toBeFocused(); await expect(region).toContainText('choose an earlier/later occurrence'); expect(state.writes).toHaveLength(0)
  await region.getByRole('combobox', { name: 'Repeated local clock times', exact: true }).selectOption('earlier')
  const review = await reviewCheckpoint(page); await expect(review).toContainText('Europe/London'); await expect(review).toContainText('earlier')
  expect(state.writes).toHaveLength(0)
})
test('native time preview keeps the live-to-cutoff boundary and never captures an out-of-window review', async ({ page }) => {
  const state = await plannerFixture(page); await openPlanner(page); await fillCheckpoint(page)
  await plannerRegion(page).getByLabel('Checkpoint closes', { exact: true }).fill('2027-06-20T16:00:01')
  await plannerRegion(page).getByRole('button', { name: 'Review checkpoint window', exact: true }).click()
  await expect(plannerRegion(page).getByLabel('Checkpoint closes', { exact: true })).toBeFocused()
  await expect(page.getByRole('alertdialog', { name: 'Review mentor checkpoint window', exact: true })).toHaveCount(0)
  expect(state.writes).toHaveLength(0)
})
test('lost checkpoint response remains unknown through current reads and an explicit new base does not replay it', async ({ page }) => {
  const state = await plannerFixture(page); await openPlanner(page); await fillCheckpoint(page)
  const review = await reviewCheckpoint(page); state.loseNext = true
  await review.getByRole('button', { name: 'Save reviewed checkpoint window', exact: true }).click()
  const region = plannerRegion(page)
  await expect(region).toContainText('original planning outcome is unknown'); expect(state.writes).toHaveLength(1)
  await region.getByRole('button', { name: 'Read current mentor planning', exact: true }).click()
  await expect(region).toContainText('original planning outcome is unknown'); expect(state.writes).toHaveLength(1)
  await region.getByRole('button', { name: 'Review current planner as a new starting point', exact: true }).click()
  const fresh = page.getByRole('alertdialog', { name: 'Review current mentor planning base', exact: true })
  await fresh.getByRole('button', { name: 'Use reviewed mentor planning base', exact: true }).click()
  await expect(region.getByRole('combobox', { name: 'Mentor assessment run', exact: true })).toHaveValue('')
  await region.getByRole('combobox', { name: 'Mentor assessment run', exact: true }).selectOption({ index: 1 })
  await expect(region.getByRole('textbox', { name: 'Checkpoint name', exact: true })).toHaveValue('')
  await expect(region.getByRole('textbox', { name: 'Checkpoint configuration reason', exact: true })).toHaveValue('')
  await expect(page.getByRole('complementary', { name: 'Unconfirmed command history', exact: true })).toContainText('Original outcome remains unconfirmed')
  expect(state.writes).toHaveLength(1)
})
test('lost run creation never fabricates an acknowledgement from the current run list', async ({ page }) => {
  const state = await plannerFixture(page, false); await openPlanner(page)
  const region = plannerRegion(page)
  await region.getByRole('combobox', { name: 'Approved mentor rubric', exact: true }).selectOption({ index: 1 }); await region.getByRole('textbox', { name: 'Assessment planning reason', exact: true }).fill('Uncertain original mentor run')
  await region.getByRole('button', { name: 'Review mentor assessment', exact: true }).click(); state.loseNext = true
  await page.getByRole('alertdialog', { name: 'Review mentor assessment run', exact: true }).getByRole('button', { name: 'Create reviewed mentor assessment', exact: true }).click()
  await expect(region).toContainText('original planning outcome is unknown')
  await region.getByRole('button', { name: 'Read current mentor planning', exact: true }).click()
  await expect(region).toContainText('original planning outcome is unknown'); expect(state.configuration.runs).toHaveLength(1); expect(state.writes).toHaveLength(1)
})
test('a changed event revision fences captured checkpoint confirmation without a write or silent retry', async ({ page }) => {
  const state = await plannerFixture(page); await openPlanner(page); await fillCheckpoint(page)
  const review = await reviewCheckpoint(page); state.source.event.revision++
  await review.getByRole('button', { name: 'Save reviewed checkpoint window', exact: true }).click()
  await expect(review).toContainText('Nothing was sent'); await expect(review.getByRole('button', { name: 'Save reviewed checkpoint window', exact: true })).toBeDisabled()
  expect(state.writes).toHaveLength(0)
})
test('failed native preflight retains the captured review without sending checkpoint configuration', async ({ page }) => {
  const state = await plannerFixture(page); await openPlanner(page); await fillCheckpoint(page)
  const review = await reviewCheckpoint(page); state.failRead = true
  await review.getByRole('button', { name: 'Save reviewed checkpoint window', exact: true }).click()
  await expect(review).toContainText('Nothing was sent'); expect(state.writes).toHaveLength(0)
})
test('a mismatched original checkpoint version remains unknown even when current reads show the accepted version', async ({ page }) => {
  const state = await plannerFixture(page); await openPlanner(page); await fillCheckpoint(page)
  const review = await reviewCheckpoint(page); state.badReceipt = true
  await review.getByRole('button', { name: 'Save reviewed checkpoint window', exact: true }).click()
  await expect(plannerRegion(page)).toContainText('did not acknowledge the exact reviewed checkpoint')
  await plannerRegion(page).getByRole('button', { name: 'Read current mentor planning', exact: true }).click()
  await expect(plannerRegion(page)).toContainText('original planning outcome is unknown'); expect(state.writes).toHaveLength(1); expect(state.source.checkpoints.items[0].version).toBe(1)
})
test('current authority loss fences a held preflight and clears private inputs without writing', async ({ page }) => {
  const state = await plannerFixture(page); await openPlanner(page); await fillCheckpoint(page)
  const review = await reviewCheckpoint(page)
  let release!: () => void; state.readBarrier = new Promise<void>(resolve => { release = resolve })
  const before = state.reads.length
  await review.getByRole('button', { name: 'Save reviewed checkpoint window', exact: true }).click()
  await expect.poll(() => state.reads.length).toBeGreaterThan(before)
  state.denied = true; await page.evaluate(() => window.dispatchEvent(new Event('focus')))
  await expect(page.getByRole('textbox', { name: 'Checkpoint configuration reason', exact: true })).toHaveCount(0)
  release(); await page.waitForTimeout(100)
  expect(state.writes).toHaveLength(0)
  expect(await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }))).not.toContain('PRIVATE UNSENT PLANNING REASON')
})
test('safe pending metadata survives private authority teardown and becomes unknown after an original response loss', async ({ page }) => {
  const state = await plannerFixture(page); await openPlanner(page); await fillCheckpoint(page)
  const review = await reviewCheckpoint(page)
  let release!: () => void; state.postBarrier = new Promise<void>(resolve => { release = resolve })
  await review.getByRole('button', { name: 'Save reviewed checkpoint window', exact: true }).click()
  await expect.poll(() => state.writes.length).toBe(1)
  state.denied = true; await page.evaluate(() => window.dispatchEvent(new Event('focus')))
  await expect(page.getByRole('textbox', { name: 'Checkpoint configuration reason', exact: true })).toHaveCount(0)
  const history = page.getByRole('complementary', { name: 'Unconfirmed command history', exact: true })
  await expect(history).toContainText('Awaiting original acknowledgement')
  state.loseNext = true; release()
  await expect(history).toContainText('Original outcome remains unconfirmed')
  expect(state.writes).toHaveLength(1); await expect(page.locator('body')).not.toContainText('PRIVATE UNSENT PLANNING REASON')
})

})

for (const canEdit of [true, false]) test(`participant mentor roster has no planner controls or privileged setup reads, organiser=${canEdit}`, async ({ page }, info) => {
  test.skip(info.project.metadata.portal !== 'regular', 'Participant portal reader contract')
  const state = await plannerFixture(page)
  state.source.event.can_edit = canEdit
  state.source.event.can_manage_owner = canEdit
  await page.goto(`/events/${firstID}?section=mentors`)
  await expect(page.getByRole('heading', { name: 'Your mentor coverage', exact: true })).toBeVisible()
  await expect(plannerRegion(page)).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Review mentor assessment', exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Review checkpoint window', exact: true })).toHaveCount(0)
  expect(state.reads).toEqual([]); expect(state.writes).toEqual([])
})

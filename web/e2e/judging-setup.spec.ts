import { test, expect, type Page } from '@playwright/test'
import { mockWorkspace, firstID, person } from './fixtures'
import { judgingSetupGuideSource } from '../src/styleguide/judging-setup-fixture'
import { rubricGuideConfiguration } from '../src/styleguide/rubric-fixture'
import { assignmentFingerprint, type JudgeAssignmentCommand } from '../src/judging-setup-model'
import type { components } from '../src/api/schema.gen'

async function setup(page: Page, existing = false) {
  const workspace = await mockWorkspace(page), source = judgingSetupGuideSource(existing), event = workspace.events[0]
  event.stage = 'preparation'; event.can_present = false; event.published_at = source.event.published_at; event.confirmed_attendees = 30
  source.event = event
  const configuration = { ...rubricGuideConfiguration(), ...source.configuration, approved: true }
  const writes: { path: string; body: Record<string, unknown>; revision: string | undefined }[] = [], reads: string[] = [], receipts = new Map<string, components['schemas']['JudgeReceipt']>()
  const state = { source, configuration, writes, reads, receipts, loseNext: false, badReceipt: false, deny: false, failRead: false, readBarrier: undefined as Promise<void> | undefined, responseBarrier: undefined as Promise<void> | undefined }
  await page.route(`**/api/v1/events/${firstID}/**`, async route => {
    const request = route.request(), path = new URL(request.url()).pathname
    const reject = (status: number) => route.fulfill({ status, json: { error: { code: status === 403 ? 'forbidden' : status === 409 ? 'revision_conflict' : 'unavailable', message: 'Synthetic current setup rejection', request_id: 'synthetic' } } })
    if (path.endsWith('/rubric/configuration')) {
      if (state.deny) return reject(403)
      if (request.method() === 'GET') { reads.push(path); if (state.readBarrier) await state.readBarrier; if (state.failRead) return reject(503); return route.fulfill({ json: configuration }).catch(() => {}) }
      const body = request.postDataJSON(); writes.push({ path, body, revision: request.headers()['if-match'] })
      if (request.headers()['if-match'] !== `"${configuration.revision}"`) return reject(409)
      if (configuration.runs.some(run => run.purpose === 'judge_final')) return reject(409)
      const planned = judgingSetupGuideSource(true)
      source.run = planned.run; source.round = planned.round; source.criteria = planned.criteria
      configuration.revision++; configuration.runs.push(planned.run!)
      if (state.loseNext) { state.loseNext = false; return route.abort('failed') }
      return route.fulfill({ json: configuration })
    }
    if (path.endsWith('/teams')) return route.fulfill({ json: source.teams })
    if (path.endsWith('/staff')) return state.deny ? reject(403) : route.fulfill({ json: source.staff })
    if (path.endsWith('/presentation-order')) return route.fulfill({ json: { revision: 0, override_active: false, items: [] } })
    if (path.endsWith('/judging')) return route.fulfill({ json: { items: source.round ? [source.round] : [] } })
    if (path.includes('/judging-receipts/')) return route.fulfill({ json: { receipt: receipts.get(path.split('/').at(-1)!) ?? null } })
    if (source.run && path.endsWith(`/judging/${source.run.id}`)) {
      if (state.deny) return reject(403)
      if (request.method() === 'GET') { reads.push(path); if (state.failRead) return reject(503); return route.fulfill({ json: source.round }) }
      const body = request.postDataJSON() as JudgeAssignmentCommand; writes.push({ path, body, revision: request.headers()['if-match'] })
      if (request.headers()['if-match'] !== `"${source.round!.revision}"`) return reject(409)
      const revision = source.round!.revision
      source.round!.revision++
      source.round!.assignments = source.round!.assignments.filter(item => item.team_id !== body.team_id).concat({ team_id: body.team_id, judge_ids: body.judge_ids, missing: Math.max(0, 3 - body.judge_ids.length) })
      const receipt = { request_id: body.request_id, actor_id: person.id, run_id: source.run.id, team_id: body.team_id, action: 'assign', revision: revision + 1, state: 'recorded', fingerprint: await assignmentFingerprint(source.run.id, revision, body) }
      receipts.set(body.request_id, receipt)
      if (state.responseBarrier) await state.responseBarrier
      if (state.loseNext) { state.loseNext = false; return route.abort('failed') }
      return route.fulfill({ json: state.badReceipt ? { ...receipt, actor_id: 'other-actor' } : receipt })
    }
    return route.fallback()
  })
  await page.goto(`/events/${firstID}/judging`)
  await expect(page.getByRole('region', { name: 'Organiser final judging setup', exact: true })).toBeVisible()
  return state
}
const setupRegion = (page: Page) => page.getByRole('region', { name: 'Organiser final judging setup', exact: true })
async function chooseJudges(page: Page, count = 3) {
  const section = setupRegion(page)
  await section.getByRole('combobox', { name: 'Team to assign', exact: true }).selectOption({ label: 'Aurora' })
  for (const name of ['Morgan', 'Sam', 'Alex'].slice(0, count)) await section.getByRole('checkbox', { name, exact: true }).check()
  await section.getByRole('textbox', { name: 'Assignment reason', exact: true }).fill('Reviewed independent judges for Aurora')
}
test('creates one exact approved final round, reviews named assignments and reloads native coverage', async ({ page }) => {
  const state = await setup(page), section = setupRegion(page)
  await section.getByRole('button', { name: 'Review final round creation', exact: true }).click()
  await expect(section.getByRole('combobox', { name: 'Approved rubric version', exact: true })).toBeFocused()
  expect(state.writes).toHaveLength(0)
  await section.getByRole('combobox', { name: 'Approved rubric version', exact: true }).selectOption({ label: 'Synthetic common rubric · Version 1' })
  await section.getByRole('textbox', { name: 'Round creation reason', exact: true }).fill('Plan independent final judging')
  const create = section.getByRole('button', { name: 'Review final round creation', exact: true })
  await create.press('Enter')
  const review = page.getByRole('alertdialog', { name: 'Create final judging round', exact: true })
  await expect(review).toContainText('Approved version 1')
  await review.getByRole('button', { name: 'Cancel', exact: true }).click(); await expect(create).toBeFocused(); expect(state.writes).toHaveLength(0)
  await create.press('Enter'); await review.getByRole('button', { name: 'Create reviewed final round', exact: true }).click()
  await expect(section.getByRole('combobox', { name: 'Team to assign', exact: true })).toBeVisible()
  expect(state.writes).toHaveLength(1); expect(state.writes[0].body).toEqual({ action: 'create_run', purpose: 'judge_final', version: 1, reason: 'Plan independent final judging' })
  await chooseJudges(page)
  await section.getByRole('button', { name: 'Review judge assignment', exact: true }).press('Enter')
  const assignment = page.getByRole('alertdialog', { name: 'Review final judge assignment', exact: true })
  await expect(assignment).toContainText('Aurora'); for (const name of ['Morgan', 'Sam', 'Alex']) await expect(assignment).toContainText(name)
  await assignment.getByRole('button', { name: 'Save reviewed judge assignment', exact: true }).click()
  await expect(section.getByRole('status', { name: 'Last acknowledged setup change', exact: true })).toContainText('assignment was acknowledged')
  expect(state.writes).toHaveLength(2); expect(state.writes[1].revision).toBe('"0"'); expect(state.source.run!.started_at).toBeNull()
  await page.reload(); await expect(section.getByRole('region', { name: 'Current final judge assignments', exact: true })).toContainText('Morgan, Sam, Alex')
  expect(state.writes).toHaveLength(2)
})
test('read failure sends nothing and stale reviewed setup cannot silently create a round', async ({ page }) => {
  const state = await setup(page), section = setupRegion(page)
  await section.getByRole('combobox', { name: 'Approved rubric version', exact: true }).selectOption({ index: 1 })
  await section.getByRole('textbox', { name: 'Round creation reason', exact: true }).fill('Retained private review')
  await section.getByRole('button', { name: 'Review final round creation', exact: true }).click()
  const review = page.getByRole('alertdialog', { name: 'Create final judging round', exact: true })
  state.failRead = true; await review.getByRole('button', { name: 'Create reviewed final round', exact: true }).click()
  await expect(review).toContainText('Nothing was sent'); expect(state.writes).toHaveLength(0)
  state.failRead = false; state.configuration.revision++
  await review.getByRole('button', { name: 'Create reviewed final round', exact: true }).click()
  await expect(review.getByRole('button', { name: 'Create reviewed final round', exact: true })).toBeDisabled(); expect(state.writes).toHaveLength(0)
})
test('lost original assignment stays unknown through current reads and exact recovery does not replay it', async ({ page }) => {
  const state = await setup(page, true), section = setupRegion(page)
  await chooseJudges(page); state.loseNext = true
  await section.getByRole('button', { name: 'Review judge assignment', exact: true }).click()
  await page.getByRole('alertdialog', { name: 'Review final judge assignment', exact: true }).getByRole('button', { name: 'Save reviewed judge assignment', exact: true }).click()
  await expect(section).toContainText('original change outcome is unknown')
  await section.getByRole('button', { name: 'Read current judging setup', exact: true }).click()
  await expect(section).toContainText('original change outcome is unknown'); expect(state.writes).toHaveLength(1)
  await section.getByRole('button', { name: 'Read original assignment acknowledgement', exact: true }).click()
  await expect(section.getByRole('status', { name: 'Last acknowledged setup change', exact: true })).toContainText('exact original assignment acknowledgement was recovered')
  expect(state.writes).toHaveLength(1)
})
test('current denial clears a held private review and sends no assignment', async ({ page }) => {
  const state = await setup(page, true), section = setupRegion(page)
  await chooseJudges(page)
  await section.getByRole('button', { name: 'Review judge assignment', exact: true }).click()
  state.deny = true
  await page.getByRole('alertdialog', { name: 'Review final judge assignment', exact: true }).getByRole('button', { name: 'Save reviewed judge assignment', exact: true }).click()
  await expect(page.getByRole('alertdialog', { name: 'Review final judge assignment', exact: true })).toHaveCount(0)
  await expect(section.getByRole('textbox', { name: 'Assignment reason', exact: true })).toHaveCount(0)
  await expect(section).not.toContainText('Reviewed independent judges for Aurora'); expect(state.writes).toHaveLength(0)
})

test('lost creation is not inferred from a current run and a reviewed new base never replays creation', async ({ page }) => {
  const state = await setup(page), section = setupRegion(page)
  await section.getByRole('combobox', { name: 'Approved rubric version', exact: true }).selectOption({ index: 1 })
  await section.getByRole('textbox', { name: 'Round creation reason', exact: true }).fill('Unknown creation remains distinct')
  state.loseNext = true
  await section.getByRole('button', { name: 'Review final round creation', exact: true }).click()
  await page.getByRole('alertdialog', { name: 'Create final judging round', exact: true }).getByRole('button', { name: 'Create reviewed final round', exact: true }).click()
  await expect(section).toContainText('original change outcome is unknown')
  await section.getByRole('button', { name: 'Read current judging setup', exact: true }).click()
  await expect(section.getByRole('combobox', { name: 'Team to assign', exact: true })).toBeDisabled()
  await expect(section).toContainText('original change outcome is unknown')
  await section.getByRole('button', { name: 'Review current setup as a new starting point', exact: true }).click()
  const fresh = page.getByRole('alertdialog', { name: 'Review current judging starting point', exact: true })
  await expect(fresh).toContainText('Earlier unknown requests remain unknown')
  await fresh.getByRole('button', { name: 'Use reviewed current setup', exact: true }).click()
  await expect(section.getByRole('combobox', { name: 'Team to assign', exact: true })).toBeEnabled()
  await expect(page.getByRole('complementary', { name: 'Unconfirmed command history', exact: true })).toContainText('Original outcome remains unconfirmed')
  await expect(section.getByRole('button', { name: 'Review final round creation', exact: true })).toHaveCount(0)
  expect(state.writes).toHaveLength(1)
})
test('partial coverage requires deliberate acknowledgement and retains its actual unfilled count', async ({ page }) => {
  const state = await setup(page, true), section = setupRegion(page)
  await chooseJudges(page, 1)
  await section.getByRole('button', { name: 'Review judge assignment', exact: true }).click()
  const gap = section.getByRole('checkbox', { name: 'I understand this leaves 2 required judge places unfilled', exact: true })
  await expect(gap).toBeFocused(); expect(state.writes).toHaveLength(0)
  await gap.check(); await section.getByRole('button', { name: 'Review judge assignment', exact: true }).click()
  const review = page.getByRole('alertdialog', { name: 'Review final judge assignment', exact: true })
  await expect(review).toContainText('Unfilled places explicitly acknowledged')
  await review.getByRole('button', { name: 'Save reviewed judge assignment', exact: true }).click()
  await expect(section.getByRole('status', { name: 'Last acknowledged setup change', exact: true })).toContainText('assignment was acknowledged')
  expect(state.source.round!.assignments[0].missing).toBe(2); expect(state.writes).toHaveLength(1)
})
test('mismatched original receipts cannot settle an accepted assignment', async ({ page }) => {
  const state = await setup(page, true), section = setupRegion(page)
  await chooseJudges(page); state.badReceipt = true
  await section.getByRole('button', { name: 'Review judge assignment', exact: true }).click()
  await page.getByRole('alertdialog', { name: 'Review final judge assignment', exact: true }).getByRole('button', { name: 'Save reviewed judge assignment', exact: true }).click()
  await expect(section).toContainText('original change outcome is unknown')
  const original = [...state.receipts.values()][0]
  state.receipts.set(original.request_id, { ...original, fingerprint: 'b'.repeat(64) })
  await section.getByRole('button', { name: 'Read original assignment acknowledgement', exact: true }).click()
  await expect(section).toContainText('No matching original assignment acknowledgement is available')
  await expect(section).toContainText('original change outcome is unknown'); expect(state.writes).toHaveLength(1)
})
test('authority loss fences a held preflight without retaining private review or sending a command', async ({ page }) => {
  const state = await setup(page, true), section = setupRegion(page)
  await chooseJudges(page)
  await section.getByRole('button', { name: 'Review judge assignment', exact: true }).click()
  let release!: () => void
  state.readBarrier = new Promise<void>(resolve => { release = resolve })
  const held = page.waitForRequest(request => new URL(request.url()).pathname.endsWith('/rubric/configuration'))
  await page.getByRole('alertdialog', { name: 'Review final judge assignment', exact: true }).getByRole('button', { name: 'Save reviewed judge assignment', exact: true }).click()
  await held; state.source.event.can_edit = false
  await page.evaluate(() => window.dispatchEvent(new Event('focus')))
  await expect(page.getByRole('alertdialog', { name: 'Review final judge assignment', exact: true })).toHaveCount(0)
  release(); state.readBarrier = undefined
  await expect(page.getByRole('textbox', { name: 'Assignment reason', exact: true })).toHaveCount(0)
  expect(state.writes).toHaveLength(0)
})

test('safe pending and unknown setup metadata survives private authority teardown without restoring the reason', async ({ page }) => {
  const state = await setup(page, true), section = setupRegion(page)
  await chooseJudges(page); state.loseNext = true
  let release!: () => void
  state.responseBarrier = new Promise<void>(resolve => { release = resolve })
  await section.getByRole('button', { name: 'Review judge assignment', exact: true }).click()
  const sent = page.waitForRequest(request => request.method() === 'POST' && new URL(request.url()).pathname.includes('/judging/'))
  await page.getByRole('alertdialog', { name: 'Review final judge assignment', exact: true }).getByRole('button', { name: 'Save reviewed judge assignment', exact: true }).click()
  await sent; state.source.event.can_edit = false
  await page.evaluate(() => window.dispatchEvent(new Event('focus')))
  await expect(page.getByRole('alertdialog', { name: 'Review final judge assignment', exact: true })).toHaveCount(0)
  const history = page.getByRole('complementary', { name: 'Unconfirmed command history', exact: true })
  await expect(history).toContainText('Awaiting original acknowledgement')
  await expect(page.getByRole('textbox', { name: 'Assignment reason', exact: true })).toHaveCount(0)
  release(); state.responseBarrier = undefined
  await expect(history).toContainText('Original outcome remains unconfirmed')
  await expect(page.locator('body')).not.toContainText('Reviewed independent judges for Aurora')
  expect(state.writes).toHaveLength(1)
})

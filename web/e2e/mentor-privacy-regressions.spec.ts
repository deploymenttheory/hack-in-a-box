import { expect, test, type Page } from '@playwright/test'
import { firstID } from './fixtures'
import { helpState, mockHelp } from './help-workspace-fixtures'
import { fixture as mentorFixture, openNew, typeNote, saveObservation } from './mentor-workspace-fixtures'
import { fixture as cardFixture, open, score, reviewCard, team as cardTeam } from './scorecard-workspace-fixtures'
import { clarificationFixture, feedPath, questionPath, questionID, teamID } from './clarification-workspace-fixtures'
import type { components } from '../src/api/schema.gen'

function held() { let release!: () => void; return { gate: new Promise<void>(resolve => { release = resolve }), release: () => release() } }
const history = (page: Page) => page.getByRole('complementary', { name: 'Unconfirmed command history' })

test('held help POST keeps a minimal replay barrier after current team permission loss and restoration', async ({ page }) => {
  const state = helpState(); await mockHelp(page, state); await page.clock.install(); await page.goto(`/events/${firstID}/help`)
  const wait = held(); let posts = 0, replied = false
  await page.route(`**/api/v1/events/${firstID}/help/*`, async route => {
    if (route.request().method() !== 'PUT') return route.fallback()
    posts++; expect(route.request().headers()['if-match']).toBe('"0"'); expect(route.request().headers()['idempotency-key']).toMatch(/^[a-f0-9-]{36}$/i)
    await wait.gate; await route.fulfill({ status: 503, json: { error: { code: 'unavailable' } } }); replied = true
  })
  await page.getByLabel('What do you need help with?').fill('PRIVATE-HELD-HELP-INPUT')
  await page.getByLabel('Location (or an approved meeting route required)', { exact: true }).fill('Private synthetic desk')
  await page.getByRole('button', { name: 'Review help request', exact: true }).click()
  await page.getByRole('button', { name: 'Submit help request', exact: true }).click()
  await expect.poll(() => posts).toBe(1)
  state.switched = true; await page.clock.fastForward(16_000)
  await expect(page.getByLabel('What do you need help with?')).toHaveCount(0)
  await expect(history(page)).toContainText('Create help request')
  await expect(page.locator('body')).not.toContainText('PRIVATE-HELD-HELP-INPUT')
  wait.release(); await expect.poll(() => replied).toBe(true)
  state.switched = false; await page.clock.fastForward(16_000)
  await expect(page.getByRole('heading', { name: 'Outcome unknown', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Check original outcome', exact: true }).click()
  await expect(history(page)).toContainText('Original outcome remains unconfirmed')
  await expect(page.getByRole('button', { name: 'Submit help request', exact: true })).toHaveCount(0)
  expect(posts).toBe(1)
})

test('held observation POST survives a formal capability loss only as minimal unresolved metadata', async ({ page }) => {
  const state = await mentorFixture(page); await page.clock.install(); await openNew(page); await typeNote(page, 'PRIVATE-HELD-OBSERVATION')
  const wait = held(); let posts = 0, replied = false
  await page.route(`**/events/${firstID}/teams/*/mentor-observations`, async route => {
    if (route.request().method() !== 'POST') return route.fallback()
    posts++; expect(route.request().postDataJSON().evidence.evidence).toBe('PRIVATE-HELD-OBSERVATION')
    await wait.gate; await route.fulfill({ status: 503, json: { error: { code: 'unavailable' } } }); replied = true
  })
  await saveObservation(page); await expect.poll(() => posts).toBe(1)
  state.open = false; await page.clock.fastForward(16_000)
  await expect(page.getByLabel('Evidence (private)', { exact: true })).toHaveCount(0)
  await expect(history(page)).toContainText('Private observation')
  await expect(page.locator('body')).not.toContainText('PRIVATE-HELD-OBSERVATION')
  wait.release(); await expect.poll(() => replied).toBe(true)
  state.open = true; await page.clock.fastForward(16_000)
  await expect(page.getByRole('heading', { name: 'Outcome unknown', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Check whether this note was saved', exact: true }).click()
  await expect(history(page)).toContainText('Original outcome remains unconfirmed')
  await expect(page.getByRole('button', { name: 'Save private observation', exact: true })).toHaveCount(0)
  expect(posts).toBe(1)
})

test('held scorecard POST retains its original barrier when only the parent checkpoint collection returns', async ({ page }) => {
  const state = await cardFixture(page); await page.clock.install(); await open(page); await score(page, '4')
  await page.getByLabel('Private evidence note for Technical merit').fill('PRIVATE-HELD-SCORECARD')
  const wait = held(); let posts = 0, replied = false
  await page.route(`**/events/${firstID}/teams/${cardTeam}/scorecards`, async route => {
    if (route.request().method() !== 'POST') return route.fallback()
    posts++; expect(route.request().headers()['if-match']).toBe('"10"')
    await wait.gate; await route.fulfill({ status: 503, json: { error: { code: 'unavailable' } } }); replied = true
  })
  await reviewCard(page, 'Save private draft'); await expect.poll(() => posts).toBe(1)
  state.denied = true; await page.clock.fastForward(16_000)
  await expect(page.getByRole('region', { name: 'Your private scorecard' })).toHaveCount(0)
  await expect(history(page)).toContainText('Private scorecard')
  await expect(page.locator('body')).not.toContainText('PRIVATE-HELD-SCORECARD')
  wait.release(); await expect.poll(() => replied).toBe(true)
  state.denied = false; await page.getByRole('button', { name: 'Read current checkpoints', exact: true }).click()
  await page.getByRole('button', { name: 'Open Technical checkpoint', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Outcome unknown', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Check original outcome', exact: true }).click()
  await expect(history(page)).toContainText('Original outcome remains unconfirmed')
  await expect(page.getByRole('button', { name: 'Save private draft', exact: true })).toHaveCount(0)
  expect(posts).toBe(1)
})

test('a held common-answer POST loses private text on review loss while its original intent remains fenced', async ({ page }) => {
  const fixture = await clarificationFixture(page, 'staff'); await page.clock.install(); await page.goto(questionPath)
  await page.getByLabel('Common answer title').fill('Private held draft')
  await page.getByLabel('Common official answer text').fill('PRIVATE-HELD-COMMON-ANSWER')
  const wait = held(); let posts = 0, replied = false
  await page.route(`**/events/${firstID}/questions/${questionID}/answer`, async route => {
    posts++; expect(route.request().headers()['if-match']).toBe('"0"')
    await wait.gate; await route.fulfill({ status: 503, json: { error: { code: 'unavailable' } } }); replied = true
  })
  await page.getByRole('button', { name: 'Review draft version', exact: true }).click()
  await page.getByRole('button', { name: 'Save a new draft version', exact: true }).click()
  await expect.poll(() => posts).toBe(1)
  fixture.state.reviewAllowed = false; await page.clock.fastForward(11_000)
  await expect(page.getByLabel('Common official answer text')).toHaveCount(0)
  await expect(page.locator('body')).not.toContainText('PRIVATE-HELD-COMMON-ANSWER')
  await expect(history(page)).toContainText('Common answer save')
  wait.release(); await expect.poll(() => replied).toBe(true)
  fixture.state.reviewAllowed = true
  await page.getByRole('button', { name: 'Try again', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Outcome unknown', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Check the exact server outcome', exact: true }).click()
  await expect(history(page)).toContainText('Original outcome remains unconfirmed')
  await expect(page.locator('body')).not.toContainText('PRIVATE-HELD-COMMON-ANSWER')
  expect(posts).toBe(1)
})

test('a fresh restricted answer projection during confirmation cannot send an old private draft', async ({ page }) => {
  const fixture = await clarificationFixture(page, 'staff'); await page.goto(questionPath)
  await page.getByLabel('Common answer title').fill('Private source review')
  await page.getByLabel('Common official answer text').fill('PRIVATE-HELD-ANSWER-PREFLIGHT')
  await page.getByRole('button', { name: 'Review draft version', exact: true }).click()
  const wait = held(); let reads = 0
  await page.route(`**/events/${firstID}/questions/${questionID}/review`, async route => {
    reads++; await wait.gate
    const workspace: components['schemas']['ClarificationWorkspace'] = { source: fixture.state.source, archived: false, team_id: null, can_ask: false, can_review: false, staff_attachments_open: false, review_audience: null }
    await route.fulfill({ json: { draft: null, workspace } })
  })
  await page.getByRole('button', { name: 'Save a new draft version', exact: true }).click()
  await expect.poll(() => reads).toBe(1); wait.release()
  await expect(page.getByLabel('Common official answer text')).toHaveCount(0)
  await expect(page.locator('body')).not.toContainText('PRIVATE-HELD-ANSWER-PREFLIGHT')
  expect(fixture.state.saves).toHaveLength(0)
  await expect(history(page)).toHaveCount(0)
})

test('attachment preflight observes current team loss after file reading and never posts its retained bytes', async ({ page }) => {
  const fixture = await clarificationFixture(page); await page.goto(feedPath)
  await page.getByRole('button', { name: "Your team's private questions", exact: true }).click()
  await page.getByLabel('Your question', { exact: true }).fill('PRIVATE-HELD-ATTACHMENT-QUESTION')
  await page.getByLabel('Choose a private attachment').setInputFiles({ name: 'private-held.txt', mimeType: 'text/plain', buffer: Buffer.from('PRIVATE-HELD-BYTES') })
  const wait = held(); let eventRead = false, revoked = false
  await page.route(`**/api/v1/events/${firstID}`, async route => { eventRead = true; await wait.gate; await route.fulfill({ json: fixture.events[0] }) })
  await page.route(`**/events/${firstID}/clarification-workspace`, route => route.fulfill({ json: { source: fixture.state.source, archived: false, team_id: revoked ? null : teamID, can_ask: !revoked, can_review: false, staff_attachments_open: false, review_audience: null } }))
  await page.getByRole('button', { name: 'Upload private attachment', exact: true }).click()
  await expect.poll(() => eventRead).toBe(true); revoked = true; wait.release()
  await expect(page.getByLabel('Your question', { exact: true })).toHaveCount(0)
  await expect(page.locator('body')).not.toContainText('PRIVATE-HELD-ATTACHMENT-QUESTION')
  expect(fixture.state.uploads).toHaveLength(0); expect(fixture.state.asks).toHaveLength(0)
  await expect(history(page)).toHaveCount(0)
})

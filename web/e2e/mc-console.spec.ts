import { openEventSection, expectEventSection } from './event-navigation-fixture'
import { test, expect, type Page } from '@playwright/test'
import { firstID, mockWorkspace, person } from './fixtures'
import type { components } from '../src/api/schema.gen'

type Cue = components['schemas']['PresentationCue']
type ConsolePage = components['schemas']['PresentationPage']
type Command = components['schemas']['PresentationCommand']
const cue = (id: string, title: string): Cue => ({ id, title, text: 'Synthetic public room cue', url: '', duration_seconds: 120, content_id: null })
const opening = cue('a0b7bcb4-39a2-4be6-a558-b5cedbab1857', 'Planned opening'), building = cue('c0789650-02c6-4212-a464-97b70e9cb74c', 'Planned building')
function fixture() {
  return { page: { revision: 0, can_control: true, server_now: new Date().toISOString(), timezone: 'Europe/London', draft: [opening, building], published: [opening, building], current_index: 0, mode: 'running', remaining_seconds: 120, timer_ends_at: new Date(Date.now() + 120_000).toISOString(), override: null, override_mode: 'held', override_remaining_seconds: 0, override_timer_ends_at: null, pairings: [] } as ConsolePage, requests: [] as { body: Command; revision: string }[], authorized: true }
}
async function prepare(page: Page, data = fixture(), pausePolling = false) {
  const workspace = await mockWorkspace(page), event = workspace.events[0]
  event.can_present = true; event.can_edit = false; event.can_read_staff = false; event.can_manage_owner = false; event.can_transition = false
  await page.route('**/api/v1/session', route => route.fulfill({ json: { person: { ...person, platform_admin: false }, csrf_token: 'synthetic-csrf-token-32-characters', can_create_events: false } }))
  const original = structuredClone(data.page)
  await page.route(`**/events/${firstID}/presentation**`, async route => {
    const req = route.request(), path = new URL(req.url()).pathname
    const fail = (status: number, code: string) => route.fulfill({ status, json: { error: { code, message: 'Synthetic current-authority rejection', request_id: 'synthetic' } } })
    if (!data.authorized) return fail(403, 'forbidden')
    if (req.method() === 'GET') return route.fulfill({ json: pausePolling ? original : data.page })
    const body = req.postDataJSON(), revision = req.headers()['if-match']
    data.requests.push({ body, revision }); pausePolling = false
    if (revision !== `"${data.page.revision}"`) return fail(409, 'revision_conflict')
    if (!body.reason || !req.headers()['x-csrf-token']) return fail(400, 'invalid')
    data.page.revision++; data.page.server_now = new Date().toISOString()
    if (path.endsWith('/pairings')) {
      const pairing = { id: '47b85018-a096-4f82-af26-7240ec37e6ce', label: body.label, created_at: data.page.server_now, expires_at: new Date(Date.now() + body.minutes * 60_000).toISOString(), revoked_at: null, last_seen_at: null, stale: true }
      data.page.pairings.push(pairing)
      return route.fulfill({ status: 201, json: { pairing, revision: data.page.revision, token: 'SyntheticDisplayOnlySecretNeverUseOutsideTests123456789' } })
    }
    if (path.endsWith('/revoke')) { data.page.pairings[0].revoked_at = data.page.server_now; return route.fulfill({ json: data.page }) }
    switch (body.action) {
      case 'save': data.page.draft = body.cues; break
      case 'activate': if (body.from_draft) data.page.published = structuredClone(data.page.draft); data.page.current_index = body.index; data.page.remaining_seconds = data.page.published[body.index].duration_seconds; data.page.mode = 'running'; break
      case 'advance': data.page.current_index++; break
      case 'back': data.page.current_index--; break
      case 'hold': if (data.page.override) data.page.override_mode = 'held'; else data.page.mode = 'held'; break
      case 'resume': if (data.page.override) data.page.override_mode = 'running'; else data.page.mode = 'running'; break
      case 'timer': if (data.page.override) data.page.override_remaining_seconds = body.seconds; else data.page.remaining_seconds = body.seconds; break
      case 'override': data.page.override = body.override; data.page.override_mode = 'running'; data.page.override_remaining_seconds = body.override.duration_seconds; break
      case 'return_to_schedule': data.page.override = null; break
    }
    return route.fulfill({ json: data.page })
  })
  await page.goto(`/events/${firstID}`)
  await openEventSection(page, 'MC console')
  await expect(page.getByRole('heading', { name: 'MC console', exact: true })).toBeVisible()
  await page.getByLabel('Reason for room changes').fill('Synthetic reviewed room operation')
  return data
}
const live = (page: Page) => page.getByRole('region', { name: 'Live room controls' })

test('rehearses and saves without changing live cues, then explicitly activates the saved program', async ({ page }, info) => {
  const data = await prepare(page)
  await expectEventSection(page,'Setup',false)
  await page.getByRole('button', { name: 'Rehearse 2: Planned building', exact: true }).click()
  await expect(live(page).getByRole('heading', { name: 'Planned opening', exact: true })).toBeVisible()
  const hostile = '<img src=x onerror="window.__injected=true"> Synthetic rehearsal'
  await page.getByLabel('Cue title', { exact: true }).fill(hostile)
  await page.getByLabel('Room text', { exact: true }).fill('A deliberately long synthetic word '.repeat(40))
  await page.getByLabel('HTTPS room link').fill('javascript:alert(1)')
  await expect(page.getByRole('button', { name: 'Save rehearsal', exact: true })).toBeDisabled()
  await page.getByLabel('HTTPS room link').fill(`https://example.org/${'long-synthetic-room-path-'.repeat(15)}`)
  expect(data.requests).toHaveLength(0)
  await expect(page.getByRole('button', { name: 'Activate saved rehearsal' })).toBeDisabled()
  await page.getByRole('button', { name: 'Save rehearsal', exact: true }).click()
  await expect(page.getByRole('status')).toContainText('The room display is unchanged.')
  expect(data.requests[0].body.action).toBe('save')
  expect(data.page.published[0].title).toBe('Planned opening')
  await expect(live(page).getByRole('heading', { name: 'Planned opening', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Activate saved rehearsal' }).click()
  await expect(live(page).getByRole('heading', { name: hostile, exact: true })).toBeVisible()
  expect(data.requests.at(-1)?.body).toMatchObject({ action: 'activate', from_draft: true, index: 1 })
  expect(await page.evaluate(() => (window as unknown as { __injected?: boolean }).__injected)).toBeUndefined()
  await expect(page.locator('.mc-console img')).toHaveCount(0)
  await expect(page.locator('.mc-console').getByRole('combobox')).toHaveCount(0)
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
  await page.screenshot({ path: info.outputPath('mc-rehearsal-activated.png'), fullPage: true })
})

test('holds, resumes, adjusts and overrides the countdown while preserving an explicit return to schedule', async ({ page }, info) => {
  const data = await prepare(page)
  await page.getByRole('button', { name: 'Hold countdown' }).click()
  await expect(live(page)).toContainText('Countdown held')
  await page.getByLabel('Set remaining seconds').fill('65')
  await page.getByRole('button', { name: 'Set countdown', exact: true }).click()
  await expect(page.getByRole('timer')).toHaveText('01:05')
  await page.getByRole('button', { name: 'Resume countdown' }).click()
  await expect(live(page)).toContainText('Countdown running')
  await page.getByRole('button', { name: 'Advance', exact: true }).click()
  await expect(live(page).getByRole('heading', { name: 'Planned building' })).toBeVisible()
  await page.getByRole('button', { name: 'Back', exact: true }).click()
  await page.getByLabel('Urgent title').fill('Urgent room update')
  await page.getByLabel('Urgent room text').fill('Synthetic announcement only')
  await page.getByRole('button', { name: 'Activate urgent announcement' }).click()
  await expect(live(page).getByRole('heading', { name: 'Urgent room update' })).toBeVisible()
  await expect(live(page).getByRole('heading', { name: 'Planned opening' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Advance', exact: true })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Activate saved rehearsal' })).toBeDisabled()
  await page.screenshot({ path: info.outputPath('mc-urgent-return.png'), fullPage: true })
  await page.getByRole('button', { name: 'Return to schedule', exact: true }).click()
  await expect(live(page).getByRole('heading', { name: 'Planned opening', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Return to schedule', exact: true })).toHaveCount(0)
  expect(data.page.current_index).toBe(0); expect(data.page.published).toEqual([opening, building]); expect(data.page.remaining_seconds).toBe(65)
})

test('two consoles preserve a rejected rehearsal and require review before another mutation', async ({ page, context }, info) => {
  const data = await prepare(page), other = await context.newPage()
  await prepare(other, data, true)
  await other.getByLabel('Cue title', { exact: true }).fill('Preserved competing rehearsal')
  await page.getByRole('button', { name: 'Hold countdown' }).click()
  await other.getByRole('button', { name: 'Save rehearsal', exact: true }).click()
  await expect(other.getByRole('heading', { name: 'Another console changed the program' })).toBeVisible()
  await expect(other.getByLabel('Cue title', { exact: true })).toHaveValue('Preserved competing rehearsal')
  await expect(other.getByRole('button', { name: 'Save rehearsal', exact: true })).toBeDisabled()
  const before = data.requests.length
  await other.screenshot({ path: info.outputPath('mc-conflict-preserved.png'), fullPage: true })
  expect(data.requests).toHaveLength(before)
  await other.getByRole('button', { name: 'Use latest revision and keep my rehearsal' }).click()
  await other.getByRole('button', { name: 'Save rehearsal', exact: true }).click()
  await expect(other.getByRole('status')).toContainText('Rehearsal saved')
  expect(data.requests.map(r => r.revision)).toEqual(['"0"', '"0"', '"1"'])
  expect(data.page.draft[0].title).toBe('Preserved competing rehearsal')
  await other.close()
})

test('shows factual display contact, returns a secret once without putting it in URLs or browser storage, and revokes it', async ({ page }, info) => {
  const data = await prepare(page), urls: string[] = []
  page.on('request', request => urls.push(request.url()))
  await page.getByLabel('Display label', { exact: true }).fill('Synthetic front room')
  await page.getByRole('button', { name: 'Pair display', exact: true }).click()
  const secret = 'SyntheticDisplayOnlySecretNeverUseOutsideTests123456789'
  await expect(page.getByRole('textbox', { name: 'New display secret', exact: true })).toHaveValue(secret)
  await expect(page.getByRole('region', { name: 'Display sessions' })).toContainText('No contact yet')
  expect(urls.some(url => url.includes(secret))).toBe(false)
  expect(await page.evaluate(value => [...Object.values(localStorage), ...Object.values(sessionStorage)].some(item => item.includes(value)), secret)).toBe(false)
  await page.getByRole('button', { name: 'Dismiss display secret' }).click()
  await expect(page.getByRole('textbox', { name: 'New display secret', exact: true })).toHaveCount(0)
  data.page.pairings[0].last_seen_at = new Date(Date.now() - 60_000).toISOString(); data.page.pairings[0].stale = true
  await expect(page.getByRole('region', { name: 'Display sessions' })).toContainText('Contact stale')
  await page.screenshot({ path: info.outputPath('mc-display-contact.png'), fullPage: true })
  await page.getByRole('button', { name: 'Revoke Synthetic front room' }).click()
  await expect(page.getByRole('region', { name: 'Display sessions' })).toContainText('Revoked')
  expect(data.page.pairings[0].revoked_at).toBeTruthy()
})

test('retains the rehearsal but disables controls when current MC access is revoked', async ({ page }) => {
  const data = await prepare(page)
  await page.getByLabel('Cue title', { exact: true }).fill('Draft retained after revocation')
  data.authorized = false
  await expect(page.getByText('The latest console snapshot could not be verified.', { exact: false })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Save rehearsal', exact: true })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Hold countdown' })).toBeDisabled()
  await expect(page.getByLabel('Cue title', { exact: true })).toHaveValue('Draft retained after revocation')
  expect(data.requests).toHaveLength(0)
})


test('jumps only to approved cues and finishes the planned program explicitly', async ({ page }) => {
  const data = await prepare(page)
  await page.getByText('Approved cue list', { exact: true }).click()
  await page.getByRole('button', { name: 'Activate approved cue 2', exact: true }).click()
  await expect(live(page).getByRole('heading', { name: 'Planned building', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Finish program', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Advance', exact: true })).toBeDisabled()
  expect(data.requests.map(request => request.body.action)).toEqual(['activate', 'advance'])
  expect(data.page.current_index).toBe(2)
})

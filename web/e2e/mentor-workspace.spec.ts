import { expect, test } from '@playwright/test'
import { firstID, person } from './fixtures'
import { teamIDs } from './mentor-fixtures'
import { fixture, saved, noteID, openNew, typeNote, restore, copies, leaveTask, saveObservation, saveStatus } from './mentor-workspace-fixtures'

test('mobile mentor records private evidence and operational check-in through the native contract', async ({ page }, info) => {
  const state = await fixture(page); await page.goto(`/events/${firstID}/mentor`)
  await expect(page.getByRole('heading', { name: 'Your assigned teams' })).toBeVisible()
  await openNew(page); await typeNote(page); await page.getByLabel('Feedback reference (private, not locally recovered)').fill('feedback-ref')
  await expect(page.getByRole('status', { name: 'Observation save state' })).toContainText('Unsent local copy only')
  await page.screenshot({ path: info.outputPath('mentor-private-mobile-entry.png'), fullPage: true })
  await saveObservation(page)
  await expect(page.getByRole('status', { name: 'Observation save state' })).toContainText('Saved on the server')
  expect(await copies(page)).toBe(0); expect(state.commands[0].evidence?.evidence).toContain('<script>')
  await page.getByLabel('Check-in reason', { exact: true }).fill('Integration currently blocked')
  await page.getByLabel('Next action (mentor/organiser-only)').fill('Pair on the test fixture')
  await saveStatus(page)
  await expect(page.getByText('Integration currently blocked', { exact: true })).toBeVisible()
  await expect.poll(() => state.commands.map(c => c.action)).toEqual(['observation', 'status'])
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
})

test('refresh and team switching require deliberate current-authorised recovery of actual typed text', async ({ page }) => {
  const state = await fixture(page); await openNew(page); await typeNote(page)
  await leaveTask(page, () => page.reload()); await expect(page.getByLabel('Evidence (private)', { exact: true })).toHaveValue('')
  await restore(page); await expect(page.getByLabel('Evidence (private)', { exact: true })).toHaveValue('UNSENT PRIVATE <script> literal evidence')
  await leaveTask(page, () => page.getByRole('navigation', { name: 'Your assigned teams' }).getByRole('link', { name: /South Stars/ }).click())
  await expect(page.getByText('UNSENT PRIVATE <script> literal evidence', { exact: true })).toHaveCount(0)
  await openNew(page); await restore(page); await expect(page.getByLabel('Evidence (private)', { exact: true })).toHaveValue('UNSENT PRIVATE <script> literal evidence')
  expect(state.commands).toHaveLength(0)
})

test('interrupted save remains unconfirmed, never auto-replays, and requires a deliberate reconnect choice', async ({ page }) => {
  const state = await fixture(page); await openNew(page); await typeNote(page); state.failWrite = true
  await saveObservation(page)
  await expect(page.getByRole('status', { name: 'Observation save state' })).toContainText('Server save outcome is unconfirmed')
  expect(await copies(page)).toBe(1)
  state.failWrite = false; state.revision++
  await leaveTask(page, () => page.reload()); await restore(page)
  await saveObservation(page)
  await expect(page.getByRole('status', { name: 'Observation save state' })).toContainText('The server changed')
  expect(state.commands).toHaveLength(1)
  await page.getByRole('button', { name: 'Compare current server version' }).click()
  await saveObservation(page)
  await expect(page.getByRole('status', { name: 'Observation save state' })).toContainText('Saved on the server'); expect(state.commands).toHaveLength(2)
})

test('accepted write with a lost response is reconciled by exact note identity without duplicate writes', async ({ page }) => {
  const state = await fixture(page); await openNew(page); await typeNote(page); state.acceptedButLost = true
  await saveObservation(page)
  await expect(page.getByRole('status', { name: 'Observation save state' })).toContainText('Server save outcome is unconfirmed')
  state.acceptedButLost = false
  await page.getByRole('button', { name: 'Check whether this note was saved' }).click()
  await expect(page.getByRole('status', { name: 'Observation save state' })).toContainText('Confirmed this exact text is saved')
  expect(state.commands).toHaveLength(1); expect(await copies(page)).toBe(0)
})

test('existing note beyond the first current page uses its exact saved version and fresh conflicts cannot be replayed', async ({ page }) => {
  const state = await fixture(page); state.notes = [saved(noteID, 7)]; state.firstPageEmpty = true
  await page.goto(`/events/${firstID}/mentor/${teamIDs[0]}?note=${noteID}`)
  await page.getByRole('button', { name: 'Load saved server text (replace editor)' }).click()
  await page.getByLabel('Evidence (private)', { exact: true }).fill('Edited version seven')
  await saveObservation(page)
  await expect(page.getByRole('status', { name: 'Observation save state' })).toContainText('Saved on the server')
  expect(state.commands[0].version).toBe(7); expect(state.exactReads).toBeGreaterThan(0)
  await page.getByLabel('Evidence (private)', { exact: true }).fill('Another proposed change')
  state.revision++; state.notes[0].version++
  await saveObservation(page)
  await expect(page.getByRole('status', { name: 'Observation save state' })).toContainText('The server changed'); expect(state.commands).toHaveLength(1)
})

test('recovered pre-live denial and direct current resource denial clear actual private editor text', async ({ page }) => {
  const state = await fixture(page); await openNew(page); await typeNote(page); await leaveTask(page, () => page.reload()); await restore(page)
  state.open = false
  await page.getByRole('button', { name: 'Check for local text', exact: true }).click()
  await expect(page.getByLabel('Evidence (private)', { exact: true })).toHaveValue(''); expect(await copies(page)).toBe(0)
  expect(state.commands).toHaveLength(0)
  state.open = true; await leaveTask(page, () => page.reload()); await typeNote(page); state.denied = true
  await page.getByRole('button', { name: 'Compare current server version' }).click()
  await expect(page.getByLabel('Evidence (private)', { exact: true })).toHaveValue(''); expect(await copies(page)).toBe(0)
})

test('a second concurrent change after comparison requires another deliberate choice', async ({ page }) => {
  const state = await fixture(page); await openNew(page); await typeNote(page); state.revision++
  await page.getByRole('button', { name: 'Compare current server version' }).click(); state.revision++
  await saveObservation(page)
  await expect(page.getByRole('status', { name: 'Observation save state' })).toContainText('The server changed'); expect(state.commands).toHaveLength(0)
})

 test('reference-only saved-note edit creates a text-only local copy and saves the new reference', async ({ page }) => {
  const state = await fixture(page); state.notes = [saved()]; await page.goto(`/events/${firstID}/mentor/${teamIDs[0]}?note=${noteID}`)
  await page.getByRole('button', { name: 'Load saved server text (replace editor)' }).click()
  await page.getByLabel('Feedback reference (private, not locally recovered)').fill('changed-reference-only')
  expect(await copies(page)).toBe(1)
  const stored = await page.evaluate(() => Object.keys(sessionStorage).filter(k => k.startsWith('hackbox:mentor-text:')).map(k => sessionStorage.getItem(k)).join(''))
  expect(stored).not.toContain('changed-reference-only')
  await saveObservation(page)
  await expect(page.getByRole('status', { name: 'Observation save state' })).toContainText('Saved on the server')
  expect(state.commands[0].evidence?.feedback_reference).toBe('changed-reference-only')
})
 test('a newer operational check-in requires review before replacing it', async ({ page }) => {
  const state = await fixture(page); await openNew(page)
  await page.getByLabel('Check-in reason', { exact: true }).fill('My proposed operational update')
  state.status = { version: 1, actor_id: person.id, state: 'blocked', reason: 'Newer check-in', next_action: '', changed_at: '2027-06-20T09:21:00Z' }; state.revision++
  await saveStatus(page)
  await expect(page.getByText('Newer check-in', { exact: true })).toBeVisible(); expect(state.commands).toHaveLength(0)
  await page.getByRole('button', { name: 'Use displayed check-in as base' }).click()
  await saveStatus(page)
  await expect.poll(() => state.commands.length).toBe(1); await expect(page.getByText('My proposed operational update', { exact: true })).toBeVisible()
})

test('Wi-Fi interruption never queues a write; reconnect requires a fresh deliberate save', async ({ page, context }) => {
  const state = await fixture(page); await openNew(page); await typeNote(page)
  await context.setOffline(true)
  await saveObservation(page)
  await expect(page.getByRole('status', { name: 'Observation save state' })).toContainText('No server save confirmed')
  expect(state.commands).toHaveLength(0); expect(await copies(page)).toBe(1)
  await context.setOffline(false)
  await saveObservation(page)
  await expect(page.getByRole('status', { name: 'Observation save state' })).toContainText('Saved on the server'); expect(state.commands).toHaveLength(1)
})
test('expired typed text and cross-tab session end cannot restore or retain an actual private editor', async ({ page, context }) => {
  await fixture(page); await openNew(page); await typeNote(page)
  await page.evaluate(() => { for (const key of Object.keys(sessionStorage).filter(k => k.startsWith('hackbox:mentor-text:'))) { const v = JSON.parse(sessionStorage.getItem(key)!); v.createdAt = Date.now() - 31 * 60 * 1000; v.expiresAt = v.createdAt + 30 * 60 * 1000; sessionStorage.setItem(key, JSON.stringify(v)) } })
  await leaveTask(page, () => page.reload()); await page.getByRole('button', { name: 'Check for local text', exact: true }).click()
  await expect(page.getByLabel('Evidence (private)', { exact: true })).toHaveValue(''); expect(await copies(page)).toBe(0)
  await page.getByRole('button', { name: 'Discard local copy' }).click(); await typeNote(page, 'PRIVATE TAB ONE')
  const peer = await context.newPage(); await fixture(peer); await openNew(peer); await typeNote(peer, 'PRIVATE TAB TWO')
  await page.evaluate(() => window.dispatchEvent(new Event('session-ending')))
  await expect(page.getByLabel('Evidence (private)', { exact: true })).toHaveValue('')
  await expect(peer.getByLabel('Evidence (private)', { exact: true })).toHaveValue('')
  expect(await copies(page)).toBe(0); expect(await copies(peer)).toBe(0)
})

test('active no-focus native revocation clears the restored editor and only that team recovery copy', async ({ page }) => {
  const state = await fixture(page); await page.clock.install(); await openNew(page); await typeNote(page, 'PRIVATE FIRST TEAM')
  await leaveTask(page, () => page.goto(`/events/${firstID}/mentor/${teamIDs[1]}?note=${noteID}&fresh=true`)); await typeNote(page, 'PRIVATE UNRELATED TEAM')
  await openNew(page); await restore(page); expect(await copies(page)).toBe(2)
  state.denied = true; await page.clock.fastForward(16_000)
  await expect(page.getByLabel('Evidence (private)', { exact: true })).toHaveCount(0)
  await expect.poll(() => copies(page)).toBe(1)
  const remaining = await page.evaluate(() => Object.keys(sessionStorage).filter(k => k.startsWith('hackbox:mentor-text:')).map(k => sessionStorage.getItem(k)).join(''))
  expect(remaining).toContain('PRIVATE UNRELATED TEAM'); expect(remaining).not.toContain('PRIVATE FIRST TEAM')
  expect(state.commands).toHaveLength(0)
})
test('a known saved note missing from its exact native identity is never recreated as a new note', async ({ page }) => {
  const state = await fixture(page); await page.goto(`/events/${firstID}/mentor/${teamIDs[0]}?note=${noteID}`)
  await expect(page.getByText('Private observation entry is unavailable.', { exact: false })).toBeVisible()
  await expect(page.getByLabel('Evidence (private)', { exact: true })).toHaveCount(0); expect(state.commands).toHaveLength(0)
})

test('active live-window closure removes private copies while transient read failure retains them', async ({ page }) => {
  const state = await fixture(page); await page.clock.install(); await openNew(page); await typeNote(page)
  state.readFailure = true; await page.clock.fastForward(16_000)
  await expect(page.getByLabel('Evidence (private)', { exact: true })).not.toBeVisible(); expect(await copies(page)).toBe(1)
  state.readFailure = false; await page.clock.fastForward(16_000)
  await expect(page.getByLabel('Evidence (private)', { exact: true })).toBeVisible(); await restore(page)
  state.open = false; await page.clock.fastForward(16_000)
  await expect(page.getByLabel('Evidence (private)', { exact: true })).toHaveCount(0); expect(await copies(page)).toBe(0)
  expect(state.commands).toHaveLength(0)
})

for (const mode of ['expired', 'new-person'] as const) {
  test(`actual typed mentor text is cleared after native session ${mode}`, async ({ page }) => {
    const state = await fixture(page); let switched = false
    await page.route('**/api/v1/session', route => switched && mode === 'expired' ? route.fulfill({ status: 401, json: { error: { code: 'unauthorized', message: 'Session expired', request_id: 'synthetic' } } }) : route.fulfill({ json: { person: switched ? { ...person, id: '77777777-7777-4777-8777-777777777777', display_name: 'Different mentor' } : person, draft_scope: switched ? '88888888-8888-4888-8888-888888888888' : '22222222-2222-4222-8222-222222222222', csrf_token: 'synthetic-csrf-token-32-characters', can_create_events: false } }))
    await openNew(page); await typeNote(page, 'PRIVATE OLD ACCOUNT TEXT'); expect(await copies(page)).toBe(1)
    switched = true; await leaveTask(page, () => page.reload())
    await expect(page.getByLabel('Evidence (private)', { exact: true })).toHaveCount(0)
    if (mode === 'expired') await expect(page.getByRole('heading', { name: /session|Sign in|Welcome/i })).toBeVisible()
    else await expect(page.getByRole('button', { name: 'Different mentor', exact: true })).toBeVisible()
    expect(await copies(page)).toBe(0); await expect(page.getByText('PRIVATE OLD ACCOUNT TEXT', { exact: true })).toHaveCount(0)
    expect(state.commands).toHaveLength(0)
  })
}

test('native byte-limit validation rejection retains unsent text and is never reported as ambiguous or saved', async ({ page }) => {
  const state = await fixture(page); await openNew(page); await typeNote(page)
  await page.getByLabel('Feedback reference (private, not locally recovered)').fill('é'.repeat(200))
  await saveObservation(page)
  const notice = page.getByRole('status', { name: 'Observation save state' })
  await expect(notice).toContainText('Validation rejected this input'); await expect(notice).toContainText('no server save was made')
  await expect(notice).not.toContainText('unconfirmed'); await expect(notice).not.toContainText('Saved on the server')
  await expect(page.getByRole('alert')).toBeVisible(); expect(await copies(page)).toBe(1); expect(state.notes).toHaveLength(0)
  await expect(page.getByLabel('Evidence (private)', { exact: true })).toHaveValue('UNSENT PRIVATE <script> literal evidence')
})


test('restored parent team collection cannot revive an absent formerly opened private note', async ({ page }) => {
 const state = await fixture(page); state.notes = [saved()]; await page.clock.install()
 await page.goto(`/events/${firstID}/mentor/${teamIDs[0]}?note=${noteID}`)
 await page.getByRole('button', { name: 'Load saved server text (replace editor)' }).click()
 await expect(page.getByLabel('Evidence (private)', { exact: true })).toHaveValue('SAVED PRIVATE EVIDENCE')
 state.denied = true; await page.clock.fastForward(16_000)
 await expect(page.getByLabel('Evidence (private)', { exact: true })).toHaveCount(0)
 state.notes = []; state.denied = false; await page.clock.fastForward(16_000)
 await expect(page.getByText('Private observation entry is unavailable.', { exact: false })).toBeVisible()
 await expect(page.locator('body')).not.toContainText('SAVED PRIVATE EVIDENCE')
 await expect(page.getByRole('button', { name: 'Load saved server text (replace editor)' })).toHaveCount(0)
 await page.clock.fastForward(20_000)
 await expect(page.getByLabel('Evidence (private)', { exact: true })).toHaveCount(0)
 expect(state.commands).toHaveLength(0)
})

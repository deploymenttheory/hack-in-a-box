import { test, expect, type Page } from '@playwright/test'
import { firstID, mockWorkspace, person } from './fixtures'
import type { components } from '../src/api/schema.gen'

type Entry = components['schemas']['AttendanceEntry']
type Task = components['schemas']['PreparationTask']
type Update = components['schemas']['PreparationUpdate']
const alex = '41ac428a-a28d-41f2-9a9a-e058109e2f83', blair = 'a76caa74-7986-48a3-af56-28620daf0489'
const venueID = '048eab9c-385e-4c12-a839-7b70ede3cb80', checkID = 'f47e4656-a217-4999-bff2-79fcaee61043'
const cutoff = '2027-06-20T09:00:00Z', days = ['2027-06-20', '2027-06-21']
function entry(id: string, name: string): Entry { return { person_id: id, display_name: name, active: true, present: false, revision: 0, updated_at: null } }
function task(): Task {
  return { definition: { id: venueID, title: 'Venue readiness', description: 'Confirm the venue', owner_id: person.id, owner_role: 'logistics', status: 'blocked', schedule_id: null, fixed_due_at: cutoff, depends_on: [], checklist: [{ id: checkID, title: 'Confirm accessible routes', complete: false }], evidence: [] }, reason: 'Access code still needed', override_dependencies: false, revision: 1, due_at: cutoff, scheduling_issue: false, overdue: false, unowned: false, blocked: true, can_edit: true, restricted_evidence_count: 0, hidden_dependencies: 0 }
}
async function prepare(page: Page, logisticsOnly = false) {
  const workspace = await mockWorkspace(page), event = workspace.events[0]
  Object.assign(event, { can_read_attendance: true, can_read_logistics: logisticsOnly, can_edit: !logisticsOnly, can_read_staff: !logisticsOnly, can_transition: !logisticsOnly, confirmed_attendees: 2 })
  event.configuration.live_end = '2027-06-21T17:00:00Z'
  event.blockers = [{ field: 'minimum_attendance', code: 'attendance_shortfall', message: 'Confirmed competitors are below the configured minimum.' }]
  workspace.staff.push({ person_id: person.id, display_name: person.display_name, role: 'logistics', active: true, changed_at: cutoff })
  if (logisticsOnly) await page.route('**/api/v1/session', route => route.fulfill({ json: { person: { ...person, platform_admin: false }, csrf_token: 'synthetic-csrf-token-32-characters', can_create_events: false } }))
  const state = {
    canCheckIn: true, canConfigure: !logisticsOnly, revoked: false, failArrivalOnce: false,
    buffer: 4, staffRequests: 0, logisticsRequests: 0, exportRequests: 0,
    records: Object.fromEntries(days.map(d => [d, [entry(alex, 'Alex Arrival'), entry(blair, 'Blair Builder')]])) as Record<string, Entry[]>,
  }
  const arrivalRequests: { id: string; revision: string; present: boolean; reason: string }[] = []
  const bufferRequests: { buffer: number; revision: string }[] = []
  const history: components['schemas']['AttendanceChange'][] = []
  const preparation: components['schemas']['PreparationPage'] = { revision: 1, can_manage: !logisticsOnly, timezone: 'Europe/London', items: [task()] }
  const taskRequests: { body: Update; revision: string }[] = []
  const attendance = (day: string): components['schemas']['AttendancePage'] => ({
    revision: event.revision, day, days, timezone: 'Europe/London', roster_as_of: cutoff,
    can_check_in: state.canCheckIn, can_configure: state.canConfigure, can_read_logistics: !!event.can_read_logistics,
    counts: { confirmed_competitors: 2, minimum_competitors: 30, checked_in_competitors: state.records[day].filter(p => p.present).length, staff: 36, staff_also_competing: 1, catering_buffer: state.buffer, catering_total: 37 + state.buffer },
    items: state.records[day],
  })
  await page.route(`**/events/${firstID}/staff`, route => { state.staffRequests++; return route.fulfill({ json: { items: workspace.staff } }) })
  await page.route(`**/events/${firstID}/attendance**`, async route => {
    const req = route.request(), url = new URL(req.url()), path = url.pathname
    const fail = (status: number, code: string) => route.fulfill({ status, json: { error: { code, message: 'Synthetic rejection', request_id: 'synthetic' } } })
    if (path.endsWith('/history')) return route.fulfill({ json: { items: history } })
    if (req.method() === 'GET') return route.fulfill({ json: attendance(url.searchParams.get('day') || days[0]) })
    expect(req.headers()['x-csrf-token']).toBe('synthetic-csrf-token-32-characters')
    if (path.endsWith('/catering')) {
      const body = req.postDataJSON(); bufferRequests.push({ ...body, revision: req.headers()['if-match'] })
      if (!state.canConfigure) return fail(403, 'forbidden')
      if (req.headers()['if-match'] !== `"${event.revision}"`) return fail(409, 'revision_conflict')
      state.buffer = body.buffer; event.revision++
      return route.fulfill({ json: attendance(days[0]) })
    }
    const body = req.postDataJSON(), parts = path.split('/'), day = parts.at(-1)!, id = parts.at(-2)!
    arrivalRequests.push({ id: body.request_id, revision: req.headers()['if-match'], present: body.present, reason: body.reason })
    if (state.failArrivalOnce) { state.failArrivalOnce = false; return fail(503, 'unavailable') }
    if (!state.canCheckIn) return fail(403, 'forbidden')
    const row = state.records[day].find(p => p.person_id === id)
    if (!row) return fail(403, 'forbidden')
    if (req.headers()['if-match'] !== `"${row.revision}"`) return fail(409, 'revision_conflict')
    row.present = body.present; row.revision++; row.updated_at = cutoff
    history.unshift({ request_id: body.request_id, actor_id: person.id, present: row.present, revision: row.revision, reason: body.reason, at: cutoff })
    return route.fulfill({ json: row })
  })
  await page.route(`**/events/${firstID}/logistics**`, route => {
    const url = new URL(route.request().url()), exporting = url.pathname.endsWith('/export')
    if (exporting) state.exportRequests++; else state.logisticsRequests++
    if (state.revoked) return route.fulfill({ status: 403, json: { error: { code: 'forbidden', message: 'Synthetic revocation' } } })
    const next = url.searchParams.get('after') === alex
    return route.fulfill({ json: { roster_as_of: cutoff, next_cursor: next ? null : alex, items: [{ person_id: next ? blair : alex, display_name: next ? 'Blair Builder' : 'Alex Arrival', competing: true, contact_email: next ? 'blair@example.test' : 'alex@example.test', contact_phone: '01234 synthetic', dietary: 'Synthetic dietary note', accessibility: 'Synthetic accessibility note', travel_details: 'Synthetic travel details', social_attendance: 'unknown' }] } })
  })
  await page.route(`**/events/${firstID}/preparation/tasks**`, route => {
    const req = route.request()
    if (new URL(req.url()).pathname.endsWith('/history')) return route.fulfill({ json: { items: [] } })
    if (req.method() === 'GET') return route.fulfill({ json: preparation })
    const body = req.postDataJSON() as Update
    taskRequests.push({ body, revision: req.headers()['if-match'] })
    if (state.revoked) { preparation.items.forEach(t => { t.can_edit = false }); return route.fulfill({ status: 403, json: { error: { code: 'forbidden', message: 'Synthetic revocation' } } }) }
    if (req.headers()['if-match'] !== `"${preparation.revision}"`) return route.fulfill({ status: 409, json: { error: { code: 'revision_conflict', message: 'Synthetic conflict' } } })
    preparation.revision++; event.revision = preparation.revision
    preparation.items = preparation.items.filter(t => t.definition.id !== body.definition.id).concat({ ...task(), ...body, due_at: body.definition.fixed_due_at, blocked: body.definition.status === 'blocked', revision: preparation.revision })
    return route.fulfill({ json: preparation })
  })
  await page.goto(`/events/${firstID}`)
  await page.getByRole('button', { name: 'Attendance', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Arrivals and headcounts' })).toBeVisible()
  return { state, event, preparation, arrivalRequests, bufferRequests, taskRequests }
}
const row = (page: Page, name: string) => page.locator('.fleet-record-list > li, .fleet-collection-table tbody > tr').filter({ has: page.getByText(name, { exact: true }) })
const metric = (page: Page, name: string) => page.locator('.fleet-planning-metrics > div').filter({ has: page.locator('dt').filter({ hasText: name }) })

test('uses keyboard check-in on each day and keeps admission, staff and catering distinct', async ({ page }, info) => {
  const { state } = await prepare(page)
  await expect(metric(page, 'Confirmed competitors')).toContainText('Below the event minimum')
  await expect(metric(page, 'Catering total')).toContainText('41')
  await expect(metric(page, 'Active staff')).toContainText('36')
  await expect(page.getByText(/Source roster cutoff:/)).toContainText('20 Jun 2027, 10:00')
  const search = page.getByLabel('Search confirmed competitors')
  await search.focus(); await page.keyboard.type('Alex')
  await expect(row(page, 'Blair Builder')).toHaveCount(0)
  await page.keyboard.press('Tab'); await expect(page.getByRole('button', { name: 'Check in Alex Arrival' })).toBeFocused()
  await page.keyboard.press('Enter'); await expect(row(page, 'Alex Arrival')).toContainText('Checked in')
  await expect(metric(page, 'Checked in on this day').locator('dd')).toHaveText('1')
  await page.getByLabel('Hack day').selectOption(days[1]); await expect(metric(page, 'Checked in on this day').locator('dd')).toHaveText('0')
  await expect(page.getByRole('button', { name: 'Check in Alex Arrival' })).toBeEnabled()
  await page.getByLabel('Hack day').selectOption(days[0]); await search.fill('')
  await expect(metric(page, 'Confirmed competitors')).toContainText('Below the event minimum')
  expect(state.logisticsRequests).toBe(0)
  await expect(page.getByText(/Synthetic dietary|Synthetic accessibility|Synthetic travel|alex@example/)).toHaveCount(0)
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
  await page.screenshot({ path: info.outputPath('attendance.png'), fullPage: true })
})

test('retries an interrupted arrival with the same request identifier', async ({ page }) => {
  const { state, arrivalRequests } = await prepare(page)
  state.failArrivalOnce = true
  await page.getByRole('button', { name: 'Check in Alex Arrival' }).click()
  await expect(page.getByRole('alert')).toContainText('could not complete')
  await page.getByRole('button', { name: 'Check in Alex Arrival' }).click()
  await expect(row(page, 'Alex Arrival')).toContainText('Checked in')
  expect(arrivalRequests).toHaveLength(2); expect(arrivalRequests[0].id).toBe(arrivalRequests[1].id)
  expect(arrivalRequests.map(r => r.revision)).toEqual(['"0"', '"0"'])
})

test('requires reasoned corrections and explicit review of concurrent arrivals', async ({ page }) => {
  const { state, arrivalRequests } = await prepare(page)
  await page.getByRole('button', { name: 'Check in Alex Arrival' }).click()
  await page.getByRole('button', { name: 'Correct Alex Arrival', exact: true }).click()
  const editor = page.getByRole('form', { name: 'Arrival correction' })
  await editor.getByLabel('Arrival status').selectOption('no'); await editor.getByRole('button', { name: 'Save arrival correction' }).click()
  await expect(editor.getByRole('alert')).toContainText('Record why'); expect(arrivalRequests).toHaveLength(1)
  await editor.getByLabel('Correction reason').fill('Scanned the wrong person')
  state.records[days[0]][0].revision = 2
  await editor.getByRole('button', { name: 'Save arrival correction' }).click()
  await expect(editor.getByRole('alert')).toContainText('Your correction is preserved')
  await expect(editor.getByLabel('Correction reason')).toHaveValue('Scanned the wrong person')
  await expect(editor.getByRole('button', { name: 'Save arrival correction' })).toBeDisabled()
  await editor.getByRole('button', { name: 'Review latest arrival' }).click()
  await editor.getByLabel('Apply my correction after reviewing the latest status').check()
  await editor.getByRole('button', { name: 'Save arrival correction' }).click()
  await expect(editor.getByText('Arrival correction saved.')).toBeVisible()
  await expect(editor.locator('.history')).toContainText('Scanned the wrong person')
  expect(arrivalRequests.at(-1)).toMatchObject({ revision: '"2"', present: false, reason: 'Scanned the wrong person' })
  await editor.getByLabel('Correction reason').fill('Discard me')
  page.once('dialog', d => d.dismiss()); await page.getByRole('button', { name: 'Overview', exact: true }).click()
  await expect(editor.getByLabel('Correction reason')).toHaveValue('Discard me')
  page.once('dialog', d => d.accept()); await editor.getByRole('button', { name: 'Close correction' }).click()
  await page.getByRole('button', { name: 'Correct Alex Arrival', exact: true }).click()
  await expect(editor.getByLabel('Correction reason')).toHaveValue('')
})

test('disables correction after current check-in authority is revoked', async ({ page }) => {
  const { state } = await prepare(page)
  await page.getByRole('button', { name: 'Check in Alex Arrival' }).click()
  await page.getByRole('button', { name: 'Correct Alex Arrival', exact: true }).click()
  await page.getByLabel('Correction reason').fill('Keep this draft')
  state.canCheckIn = false
  await page.getByRole('button', { name: 'Save arrival correction' }).click()
  await expect(page.getByRole('button', { name: 'Save arrival correction' })).toBeDisabled()
  await expect(page.getByLabel('Correction reason')).toHaveValue('Keep this draft')
  await expect(page.getByRole('alert')).toContainText('do not have access')
})

test('reviews a stale catering buffer without using catering places to satisfy the minimum', async ({ page }) => {
  const { event, bufferRequests } = await prepare(page)
  await page.getByRole('button', { name: 'Configure catering buffer' }).click()
  await page.getByLabel('Extra catering places').fill('10'); event.revision = 2
  await page.getByRole('button', { name: 'Save catering buffer' }).click()
  await expect(page.getByRole('button', { name: 'Save catering buffer' })).toBeDisabled()
  await page.getByRole('button', { name: 'Review latest headcounts' }).click()
  await expect(page.getByLabel('Extra catering places')).toHaveValue('10')
  await page.getByRole('button', { name: 'Save catering buffer' }).click()
  await expect(page.getByText('Catering buffer saved.')).toBeVisible()
  expect(bufferRequests.map(r => r.revision)).toEqual(['"1"', '"2"'])
  await expect(metric(page, 'Catering total').locator('dd')).toHaveText('47')
  await expect(metric(page, 'Confirmed competitors')).toContainText('Below the event minimum')
})

test('keeps restricted logistics separate, paginates exports and rechecks revoked access', async ({ page }, info) => {
  const { state } = await prepare(page, true)
  await expect(page.getByRole('button', { name: 'Configure catering buffer' })).toHaveCount(0)
  expect(state.staffRequests).toBe(0); expect(state.logisticsRequests).toBe(0)
  await page.getByRole('button', { name: 'Open restricted logistics' }).click()
  await expect(page.getByText('alex@example.test')).toBeVisible()
  await page.getByRole('button',{name:'Review Alex Arrival',exact:true}).click();await expect(page.getByText('Unknown — not confirmed')).toBeVisible()
  await page.getByLabel('Search this logistics page').fill('Blair'); await expect(page.locator('.fleet-record-list > li, .fleet-collection-table tbody > tr')).toHaveCount(0)
  await page.getByRole('button', { name: 'More records', exact: true }).click()
  await expect(page.getByText('blair@example.test')).toBeVisible()
  const download = page.waitForEvent('download')
  await page.getByRole('button', { name: 'Download page 2' }).click()
  expect((await download).suggestedFilename()).toBe('logistics-page-2.json'); expect(state.exportRequests).toBe(1)
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
  await page.screenshot({ path: info.outputPath('restricted-logistics.png'), fullPage: true })
  await page.getByRole('button', { name: 'Return to shared check-in' }).click()
  await expect(page.getByText(/Synthetic dietary|Synthetic accessibility|Synthetic travel|blair@example/)).toHaveCount(0)
  await page.getByRole('button', { name: 'Open restricted logistics' }).click()
  await expect(page.getByText('alex@example.test')).toBeVisible()
  expect(state.logisticsRequests).toBeGreaterThanOrEqual(3)
  let unexpectedDownload = false; page.on('download', () => { unexpectedDownload = true })
  state.revoked = true
  await page.getByRole('button', { name: 'Download page 1' }).click()
  await expect(page.getByText('alex@example.test')).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Download page 1' })).toHaveCount(0)
  expect(unexpectedDownload).toBe(false); expect(state.staffRequests).toBe(0)
})

test('logistics assignees update checklist progress while retaining its assigned owner and deadline', async ({ page }, info) => {
  const { state, taskRequests, preparation } = await prepare(page, true)
  const original = structuredClone(preparation.items[0].definition)
  await page.getByRole('button', { name: 'Operational checklists', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Assign owners, deadlines and checklists' })).toHaveCount(0)
  await page.getByRole('button', { name: 'Update Venue readiness' }).click()
  await expect(page.getByLabel('Named owner', { exact: true })).toHaveCount(0)
  await page.getByLabel('Confirm accessible routes').check()
  await page.getByLabel('Checklist status').selectOption('complete')
  await page.getByRole('button', { name: 'Save checklist progress' }).click()
  await expect(page.getByRole('alert')).toContainText('Record a reason'); expect(taskRequests).toHaveLength(0)
  await page.getByLabel('Progress or blocker reason').fill('Venue checks complete')
  await page.getByRole('button', { name: 'Save checklist progress' }).click()
  await expect(page.getByText('Checklist progress saved.')).toBeVisible()
  expect(taskRequests[0].body).toEqual({ definition: { ...original, status: 'complete', checklist: [{ ...original.checklist[0], complete: true }] }, reason: 'Venue checks complete', override_dependencies: false })
  expect(state.staffRequests).toBe(0)
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
  await page.screenshot({ path: info.outputPath('logistics-checklist.png'), fullPage: true })
})

test('retains progress across conflicts, preserves new checklist items and respects permission loss', async ({ page }) => {
  const { state, taskRequests, preparation } = await prepare(page, true)
  await page.getByRole('button', { name: 'Operational checklists', exact: true }).click()
  await page.getByRole('button', { name: 'Update Venue readiness' }).click()
  await page.getByLabel('Confirm accessible routes').check()
  await page.getByLabel('Checklist status').selectOption('in_progress')
  preparation.revision = 2
  preparation.items[0].definition.checklist.push({ id: 'ee68b07d-d342-4d1b-a1a1-f7c16d28d653', title: 'New power check', complete: false })
  await page.getByRole('button', { name: 'Save checklist progress' }).click()
  await expect(page.getByRole('alert')).toContainText('Your progress edits are preserved')
  await page.getByRole('button', { name: 'Review latest checklist' }).click()
  await expect(page.getByRole('alert')).toContainText('New power check: To do')
  await page.getByLabel('Apply my progress after reviewing the latest checklist').check()
  await page.getByRole('button', { name: 'Save checklist progress' }).click()
  await expect(page.getByText('Checklist progress saved.')).toBeVisible()
  expect(taskRequests.at(-1)?.revision).toBe('"2"')
  expect(taskRequests.at(-1)?.body.definition.checklist).toMatchObject([{ complete: true }, { title: 'New power check', complete: false }])
  state.revoked = true; await page.getByLabel('Progress or blocker reason').fill('Unsaved follow-up')
  await page.getByRole('button', { name: 'Save checklist progress' }).click()
  await expect(page.getByRole('button', { name: 'Save checklist progress' })).toBeDisabled()
  await expect(page.getByLabel('Progress or blocker reason')).toHaveValue('Unsaved follow-up')
})

test('requires named owners and deadlines and shows blocked operational tasks in organiser readiness', async ({ page }, info) => {
  const { taskRequests } = await prepare(page)
  await page.getByRole('button', { name: 'Overview', exact: true }).click()
  const readiness = page.getByRole('region', { name: 'Operational readiness' })
  await expect(readiness).toContainText('Owner: Synthetic Person · logistics')
  await expect(readiness).toContainText('Due: 20 Jun 2027, 10:00')
  await expect(readiness).toContainText('Access code still needed')
  await page.screenshot({ path: info.outputPath('organiser-readiness.png'), fullPage: true })
  await page.getByRole('button', { name: 'Preparation', exact: true }).click()
  for (const kind of ['venue', 'av', 'catering']) await expect(page.getByRole('button', { name: `New ${kind} checklist` })).toBeVisible()
  await page.getByRole('button', { name: 'New catering checklist' }).click()
  await page.getByRole('button', { name: 'Save task', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('Assign a named owner and a deadline'); expect(taskRequests).toHaveLength(0)
  await page.getByLabel('Named owner', { exact: true }).selectOption(person.id)
  await page.getByLabel('Fixed due date').fill('')
  await page.getByRole('button', { name: 'Save task', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('Assign a named owner and a deadline'); expect(taskRequests).toHaveLength(0)
  await page.getByLabel('Fixed due date').fill('2027-06-20T10:00')
  await page.getByLabel('Task status').selectOption('blocked')
  await page.getByLabel('Reason for this change').fill('Caterer delivery needs confirmation')
  await page.getByRole('button', { name: 'Save task', exact: true }).click()
  await expect(page.getByText('Task saved.', { exact: true })).toBeVisible()
  expect(taskRequests[0].body.definition).toMatchObject({ title: 'Catering readiness', owner_role: 'logistics', owner_id: person.id, status: 'blocked' })
  expect(taskRequests[0].body.definition.checklist).toHaveLength(3)
  await page.getByRole('button', { name: 'Overview', exact: true }).click()
  await expect(readiness).toContainText('Caterer delivery needs confirmation')
})

test('unassigned operational tasks cannot progress until an organiser supplies a deadline and owner', async ({ page }) => {
  const { preparation } = await prepare(page, true)
  preparation.items[0].definition.owner_id = null; preparation.items[0].due_at = null
  await page.getByRole('button', { name: 'Operational checklists', exact: true }).click()
  await page.getByRole('button', { name: 'Update Venue readiness' }).click()
  await expect(page.getByRole('alert')).toContainText('assign a named owner and a valid deadline')
  await expect(page.getByRole('button', { name: 'Save checklist progress' })).toBeDisabled()
})

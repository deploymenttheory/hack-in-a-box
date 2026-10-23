import { test, expect, type Page } from '@playwright/test'
import { mockWorkspace, firstID, person } from './fixtures'
import type { components } from '../src/api/schema.gen'

type Task = components['schemas']['PreparationTask']
type Update = components['schemas']['PreparationUpdate']
const venueID = '048eab9c-385e-4c12-a839-7b70ede3cb80', avID = '7c65ab5b-512e-4b9f-884d-c847aa23bbd7'
const mentor = '6aa1aaf7-611d-4c11-83f0-a2de4d08f571'
function task(id: string, title: string): Task {
  return { definition: { id, title, description: '', owner_id: null, owner_role: '', status: 'not_started', schedule_id: null, fixed_due_at: '2027-06-19T08:00:00Z', depends_on: [], checklist: [], evidence: [] }, reason: '', override_dependencies: false, revision: 1, due_at: '2027-06-19T08:00:00Z', scheduling_issue: false, overdue: true, unowned: true, blocked: false, can_edit: true, restricted_evidence_count: 0, hidden_dependencies: 0 }
}
async function prepare(page: Page) {
  const workspace = await mockWorkspace(page)
  await page.route(`**/events/${firstID}/registrations/review*`, route => route.fulfill({ json: { items: [], next_cursor: null } }))
  await page.route(`**/events/${firstID}/registrations/offer-policy`, route => route.fulfill({ json: { revision: 0, automated: false, working_days: 2 } }))
  workspace.staff.push({ person_id: mentor, display_name: 'Morgan Mentor', role: 'mentor', active: true, changed_at: '2027-01-01T00:00:00Z' })
  workspace.events[0].confirmed_attendees = 28
  workspace.events[0].blockers = [{ field: 'minimum_attendance', code: 'attendance_shortfall', message: 'Two more confirmed competitors are required.' }]
  const data: components['schemas']['PreparationPage'] = { revision: 1, can_manage: true, timezone: 'Europe/London', items: [task(venueID, 'Book venue'), task(avID, 'Check AV')] }
  data.items[1].definition.depends_on = [venueID]; data.items[1].blocked = true
  const requests: { body: Update; revision: string }[] = [], history: components['schemas']['PreparationChange'][] = []
  let rejectPermission = false
  await page.route(`**/events/${firstID}/preparation/tasks**`, async route => {
    const req = route.request(), path = new URL(req.url()).pathname
    if (path.endsWith('/history')) return route.fulfill({ json: { items: history } })
    if (req.method() === 'GET') return route.fulfill({ json: data })
    const body = req.postDataJSON() as Update
    requests.push({ body, revision: req.headers()['if-match'] })
    const fail = (status: number, code: string, blockers?: components['schemas']['Blocker'][]) => route.fulfill({ status, json: { error: { code, message: 'Synthetic rejection', request_id: 'synthetic', blockers } } })
    if (rejectPermission) { data.can_manage = false; data.items.forEach(t => { t.can_edit = false }); return fail(403, 'forbidden') }
    if (req.headers()['if-match'] !== `"${data.revision}"`) return fail(409, 'revision_conflict')
    if (body.definition.status === 'complete' && body.definition.depends_on.length && !body.override_dependencies) return fail(409, 'event_blocked', [{ field: 'depends_on', code: 'unfinished_dependencies', message: 'Complete the venue dependency first.' }])
    data.revision++; workspace.events[0].revision = data.revision
    const old = data.items.find(t => t.definition.id === body.definition.id)
    const saved: Task = { ...(old ?? task(body.definition.id, body.definition.title)), ...body, revision: (old?.revision ?? 0) + 1, due_at: body.definition.fixed_due_at, unowned: !body.definition.owner_id, blocked: body.definition.status === 'blocked', overdue: !['complete', 'waived'].includes(body.definition.status) }
    data.items = data.items.filter(t => t.definition.id !== body.definition.id).concat(saved)
    history.unshift({ ...body, revision: saved.revision, actor_id: person.id, created_at: '2027-06-19T10:00:00Z' })
    return route.fulfill({ json: data })
  })
  await page.goto(`/events/${firstID}`)
  await page.getByRole('button', { name: 'Preparation', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Preparation', exact: true })).toBeVisible()
  return { data, requests, history, revoke: () => { rejectPermission = true } }
}
const editor = (page: Page) => page.getByRole('form', { name: 'Task editor' })

test('assigns a task, edits checklist and links, filters attention and shows a local run sheet', async ({ page }, info) => {
  const { requests } = await prepare(page)
  await expect(page.getByText('28 confirmed competitors · Minimum 30')).toBeVisible()
  for (const title of ['Final admission cutoff', 'Team freeze', 'Common challenge publication', 'Live start', 'Submission cutoff']) await expect(page.locator('dt', { hasText: title })).toBeVisible()
  await page.getByLabel('Needs attention').selectOption('blocked')
  await expect(page.getByRole('button', { name: 'Edit Check AV' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Edit Book venue' })).toHaveCount(0)
  await page.getByLabel('Needs attention').selectOption('unowned')
  await page.getByRole('button', { name: 'Edit Book venue' }).click()
  await page.getByLabel('Named owner').selectOption(mentor); await page.getByLabel('Assigned role').selectOption('mentor')
  await page.getByLabel('Task status').selectOption('in_progress')
  await page.getByRole('button',{name:'Checklist',exact:true}).click()
  await page.getByRole('button', { name: 'Add checklist item' }).click()
  await page.getByLabel('Checklist item 1', { exact: true }).fill('Contract signed')
  await editor(page).getByLabel('Done', { exact: true }).check()
  await page.getByRole('button',{name:'Evidence',exact:true}).click()
  await page.getByRole('button', { name: 'Add evidence link' }).click()
  await page.getByLabel('Evidence label 1').fill('Venue confirmation')
  await page.getByLabel('Evidence URL 1').fill('https://example.org/confirmation')
  await page.getByRole('button', { name: 'Save task', exact: true }).click()
  await expect(page.getByText('Task saved.', { exact: true })).toBeVisible()
  expect(requests.at(-1)?.body.definition).toMatchObject({ owner_id: mentor, owner_role: 'mentor', status: 'in_progress', checklist: [{ title: 'Contract signed', complete: true }], evidence: [{ visibility: 'organisers', url: 'https://example.org/confirmation' }] })
  await page.getByLabel('Needs attention').selectOption(''); await page.getByLabel('Filter by owner').selectOption(mentor)
  await page.getByRole('button', { name: 'List', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Edit Check AV' })).toHaveCount(0)
  await page.getByRole('button',{name:'Back to preparation tasks',exact:true}).click()
  await page.getByRole('button', { name: 'Run sheet', exact: true }).click()
  const run = page.getByRole('region', { name: 'Run sheet', exact: true })
  await expect(run).toContainText('19 Jun 2027, 09:00'); await expect(run).toContainText('Morgan Mentor')
  await expect(run.getByRole('link', { name: 'Venue confirmation' })).toHaveAttribute('href', 'https://example.org/confirmation')
  await page.getByRole('button', { name: 'Board', exact: true }).click(); await page.getByLabel('Filter by owner').selectOption('')
  await expect(page.getByRole('button',{name:'Close task',exact:true})).toHaveCount(0)
  await page.screenshot({ path: info.outputPath('preparation-board.png'), fullPage: true })
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
  await page.getByRole('button', { name: 'Review minimum attendance' }).click()
  await expect(page.getByRole('heading', { name: 'Applicant review' })).toBeVisible()
})

test('requires reasons, retains rejected completion, and records an explicit waiver', async ({ page }) => {
  const { requests } = await prepare(page)
  await page.getByRole('button', { name: 'Edit Check AV' }).click()
  await page.getByLabel('Task status').selectOption('blocked'); await page.getByRole('button', { name: 'Save task' }).click()
  await expect(page.getByRole('alert')).toContainText('Record a reason'); expect(requests).toHaveLength(0)
  await page.getByLabel('Reason for this change').fill('Waiting for venue'); await page.getByRole('button', { name: 'Save task' }).click()
  await expect(page.getByText('Task saved.', { exact: true })).toBeVisible()
  await page.getByRole('button',{name:'Task and ownership',exact:true}).click();await page.getByLabel('Task status').selectOption('complete'); await page.getByRole('button',{name:'Decision record',exact:true}).click();await page.getByLabel('Reason for this change').fill('AV checked')
  await page.getByRole('button', { name: 'Save task' }).click()
  await expect(page.getByRole('alert')).toContainText('Complete the venue dependency first.')
  await expect(page.getByLabel('Reason for this change')).toHaveValue('AV checked')
  await expect(page.getByRole('button', { name: 'Review latest version' })).toHaveCount(0)
  await page.getByLabel('Override unfinished dependencies').check(); await page.getByRole('button', { name: 'Save task' }).click()
  await expect(page.getByText('Task saved.', { exact: true })).toBeVisible()
  expect(requests.at(-1)?.body.override_dependencies).toBe(true)
  await page.getByRole('button',{name:'Task and ownership',exact:true}).click();await page.getByLabel('Task status').selectOption('waived'); await page.getByRole('button',{name:'Decision record',exact:true}).click();await page.getByLabel('Reason for this change').fill('Venue provides AV')
  await page.getByRole('button', { name: 'Save task' }).click(); await expect(page.getByText('Task saved.', { exact: true })).toBeVisible()
  expect(requests.at(-1)?.body).toMatchObject({ definition: { status: 'waived' }, reason: 'Venue provides AV', override_dependencies: false })
  await page.getByText('Task history', { exact: true }).click(); await expect(page.locator('.history')).toContainText('Venue provides AV')
  await expect(page.getByText('Two more confirmed competitors are required.')).toBeVisible()
})

test('preserves conflicting edits and requires review before saving against the latest revision', async ({ page }, info) => {
  const { data, requests } = await prepare(page)
  await page.getByRole('button', { name: 'Edit Book venue' }).click(); await page.getByLabel('Task title').fill('Book larger venue')
  data.revision = 2; data.items[0].definition.title = 'Venue reserved by colleague'; data.items[0].reason = 'Reservation received'
  await page.getByRole('button', { name: 'Save task' }).click()
  await expect(page.getByRole('alert')).toContainText('Your draft is preserved')
  await expect(page.getByLabel('Task title')).toHaveValue('Book larger venue'); await expect(page.getByRole('button', { name: 'Save task' })).toBeDisabled()
  await page.getByRole('button', { name: 'Review latest version' }).click()
  await expect(page.getByLabel('Latest saved task')).toContainText('Venue reserved by colleague')
  await page.screenshot({ path: info.outputPath('preparation-conflict.png'), fullPage: true })
  await expect(page.getByRole('button', { name: 'Save task' })).toBeDisabled()
  await page.getByLabel('I reviewed the latest version').check(); await page.getByRole('button', { name: 'Save task' }).click()
  await expect(page.getByText('Task saved.', { exact: true })).toBeVisible()
  expect(requests.map(r => r.revision)).toEqual(['"1"', '"2"']); expect(data.items.find(t => t.definition.id === venueID)?.definition.title).toBe('Book larger venue')
})

test('retains a draft when permissions change and prevents leaving without consent', async ({ page }) => {
  const { revoke } = await prepare(page)
  await page.getByRole('button', { name: 'Edit Book venue' }).click(); await page.getByLabel('Description').fill('Keep this draft')
  page.once('dialog', d => d.dismiss()); await page.getByRole('button', { name: 'Overview', exact: true }).click()
  await expect(page.getByLabel('Description')).toHaveValue('Keep this draft')
  revoke(); await page.getByRole('button', { name: 'Save task' }).click()
  await expect(page.getByRole('alert')).toContainText('do not have access')
  await expect(page.getByRole('button', { name: 'Save task' })).toBeDisabled(); await expect(page.getByLabel('Description')).toHaveValue('Keep this draft')
  await expect(page.getByRole('button', { name: 'New task', exact: true })).toHaveCount(0)
})

test('creates a task and safely retries a protected evidence upload', async ({ page }) => {
  const { requests } = await prepare(page)
  const uploadKeys: string[] = []
  await page.route(`**/events/${firstID}/files`, route => {
    uploadKeys.push(route.request().headers()['idempotency-key'])
    if (uploadKeys.length === 1) return route.fulfill({ status: 503, json: { error: { code: 'unavailable', message: 'Synthetic interruption' } } })
    return route.fulfill({ status: 201, json: { id: '454d6575-9a8a-44db-96ed-387bb60516fc', filename: 'receipt.txt' } })
  })
  await page.getByRole('button', { name: 'New task', exact: true }).click(); await page.getByLabel('Task title').fill('Book catering')
  await page.getByRole('button',{name:'Evidence',exact:true}).click()
  await page.getByLabel('Upload evidence file').setInputFiles({ name: 'receipt.txt', mimeType: 'text/plain', buffer: Buffer.from('Synthetic receipt') })
  await page.getByRole('button', { name: 'Upload selected file' }).click(); await expect(page.getByRole('alert')).toContainText('could not complete')
  await page.getByRole('button', { name: 'Upload selected file' }).click(); await expect(page.getByText('File uploaded. Save the task to attach it.')).toBeVisible()
  expect(uploadKeys[0]).toBe(uploadKeys[1])
  await page.getByRole('button', { name: 'Save task' }).click(); await expect(page.getByText('Task saved.', { exact: true })).toBeVisible()
  expect(requests.at(-1)?.body.definition.evidence[0]).toMatchObject({ kind: 'file', label: 'receipt.txt', visibility: 'organisers', file_id: '454d6575-9a8a-44db-96ed-387bb60516fc' })
})

test('resolves edited local deadlines and retains the draft when a DST choice is required', async ({ page }) => {
  const { requests } = await prepare(page)
  await page.route('**/api/v1/time/resolve', route => {
    const input = route.request().postDataJSON()
    expect(input.timezone).toBe('Europe/London'); expect(input.wall_time).toBe('2027-10-31T01:30:00')
    if (!input.dst_choice) return route.fulfill({ status: 409, json: { error: { code: 'event_blocked', message: 'Choose a time', blockers: [{ field: 'due', code: 'ambiguous_time', message: 'This local time occurs twice.' }] } } })
    expect(input.dst_choice).toBe('earlier')
    return route.fulfill({ json: { instant: '2027-10-31T00:30:00Z' } })
  })
  await page.getByRole('button', { name: 'Edit Book venue' }).click(); await page.getByRole('button',{name:'Due time',exact:true}).click(); await page.getByLabel('Fixed due date').fill('2027-10-31T01:30')
  await page.getByRole('button', { name: 'Save task' }).click(); await expect(page.getByRole('alert')).toContainText('This local time occurs twice.')
  await expect(page.getByLabel('Fixed due date')).toHaveValue('2027-10-31T01:30')
  expect(requests).toHaveLength(0)
  await page.getByLabel('When this time occurs twice').selectOption('earlier'); await page.getByRole('button', { name: 'Save task' }).click()
  await expect(page.getByText('Task saved.', { exact: true })).toBeVisible(); expect(requests[0].body.definition.fixed_due_at).toBe('2027-10-31T00:30:00Z')
})

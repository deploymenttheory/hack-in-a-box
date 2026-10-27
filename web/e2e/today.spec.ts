import { test, expect } from '@playwright/test'
import { todayFixture, todayRubricID, preparation } from './today-fixtures'
import { firstID, person } from './fixtures'
import { openEventSection, expectEventSection } from './event-navigation-fixture'
async function openToday(page: import('@playwright/test').Page) { await page.goto(`/events/${firstID}`); await openEventSection(page, 'Today'); await expect(page.getByRole('heading', { name: 'Next relevant actions' })).toBeVisible() }

test('current participation keeps preparation, frozen smaller teams and formal gates separate', async ({ page }) => {
  const f = await todayFixture(page)
  await openToday(page)
  await expect(page.getByText('Published shared room activity', { exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Your preparation check' })).toBeVisible()
  await expect(page.getByText('Overdue — server confirmed', { exact: true })).toBeVisible()
  await expect(page.getByText(/2 published members/)).toBeVisible()
  await expect(page.getByText(/Membership is frozen; a smaller team may continue/)).toBeVisible()
  await expect(page.getByText(/formal submission is not open for this team/)).toBeVisible()
  await expect(page.getByText('You acknowledged version 1. Read and acknowledge the updated version 2.', { exact: true })).toBeVisible()
  expect(f.calls.filter(c => c.method !== 'GET')).toEqual([])
  expect(f.calls.some(c => c.path.endsWith('/invitations'))).toBe(false)
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
  await page.emulateMedia({ media: 'print' })
  await expect(page.getByRole('heading', { name: 'Your preparation check' })).not.toBeVisible()
})

test('actual compatible roles select assigned work without creating judging permissions', async ({ page }) => {
  const f = await todayFixture(page, ['mentor', 'judge'])
  await openToday(page)
  const context = page.getByRole('combobox', { name: 'Today context' })
  await context.selectOption('mentor')
  await expect(page.getByRole('heading', { name: 'Assigned mentor preparation' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Assigned primary mentor: Assigned synthetic team' })).toBeVisible()
  await context.selectOption('judge')
  await expect(page.getByRole('heading', { name: 'Released judging rubric instructions' })).toBeVisible()
  await expect(page.getByText('Judging task status unavailable', { exact: true })).toBeVisible()
  await expect(context.locator('option[value="organiser"]')).toHaveCount(0)
  await context.evaluate(element => { const select = element as HTMLSelectElement; select.add(new Option('Forged organiser', 'organiser')); select.value = 'organiser'; select.dispatchEvent(new Event('change', { bubbles: true })) })
  expect(f.calls.some(c => c.path.endsWith('/schedule') || c.path.endsWith('/staff'))).toBe(false)
  expect(f.calls.filter(c => c.method !== 'GET')).toEqual([])
  expect(f.calls.some(c => c.path.includes(todayRubricID))).toBe(false)
})

test('temporary failure pauses actions while definitive current denial clears private tasks', async ({ page }) => {
  const f = await todayFixture(page)
  f.state.task.definition.title = 'Private assigned canary'
  await openToday(page)
  f.state.failed = true
  await page.getByRole('button', { name: 'Refresh Today facts' }).click()
  await expect(page.getByRole('alert')).toContainText('actions are paused')
  await expect(page.getByRole('heading', { name: 'Private assigned canary' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Open your preparation workspace', exact: true })).toBeDisabled()
  f.state.failed = false; f.state.denied = 403
  await page.getByRole('button', { name: 'Refresh Today facts' }).click()
  await expect(page.getByRole('heading', { name: 'Private assigned canary' })).toHaveCount(0)
  expect(f.calls.filter(c => c.method !== 'GET')).toEqual([])
})

test('required released documents follow opaque cursor pages without invented global completion', async ({ page }) => {
  const f = await todayFixture(page)
  f.state.documentsNext = true
  await openToday(page)
  await page.getByRole('button', { name: 'Next released-document page' }).click()
  await expect(page.getByRole('heading', { name: 'Another required released document' })).toBeVisible()
  await expect(page.getByText('Only the current released-document page is represented; no all-documents completion is inferred.')).toBeVisible()
  await page.getByRole('button', { name: 'First released-document page' }).click()
  await expect(page.getByRole('heading', { name: 'Another required released document' })).toHaveCount(0)
  expect(f.calls.filter(c => c.method !== 'GET')).toEqual([])
})

test('device time does not open formal submission or replace the native cutoff outcome', async ({ page }) => {
  await page.clock.install({ time: new Date('2040-01-01T12:00:00Z') })
  const f = await todayFixture(page)
  f.state.task.overdue = false; f.state.task.due_at = '2027-06-19T09:00:00Z'; f.state.task.definition.fixed_due_at = f.state.task.due_at
  await openToday(page)
  await expect(page.getByText('Common release not recorded', { exact: true })).toBeVisible()
  await expect(page.getByText('Cutoff not reached at the server observation', { exact: true })).toBeVisible()
  await expect(page.getByText('Overdue — server confirmed', { exact: true })).toHaveCount(0)
  await expect(page.getByText(/Working draft access is not currently open/)).toBeVisible()
  await expect(page.getByText(/formal submission is not open for this team/)).toBeVisible()
  f.state.serverNow = '2027-06-14T12:00:00Z'; f.state.published = true; f.state.draft = true
  await page.getByRole('button', { name: 'Refresh Today facts' }).click()
  await expect(page.getByText('Actual common release recorded', { exact: true })).toBeVisible()
  await expect(page.getByText(/Private working drafts are permitted/)).toBeVisible()
  await expect(page.getByText(/formal submission is not open for this team/)).toBeVisible()
  f.state.serverNow = '2027-06-20T10:00:00Z'; f.state.submit = true; f.state.task.overdue = true
  await page.getByRole('button', { name: 'Refresh Today facts' }).click()
  await expect(page.getByText('Formal submission open', { exact: true })).toBeVisible()
  f.state.serverNow = '2027-06-21T12:00:00Z'; f.state.outcome = 'not_submitted'; f.state.task.overdue = true; f.state.submit = false; f.state.draft = false
  await page.getByRole('button', { name: 'Refresh Today facts' }).click()
  await expect(page.getByRole('heading', { name: 'Team deliverables: not submitted' })).toBeVisible()
  await expect(page.getByText('Cutoff reached at the server observation', { exact: true })).toBeVisible()
  await expect(page.getByText('Frozen output confirmed by the submission owner', { exact: true })).toHaveCount(0)
  expect(f.calls.filter(c => c.method !== 'GET')).toEqual([])
})

test('a prior linked GitHub account does not establish current repository readiness', async ({ page }) => {
  const f = await todayFixture(page)
  f.state.published = true; f.state.serverNow = '2027-06-14T12:00:00Z'; f.state.oldGitHubGrant = true
  await openToday(page)
  const repository = page.getByRole('listitem').filter({ has: page.getByRole('heading', { name: 'Repository access readiness' }) })
  await expect(repository).toContainText('No own collaborator grant was returned.')
  await expect(repository).toContainText('Repository access needs review')
  await expect(repository).not.toContainText('Accepted collaborator state observed')
  await expect(page.getByRole('heading', { name: 'Assigned access code' })).toBeVisible()
  expect(f.calls.some(c => /reveal|access-code/.test(c.path))).toBe(false)
})

test('current role revocation fences a held old private read and partial restoration cannot revive it', async ({ page }) => {
  const f = await todayFixture(page, ['mentor'])
  let release!: () => void, started!: () => void
  const held = new Promise<void>(resolve => { release = resolve }), received = new Promise<void>(resolve => { started = resolve })
  await page.route(`**/events/${firstID}/preparation/tasks`, async route => {
    started(); await held
    if (f.state.tasksDenied) return route.fulfill({ status: f.state.tasksDenied, json: { error: { code: 'forbidden', message: 'Current private task access denied' } } }).catch(() => {})
    await route.fulfill({ json: { revision: 7, timezone: f.event.configuration.timezone, can_manage: false, items: [{ ...f.state.task, definition: { ...f.state.task.definition, title: 'Late private mentor canary' } }] } }).catch(() => {})
  })
  await page.goto(`/events/${firstID}`); await openEventSection(page, 'Today'); await received
  f.state.denied = 403
  await page.getByRole('button', { name: 'Refresh Today facts' }).click()
  await expect(page.getByRole('region', { name: 'Your event Today' })).toHaveCount(0)
  release()
  await expect(page.getByText('Late private mentor canary')).toHaveCount(0)
  f.state.roles = ['judge']; f.state.denied = 0; f.state.tasksDenied = 403; f.event.can_read_review_coverage = false
  await page.getByRole('button', { name: 'Try again', exact: true }).click()
  await expect(page.getByRole('region', { name: 'Your event Today' })).toBeVisible()
  await expect(page.getByText('Preparation task access is unavailable in your current role.', { exact: true })).toBeVisible()
  await expect(page.getByText('Published shared room activity', { exact: true })).toBeVisible()
  await expect(page.getByText('Late private mentor canary')).toHaveCount(0)
  expect(await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }))).not.toContain('Late private mentor canary')
  expect(f.calls.some(c => c.path.endsWith('/mentor-coverage'))).toBe(false)
  expect(f.calls.filter(c => c.method !== 'GET')).toEqual([])
})

test('staff actions resolve in the configured independent portal and forged sections never mount capabilities', async ({ page }, info) => {
  const f = await todayFixture(page, ['mc'])
  f.state.administrationOrigin = 'https://admin.example.test'
  await openToday(page)
  await page.getByRole('combobox', { name: 'Today context' }).selectOption('mc')
  if (info.project.metadata.portal !== 'administrator') {
    await expect(page.getByRole('link', { name: 'Open MC console in the administration portal' })).toHaveAttribute('href', `https://admin.example.test/events/${firstID}?section=mc+console`)
  } else {
    await expect(page.getByRole('button', { name: 'Open MC console', exact: true })).toBeVisible()
  }
  f.state.roles = []; f.event.can_present = false
  await page.goto(`/events/${firstID}?section=mc+console`)
  await expectEventSection(page, 'MC console', false)
  await expect(page.getByRole('region', { name: 'Your event Today' })).toHaveCount(0)
  expect(f.calls.filter(c => c.method !== 'GET')).toEqual([])
})

for (const roles of [['judge'], ['organiser', 'judge']] as const) {
test(`${roles.join('+')} assigned preparation resolves to its real owner and reviews an exact native progress command`, async ({ page }) => {
  const f = await todayFixture(page, [...roles])
  f.state.task = preparation('Assigned judge preparation', 'judge')
  if (roles.some(role => role === 'organiser')) f.state.otherTasks = [{ ...preparation('Unrelated manager-visible mentor task', 'mentor'), definition: { ...preparation('Unrelated manager-visible mentor task', 'mentor').definition, id: '0a4217bb-22ef-4d9c-adcc-e4b04905c5a1' } }, { ...preparation('Earlier mentor assignment without a current mentor grant', 'mentor'), definition: { ...preparation('Earlier mentor assignment without a current mentor grant', 'mentor').definition, id: '5fdf44f7-9c2d-4544-9e9c-b977ad211992', owner_id: person.id } }]
  await openToday(page)
  await page.getByRole('combobox', { name: 'Today context' }).selectOption('judge')
  await expect(page.getByRole('heading', { name: 'Earlier mentor assignment without a current mentor grant', exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: 'Open your preparation workspace', exact: true }).click()
  await expect(page.getByRole('region', { name: 'Your preparation', exact: true }).getByText('Unrelated manager-visible mentor task', { exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: 'Open Assigned judge preparation', exact: true }).click()
  await expect(page.getByRole('region', { name: 'Personal preparation task' })).toContainText('Shared judge task')
  await page.getByRole('combobox', { name: 'Preparation status', exact: true }).selectOption('complete')
  await page.getByRole('checkbox', { name: 'Read the preparation instructions', exact: true }).check()
  await page.getByRole('textbox', { name: 'Progress reason', exact: true }).fill('Read the current released instructions')
  await page.getByRole('button', { name: 'Save preparation progress', exact: true }).click()
  await expect(page.getByRole('alertdialog', { name: 'Review preparation progress' })).toContainText('Expected revision 7')
  expect(f.calls.filter(c => c.method !== 'GET')).toEqual([])
  await page.getByRole('button', { name: 'Confirm preparation progress', exact: true }).click()
  await expect(page.getByText('Preparation progress saved.', { exact: true })).toBeVisible()
  const writes = f.calls.filter(c => c.method !== 'GET')
  expect(writes).toHaveLength(1)
  expect(writes[0]).toMatchObject({ method: 'PUT', revision: '"7"', csrf: 'synthetic-csrf-token-32-characters', body: { definition: { owner_role: 'judge', status: 'complete', checklist: [{ complete: true }] }, reason: 'Read the current released instructions', override_dependencies: false } })
  await openEventSection(page, 'Today')
  await page.getByRole('combobox', { name: 'Today context' }).selectOption('judge')
  await expect(page.getByRole('heading', { name: 'Assigned judge preparation', exact: true })).toHaveCount(0)
  await expect(page.getByText(/1 complete · 0 waived · 1 current tasks/)).toBeVisible()
})
}

test('required released-version acknowledgement follows the real owner without navigation reload or early writes', async ({ page }) => {
  const f = await todayFixture(page)
  await openToday(page)
  await page.evaluate(() => { Object.assign(window, { todayNavigationCanary: 'same-client-lifetime' }) })
  await page.getByRole('button', { name: 'Read Released event rules', exact: true }).click()
  await expect(page.getByRole('article', { name: 'Released resource text' })).toContainText('Read the updated synthetic rules.')
  expect(await page.evaluate(() => (window as unknown as { todayNavigationCanary: string }).todayNavigationCanary)).toBe('same-client-lifetime')
  await page.getByRole('button', { name: 'I have read version 2', exact: true }).click()
  const review = page.getByRole('alertdialog', { name: 'Review reading acknowledgement', includeHidden: true })
  await expect(review).toContainText('Released version 2')
  await page.emulateMedia({ media: 'print' }); await expect(review).not.toBeVisible()
  await page.emulateMedia({ media: 'screen' }); await expect(review).toBeVisible()
  expect(f.calls.filter(c => c.method !== 'GET')).toEqual([])
  await page.getByRole('button', { name: 'Confirm acknowledgement', exact: true }).click()
  await expect(page.getByRole('region', { name: 'Required acknowledgement' }).getByRole('status')).toContainText('You acknowledged version 2')
  expect(f.calls.filter(c => c.method !== 'GET')).toMatchObject([{ method: 'POST', revision: '"2"', csrf: 'synthetic-csrf-token-32-characters' }])
  await page.getByRole('button', { name: 'Back to resources', exact: true }).click()
  await openEventSection(page, 'Today')
  await expect(page.getByText(/You acknowledged version 2 on/)).toBeVisible()
})

for (const role of ['organiser', 'mentor', 'judge', 'mc', 'logistics', 'photographer'] as const) {
 test(`current ${role} context shows its actual assigned preparation and shared published room facts`, async ({ page }) => {
  const f = await todayFixture(page, [role])
  await openToday(page)
  await page.getByRole('combobox', { name: 'Today context' }).selectOption(role)
  await expect(page.getByRole('heading', { name: `Assigned ${role} preparation`, exact: true })).toBeVisible()
  await expect(page.getByText('Published shared room activity', { exact: true })).toBeVisible()
  await expect(page.getByText('Next published room activity', { exact: true })).toBeVisible()
  expect(f.calls.filter(c => c.method !== 'GET')).toEqual([])
  expect(f.calls.some(c => /\/submission\/draft|\/reveal|\/presentation\/state|\/pairings/.test(c.path))).toBe(false)
 })
}

for (const lost of [false, true]) {
 test(`organiser-to-judge teardown retains safe ${lost ? 'unknown intent' : 'pending intent until original acknowledgement'} without private task resurrection`, async ({ page }) => {
  const f = await todayFixture(page, ['organiser', 'judge'])
  f.state.task = preparation('Assigned judge preparation', 'judge')
  let release!: () => void, began!: () => void
  f.state.taskWriteHold = new Promise<void>(resolve => { release = resolve })
  const started = new Promise<void>(resolve => { began = resolve }); f.state.taskWriteStarted = began; f.state.taskWriteLost = lost
  await openToday(page)
  await page.getByRole('combobox', { name: 'Today context' }).selectOption('judge')
  await page.getByRole('button', { name: 'Open your preparation workspace', exact: true }).click()
  await page.getByRole('button', { name: 'Open Assigned judge preparation', exact: true }).click()
  await page.getByRole('checkbox', { name: 'Read the preparation instructions', exact: true }).check()
  await page.getByRole('textbox', { name: 'Progress reason', exact: true }).fill('PRIVATE HELD REASON CANARY')
  await page.getByRole('button', { name: 'Save preparation progress', exact: true }).click()
  await page.getByRole('button', { name: 'Confirm preparation progress', exact: true }).click(); await started
  const history = page.getByRole('complementary', { name: 'Unconfirmed command history', exact: true, includeHidden: true })
  await expect(history).toContainText('Awaiting original acknowledgement')
  const review = page.getByRole('alertdialog', { name: 'Review preparation progress', includeHidden: true })
  await page.emulateMedia({ media: 'print' }); await expect(review).not.toBeVisible()
  await page.emulateMedia({ media: 'screen' }); await expect(review).toBeVisible()
  f.state.roles = ['judge']; f.event.can_edit = false; f.event.can_read_staff = false; f.event.can_present = false; f.event.can_read_review_coverage = false; f.event.can_transition = false; f.event.can_read_logistics = false; f.event.can_read_attendance = false
  await page.evaluate(() => { window.dispatchEvent(new Event('focus')); window.dispatchEvent(new Event('visibilitychange')) })
  await expect(page.getByRole('textbox', { name: 'Progress reason', exact: true, includeHidden: true })).toHaveCount(0)
  await expect(page.getByRole('alertdialog', { name: 'Review preparation progress', includeHidden: true })).toHaveCount(0)
  await expect(history).toBeVisible()
  await expect(history).toContainText('Awaiting original acknowledgement')
  await expect(history).not.toContainText('PRIVATE HELD REASON CANARY')
  release()
  if (lost) {
    await expect(history).toContainText('Original outcome remains unconfirmed')
    await page.evaluate(() => { window.dispatchEvent(new Event('focus')); window.dispatchEvent(new Event('visibilitychange')) })
    await expect(history).toContainText('Original outcome remains unconfirmed')
  } else await expect(history).toHaveCount(0)
  await expect(page.getByText('Preparation progress saved.', { exact: true })).toHaveCount(0)
  await expect(page.getByText('PRIVATE HELD REASON CANARY', { exact: true })).toHaveCount(0)
  expect(f.calls.filter(c => c.method === 'PUT')).toHaveLength(1)
 })
}

test('unchanged current-role polling preserves unsent preparation input and contextual keyboard focus', async ({ page }) => {
 const f = await todayFixture(page, ['mentor', 'judge'])
 await openToday(page)
 const context = page.getByRole('combobox', { name: 'Today context' })
 await context.focus(); await context.selectOption('mentor')
 await expect(context).toBeFocused()
 await page.getByRole('button', { name: 'Open your preparation workspace', exact: true }).click()
 await page.getByRole('button', { name: 'Open Assigned mentor preparation', exact: true }).click()
 await expect(page.getByRole('region', { name: 'Personal preparation task', exact: true }).getByRole('heading', { name: 'Assigned mentor preparation', exact: true })).toBeFocused()
 await page.getByRole('textbox', { name: 'Progress reason', exact: true }).fill('Unsent reason stays local')
 const other = preparation('Unrelated mentor assignment', 'mentor'); other.definition.id = '6d411fd9-6963-496b-8324-f19213fd61ba'; f.state.otherTasks = [other]
 const before = f.calls.filter(c => c.path.endsWith('/today')).length
 await page.evaluate(() => { window.dispatchEvent(new Event('focus')); window.dispatchEvent(new Event('visibilitychange')) })
 await expect.poll(() => f.calls.filter(c => c.path.endsWith('/today')).length).toBeGreaterThan(before)
 await expect(page.getByRole('textbox', { name: 'Progress reason', exact: true })).toHaveValue('Unsent reason stays local')
 expect(f.calls.filter(c => c.method !== 'GET')).toEqual([])
})

test('uncertain repository effect is a current warning without a fabricated provider receipt or replay', async ({ page }) => {
 const f = await todayFixture(page)
 f.state.published = true; f.state.serverNow = '2027-06-14T12:00:00Z'; f.state.repositoryUncertain = true
 await openToday(page)
 const repository = page.getByRole('listitem').filter({ has: page.getByRole('heading', { name: 'Repository access readiness' }) })
 await expect(repository).toContainText('Access effect unconfirmed')
 await expect(repository).toContainText('ambiguous (effect unconfirmed)')
 await expect(repository).not.toContainText('Accepted collaborator state observed')
 expect(f.calls.filter(c => c.method !== 'GET')).toEqual([])
})

for (const lost of [false, true]) test(`held reading ACK survives current guidance denial as ${lost ? 'unknown metadata' : 'a validated original acknowledgement'} without private resurrection`, async ({ page }) => {
 await page.clock.install({ time: new Date('2027-06-14T12:00:00Z') })
 const f = await todayFixture(page)
 let release!: () => void, began!: () => void
 f.state.ackWriteHold = new Promise<void>(resolve => { release = resolve })
 const started = new Promise<void>(resolve => { began = resolve }); f.state.ackWriteStarted = began; f.state.ackWriteLost = lost
 await openToday(page)
 await page.getByRole('button', { name: 'Read Released event rules', exact: true }).click()
 await page.getByRole('button', { name: 'I have read version 2', exact: true }).click()
 await page.getByRole('button', { name: 'Confirm acknowledgement', exact: true }).click(); await started
 const history = page.getByRole('complementary', { name: 'Unconfirmed command history', exact: true, includeHidden: true })
 await expect(history).toContainText('Resource acknowledgement · Expected revision 2 · Awaiting original acknowledgement')
 f.state.guidanceDenied = 403; await page.clock.runFor(16_000)
 await expect(page.getByText('Released guidance is available after your event access is confirmed.', { exact: true })).toBeVisible()
 await expect(page.getByRole('article', { name: 'Released resource text', includeHidden: true })).toHaveCount(0)
 await expect(page.getByRole('alertdialog', { name: 'Review reading acknowledgement', includeHidden: true })).toHaveCount(0)
 await expect(history).toBeVisible()
 await expect(history).not.toContainText('Released event rules')
 release()
 if (lost) {
  await expect(history).toContainText('Original outcome remains unconfirmed')
  f.state.guidanceDenied = 0; f.state.guidanceHidden = true
  await page.getByRole('button', { name: 'Check guidance access', exact: true }).click()
  await expect(page.getByText('No rules or contact pages have been released to your current audience yet.', { exact: true })).toBeVisible()
  await expect(history).toContainText('Original outcome remains unconfirmed')
 } else await expect(history).toHaveCount(0)
 await expect(page.getByRole('article', { name: 'Released resource text' })).toHaveCount(0)
 expect(f.calls.filter(call => call.method === 'POST')).toMatchObject([{ revision: '"2"', csrf: 'synthetic-csrf-token-32-characters' }])
})

for (const denied of [false, true]) test(`held old Today cannot restore ${denied ? 'a denied event' : 'equal-revision revoked capabilities'} over the actual parent GET`, async ({ page }) => {
 const f = await todayFixture(page, ['mc'])
 await openToday(page)
 await page.getByRole('combobox', { name: 'Today context' }).selectOption('mc')
 let release!: () => void, began!: () => void
 f.state.todayReadHold = new Promise<void>(resolve => { release = resolve })
 const started = new Promise<void>(resolve => { began = resolve }); f.state.todayReadStarted = began
 const originalRevision = f.event.revision
 await page.getByRole('button', { name: 'Refresh Today facts', exact: true }).click(); await started
 f.state.roles = ['judge']; f.event.can_present = false; f.state.task = preparation('Current judge preparation', 'judge'); if (denied) f.state.parentDenied = 403
 await page.evaluate(() => { window.dispatchEvent(new Event('focus')); window.dispatchEvent(new Event('visibilitychange')) })
 if (denied) {
  await expect(page.getByRole('navigation', { name: 'Event sections', exact: true })).toHaveCount(0)
  await expect(page.getByRole('region', { name: 'Your event Today', exact: true })).toHaveCount(0)
 } else {
  await expect(page.getByRole('combobox', { name: 'Today context' }).locator('option[value="judge"]')).toHaveCount(1)
  await expect(page.getByRole('combobox', { name: 'Today context' }).locator('option[value="mc"]')).toHaveCount(0)
 }
 release()
 if (denied) {
  await expect(page.getByRole('navigation', { name: 'Event sections', exact: true })).toHaveCount(0)
  f.state.parentDenied = 0
  await page.getByRole('button', { name: 'Try again', exact: true }).click()
  await expect(page.getByRole('combobox', { name: 'Today context' }).locator('option[value="judge"]')).toHaveCount(1)
 }
 await expect(page.getByRole('combobox', { name: 'Today context' }).locator('option[value="mc"]')).toHaveCount(0)
 await page.getByRole('combobox', { name: 'Today context' }).selectOption('judge')
 await expect(page.getByRole('heading', { name: 'Current judge preparation', exact: true })).toBeVisible()
 expect(f.event.revision).toBe(originalRevision)
 expect(f.calls.filter(call => call.method !== 'GET')).toEqual([])
})


for (const remove of [false, true]) test(`unrelated assignment ${remove ? 'removal' : 'addition'} preserves a newer current page after the selected original acknowledgement`, async ({ page }) => {
 await page.clock.install({ time: new Date('2027-06-14T12:00:00Z') })
 const f = await todayFixture(page, ['judge'])
 const other = preparation('Another judge assignment', 'judge'); other.definition.id = '6d411fd9-6963-496b-8324-f19213fd61ba'
 if (remove) f.state.otherTasks = [other]
 let release!: () => void, began!: () => void
 f.state.taskWriteHold = new Promise<void>(resolve => { release = resolve })
 const started = new Promise<void>(resolve => { began = resolve }); f.state.taskWriteStarted = began
 await openToday(page)
 await page.getByRole('combobox', { name: 'Today context' }).selectOption('judge')
 await page.getByRole('listitem').filter({ has: page.getByRole('heading', { name: 'Assigned judge preparation', exact: true }) }).getByRole('button', { name: 'Open your preparation workspace', exact: true }).click()
 await page.getByRole('button', { name: 'Open Assigned judge preparation', exact: true }).click()
 await page.getByRole('textbox', { name: 'Progress reason', exact: true }).fill('Selected owner remains authorised')
 await page.getByRole('button', { name: 'Save preparation progress', exact: true }).click()
 await page.getByRole('button', { name: 'Confirm preparation progress', exact: true }).click(); await started
 f.state.otherTasks = remove ? [] : [other]; f.state.preparationRevision++; f.event.revision = f.state.preparationRevision
 const reads = f.calls.filter(c => c.path.endsWith('/preparation/tasks')).length
 await page.clock.runFor(16_000)
 await expect.poll(() => f.calls.filter(c => c.path.endsWith('/preparation/tasks')).length).toBeGreaterThan(reads)
 await expect(page.getByRole('complementary', { name: 'Unconfirmed command history', includeHidden: true })).toContainText('Awaiting original acknowledgement')
 release()
 await expect(page.getByText('Preparation progress saved.', { exact: true })).toBeVisible()
 await expect(page.getByRole('alertdialog', { name: 'Review preparation progress', includeHidden: true })).toHaveCount(0)
 await expect(page.getByRole('button', { name: 'Close preparation task', exact: true })).toBeEnabled()
 await page.getByRole('button', { name: 'Close preparation task', exact: true }).click()
 await expect(page.getByRole('button', { name: 'Open Assigned judge preparation', exact: true })).toBeFocused()
 const unrelated = page.getByRole('button', { name: 'Open Another judge assignment', exact: true })
 if (remove) await expect(unrelated).toHaveCount(0); else await expect(unrelated).toBeVisible()
 await page.getByRole('button', { name: 'Open Assigned judge preparation', exact: true }).click()
 await page.getByRole('textbox', { name: 'Progress reason', exact: true }).fill('A fresh deliberate review')
 await page.getByRole('button', { name: 'Save preparation progress', exact: true }).click()
 await expect(page.getByRole('alertdialog', { name: 'Review preparation progress' })).toContainText('Expected revision 9')
 await page.getByRole('button', { name: 'Cancel', exact: true }).click()
 await expect(page.getByRole('complementary', { name: 'Unconfirmed command history', includeHidden: true })).toHaveCount(0)
 expect(f.calls.filter(c => c.method === 'PUT')).toHaveLength(1)
 expect(f.calls.find(c => c.method === 'PUT')).toMatchObject({ revision: '"7"', csrf: 'synthetic-csrf-token-32-characters' })
})

test('staff attendance exception uses native reasons and scrubs after role loss', async ({ page }) => {
 const f = await todayFixture(page, ['organiser', 'judge'])
 f.event.confirmed_attendees = 20; f.event.configuration.minimum_attendance = 30
 await openToday(page)
 await page.getByRole('combobox', { name: 'Today context' }).selectOption('organiser')
 const attendance = page.getByRole('row', { name: /Reasoned attendance exception/ })
 await expect(attendance).toContainText('No approved attendance exception was returned')
 f.state.decisions = [{ id: 'cfeaf816-d2ea-4761-87eb-e57e195a6b1b', actor_id: person.id, kind: 'transition_to_live', reason: 'PRIVATE approved following withdrawals', revision: 8, created_at: '2027-06-20T09:00:00Z' }]
 await page.getByRole('button', { name: 'Refresh Today facts', exact: true }).click()
 await expect(attendance).toContainText('Approved attendance exception recorded')
 await expect(attendance).toContainText('PRIVATE approved following withdrawals')
 await expect(attendance).toContainText('20 confirmed competitors · minimum 30')
 f.state.roles = ['judge']; f.event.can_edit = false; f.event.can_read_staff = false; f.event.can_transition = false
 await page.evaluate(() => { window.dispatchEvent(new Event('focus')); window.dispatchEvent(new Event('visibilitychange')) })
 await expect(page.getByText(/PRIVATE approved/)).toHaveCount(0)
 await expect(attendance).toContainText('Staff-only decision history')
 expect(f.calls.filter(c => c.method !== 'GET')).toEqual([])
})

for (const roleLoss of [false, true]) test(`early brief access keeps programme and guidance ${roleLoss ? 'after current preparation role loss' : 'without preparation authority'}`, async ({ page }) => {
 const f = await todayFixture(page, roleLoss ? ['judge', 'early_brief_access'] : ['early_brief_access'])
 if (!roleLoss) f.state.tasksDenied = 403
 await openToday(page)
 if (roleLoss) {
  await page.getByRole('combobox', { name: 'Today context' }).selectOption('judge')
  await expect(page.getByRole('heading', { name: 'Assigned judge preparation', exact: true })).toBeVisible()
  f.state.roles = ['early_brief_access']; f.state.tasksDenied = 403
  await page.getByRole('button', { name: 'Refresh Today facts', exact: true }).click()
 }
 await page.getByRole('combobox', { name: 'Today context' }).selectOption('early_brief_access')
 await expect(page.getByRole('heading', { name: 'Assigned judge preparation', exact: true })).toHaveCount(0)
 await expect(page.getByText('Preparation task access is unavailable in your current role.', { exact: true })).toBeVisible()
 await expect(page.getByText('Published shared room activity', { exact: true })).toBeVisible()
 await expect(page.getByRole('heading', { name: 'Official challenge and rubric clarifications', exact: true })).toBeVisible()
 await page.getByRole('button', { name: 'Read Released event rules', exact: true }).click()
 await expect(page.getByRole('article', { name: 'Released resource text' })).toContainText('Read the updated synthetic rules.')
 await expect(page.getByText(/0 complete.*0 current tasks/)).toHaveCount(0)
 expect(f.calls.filter(c => c.method !== 'GET')).toEqual([])
})

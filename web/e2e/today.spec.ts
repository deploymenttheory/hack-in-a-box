import { test, expect } from '@playwright/test'
import { todayFixture, todayRubricID, preparation } from './today-fixtures'
import { firstID } from './fixtures'
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
  f.state.draft = true; f.state.task.overdue = false
  await openToday(page)
  await expect(page.getByText('Common release not recorded', { exact: true })).toBeVisible()
  await expect(page.getByText('Cutoff not reached at the server observation', { exact: true })).toBeVisible()
  await expect(page.getByText('Overdue — server confirmed', { exact: true })).toHaveCount(0)
  await expect(page.getByText(/Private working drafts are permitted/)).toBeVisible()
  await expect(page.getByText(/formal submission is not open for this team/)).toBeVisible()
  f.state.serverNow = '2027-06-21T12:00:00Z'; f.state.outcome = 'not_submitted'
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
  await expect(page.getByText('Current task/readiness access is unavailable. No prior private tasks are retained.')).toBeVisible()
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
  if (roles.some(role => role === 'organiser')) f.state.otherTasks = [{ ...preparation('Unrelated manager-visible mentor task', 'mentor'), definition: { ...preparation('Unrelated manager-visible mentor task', 'mentor').definition, id: '0a4217bb-22ef-4d9c-adcc-e4b04905c5a1' } }]
  await openToday(page)
  await page.getByRole('combobox', { name: 'Today context' }).selectOption('judge')
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
  const history = page.getByRole('complementary', { name: 'Unconfirmed command history', exact: true })
  await expect(history).toContainText('Awaiting original acknowledgement')
  const review = page.getByRole('alertdialog', { name: 'Review preparation progress', includeHidden: true })
  await page.emulateMedia({ media: 'print' }); await expect(review).not.toBeVisible()
  await page.emulateMedia({ media: 'screen' }); await expect(review).toBeVisible()
  f.state.roles = ['judge']; f.event.can_edit = false; f.event.can_read_staff = false; f.event.can_present = false; f.event.can_read_review_coverage = false; f.event.can_transition = false; f.event.can_read_logistics = false; f.event.can_read_attendance = false
  await page.evaluate(() => { window.dispatchEvent(new Event('focus')); document.dispatchEvent(new Event('visibilitychange')) })
  await expect(page.getByRole('combobox', { name: 'Preparation status', exact: true })).toHaveCount(0)
  await expect(history).toContainText('Awaiting original acknowledgement')
  await expect(history).not.toContainText('PRIVATE HELD REASON CANARY')
  release()
  if (lost) {
    await expect(history).toContainText('Original outcome remains unconfirmed')
    await page.evaluate(() => { window.dispatchEvent(new Event('focus')); document.dispatchEvent(new Event('visibilitychange')) })
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
 await expect(page.getByRole('heading', { name: 'Assigned mentor preparation', exact: true })).toBeFocused()
 await page.getByRole('textbox', { name: 'Progress reason', exact: true }).fill('Unsent reason stays local')
 const before = f.calls.filter(c => c.path.endsWith('/today')).length
 await page.evaluate(() => { window.dispatchEvent(new Event('focus')); document.dispatchEvent(new Event('visibilitychange')) })
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
 const history = page.getByRole('complementary', { name: 'Unconfirmed command history', exact: true })
 await expect(history).toContainText('Resource acknowledgement · Expected revision 2 · Awaiting original acknowledgement')
 f.state.guidanceDenied = 403; await page.clock.runFor(16_000)
 await expect(page.getByText('Released guidance is available after your event access is confirmed.', { exact: true })).toBeVisible()
 await expect(page.getByRole('article', { name: 'Released resource text' })).toHaveCount(0)
 await expect(page.getByRole('alertdialog', { name: 'Review reading acknowledgement' })).toHaveCount(0)
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

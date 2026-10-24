import { test, expect } from '@playwright/test'
import { todayFixture, todayRubricID } from './today-fixtures'
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
  await expect(page.getByText(/Your version 1 acknowledgement does not cover this released version/)).toBeVisible()
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
  f.state.published = true; f.state.oldGitHubGrant = true
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
    await route.fulfill({ json: { revision: 7, timezone: f.event.configuration.timezone, can_manage: false, items: [{ ...f.state.task, definition: { ...f.state.task.definition, title: 'Late private mentor canary' } }] } }).catch(() => {})
  })
  await page.goto(`/events/${firstID}`); await openEventSection(page, 'Today'); await received
  f.state.denied = 403
  await page.getByRole('button', { name: 'Refresh Today facts' }).click()
  await expect(page.getByRole('region', { name: 'Your event Today' })).toHaveCount(0)
  release()
  await expect(page.getByText('Late private mentor canary')).toHaveCount(0)
  expect(f.calls.some(c => c.path.endsWith('/mentor-coverage'))).toBe(false)
  expect(f.calls.filter(c => c.method !== 'GET')).toEqual([])
})

test('staff actions resolve in the configured independent portal and forged sections never mount capabilities', async ({ page }, info) => {
  const f = await todayFixture(page, ['mc'])
  f.state.administrationOrigin = 'https://admin.example.test'
  await openToday(page)
  await page.getByRole('combobox', { name: 'Today context' }).selectOption('mc')
  if (info.project.metadata.portal === 'participant') {
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

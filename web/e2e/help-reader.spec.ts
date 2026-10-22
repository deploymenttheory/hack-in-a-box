import { test, expect, type Page } from '@playwright/test'
import { firstID, secondID, mockWorkspace, person } from './fixtures'
import type { components } from '../src/api/schema.gen'

const requestID = '7bcf1f17-79d3-4449-8ab7-bf343d4fa03c', helperID = '61f7b514-9403-4fb4-bff8-d49ed7df50c6'
async function setup(page: Page) {
  await mockWorkspace(page)
  let denied = false
  const writes: string[] = []
  await page.route('**/api/v1/session', route => route.fulfill({ json: { person: { ...person, platform_admin: false }, csrf_token: 'synthetic-csrf-token-32-characters', can_create_events: false } }))
  const data: components['schemas']['HelpPage'] = {
    event_name: 'North Star Hack', timezone: 'Europe/London',
    request: { id: requestID, event_id: firstID, team_id: '29bd53a6-91a9-4328-9eec-e17bfc14d29f', requester_id: person.id, revision: 2, category: 'competition_output', description: 'SYNTHETIC-PRIVATE-PROBLEM\n<script>window.untrustedHelp=true</script>\n' + 'unbroken-context-'.repeat(70), location: 'Synthetic help desk', meeting_url: 'https://meet.example.test/approved-room', status: 'claimed', assignee_id: helperID, waiting_cycle: 1, requested_at: '2027-06-20T09:00:00Z', waiting_since: '2027-06-20T09:00:00Z', first_claimed_at: '2027-06-20T09:02:00Z', claimed_at: '2027-06-20T09:02:00Z', resolved_at: null, updated_at: '2027-06-20T09:02:00Z' },
    history: [{ revision: 1, action: 'create', status: 'waiting', actor_id: person.id, assignee_id: null, at: '2027-06-20T09:00:00Z' }, { revision: 2, action: 'claim', status: 'claimed', actor_id: helperID, assignee_id: helperID, at: '2027-06-20T09:02:00Z' }], can_claim: false, can_release: false, can_transfer: false, can_resolve: false, can_cancel: true, can_reopen: false,
  }
  await page.route('**/api/v1/events/*/help/*', route => {
    if (route.request().method() !== 'GET') writes.push(route.request().url())
    if (denied || !route.request().url().includes(`/events/${firstID}/`)) return route.fulfill({ status: 404, json: { error: { code: 'not_found', message: 'Unavailable', request_id: 'synthetic-help' } } })
    return route.fulfill({ json: data })
  })
  await page.goto(`/events/${firstID}/help/${requestID}`)
  return { data, writes, deny: () => { denied = true } }
}

test('actual help resource route renders safe current context and operational history', async ({ page }, info) => {
  const { writes } = await setup(page)
  await expect(page.getByRole('heading', { name: 'Team help request', exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Claimed', exact: true })).toBeVisible()
  await expect(page.getByText('SYNTHETIC-PRIVATE-PROBLEM', { exact: false })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Open approved meeting route' })).toHaveAttribute('rel', 'noopener noreferrer')
  await expect(page.getByText('create → waiting', { exact: false })).toBeVisible()
  await expect(page.getByText('claim → claimed', { exact: false })).toBeVisible()
  expect(await page.evaluate(() => 'untrustedHelp' in window)).toBe(false)
  await expect(page.locator('article script,article img,article iframe,textarea,input')).toHaveCount(0)
  expect(writes).toEqual([])
  await page.screenshot({ path: info.outputPath('help-current-detail.png'), fullPage: true })
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
})

test('refreshing after authority loss clears cached request prose and links', async ({ page }) => {
  const state = await setup(page)
  await expect(page.getByText('SYNTHETIC-PRIVATE-PROBLEM', { exact: false })).toBeVisible()
  state.deny()
  await page.getByRole('button', { name: 'Refresh request' }).focus()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('alert')).toBeVisible()
  await expect(page.getByText('SYNTHETIC-PRIVATE-PROBLEM', { exact: false })).toHaveCount(0)
  await expect(page.getByRole('link', { name: 'Open approved meeting route' })).toHaveCount(0)
})

test('another event cannot reuse a help resource reference', async ({ page }) => {
  await setup(page)
  await expect(page.getByText('SYNTHETIC-PRIVATE-PROBLEM', { exact: false })).toBeVisible()
  await page.goto(`/events/${secondID}/help/${requestID}`)
  await expect(page.getByRole('alert')).toBeVisible()
  await expect(page.getByText('SYNTHETIC-PRIVATE-PROBLEM', { exact: false })).toHaveCount(0)
})

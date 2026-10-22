import { test, expect, type Page } from '@playwright/test'
import { mockWorkspace, firstID } from './fixtures'

async function setup(page: Page, available = true) {
  await mockWorkspace(page)
  let revision = 1
  const state = { revision, encryption_ready: available, connection: { id: '79d682c3-cb4e-42ab-80c8-b37d25c68012', version: 1, label: 'Approved channel', owner: 'Workflow owner', co_owner: 'Workflow co-owner', active: true, available, updated_at: '2026-10-22T12:00:00Z' }, history: [] as object[] }
  const requests: { path: string; body: Record<string, unknown> }[] = []
  await page.route(`**/events/${firstID}/communications**`, route => {
    if (new URL(route.request().url()).pathname.endsWith('/campaigns')) return route.fulfill({ json: { revision, timezone: 'Europe/London', can_manage: true, campaigns: [], providers: [{ channel: 'teams', available, config_revision: 'connection:' + state.connection.version, supports_self_test: false, self_test_available: false, reason: available ? 'ready' : 'not_configured', self_test_reason: 'broadcast_only', setup_url: '#teams-workflow-setup' }] } })
    return route.fulfill({ json: { revision, variables: [], preview: null, templates: [], audiences: [] } })
  })
  await page.route(`**/events/${firstID}/teams-workflow**`, route => {
    const req = route.request(), path = new URL(req.url()).pathname
    if (req.method() === 'POST') {
      const body = req.postDataJSON(); requests.push({ path, body }); revision++; state.revision = revision
      if (path.endsWith('/test')) state.history = [{ id: 'test', kind: 'connection_test', status: 'provider_accepted', reason: 'provider_accepted', attempts: 1, retry_at: null }]
      else { state.connection.version++; state.connection.label = body.label; state.connection.owner = body.owner; state.connection.co_owner = body.co_owner }
    }
    return route.fulfill({ json: state })
  })
  await page.goto(`/events/${firstID}`)
  await page.getByRole('button', { name: 'Campaigns', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Teams workflow connection' })).toBeVisible()
  return { requests }
}
test('write-only connection replacement preserves composer edits and requires current review', async ({ page }, info) => {
  const { requests } = await setup(page)
  await page.getByLabel('Campaign name', { exact: true }).fill('Unsaved organiser draft')
  const endpoint = 'https://synthetic.environment.api.powerplatform.com/?sig=SECRET-BROWSER-CANARY'
  await page.getByLabel('New secret workflow endpoint').fill(endpoint)
  await page.getByRole('button', { name: 'Save Teams connection' }).click()
  await expect(page.getByLabel('New secret workflow endpoint')).toHaveValue('')
  await expect(page.getByRole('heading', { name: 'Review changed campaign facts' })).toBeVisible()
  await expect(page.getByLabel('Campaign name', { exact: true })).toHaveValue('Unsaved organiser draft')
  expect(requests[0].body.endpoint).toBe(endpoint)
  expect(await page.locator('body').innerText()).not.toContain('SECRET-BROWSER-CANARY')
  expect(await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }))).not.toContain('SECRET-BROWSER-CANARY')
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
  await page.getByRole('heading', { name: 'Teams workflow connection' }).scrollIntoViewIfNeeded()
  await page.getByRole('region', { name: 'Teams workflow connection' }).screenshot({ path: info.outputPath('teams-setup.png') })
})
test('shared-channel TEST is explicit and acceptance never claims Teams delivery', async ({ page }) => {
  const { requests } = await setup(page)
  page.once('dialog', dialog => dialog.dismiss())
  await page.getByRole('button', { name: 'Queue shared-channel TEST' }).click()
  expect(requests).toHaveLength(0)
  page.once('dialog', dialog => dialog.accept())
  await page.getByRole('button', { name: 'Queue shared-channel TEST' }).click()
  await expect(page.getByText('Connection TEST · provider accepted')).toBeVisible()
  expect(Object.keys(requests[0].body)).toEqual(['request_id'])
  await expect(page.getByText('it does not prove the workflow posted to Teams', { exact: false })).toBeVisible()
  await expect(page.getByRole('button', { name: /Teams test to myself/ })).toHaveCount(0)
})
test('missing encryption keys fail closed in setup', async ({ page }) => {
  await setup(page, false)
  await expect(page.getByLabel('New secret workflow endpoint')).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Queue shared-channel TEST' })).toBeDisabled()
  await expect(page.getByText('Endpoint encryption is unavailable.', { exact: false })).toBeVisible()
})

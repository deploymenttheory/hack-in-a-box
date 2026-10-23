import { openEventSection } from './event-navigation-fixture'
import { expect, type Page } from '@playwright/test'
import { mockWorkspace, firstID } from './fixtures'

export async function setupTeams(page: Page, available = true) {
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
  await openEventSection(page, 'Campaigns')
  await expect(page.getByRole('heading', { name: 'Teams workflow connection' })).toBeVisible()
  return { requests }
}

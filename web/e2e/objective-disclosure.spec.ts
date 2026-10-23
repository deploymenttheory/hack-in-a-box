import { expect, test } from '@playwright/test'
import { firstID, mockWorkspace } from './fixtures'
import { objectiveID, measureID, resourceID } from './objective-fixtures'

async function releasedObjective(page: import('@playwright/test').Page, target: string | null = '0') {
  const workspace = await mockWorkspace(page)
  workspace.events[0].can_edit = false; workspace.events[0].can_read_staff = false; workspace.events[0].can_present = false
  const state = { unavailable: false, planningReads: 0 }
  await page.route(/\/api\/v1\/events\/[^/]+\/(?:objectives|objective-owners)(?:\/|\?|$)/, route => { state.planningReads++; return route.fulfill({ status: 403, json: { error: { code: 'forbidden', message: 'Planning is private', request_id: 'synthetic' } } }) })
  await page.route(`**/api/v1/events/${firstID}/content/${resourceID}**`, route => {
    if (state.unavailable) return route.fulfill({ status: 404, json: { error: { code: 'not_found', message: 'Current resource is unavailable', request_id: 'synthetic' } } })
    if (new URL(route.request().url()).pathname.includes('/objectives/')) return route.fulfill({ json: { reference: { id: objectiveID, version: 1 }, description: 'REVIEWED SHARED OBJECTIVE <script>window.disclosureLeak=true</script>', measures: [{ id: measureID, description: 'Working examples', target, unit: 'working example', method: 'An explicit later evidence review', evidence_source: 'Review source records', review_horizon: 'One month after the event' }] } })
    return route.fulfill({ json: { id: resourceID, kind: 'page', revision: 3, version: 3, state: 'released', document: { title: 'Released objective guidance', markdown: 'The exact approved objective reference is included below.', audience: { confirmed: true, roles: [], teams: [] }, attachments: [], scheduled_at: null, require_acknowledgement: false, objectives: [{ objective: { id: objectiveID, version: 1 }, measure_ids: [measureID] }] }, released_version: 3, released_at: '2027-06-13T09:00:00Z', can_manage: false, message: '', attachment_details: [], acknowledged_at: null } })
  })
  return state
}

test('attendee released resource resolves only its exact public reference without planning requests or private fields', async ({ page }, info) => {
  const state = await releasedObjective(page); await page.goto(`/events/${firstID}/resources/${resourceID}`)
  await expect(page.getByRole('heading', { name: 'Event objective · Definition version 1' })).toBeVisible(); await expect(page.getByText('0 working example', { exact: true })).toBeVisible(); await expect(page.getByText('A target is not an achieved result.', { exact: false })).toBeVisible()
  await expect(page.getByLabel('Objective owner (optional)')).toHaveCount(0); await expect(page.getByLabel('Objective description', { exact: true })).toHaveCount(0); expect(state.planningReads).toBe(0); expect(await page.evaluate(() => 'disclosureLeak' in window)).toBe(false)
  await page.screenshot({ path: info.outputPath('released-objective-attendee.png'), fullPage: true }); expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(page.viewportSize()!.width)
})

test('a missing disclosed target is unknown rather than a demonstrated zero', async ({ page }) => {
  const state = await releasedObjective(page, null); await page.goto(`/events/${firstID}/resources/${resourceID}`); await expect(page.getByText('Unknown', { exact: true })).toBeVisible(); await expect(page.getByText('0 working example', { exact: true })).toHaveCount(0); expect(state.planningReads).toBe(0)
})

test('current source withdrawal hides a previously released objective without a focus change', async ({ page }) => {
  const state = await releasedObjective(page); await page.goto(`/events/${firstID}/resources/${resourceID}`); await expect(page.getByText('REVIEWED SHARED OBJECTIVE', { exact: false })).toBeVisible(); state.unavailable = true
  await expect(page.getByText('REVIEWED SHARED OBJECTIVE', { exact: false })).toHaveCount(0, { timeout: 20_000 }); expect(state.planningReads).toBe(0)
})

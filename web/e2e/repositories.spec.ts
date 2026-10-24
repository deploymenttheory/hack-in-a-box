import { expect, test } from '@playwright/test'
import { firstID, fixtureEvent, mockWorkspace } from './fixtures'
import { openEventSection } from './event-navigation-fixture'
const teamID = '33333333-3333-4333-8333-333333333333'
test('current repository collection opens an exact team inspector with independent private read and keyboard return', async ({ page }, info) => {
  await mockWorkspace(page)
  const regular = info.project.metadata.portal === 'regular'
  await page.route(`**/api/v1/events/${firstID}`, route => route.fulfill({ json: { ...fixtureEvent(), can_read_staff: !regular, can_edit: !regular, published_at: '2026-10-01T12:00:00Z' } }))
  let reads = 0, writes = 0
  await page.route(`**/api/v1/events/${firstID}/repositories**`, async route => {
    if (route.request().method() !== 'GET') { writes++; return route.abort() }
    const path = new URL(route.request().url()).pathname
    if (path.endsWith('/access')) { reads++; return route.fulfill({ json: { revision: 1, team_id: teamID, plan: null, grants: [], execution: [], can_manage: true, grants_open: true, flags: [{ code: 'verified_account_required', person_id: '44444444-4444-4444-8444-444444444444', github_id: null }], observed_at: null } }) }
    return route.fulfill({ json: [{ team_id: teamID, name: 'synthetic-repository', organization: 'synthetic-org', github_repository_id: 123456, state: 'ready', diagnostic: '', unused: false }, ...(!regular ? [{ team_id: '55555555-5555-4555-8555-555555555555', name: 'unused-repository', organization: 'synthetic-org', github_repository_id: null, state: 'ambiguous', diagnostic: 'provider_unavailable', unused: true }] : [])] })
  })
  await page.goto(`/events/${firstID}`); await openEventSection(page, 'Repositories')
  await expect(page.getByRole('heading', { name: 'Repositories', exact: true })).toBeVisible()
  await expect(page.getByText('Repository confirmed', { exact: true })).toBeVisible()
  if (!regular) await expect(page.getByText('Unused · unscored', { exact: true })).toBeVisible()
  const opener = page.getByRole('button', { name: 'Inspect repository', exact: true }).first()
  await opener.focus(); await page.keyboard.press('Enter')
  const inspector = page.getByRole('region', { name: 'synthetic-org/synthetic-repository', exact: true })
  await expect(inspector).toContainText(teamID); await expect(inspector).toContainText('123456')
  if (!regular) { await expect(inspector).toContainText('verified account required'); expect(reads).toBe(1) }
  else expect(reads).toBe(0)
  await inspector.getByRole('link', { name: 'Back to repositories', exact: true }).click(); await expect(opener).toBeFocused()
  expect(writes).toBe(0); expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(page.viewportSize()!.width)
})

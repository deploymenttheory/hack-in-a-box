import { expect, test } from '@playwright/test'
import { firstID } from './fixtures'
import { setup, teamID } from './deliverable-fixtures'
import { openEventSection } from './event-navigation-fixture'

for (const outcome of ['acknowledged', 'lost'] as const) test(`team membership loss fences a held private draft ${outcome} while original metadata survives`, async ({ page }) => {
  await page.clock.install({ time: new Date('2027-06-14T09:00:00Z') })
  const f = await setup(page, { live: false })
  let release!: () => void, started!: () => void
  const hold = new Promise<void>(resolve => { release = resolve }), began = new Promise<void>(resolve => { started = resolve })
  let writes = 0
  await page.route(`**/events/${firstID}/teams/${teamID}/submission/draft`, async route => {
    if (route.request().method() !== 'PUT') return route.fallback()
    writes++; const body = route.request().postDataJSON(); started(); await hold
    if (outcome === 'lost') return route.fulfill({ status: 503, json: { error: { code: 'unavailable', message: 'Synthetic lost original', request_id: 'synthetic' } } })
    return route.fulfill({ json: { revision: 1, evidence: body.evidence, updated_at: '2027-06-14T09:00:00Z' } })
  })
  await page.getByLabel('Evidence type for Repository', { exact: true }).selectOption('repository')
  await page.getByLabel('Reference URL for Repository', { exact: true }).fill('https://example.test/PRIVATE-HELD-DRAFT')
  await page.getByRole('button', { name: 'Save private draft', exact: true }).click()
  await page.getByRole('button', { name: 'Confirm private draft', exact: true }).click(); await began
  await expect(page.getByRole('complementary', { name: 'Unconfirmed command history', exact: true, includeHidden: true })).toContainText('Awaiting original acknowledgement')
  await page.route(`**/events/${firstID}/teams`, route => route.fulfill({ json: { revision: f.event.revision, can_manage: false, can_create: false, frozen: true, freeze_at: f.event.configuration.team_freeze, teams: [], people: [], requests: [] } }))
  const currentTeams = page.waitForResponse(response => new URL(response.url()).pathname === `/api/v1/events/${firstID}/teams` && response.request().method() === 'GET')
  await page.clock.runFor(16_000)
  await currentTeams
  await expect(page.getByRole('region', { name: 'Team evidence editor', exact: true, includeHidden: true })).toHaveCount(0)
  await expect(page.getByRole('alertdialog')).toHaveCount(0)
  await expect(page.getByRole('complementary', { name: 'Unconfirmed command history', exact: true })).not.toContainText('PRIVATE-HELD-DRAFT')
  release()
  if (outcome === 'acknowledged') await expect(page.getByRole('complementary', { name: 'Unconfirmed command history', exact: true })).toHaveCount(0)
  else {
    await expect(page.getByRole('complementary', { name: 'Unconfirmed command history', exact: true })).toContainText('Original outcome remains unconfirmed')
    page.on('dialog', dialog => { void dialog.accept() })
    await openEventSection(page, 'Overview'); await openEventSection(page, 'Deliverables')
    await expect(page.getByRole('complementary', { name: 'Unconfirmed command history', exact: true })).toContainText('Expected revision 0')
  }
  expect(await page.locator('body').textContent()).not.toContain('PRIVATE-HELD-DRAFT')
  expect(writes).toBe(1)
})

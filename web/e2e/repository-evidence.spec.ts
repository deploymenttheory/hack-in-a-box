import { expect, test, type Locator, type Page } from '@playwright/test'
import { firstID, person, fixtureEvent, mockWorkspace } from './fixtures'
import { expectEventSection, openEventSection } from './event-navigation-fixture'
import type { components } from '../src/api/schema.gen'
type S = components['schemas']
const teamID = '33333333-3333-4333-8333-333333333333'
// The identical harness runs against the real baseline and current production
// bundles. Baseline has no repository consumer; capture its adjacent task context
// honestly, rather than inventing a previous repository screen.
async function evidenceFixture(page: Page, regular: boolean) {
  await mockWorkspace(page)
  const event: S['Event'] = { ...fixtureEvent(), stage: 'preparation', published_at: '2026-10-01T12:00:00Z', can_read_staff: !regular, can_edit: !regular }
  const member: S['RepositoryAccessMember'] = { person_id: person.id, github_id: 98765, link_revision: 2, permission: 'push', role: 'competitor', coverage_start: null, coverage_end: null }
  const plan: S['RepositoryAccessPlan'] = { id: '44444444-4444-4444-8444-444444444444', version: 1, event_id: firstID, team_id: teamID, repository_id: 123456, fingerprint: 'synthetic-exact-membership', reason: 'Review current published competitors', approval_reason: '', members: [member], actions: [{ id: '55555555-5555-4555-8555-555555555555', kind: 'invite', member, grant_id: null, previous_state: '' }], flags: [], created_at: '2026-10-24T12:00:00Z', approved_at: null, approved_by: null }
  let writes = 0
  await page.route(`**/api/v1/events/${firstID}**`, route => {
    const request = route.request(), path = new URL(request.url()).pathname
    if (request.method() !== 'GET') { writes++; return route.fulfill({ status: 403, json: { error: { code: 'forbidden', message: 'Evidence never executes external actions' } } }) }
    if (path === `/api/v1/events/${firstID}`) return route.fulfill({ json: event })
    if (path.endsWith('/teams')) return route.fulfill({ json: { revision: 1, can_manage: !regular, can_create: false, frozen: true, freeze_at: event.configuration.team_freeze, teams: [{ id: teamID, name: 'North Star builders', state: 'published', captain: person.id, published_captain: person.id, invitation_code: null, frozen: true, draft: [], published: [{ id: person.id, name: person.display_name }], allocation_locked: true, capacity: null }], people: [], requests: [] } })
    if (path.endsWith('/credentials')) return route.fulfill({ json: { revision: 1, can_manage: false, items: [] } })
    if (path.endsWith('/github-app')) return route.fulfill({ json: { revision: 1, configured: true, installation_id: 45678, webhook_configured: true, configuration: { app_id: 42, app_slug: 'north-star-app', organization_id: 99, organization: 'north-star-builders', template_repository: 'hack-starter' }, diagnostics: { code: 'healthy', checks: [{ operation: 'manage_repository_invitations', required: ['administration:write'], allowed: true }], checked_at: '2026-10-24T12:00:00Z' } } })
    if (path.endsWith('/repositories')) return route.fulfill({ json: [{ team_id: teamID, name: 'north-star-builders', organization: 'north-star-hack', github_repository_id: 123456, state: 'ready', diagnostic: '', unused: false }] })
    if (path.endsWith('/access')) return route.fulfill({ json: { revision: 1, team_id: teamID, plan, grants: [], execution: [], can_manage: !regular, grants_open: true, flags: [], observed_at: '2026-10-24T12:00:00Z' } })
    if (path.endsWith('/invitations')) return route.fulfill({ json: { team_id: teamID, repository_id: 123456, current_github_id: 98765, link_revision: 2, grants: [{ id: '88888888-8888-4888-8888-888888888888', member, app_managed: true, effect_uncertain: false, state: 'pending', invitation_id: 91234, diagnostic: 'awaiting_acceptance', observed_at: '2026-10-24T12:00:00Z' }], observed_at: '2026-10-24T12:00:00Z', diagnostic: '' } })
    return route.fallback()
  })
  return () => expect(writes).toBe(0)
}
async function capture(page: Page, target: Locator, path: string) {
  if (process.env.REPOSITORY_EVIDENCE_ONLY && !path.endsWith(`-${process.env.REPOSITORY_EVIDENCE_ONLY}.png`)) return
  const original = page.viewportSize()!
  // Extend only the browser viewport height so the real scroll container and
  // complete action region fit. Product styles and layout width stay unchanged.
  const bounds = await target.boundingBox(); if (!bounds) throw new Error('Missing actual evidence region')
  await page.setViewportSize({ width: original.width, height: Math.ceil(bounds.height + 500) })
  await target.scrollIntoViewIfNeeded(); await expect(target).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(original.width)
  await target.screenshot({ path, animations: 'disabled' }); await page.setViewportSize(original)
}
test('matched synthetic repository task evidence without provider effects', async ({ page }, info) => {
  test.skip(!process.env.REPOSITORY_EVIDENCE_DIR, 'Explicit visual evidence capture only')
  const regular = info.project.metadata.portal === 'regular', baseline = process.env.REPOSITORY_BASELINE === '1', noWrites = await evidenceFixture(page, regular)
  const name = `${baseline ? 'before' : 'after'}-${regular ? 'participant' : 'admin'}-${info.project.name}`
  const path = (state: string) => `${process.env.REPOSITORY_EVIDENCE_DIR}/${name}-${state}.png`
  await page.goto(`/events/${firstID}`)
  if (baseline) {
    await expectEventSection(page, 'Repositories', false)
    await openEventSection(page, regular ? 'Team access' : 'Teams')
    await expect(page.getByRole('heading', { name: regular ? 'Team access' : 'Team allocation', exact: true })).toBeVisible()
    await capture(page, page.locator('.fleet-planning-layout'), path('adjacent-task-context')); noWrites(); return
  }
  await openEventSection(page, 'Repositories'); await expect(page.getByText('Repository confirmed', { exact: true })).toBeVisible()
  await capture(page, page.locator('.fleet-planning-layout'), path('directory'))
  await page.getByRole('button', { name: 'Inspect repository', exact: true }).click()
  const inspector = page.getByRole('region', { name: 'north-star-hack/north-star-builders', exact: true })
  if (regular) await expect(inspector).toContainText('Invitation 91234')
  else await expect(inspector.getByRole('region', { name: 'Exact repository access plan', exact: true })).toContainText('Numeric repository 123456')
  await capture(page, inspector, path(regular ? 'own-invitation' : 'access-plan'))
  if (!regular) {
    await page.getByLabel('Reason for repository access change', { exact: true }).fill('Grant the reviewed published team access to this repository')
    await page.getByRole('button', { name: 'Review exact access approval', exact: true }).click()
    const review = page.getByRole('region', { name: 'Review repository access change', exact: true }); await expect(review).toContainText('123456')
    await capture(page, review, path('approval-review'))
    await page.getByRole('button', { name: 'Correct details', exact: true }).click()
    await page.getByRole('button', { name: 'Open App setup', exact: true }).click()
    await page.getByRole('button', { name: 'Review organisation installation', exact: true }).click()
    const installation = page.getByRole('region', { name: 'Review App setup change', exact: true }); await expect(installation).toContainText('north-star-app')
    await capture(page, installation, path('installation-review'))
  }
  noWrites()
})

import { test, expect } from '@playwright/test'
import { mockWorkspace, firstID, person } from './fixtures'
import { registrationEvent, fixtureRegistration } from './registration-fixtures'
import { defaultBranding } from '../src/branding'
import type { components } from '../src/api/schema.gen'

const teamID = '00000000-0000-4000-8000-000000000011', joinerID = '00000000-0000-4000-8000-000000000012'
function teamPage(manage: boolean): components['schemas']['TeamPage'] {
  return { revision: 2, can_manage: manage, can_create: true, frozen: false, freeze_at: registrationEvent.team_freeze, people: manage ? [{ id: person.id, name: person.display_name, preference: 'captain' }, { id: joinerID, name: 'Solo competitor', preference: 'solo' }] : [], requests: [], teams: [{ id: teamID, name: 'North Stars', state: 'draft', captain: person.id, published_captain: null, invitation_code: teamID, frozen: false, draft: [{ id: person.id, name: person.display_name }], published: [] }] }
}
test('captain sees pending teammates without seats and draft membership', async ({ page }) => {
  await mockWorkspace(page)
  const teams = teamPage(false)
  teams.requests = [{ id: joinerID, team_id: teamID, person: { id: joinerID, name: 'Waiting teammate' }, confirmed: false }]
  await page.route('**/api/v1/registration/events/**', route => route.fulfill({ json: { event: registrationEvent, branding: defaultBranding, forms: [], registrations: [fixtureRegistration()] } }))
  await page.route(`**/events/${firstID}/teams`, route => route.fulfill({ json: teams }))
  await page.goto(`/register/${firstID}`)
  await expect(page.getByText(/Waiting teammate · No confirmed event seat/)).toBeVisible()
  await expect(page.getByRole('button', { name: 'Approve Waiting teammate' })).toBeDisabled()
  await expect(page.getByText('No published assignment yet.')).toBeVisible()
  await expect(page.getByText('Draft membership · 1')).toBeVisible()
  await expect(page.getByLabel('Share this invitation code')).toHaveValue(teamID)
  await expect(page.getByText(/If you chose solo placement/)).toBeVisible()
})
test('organiser freeze displays server blockers and then locks membership', async ({ page }, testInfo) => {
  const { events } = await mockWorkspace(page)
  events[0].stage = 'preparation'; events[0].confirmed_attendees = 3; events[0].configuration.minimum_attendance = 3; events[0].configuration.capacity = 5
  let teams = teamPage(true), attempts = 0
  await page.route(`**/events/${firstID}/teams`, route => {
    if (route.request().method() === 'POST') {
      const body = route.request().postDataJSON(); expect(body.action).toBe('freeze'); expect(body.reason).toBe('All competitors reviewed'); attempts++
      if (attempts === 1) return route.fulfill({ status: 409, json: { error: { code: 'teams_unready', message: 'Resolve', request_id: 'synthetic' } } })
      teams = { ...teams, frozen: true, revision: teams.revision + 1, teams: teams.teams.map(t => ({ ...t, state: 'published', published_captain: t.captain, published: [...t.draft, { id: joinerID, name: 'Solo competitor' }, { id: '00000000-0000-4000-8000-000000000013', name: 'Another competitor' }], draft: [...t.draft, { id: joinerID, name: 'Solo competitor' }, { id: '00000000-0000-4000-8000-000000000013', name: 'Another competitor' }], frozen: true })) }
    }
    return route.fulfill({ json: teams })
  })
  await page.goto(`/events/${firstID}`); await page.getByRole('button', { name: 'Teams', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Publish and freeze teams' })).toBeDisabled()
  await page.getByLabel('Allocation decision reason').fill('All competitors reviewed')
  page.on('dialog', dialog => void dialog.accept())
  await page.getByRole('button', { name: 'Publish and freeze teams' }).click()
  await expect(page.getByRole('alert')).toContainText('resolve teams below three')
  await page.getByRole('button', { name: 'Publish and freeze teams' }).click()
  await expect(page.getByRole('button', { name: 'Publish and freeze teams' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Assign draft member' })).toBeDisabled()
  await expect(page.getByText(/Joining, moving and backfill are unavailable/)).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
  await page.screenshot({ path: testInfo.outputPath('frozen-teams.png'), fullPage: true })
})

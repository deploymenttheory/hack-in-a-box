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
  await expect(page.getByRole('button', { name: 'Review publication and freeze' })).toBeDisabled()
  await page.getByLabel('Allocation decision reason').fill('All competitors reviewed')
  await page.getByRole('button', { name: 'Review publication and freeze' }).click()
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

test('organiser reviews explanations and cancels publication without writing', async ({ page }, testInfo) => {
  await mockWorkspace(page)
  const teams = teamPage(true)
  let writes = 0
  await page.route(`**/events/${firstID}/teams`, route => { if (route.request().method() === 'POST') writes++; return route.fulfill({ json: teams }) })
  await page.route(`**/events/${firstID}/allocation-suggestions`, route => {
    expect(route.request().headers()['if-match']).toBe('"2"')
    expect(route.request().postDataJSON().declarations).toEqual([])
    return route.fulfill({ json: { revision: 2, seed: 'review-1', preferences: { target: 5, preferred_min: 4, preferred_max: 6, minimum: 3 }, limits: { maximum_size: null, maximum_teams: null }, teams: [], unresolved: [joinerID], retired_teams: [], issues: [{ code: 'minimum_size', people: [joinerID], team_id: null, message: 'Too few people for another viable group.' }] } })
  })
  await page.goto(`/events/${firstID}`); await page.getByRole('button', { name: 'Teams', exact: true }).click()
  await page.getByLabel('Allocation decision reason').fill('Review synthetic roster')
  await page.getByRole('button', { name: 'Generate allocation proposal' }).click()
  await expect(page.getByRole('alert')).toContainText('Unresolved competitors: Solo competitor')
  await expect(page.getByText('Too few people for another viable group.', { exact: false })).toBeVisible()
  await page.getByRole('button', { name: 'Review publication', exact: true }).click()
  await expect(page.getByRole('region', { name: 'Publication preview' })).toContainText('Unassigned → North Stars')
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
  await page.screenshot({ path: testInfo.outputPath('allocation-review.png'), fullPage: true })
  await page.getByRole('button', { name: 'Cancel publication review' }).click()
  await expect(page.getByRole('button', { name: 'Publish assignments', exact: true })).toHaveCount(0)
  expect(writes).toBe(0)
})

test('locked teams explain invalid moves and keyboard controls save only a draft', async ({ page }) => {
  await mockWorkspace(page)
  let teams = teamPage(true)
  const secondTeamID = '00000000-0000-4000-8000-000000000014'
  teams.teams[0].allocation_locked = true
  teams.teams.push({ ...teams.teams[0], id: secondTeamID, name: 'South Stars', draft: [], allocation_locked: false })
  const actions: string[] = []
  await page.route(`**/events/${firstID}/teams`, route => {
    if (route.request().method() === 'POST') {
      const body = route.request().postDataJSON(); actions.push(body.action)
      if (body.action === 'unlock') teams = { ...teams, revision: teams.revision + 1, teams: teams.teams.map(t => t.id === teamID ? { ...t, allocation_locked: false } : t) }
      else if (body.action === 'move') {
        expect(body.person_id).toBe(person.id); expect(body.team_id).toBe(secondTeamID)
        teams = { ...teams, revision: teams.revision + 1, teams: teams.teams.map(t => ({ ...t, draft: t.id === secondTeamID ? [{ id: person.id, name: person.display_name }] : [] })) }
      } else throw new Error(`Unexpected publication: ${body.action}`)
    }
    return route.fulfill({ json: teams })
  })
  await page.goto(`/events/${firstID}`); await page.getByRole('button', { name: 'Teams', exact: true }).click()
  await page.getByLabel('Allocation decision reason').fill('Reviewed explicit unlock and reassignment')
  await page.locator('#move-person').selectOption(person.id); await page.locator('#move-team').selectOption(secondTeamID)
  await expect(page.getByRole('button', { name: 'Assign draft member' })).toBeDisabled()
  await expect(page.locator('#move-explanation')).toContainText('Explicitly unlock')
  await page.getByRole('button', { name: 'Unlock team', exact: true }).focus(); await page.keyboard.press('Enter')
  const move = page.getByRole('button', { name: 'Assign draft member' })
  await expect(move).toBeEnabled(); await move.focus(); await page.keyboard.press('Enter')
  await expect(page.getByRole('status').filter({ hasText: 'Team change saved' })).toBeVisible()
  expect(actions).toEqual(['unlock', 'move'])
})

test('stale publication discards the preview and requires renewed review', async ({ page }) => {
  await mockWorkspace(page)
  let teams = teamPage(true), publications = 0
  await page.route(`**/events/${firstID}/teams`, route => {
    if (route.request().method() === 'POST') {
      publications++; expect(route.request().headers()['if-match']).toBe('"2"')
      teams = { ...teams, revision: 3 }
      return route.fulfill({ status: 409, json: { error: { code: 'conflict', message: 'Roster changed', request_id: 'synthetic' } } })
    }
    return route.fulfill({ json: teams })
  })
  await page.goto(`/events/${firstID}`); await page.getByRole('button', { name: 'Teams', exact: true }).click()
  await page.getByLabel('Allocation decision reason').fill('Reviewed revision two')
  await page.getByRole('button', { name: 'Review publication', exact: true }).click()
  await page.getByRole('button', { name: 'Publish assignments', exact: true }).click()
  await expect(page.getByText('The roster changed. Review the current changes again before publishing.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Publish assignments', exact: true })).toHaveCount(0)
  expect(publications).toBe(1)
})

test('event cancellation during allocation review rejects publication without retry', async ({ page }) => {
  await mockWorkspace(page)
  let writes = 0
  await page.route(`**/events/${firstID}/teams`, route => {
    if (route.request().method() === 'POST') {
      writes++
      return route.fulfill({ status: 403, json: { error: { code: 'forbidden', message: 'Event cancelled', request_id: 'synthetic' } } })
    }
    return route.fulfill({ json: teamPage(true) })
  })
  await page.goto(`/events/${firstID}`); await page.getByRole('button', { name: 'Teams', exact: true }).click()
  await page.getByLabel('Allocation decision reason').fill('Reviewed before cancellation')
  await page.getByRole('button', { name: 'Review publication', exact: true }).click()
  await page.getByRole('button', { name: 'Publish assignments', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('You do not have access to this resource.')
  expect(writes).toBe(1)
})

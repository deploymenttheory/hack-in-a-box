import { test, expect, type Page } from '@playwright/test'
import { mockWorkspace, firstID, person } from './fixtures'
import { attendeeForm, fixtureRegistration, questionID, registrationEvent } from './registration-fixtures'
import { defaultBranding } from '../src/branding'
import type { components } from '../src/api/schema.gen'

const invitation = '00000000-0000-4000-8000-000000000011'
type Registration = components['schemas']['Registration']
type TeamPage = components['schemas']['TeamPage']

async function participantRegistration(page: Page, preference: 'join' | 'solo', state: 'confirmed' | 'waitlisted') {
  await mockWorkspace(page)
  await page.route('**/api/v1/session', route => route.fulfill({ json: { person: { ...person, platform_admin: false }, csrf_token: 'synthetic-csrf-token-32-characters', can_create_events: false } }))
  let registrations: Registration[] = []
  const commands: unknown[] = []
  let teams: TeamPage = { revision: 1, can_manage: false, can_create: state === 'confirmed', frozen: false, freeze_at: registrationEvent.team_freeze, teams: [], people: [], requests: [] }
  await page.route('**/api/v1/registration/events/**', route => route.fulfill({ json: { event: registrationEvent, branding: defaultBranding, forms: [attendeeForm], registrations } }))
  await page.route(`**/events/${firstID}/registrations`, route => {
    const body = route.request().postDataJSON()
    expect(body.team_preference).toBe(preference)
    expect(body.invitation_reference).toBe(preference === 'join' ? invitation : '')
    expect(body.answers[questionID]).toBe('Synthetic collaboration goal')
    registrations = [{ ...fixtureRegistration(state), id: body.id, team_preference: preference, invitation_reference: body.invitation_reference }]
    return route.fulfill({ json: registrations[0] })
  })
  await page.route(`**/events/${firstID}/teams`, route => {
    if (route.request().method() === 'POST') {
      const body = route.request().postDataJSON()
      commands.push(body)
      expect(body).toEqual({ action: 'join', invitation_code: invitation })
      expect(route.request().headers()['if-match']).toBe('"1"')
      teams = { ...teams, revision: 2, requests: [{ id: '00000000-0000-4000-8000-000000000012', team_id: invitation, person: { id: person.id, name: person.display_name }, confirmed: state === 'confirmed' }] }
    }
    return route.fulfill({ json: teams })
  })
  await page.goto(`/register/${firstID}`)
  await page.getByLabel('What would you like to learn? (required)').fill('Synthetic collaboration goal')
  await page.getByLabel('Team preference', { exact: true }).selectOption(preference)
  if (preference === 'join') await page.getByLabel('Team invitation reference (optional)').fill(invitation)
  await page.getByRole('button', { name: 'Review application', exact: true }).click()
  await page.getByRole('button', { name: 'Submit application' }).click()
  await expect(page.getByRole('heading', { name: 'Your team', exact: true })).toBeVisible()
  expect(commands).toEqual([])
  return { commands, registrations: () => registrations, teams: () => teams }
}

test('confirmed joiner registers and explicitly requests an opaque code without automatic assignment', async ({ page }) => {
  const state = await participantRegistration(page, 'join', 'confirmed')
  await expect(page.getByRole('region', { name: 'attendee application status' }).getByRole('status')).toHaveText('Your competitor place is confirmed.')
  await page.getByLabel('Captain’s invitation code').fill(invitation)
  await page.getByRole('button', { name: 'Request to join', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Pending join requests' })).toBeVisible()
  await expect(page.getByText(/Synthetic Person · Confirmed competitor/)).toBeVisible()
  await expect(page.getByRole('button', { name: /Approve/ })).toHaveCount(0)
  expect(state.commands).toHaveLength(1)
  expect(state.registrations()).toHaveLength(1)
  expect(state.teams().teams).toEqual([])
  expect(state.teams().people).toEqual([])
})

test('solo registration explains organiser placement without requesting or creating a team', async ({ page }) => {
  const state = await participantRegistration(page, 'solo', 'confirmed')
  await expect(page.getByText(/If you chose solo placement, the organiser will assign you a team/)).toBeVisible()
  await expect(page.getByText(/Target five competitors; four to six is preferred/)).toBeVisible()
  await expect(page.getByText(/Hard team freeze:/)).toBeVisible()
  expect(state.commands).toEqual([])
  expect(state.teams().teams).toEqual([])
  expect(state.registrations()[0].team_preference).toBe('solo')
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
})

test('waitlisted joiner can request a team but has no confirmed seat or private membership', async ({ page }) => {
  const state = await participantRegistration(page, 'join', 'waitlisted')
  await expect(page.getByText('Waitlist position: 2')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Create draft team' })).toBeDisabled()
  await page.getByLabel('Captain’s invitation code').fill(invitation)
  await page.getByRole('button', { name: 'Request to join', exact: true }).click()
  await expect(page.getByText(/Synthetic Person · No confirmed event seat/)).toBeVisible()
  await expect(page.getByText(/A pending request holds no event seat/)).toBeVisible()
  await expect(page.getByRole('button', { name: /Approve/ })).toHaveCount(0)
  expect(state.commands).toHaveLength(1)
  expect(state.registrations()[0].state).toBe('waitlisted')
  expect(state.teams().teams).toEqual([])
  expect(state.teams().requests[0].confirmed).toBe(false)
})

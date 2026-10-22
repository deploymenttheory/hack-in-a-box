import { test, expect } from '@playwright/test'
import { firstID } from './fixtures'
import { defaultBranding } from '../src/branding'
import type { components } from '../src/api/schema.gen'

import { registrationEvent, questionID, attendeeForm, fixtureRegistration, mockApplicantWorkspace } from './registration-fixtures'
test('attendee discovery and retry preserve one application identity', async ({ page }) => {
  await mockApplicantWorkspace(page)
  let registrations: components['schemas']['Registration'][] = [], attempts = 0, firstRequest = ''
  await page.route('**/api/v1/registration/events**', route => route.fulfill({ json: new URL(route.request().url()).pathname.endsWith(firstID) ? { event: registrationEvent, branding: defaultBranding, forms: [attendeeForm], registrations } : { items: [registrationEvent], next_cursor: null } }))
  await page.route(`**/events/${firstID}/registrations`, async route => {
    const body = route.request().postDataJSON(); attempts++
    if (attempts === 1) { firstRequest = body.id; return route.fulfill({ status: 503, json: { error: { code: 'unavailable', message: 'Retry', request_id: 'synthetic' } } }) }
    expect(body.id).toBe(firstRequest); expect(body.team_preference).toBe('captain'); expect(body.answers[questionID]).toBe('Collaboration')
    registrations = [{ ...fixtureRegistration(), id: body.id }]; return route.fulfill({ json: registrations[0] })
  })
  await page.goto('/register'); await page.getByRole('link', { name: 'Open Hack', exact: true }).click()
  await page.getByLabel('What would you like to learn? (required)').fill('Collaboration')
  await page.getByLabel('Team preference', { exact: true }).selectOption('captain')
  await page.getByRole('button', { name: 'Submit application' }).click(); await expect(page.getByRole('alert')).toBeVisible()
  await page.getByRole('button', { name: 'Submit application' }).click(); await expect(page.getByRole('region', { name: 'attendee application status' }).getByRole('status')).toHaveText('Your competitor place is confirmed.')
  expect(attempts).toBe(2); await expect(page.getByRole('button', { name: 'Submit application' })).toHaveCount(0)
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
})
test('facilitator receipt and a closed waitlist show distinct outcomes', async ({ page }) => {
  await mockApplicantWorkspace(page)
  const received = { ...fixtureRegistration('received'), id: '00000000-0000-4000-8000-000000000005', kind: 'facilitator', audience: 'mentor' }
  await page.route('**/api/v1/registration/events/**', route => route.fulfill({ json: { event: { ...registrationEvent, open: false, stage: 'preparation' }, branding: defaultBranding, forms: [], registrations: [fixtureRegistration('waitlisted'), received] } }))
  await page.goto(`/register/${firstID}`)
  await expect(page.getByText('Waitlist position: 2')).toBeVisible(); await expect(page.getByText('Your facilitator application has been received for review.')).toBeVisible()
  await expect(page.getByText('New applications are closed. Existing registrations remain valid.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Submit application' })).toHaveCount(0)
})

test('facilitator application records fallback preference without enrolling a competitor', async ({ page }) => {
  await mockApplicantWorkspace(page)
  const form = { ...attendeeForm, definition: { ...attendeeForm.definition, audience: 'mentor', name: 'Mentor application' } }
  let registrations: components['schemas']['Registration'][] = []
  await page.route('**/api/v1/registration/events/**', route => route.fulfill({ json: { event: registrationEvent, branding: defaultBranding, forms: [form], registrations } }))
  await page.route(`**/events/${firstID}/registrations`, route => {
    const body = route.request().postDataJSON(); expect(body.fallback_to_attendee).toBe(true)
    registrations = [{ ...fixtureRegistration('received'), id: body.id, audience: 'mentor', kind: 'facilitator', fallback_to_attendee: true }]
    return route.fulfill({ json: registrations[0] })
  })
  await page.goto(`/register/${firstID}`)
  await page.getByLabel('What would you like to learn? (required)').fill('Support new teams')
  await page.getByLabel(/If I am not selected, invite me/).check()
  await page.getByRole('button', { name: 'Submit application' }).click()
  await expect(page.getByText('Your facilitator application has been received for review.')).toBeVisible()
  await expect(page.getByText('Your competitor place is confirmed.')).toHaveCount(0)
  expect(registrations).toHaveLength(1)
})

test('ordinary applicant branches stay conditional and permanent ineligibility has a safe explanation', async ({ page }) => {
  await mockApplicantWorkspace(page)
  const choice = '00000000-0000-4000-8000-000000000011', details = '00000000-0000-4000-8000-000000000012'
  const form = { ...attendeeForm, definition: { ...attendeeForm.definition, fields: [
    { id: choice, label: 'Need equipment?', type: 'checkbox', required: true, help: '', choices: [] },
    { id: details, label: 'Equipment details', type: 'text', required: true, help: '', choices: [], visible_when: { field_id: choice, equals: 'true' } },
  ] } }
  await page.route('**/api/v1/registration/events/**', route => route.fulfill({ json: { event: registrationEvent, branding: defaultBranding, forms: [form], registrations: [] } }))
  let attempts = 0
  await page.route(`**/events/${firstID}/registrations`, route => {
    const body = route.request().postDataJSON(); attempts++
    expect(body.form_version).toBe(1); expect(body.answers[choice]).toBe(false)
    return route.fulfill({ status: 409, json: { error: { code: 'competition_ineligible', message: 'Safe fixture', request_id: 'synthetic' } } })
  })
  await page.goto(`/register/${firstID}`)
  await expect(page.getByLabel('Equipment details (required)')).toHaveCount(0)
  await page.getByLabel('Need equipment? (required)').selectOption('true')
  await page.getByLabel('Equipment details (required)').fill('Synthetic branch answer')
  await page.getByLabel('Need equipment? (required)').selectOption('false')
  await expect(page.getByLabel('Equipment details (required)')).toHaveCount(0)
  await page.getByRole('button', { name: 'Submit application' }).click()
  await expect(page.getByRole('alert')).toContainText('Previous organiser or early brief access permanently prevents competition in this event.')
  await expect(page.getByText('Your competitor place is confirmed.')).toHaveCount(0)
  await expect(page.getByLabel('Need equipment? (required)')).toHaveValue('false')
  expect(attempts).toBe(1)
})

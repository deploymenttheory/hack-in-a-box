import { test, expect } from '@playwright/test'
import { mockWorkspace, firstID } from './fixtures'
import { defaultBranding } from '../src/branding'
import type { components } from '../src/api/schema.gen'

import { registrationEvent, questionID, attendeeForm, fixtureRegistration } from './registration-fixtures'
test('attendee discovery and retry preserve one application identity', async ({ page }) => {
  await mockWorkspace(page)
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
  await page.getByRole('button', { name: 'Submit application' }).click(); await expect(page.getByRole('status')).toHaveText('Your competitor place is confirmed.')
  expect(attempts).toBe(2); await expect(page.getByRole('button', { name: 'Submit application' })).toHaveCount(0)
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
})
test('facilitator receipt and a closed waitlist show distinct outcomes', async ({ page }) => {
  await mockWorkspace(page)
  const received = { ...fixtureRegistration('received'), id: '00000000-0000-4000-8000-000000000005', kind: 'facilitator', audience: 'mentor' }
  await page.route('**/api/v1/registration/events/**', route => route.fulfill({ json: { event: { ...registrationEvent, open: false, stage: 'preparation' }, branding: defaultBranding, forms: [], registrations: [fixtureRegistration('waitlisted'), received] } }))
  await page.goto(`/register/${firstID}`)
  await expect(page.getByText('Waitlist position: 2')).toBeVisible(); await expect(page.getByText('Your facilitator application has been received for review.')).toBeVisible()
  await expect(page.getByText('New applications are closed. Existing registrations remain valid.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Submit application' })).toHaveCount(0)
})

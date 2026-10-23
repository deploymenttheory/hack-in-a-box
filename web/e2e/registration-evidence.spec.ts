import { test, expect } from '@playwright/test'
import { openEventSection } from './event-navigation-fixture'
import { mockWorkspace, firstID } from './fixtures'
import { mockApplicantWorkspace, registrationEvent, attendeeForm, fixtureRegistration } from './registration-fixtures'
import { defaultBranding } from '../src/branding'
import { mkdir } from 'node:fs/promises'
const baseline = process.env.REGISTRATION_EVIDENCE_BASELINE === '1'
const directory = '../docs/design/evidence/registration'
test('actual synthetic registration, applicant review and versioned form screens', async ({ page }, info) => {
  await mkdir(directory, { recursive: true })
  const save = async (name: string) => { await page.evaluate(() => document.fonts.ready); await page.screenshot({ path: `${directory}/${baseline ? 'before' : 'after'}-${name}-${info.project.name}.png`, fullPage: true }); expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false) }
  await mockApplicantWorkspace(page)
  await page.route('**/api/v1/registration/events/**', route => route.fulfill({ json: { event: registrationEvent, branding: defaultBranding, forms: [attendeeForm], registrations: [] } }))
  await page.goto(`http://127.0.0.1:4370/register/${firstID}`); await expect(page.getByRole('button', { name: baseline ? 'Submit application' : 'Review application' })).toBeVisible(); await save('application')
  await page.unrouteAll({ behavior: 'wait' }); await mockWorkspace(page)
  const r = { ...fixtureRegistration('received'), kind: 'facilitator', audience: 'mentor' }
  await page.route(`**/events/${firstID}/registrations/**`, route => {
    const path = new URL(route.request().url()).pathname
    if (path.endsWith('/offer-policy')) return route.fulfill({ json: { revision: 1, automated: false, working_days: 2 } })
    const applicant = { registration: r, display_name: 'Synthetic Mentor' }
    return route.fulfill({ json: path.includes(r.id) ? { applicant, response: { definition: attendeeForm.definition, answers: { [attendeeForm.definition.fields[0].id]: 'Build accessible public services' }, revision: 1 }, history: [], fallback_available: false } : { items: [applicant], next_cursor: null } })
  })
  await page.route(`**/events/${firstID}/registration/forms**`, route => route.fulfill({ json: { items: [{ definition: { ...attendeeForm.definition, name: 'Competitor application' }, revision: 1, version: 1, published_version: null }] } }))
  await page.goto(`http://127.0.0.1:4371/events/${firstID}`); await openEventSection(page, 'Applicants'); await page.getByRole('button', { name: 'Review Synthetic Mentor' }).click(); await expect(page.getByText('Build accessible public services')).toBeVisible(); await save('applicant-review')
  await openEventSection(page, 'Forms'); await page.getByRole('button', { name: 'Competitor application · Draft' }).click(); if (!baseline) await page.getByRole('button', { name: 'Edit What would you like to learn?' }).click(); await expect(page.getByLabel('Form name', { exact: true })).toBeVisible(); await save('form-builder')
})

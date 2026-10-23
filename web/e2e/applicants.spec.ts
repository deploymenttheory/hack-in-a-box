import { openEventSection } from './event-navigation-fixture'
import { test, expect } from '@playwright/test'
import { mockWorkspace, firstID } from './fixtures'
import { fixtureRegistration } from './registration-fixtures'

test('organiser reviews facilitator conflicts and records a backup decision', async ({ page }) => {
  await mockWorkspace(page)
  let r = { ...fixtureRegistration('received'), kind: 'facilitator', audience: 'mentor', fallback_to_attendee: true }
  const history: { action: string; from: string; to: string; reason: string; at: string }[] = []
  await page.route(`**/events/${firstID}/registrations/**`, route => {
    const path = new URL(route.request().url()).pathname
    if (path.endsWith('/offer-policy')) return route.fulfill({ json: { revision: 0, automated: false, working_days: 2 } })
    if (path.endsWith('/decision')) {
      const body = route.request().postDataJSON()
      expect(body.reason).toBe('Applicant availability')
      if (body.action === 'selected') return route.fulfill({ status: 409, json: { error: { code: 'assessment_conflict', message: 'Conflict', request_id: 'synthetic' } } })
      history.push({ action: 'reviewed', from: r.state, to: body.action, reason: body.reason, at: '2027-05-01T12:00:00Z' }); r = { ...r, state: body.action, revision: r.revision + 1 }
      return route.fulfill({ json: r })
    }
    if (path.includes(r.id)) return route.fulfill({ json: { applicant: { registration: r, display_name: 'Synthetic Mentor' }, response: { definition: { id: r.form_id, name: 'Mentors', audience: 'mentor', fields: [] }, answers: {}, revision: r.revision }, history, fallback_available: false } })
    return route.fulfill({ json: { items: [{ registration: r, display_name: 'Synthetic Mentor' }], next_cursor: null } })
  })
  await page.goto(`/events/${firstID}`); await openEventSection(page, 'Applicants')
  await expect(page.getByLabel('Approve automatic FIFO offers')).not.toBeChecked()
  await page.getByRole('button', { name: 'Review Synthetic Mentor' }).click()
  await expect(page.getByRole('button', { name: 'Select facilitator' })).toBeDisabled()
  await page.getByLabel('Decision or exception reason').fill('Applicant availability')
  await page.getByRole('button', { name: 'Select facilitator' }).click(); await page.getByRole('button', { name: 'Confirm decision' }).click(); await expect(page.getByRole('alertdialog').getByRole('alert')).toContainText('cannot compete and assess')
  await page.getByRole('button', { name: 'Cancel', exact: true }).click(); await page.getByRole('button', { name: 'Keep as backup' }).click(); await page.getByRole('button', { name: 'Confirm decision' }).click(); await expect(page.getByRole('status').filter({ hasText: 'Decision recorded.' })).toBeVisible()
  await expect(page.getByText(/reviewed · received → backup · Applicant availability/)).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
})

test('applicant search explicitly covers loaded records and local review returns keyboard focus', async ({ page }) => {
  await mockWorkspace(page)
  const r = { ...fixtureRegistration('received'), kind: 'facilitator', audience: 'mentor' }
  let pages = 0
  await page.route(`**/events/${firstID}/registrations/**`, route => {
    const path = new URL(route.request().url()).pathname
    if (path.endsWith('/offer-policy')) return route.fulfill({ json: { revision: 0, automated: false, working_days: 2 } })
    if (path.includes(r.id)) return route.fulfill({ json: { applicant: { registration: r, display_name: 'First Mentor' }, response: null, history: [], fallback_available: false } })
    pages++; const after = new URL(route.request().url()).searchParams.get('after')
    return route.fulfill({ json: { items: [{ registration: after === '0' ? r : { ...r, id: '00000000-0000-4000-8000-000000000010' }, display_name: after === '0' ? 'First Mentor' : 'Later Mentor' }], next_cursor: after === '0' ? 1 : null } })
  })
  await page.goto(`/events/${firstID}`); await openEventSection(page, 'Applicants')
  await page.getByLabel('Find in loaded applicants').fill('Later')
  await expect(page.getByText('0 matching in 1 loaded applicants. Load more to inspect later records.')).toBeVisible(); expect(pages).toBe(1)
  await page.getByRole('button', { name: 'More records' }).click()
  await expect(page.getByRole('button', { name: 'Review Later Mentor' })).toBeVisible(); expect(pages).toBe(2)
  await page.getByLabel('Find in loaded applicants').fill('')
  const button = page.getByRole('button', { name: 'Review First Mentor' }); await button.focus(); await page.keyboard.press('Enter')
  await expect(page.getByText('Answers have been removed under the event retention policy.')).toBeVisible()
  await page.getByRole('button', { name: 'Back to applicants' }).click(); await expect(button).toBeFocused()
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
})

test('offer policy reviews captured FIFO settings and saves the native revision without confirming attendance', async ({ page }) => {
  await mockWorkspace(page)
  let policy = { revision: 1, automated: false, working_days: 2 }, writes = 0
  await page.route(`**/events/${firstID}/registrations/**`, route => {
    if (new URL(route.request().url()).pathname.endsWith('/offer-policy')) {
      if (route.request().method() === 'PUT') { writes++; expect(route.request().headers()['if-match']).toBe('"1"'); expect(route.request().headers()['x-csrf-token']).toBe('synthetic-csrf-token-32-characters'); expect(route.request().postDataJSON()).toEqual({ automated: true, working_days: 4 }); policy = { revision: 2, automated: true, working_days: 4 } }
      return route.fulfill({ json: policy })
    }
    return route.fulfill({ json: { items: [], next_cursor: null } })
  })
  await page.goto(`/events/${firstID}`); await openEventSection(page, 'Applicants')
  await page.getByText('Waitlist automation and offer policy', { exact: true }).click()
  await page.getByLabel('Working days to respond').fill('4'); await page.getByLabel('Approve automatic FIFO offers').check(); await page.getByRole('button', { name: 'Review offer policy' }).click()
  const dialog = page.getByRole('alertdialog'); await expect(dialog).toContainText('4 working days'); expect(writes).toBe(0)
  await dialog.getByRole('button', { name: 'Save offer policy' }).click()
  await expect(page.getByRole('status').filter({ hasText: 'Offer policy saved at revision 2.' })).toBeVisible(); expect(writes).toBe(1)
  await expect(page.getByText(/0 confirmed competitors · capacity 200 · minimum 30/)).toBeVisible()
})

for (const denial of ['write', 'read'] as const) test(`explicit offer policy ${denial} denial destroys private policy input and review`, async ({ page }) => {
  await mockWorkspace(page); let writes = 0
  await page.route(`**/events/${firstID}/registrations/**`, route => {
    if (new URL(route.request().url()).pathname.endsWith('/offer-policy')) {
      if (route.request().method() === 'PUT') { writes++; return route.fulfill({ status: denial === 'write' ? 403 : 503, json: { error: { code: denial === 'write' ? 'forbidden' : 'unavailable', request_id: 'synthetic' } } }) }
      if (writes) return route.fulfill({ status: 403, json: { error: { code: 'forbidden', request_id: 'synthetic' } } })
      return route.fulfill({ json: { revision: 1, automated: false, working_days: 2 } })
    }
    return route.fulfill({ json: { items: [], next_cursor: null } })
  })
  await page.goto(`/events/${firstID}`); await openEventSection(page, 'Applicants'); await page.getByText('Waitlist automation and offer policy', { exact: true }).click(); await page.getByLabel('Working days to respond').fill('17'); await page.getByLabel('Approve automatic FIFO offers').check(); await page.getByRole('button', { name: 'Review offer policy' }).click(); await page.getByRole('button', { name: 'Save offer policy' }).click()
  if (denial === 'read') { await expect(page.getByRole('alertdialog')).toContainText('Outcome unknown'); await page.getByRole('alertdialog').getByRole('button', { name: 'Check current status' }).click() }
  await expect(page.getByRole('alert')).toContainText('do not have access'); await expect(page.getByLabel('Working days to respond')).toHaveCount(0); await expect(page.getByRole('alertdialog')).toHaveCount(0); await expect(page.getByRole('button', { name: 'Review offer policy' })).toHaveCount(0); expect(writes).toBe(1)
})

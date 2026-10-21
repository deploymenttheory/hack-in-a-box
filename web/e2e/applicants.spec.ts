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
  await page.goto(`/events/${firstID}`); await page.getByRole('button', { name: 'Applicants', exact: true }).click()
  await expect(page.getByLabel('Approve automatic FIFO offers')).not.toBeChecked()
  await page.getByRole('button', { name: 'Review Synthetic Mentor' }).click()
  await expect(page.getByRole('button', { name: 'Select facilitator' })).toBeDisabled()
  await page.getByLabel('Decision or exception reason').fill('Applicant availability')
  await page.getByRole('button', { name: 'Select facilitator' }).click(); await expect(page.getByRole('alert')).toContainText('cannot compete and assess')
  await page.getByRole('button', { name: 'Keep as backup' }).click(); await expect(page.getByRole('status')).toHaveText('Decision recorded.')
  await expect(page.getByText(/reviewed · received → backup · Applicant availability/)).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
})

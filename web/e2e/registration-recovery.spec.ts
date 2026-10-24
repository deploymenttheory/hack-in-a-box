import { test, expect } from '@playwright/test'
import { mockWorkspace, firstID } from './fixtures'
import { fixtureRegistration } from './registration-fixtures'
import { openEventSection } from './event-navigation-fixture'

test('unknown form command survives local section replacement without replay or publication', async ({ page }) => {
  await mockWorkspace(page); let writes = 0, reads = 0
  const form = { revision: 1, version: 1, published_version: null, definition: { id: '00000000-0000-4000-8000-000000000003', name: 'Recovery draft', audience: 'attendee', fields: [] } }
  await page.route(`**/events/${firstID}/registration/forms**`, route => {
    if (route.request().method() === 'PUT') { writes++; expect(route.request().headers()['if-match']).toBe('"1"'); expect(route.request().headers()['x-csrf-token']).toBeTruthy(); expect(route.request().postDataJSON().name).toBe('Original unknown definition'); return route.fulfill({ status: 503, json: { error: { code: 'unavailable' } } }) }
    reads++; return route.fulfill({ json: { items: [{ ...form, revision: reads > 1 ? 2 : 1 }] } })
  })
  await page.goto(`/events/${firstID}`); await openEventSection(page, 'Forms'); await page.getByRole('button', { name: 'Recovery draft · Draft' }).click()
  await page.getByLabel('Form name', { exact: true }).fill('Original unknown definition'); await page.getByRole('button', { name: 'Save form draft' }).click()
  await expect(page.getByRole('heading', { name: 'Outcome unknown' })).toBeVisible()
  page.on('dialog', dialog => void dialog.accept()); await openEventSection(page, 'Overview'); await openEventSection(page, 'Forms')
  await page.getByRole('button', { name: 'Recovery draft · Draft' }).click()
  await expect(page.getByRole('heading', { name: 'Outcome unknown' })).toBeVisible()
  await expect(page.getByRole('status').filter({ hasText: 'Outcome unknown' })).toContainText('Expected revision 1')
  await page.getByRole('button', { name: 'Check current status' }).click()
  await expect(page.getByRole('button', { name: 'Save form draft' })).toBeDisabled(); await expect(page.getByRole('button', { name: /Publish form version/ })).toBeDisabled()
  expect(writes).toBe(1); expect(reads).toBeGreaterThan(1)
})

test('pending and unknown policy commands retain their barrier across section changes', async ({ page }) => {
  await mockWorkspace(page); let writes = 0, release!: () => void
  const gate = new Promise<void>(resolve => { release = resolve })
  await page.route(`**/events/${firstID}/registrations/**`, async route => {
    if (!new URL(route.request().url()).pathname.endsWith('/offer-policy')) return route.fulfill({ json: { items: [], next_cursor: null } })
    if (route.request().method() === 'PUT') { writes++; expect(route.request().headers()['if-match']).toBe('"1"'); expect(route.request().postDataJSON()).toEqual({ automated: true, working_days: 2 }); await gate; return route.fulfill({ status: 503, json: { error: { code: 'unavailable' } } }) }
    return route.fulfill({ json: { revision: writes ? 2 : 1, automated: false, working_days: 2 } })
  })
  await page.goto(`/events/${firstID}`); await openEventSection(page, 'Applicants'); await page.getByText('Waitlist automation and offer policy', { exact: true }).click(); await page.getByLabel('Approve automatic FIFO offers').check(); await page.getByRole('button', { name: 'Review offer policy' }).click(); await page.getByRole('button', { name: 'Save offer policy' }).click()
  await expect.poll(() => writes).toBe(1); release(); await expect(page.getByRole('alertdialog')).toContainText('Outcome unknown')
  // A read closes only the presentation review; it cannot settle the command.
  await page.getByRole('alertdialog').getByRole('button', { name: 'Check current status' }).click(); await expect(page.getByRole('alertdialog')).toHaveCount(0); page.on('dialog', dialog => void dialog.accept())
  await openEventSection(page, 'Overview'); await openEventSection(page, 'Applicants'); await page.getByText('Waitlist automation and offer policy', { exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Outcome unknown' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Review offer policy' })).toBeDisabled(); expect(writes).toBe(1)
})

test('long applicant names wrap without overflowing the actual tablet collection', async ({ page }) => {
  await mockWorkspace(page); await page.setViewportSize({ width: 1024, height: 900 })
  const registration = { ...fixtureRegistration('received'), kind: 'facilitator', audience: 'mentor' }, name = 'SyntheticApplicant'.repeat(8)
  await page.route(`**/events/${firstID}/registrations/**`, route => route.fulfill({ json: new URL(route.request().url()).pathname.endsWith('/offer-policy') ? { revision: 1, automated: false, working_days: 2 } : { items: [{ registration, display_name: name }], next_cursor: null } }))
  await page.goto(`/events/${firstID}`); await openEventSection(page, 'Applicants'); await expect(page.getByRole('button', { name: 'Review ' + name })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(1024)
})

test('applicant canvas breakpoint moves keyboard focus to visible detail and back to its source', async ({ page }) => {
  await mockWorkspace(page); await page.setViewportSize({ width: 1440, height: 900 })
  const registration = { ...fixtureRegistration('received'), kind: 'facilitator', audience: 'mentor' }
  await page.route(`**/events/${firstID}/registrations/**`, route => {
    const path = new URL(route.request().url()).pathname
    return route.fulfill({ json: path.endsWith('/offer-policy') ? { revision: 1, automated: false, working_days: 2 } : path.includes(registration.id) ? { applicant: { registration, display_name: 'Synthetic Mentor' }, response: null, history: [], fallback_available: false } : { items: [{ registration, display_name: 'Synthetic Mentor' }], next_cursor: null } })
  })
  await page.goto(`/events/${firstID}`); await openEventSection(page, 'Applicants'); const opener = page.getByRole('button', { name: 'Review Synthetic Mentor' })
  await opener.click(); await expect(page.getByRole('heading', { name: 'Reviewing Synthetic Mentor' })).toBeVisible(); await opener.focus()
  await page.setViewportSize({ width: 1024, height: 900 })
  await expect(page.getByRole('heading', { name: 'Reviewing Synthetic Mentor' })).toBeFocused()
  await page.getByRole('button', { name: 'Back to applicants' }).click(); await expect(opener).toBeFocused()
})

import { test, expect } from '@playwright/test'
import { mockWorkspace, firstID, person } from './fixtures'

test('ordinary participant reaches released contacts from Today and preparation without admin navigation', async ({ page }, info) => {
  const { events } = await mockWorkspace(page), event = events[0]
  event.stage = 'preparation'; event.can_read_staff = false; event.can_edit = false; event.can_present = false
  await page.route('**/api/v1/session', route => route.fulfill({ json: { person: { ...person, platform_admin: false }, csrf_token: 'synthetic-csrf-token-32-characters', can_create_events: false } }))
  await page.route(`**/events/${firstID}/guidance`, route => route.fulfill({ json: { items: [{ id: '82fe31a6-a289-4a2b-80c2-ce0d677d7871', purpose: 'support', title: 'Support for this event', version: 1, contacts: [{ label: 'Published support route', url: 'https://example.test/support' }], require_acknowledgement: false, acknowledged_at: null, previous_acknowledgement: null }], warnings: [], can_manage: false } }))
  await page.goto(`/events/${firstID}`)
  await page.getByRole('button', { name: 'Today', exact: true }).click()
  await expect(page.getByRole('link', { name: 'Published support route' })).toHaveAttribute('href', 'https://example.test/support')
  for (const name of ['Content', 'Setup', 'Staff', 'Inventory']) await expect(page.getByRole('button', { name, exact: true })).toHaveCount(0)
  await page.screenshot({ path: info.outputPath('participant-today-guidance.png'), fullPage: true })
  await page.getByRole('button', { name: 'Deliverables', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Your preparation' })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Published support route' })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
})

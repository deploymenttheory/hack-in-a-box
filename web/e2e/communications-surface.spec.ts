import { expect, test } from '@playwright/test'
import { expectEventSection } from './event-navigation-fixture'
import { firstID, mockWorkspace } from './fixtures'

test('communications keep administrator tools on their portal and personal inbox in both', async ({ page }, info) => {
  expect(['administrator', 'regular']).toContain(info.project.metadata.portal)
  await mockWorkspace(page)
  await page.route('**/api/v1/inbox**', route => route.fulfill({ json: { items: [], next_cursor: null, unread: 0 } }))
  await page.goto(`/events/${firstID}`)
  const sections = page.getByRole('navigation', { name: 'Event sections', exact: true })
  await expect(sections).toBeVisible()
  await expectEventSection(page, 'Campaigns', info.project.metadata.portal === 'administrator')
  await page.getByRole('link', { name: /^Inbox/ }).click()
  await expect(page.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible()
  await expect(page.getByRole('status').filter({ hasText: 'unread notifications' })).toContainText('0 unread notifications')
})

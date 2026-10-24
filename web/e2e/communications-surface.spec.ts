import { expect, test } from '@playwright/test'
import { expectEventSection } from './event-navigation-fixture'
import { firstID, mockWorkspace } from './fixtures'

test('communications keep administrator tools on their portal and personal inbox in both', async ({ page }, info) => {
  await mockWorkspace(page)
  await page.route('**/api/v1/inbox**', route => route.fulfill({ json: { items: [], next_cursor: null, unread: 0 } }))
  await page.goto(`/events/${firstID}`)
  const sections = page.getByRole('navigation', { name: 'Event sections', exact: true })
  await expect(sections).toBeVisible()
  await expectEventSection(page, 'Campaigns', info.project.name.startsWith('admin-'))
  await page.getByRole('link', { name: /^Inbox/ }).click()
  await expect(page.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible()
  await expect(page.getByRole('status').filter({ hasText: 'unread notifications' })).toContainText('0 unread notifications')
})

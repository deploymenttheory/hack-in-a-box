import { expect, test } from '@playwright/test'
import { mockWorkspace } from './fixtures'
test('public support link reaches the independent guide without carrying session authority', async ({ page }) => {
  await mockWorkspace(page)
  await page.goto('/')
  const link = page.getByRole('link', { name: 'About this interface · Style guide', exact: true })
  await expect(link).toHaveAttribute('href', 'http://127.0.0.1:8082')
  await expect(link).toHaveAttribute('target', '_blank')
  await expect(link).toHaveAttribute('rel', 'noopener noreferrer')
  await page.evaluate(() => window.dispatchEvent(new Event('session-expired')))
  await expect(link).toBeVisible()
})

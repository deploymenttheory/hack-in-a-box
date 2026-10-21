import { test, expect } from '@playwright/test'
import { mockWorkspace } from './fixtures'
test.beforeEach(async ({ page }) => {
  await mockWorkspace(page)
})
test('navigation, forms and event deep links work on both screen sizes', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Your events' })).toBeVisible()
  await page.keyboard.press('Tab')
  await expect(page.getByRole('link', { name: 'Skip to content' })).toBeFocused()
  await page.getByRole('link', { name: /North Star Hack/ }).click()
  await page.reload()
  await expect(page.getByRole('heading', { name: 'North Star Hack' })).toBeVisible()
  await page.getByRole('button', { name: 'Synthetic Person' }).click()
  await expect(page.getByRole('menuitem', { name: 'Sign out' })).toBeVisible()
  expect(await page.locator('[data-theme-portals] [role="menu"]').count()).toBe(1)
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
})
test('unknown pages and failed API calls have safe states', async ({ page }) => {
  await page.goto('/not-a-route')
  await expect(page.getByRole('heading', { name: 'Page not found' })).toBeVisible()
  await page.route('**/api/v1/session', (route) => route.fulfill({ status: 503, json: { error: { code: 'unavailable', message: 'raw-secret-database', request_id: 'test-reference' } } }))
  await page.goto('/')
  await expect(page.getByRole('alert')).toContainText('Please try again')
  await expect(page.getByText('raw-secret-database')).toHaveCount(0)
})
test('expired session discards the private workspace', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Your events' })).toBeVisible()
  await page.evaluate(() => window.dispatchEvent(new Event('session-expired')))
  await expect(page.getByRole('link', { name: /Sign in with your organisation/ })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Your events' })).toHaveCount(0)
})

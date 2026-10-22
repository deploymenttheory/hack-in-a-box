import { test, expect } from '@playwright/test'
import { firstID, mockWorkspace } from './fixtures'

test('participant build omits organiser navigation even for an organiser identity', async ({ page }) => {
  await mockWorkspace(page)
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Your events' })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Create event' })).toHaveCount(0)
  await expect(page.getByRole('link', { name: 'Event templates' })).toHaveCount(0)
  await expect(page.getByRole('link', { name: 'Find events' })).toBeVisible()
  await page.getByRole('link', { name: /North Star Hack/ }).click()
  await expect(page.getByRole('heading', { name: 'North Star Hack' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Resources', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Deliverables', exact: true })).toBeVisible()
  for (const name of ['Setup', 'Staff', 'Applicants', 'Teams', 'Content', 'Advance to registration']) {
    await expect(page.getByRole('button', { name, exact: true })).toHaveCount(0)
  }
  for (const path of ['/events/new', '/templates']) {
    await page.goto(path)
    await expect(page.getByRole('heading', { name: 'Page not found' })).toBeVisible()
  }
  await page.goto(`/events/${firstID}`)
  await expect(page.getByRole('heading', { name: 'North Star Hack' })).toBeVisible()
})

import { openEventSection } from './event-navigation-fixture'
import { test, expect } from '@playwright/test'
import { setup } from './deliverable-fixtures'

test('participant requirements stay private until actual common publication, then open only private preparation', async ({ page }) => {
  await page.clock.install({ time: new Date('2027-06-14T09:00:00Z') })
  const state = await setup(page, { live: false, published: false, canary: true, participant: true })
  await expect(page.getByText('Competition requirements will appear after the common challenge is published.', { exact: false })).toBeVisible()
  await expect(page.getByText(/SYNTHETIC-THEME-CANARY|SYNTHETIC-EVIDENCE-INSTRUCTIONS-CANARY/)).toHaveCount(0)
  await expect(page.getByRole('form', { name: 'Team evidence editor' })).toHaveCount(0)
  await expect(page.getByText('Prepare your laptop', { exact: true })).toBeVisible()
  expect(state.reads.some(path => path.endsWith('/draft') || path.endsWith('/history'))).toBe(false)
  expect(state.writes).toEqual([])

  state.event.published_at = '2027-06-14T09:00:00Z'
  state.value.can_read_draft = true
  state.value.can_draft = true
  await page.reload()
  await openEventSection(page, 'Deliverables')
  await expect(page.getByText('SYNTHETIC-EVIDENCE-INSTRUCTIONS-CANARY', { exact: true })).toBeVisible()
  await expect(page.getByRole('form', { name: 'Team evidence editor' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Submit formal version' })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Record completion feedback' })).toHaveCount(0)
  await expect(page.getByText('No formal submission accepted.', { exact: true })).toBeVisible()
  expect(state.writes).toEqual([])
})

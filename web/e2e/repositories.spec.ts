import { expect, test } from '@playwright/test'
import { firstID } from './fixtures'
import { repositoryFixture, repositoryTeam } from './repository-fixtures'
import { openEventSection } from './event-navigation-fixture'
const teamID = repositoryTeam
test('current repository collection opens an exact team inspector with independent private read and keyboard return', async ({ page }, info) => {
  const regular = info.project.metadata.portal === 'regular'
  const state = await repositoryFixture(page, regular)
  await page.goto(`/events/${firstID}`); await openEventSection(page, 'Repositories')
  await expect(page.getByRole('heading', { name: 'Repositories', exact: true })).toBeVisible()
  await expect(page.getByText('Repository confirmed', { exact: true })).toBeVisible()
  if (!regular) await expect(page.getByText('Unused · unscored', { exact: true })).toBeVisible()
  const opener = page.getByRole('button', { name: 'Inspect repository', exact: true }).first()
  await opener.focus(); await page.keyboard.press('Enter')
  const inspector = page.getByRole('region', { name: 'synthetic-org/synthetic-repository', exact: true })
  await expect(inspector).toContainText(teamID); await expect(inspector).toContainText('123456')
  if (!regular) { await expect(inspector).toContainText('verified account required'); expect(state.accessReads).toBe(1) }
  else { expect(state.accessReads).toBe(0); await expect(inspector).toContainText('Your repository invitation') }
  await inspector.getByRole('link', { name: 'Back to repositories', exact: true }).click(); await expect(opener).toBeFocused()
  expect(state.writes).toHaveLength(0); expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(page.viewportSize()!.width)
})

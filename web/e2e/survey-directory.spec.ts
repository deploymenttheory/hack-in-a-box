import { expect, test } from '@playwright/test'
import { firstID } from './fixtures'
import { surveyFixture, surveyID, surveyPath, managePath } from './survey-workspace-fixtures'

test('invitation directory exposes native current response access and real keyboard destinations', async ({ page }, info) => {
  const fixture = await surveyFixture(page)
  await page.goto(`/events/${firstID}/surveys`)
  const directory = page.getByRole(info.project.name === 'mobile' ? 'list' : 'table', { name: 'Your survey invitations' })
  await expect(page.getByRole('heading', { name: 'Surveys and feedback', exact: true })).toBeVisible()
  await expect(directory).toContainText('No response yet')
  await expect(directory).toContainText('Open for edits')
  await expect(page.getByRole('searchbox')).toHaveCount(0)
  await expect(page.getByRole('heading', { name: 'Organiser survey administration', exact: true })).toHaveCount(0)
  fixture.state.open = false
  await page.getByRole('button', { name: 'Refresh invitations', exact: true }).click()
  await expect(directory).toContainText('Read-only')
  const link = page.getByRole('link', { name: 'Open your survey', exact: true })
  await expect(link).toHaveAttribute('href', surveyPath)
  await page.screenshot({ path: info.outputPath('survey-directory.png'), fullPage: true })
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
  await link.focus(); await page.keyboard.press('Enter')
  await expect(page).toHaveURL(surveyPath)
})

test('current organiser directory retains server versions and administration route without fabricated totals', async ({ page }) => {
  await surveyFixture(page, true); await page.goto(`/events/${firstID}/surveys`)
  await expect(page.getByRole('heading', { name: 'Organiser survey administration', exact: true })).toBeVisible()
  const link = page.getByRole('link', { name: 'Manage survey and restricted results', exact: true })
  await expect(link).toHaveAttribute('href', managePath)
  await expect(page.getByRole('link', { name: 'Create survey', exact: true })).toHaveAttribute('href', `/events/${firstID}/surveys/new`)
  await expect(page.getByText('Published, fixed questions', { exact: true })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
})

test('native directory denial hides invitation records and actions on refreshed authority', async ({ page }) => {
  const fixture = await surveyFixture(page); await page.goto(`/events/${firstID}/surveys`)
  await expect(page.getByRole('link', { name: 'Open your survey', exact: true })).toBeVisible()
  fixture.state.allowed = false
  await page.getByRole('button', { name: 'Refresh invitations', exact: true }).click()
  await expect(page.getByText('Current survey access is unavailable.', { exact: true })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Open your survey', exact: true })).toHaveCount(0)
  await expect(page.getByRole('link', { name: 'Reviewed report', exact: true })).toHaveCount(0)
  await expect(page.locator(`[href*="${surveyID}"]`)).toHaveCount(0)
})

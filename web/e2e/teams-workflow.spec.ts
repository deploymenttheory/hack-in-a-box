import { test, expect } from '@playwright/test'
import { setupTeams as setup } from './teams-workflow-fixtures'

test('write-only connection replacement preserves composer edits and requires current review', async ({ page }, info) => {
  const { requests } = await setup(page)
  await page.getByLabel('Campaign name', { exact: true }).fill('Unsaved organiser draft')
  const endpoint = 'https://synthetic.environment.api.powerplatform.com/?sig=SECRET-BROWSER-CANARY'
  await page.getByLabel('New secret workflow endpoint').fill(endpoint)
  await page.getByRole('button', { name: 'Save Teams connection' }).click()
  await expect(page.getByLabel('New secret workflow endpoint')).toHaveValue('')
  await expect(page.getByRole('heading', { name: 'Review changed campaign facts' })).toBeVisible()
  await expect(page.getByLabel('Campaign name', { exact: true })).toHaveValue('Unsaved organiser draft')
  expect(requests[0].body.endpoint).toBe(endpoint)
  expect(await page.locator('body').innerText()).not.toContain('SECRET-BROWSER-CANARY')
  expect(await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }))).not.toContain('SECRET-BROWSER-CANARY')
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
  await page.getByRole('heading', { name: 'Teams workflow connection' }).scrollIntoViewIfNeeded()
  await page.getByRole('region', { name: 'Teams workflow connection' }).screenshot({ path: info.outputPath('teams-setup.png') })
})
test('shared-channel TEST is explicit and acceptance never claims Teams delivery', async ({ page }) => {
  const { requests } = await setup(page)
  await page.getByRole('button', { name: 'Queue shared-channel TEST' }).click()
  await page.getByRole('button', { name: 'Cancel', exact: true }).click()
  expect(requests).toHaveLength(0)
  await page.getByRole('button', { name: 'Queue shared-channel TEST' }).click()
  await page.getByRole('button', { name: 'Queue TEST message', exact: true }).click()
  await expect(page.getByRole('listitem').filter({ hasText: 'Connection TEST' })).toContainText('provider accepted')
  expect(Object.keys(requests[0].body)).toEqual(['request_id'])
  await expect(page.getByText('it does not prove the workflow posted to Teams', { exact: false })).toBeVisible()
  await expect(page.getByRole('button', { name: /Teams test to myself/ })).toHaveCount(0)
})
test('missing encryption keys fail closed in setup', async ({ page }) => {
  await setup(page, false)
  await expect(page.getByLabel('New secret workflow endpoint')).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Queue shared-channel TEST' })).toBeDisabled()
  await expect(page.getByText('Endpoint encryption is unavailable.', { exact: false })).toBeVisible()
})

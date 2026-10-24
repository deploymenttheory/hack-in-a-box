import { test, expect } from '@playwright/test'
import { accessFixture } from './team-access-fixtures'

test('synthetic private metadata evidence contains no revealed value', async ({ page }, info) => {
  const { state, secret } = await accessFixture(page)
  await expect(page.getByLabel('Value hidden')).toHaveCount(2)
  await expect(page.getByText(secret(), { exact: true })).toHaveCount(0)
  expect(state.reveals).toHaveLength(0)
  await page.screenshot({ path: info.outputPath('private-metadata.png'), fullPage: true })
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
})

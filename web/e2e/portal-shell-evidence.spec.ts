import { test, expect } from '@playwright/test'
import { mkdirSync } from 'node:fs'
import { resolve } from 'node:path'
import { mockWorkspace } from './fixtures'

test('synthetic directory and identity evidence', async ({ page }, info) => {
  await mockWorkspace(page)
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Your events', exact: true })).toBeVisible()
  await expect(page.getByRole('link', { name: /North Star Hack/ })).toBeVisible()
  const folder = resolve('../docs/verification/portal-shells')
  const capture = async (screen: string) => {
    if (!process.env.FLEET_CAPTURE) return
    mkdirSync(folder, { recursive: true })
    await page.screenshot({ path: resolve(folder, `${process.env.FLEET_PORTAL ?? 'admin'}-${info.project.name}-${screen}-${process.env.FLEET_CAPTURE}.png`), fullPage: true })
  }
  await capture('directory')
  await page.goto('/account'); await expect(page.getByRole('heading', { name: 'Your account', exact: true })).toBeVisible()
  await expect(page.getByText('No verified GitHub account linked.')).toBeVisible()
  await capture('account')
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})

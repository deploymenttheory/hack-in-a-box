import { test, expect } from '@playwright/test'
import { activate, scan, tabTo, typeInto } from './checks'

test('independently hosted guide navigation and no-match recovery are keyboard reachable', async ({ page }, info) => {
  await page.goto('/')
  await scan(page, info, 'independent-guide')
  await activate(page, page.getByRole('link', { name: '100 quality guidelines', exact: true }))
  await typeInto(page, page.getByRole('searchbox', { name: 'Find a guideline', exact: true }), 'no-synthetic-match')
  await expect(page.getByRole('heading', { name: 'No matching guidelines', exact: true })).toBeVisible()
  await scan(page, info, 'guide-no-match')
  await activate(page, page.getByRole('button', { name: 'Clear search', exact: true }))
  await expect(page.locator('.guide-quality-list > li')).toHaveCount(100)
})

test('production guide controls expose validation, dialog and keyboard dismissal in light and dark themes', async ({ page }, info) => {
  await page.goto('/samples.html')
  const validate = page.getByRole('button', { name: 'Validate example', exact: true })
  await activate(page, validate)
  await expect(page.getByLabel('Example label (required)', { exact: true })).toBeFocused()
  await scan(page, info, 'production-field-error')
  await page.keyboard.type('Synthetic keyboard label')
  await activate(page, validate)
  await expect(page.getByText('Example validated. No application request was sent.')).toBeVisible()
  const opener = page.getByRole('button', { name: 'Open example dialog', exact: true })
  await activate(page, opener)
  await expect(page.getByRole('dialog', { name: 'Example details', exact: true })).toBeVisible()
  await scan(page, info, 'production-dialog')
  await page.keyboard.press('Escape'); await expect(opener).toBeFocused()
  await page.goto('/')
  await tabTo(page, page.getByLabel('Theme', { exact: true }))
  await page.keyboard.press('d'); await page.keyboard.press('Enter')
  await expect(page.getByLabel('Theme', { exact: true })).toHaveValue('dark')
  await scan(page, info, 'guide-dark')
})

import { openEventSection } from './event-navigation-fixture'
import { test, expect } from '@playwright/test'
import { mockWorkspace, firstID, secondID } from './fixtures'

test('branding previews before save, rejects unreadable palettes and stays scoped to one event', async ({ page }) => {
  await mockWorkspace(page)
  let primary = '#173a35', saves = 0
  await page.route(`**/events/${firstID}/branding`, async route => {
    if (route.request().method() === 'PUT') { primary = route.request().postDataJSON().primary; saves++ }
    await route.fulfill({ json: { revision: 1 + saves, branding: { primary, accent: '#d17c32', background: '#f8f9f5', foreground: '#173a35', assets: [] } } })
  })
  await page.goto(`/events/${firstID}`)
  await openEventSection(page, 'Branding')
  await page.getByLabel('Primary colour').fill('#233c78')
  expect(saves).toBe(0)
  await page.getByRole('button',{name:'Preview',exact:true}).click()
  await expect(page.getByRole('region', { name: 'Branding preview' })).toHaveCSS('--primary', '#233c78')
  await page.getByRole('button',{name:'Colours',exact:true}).click()
  await page.getByLabel('Text colour').fill('#f8f9f5')
  await expect(page.getByRole('button', { name: 'Save branding' })).toBeDisabled()
  await expect(page.getByRole('alert')).toContainText('contrast')
  await page.getByLabel('Text colour').fill('#173a35')
  await page.getByRole('button', { name: 'Save branding' }).click()
  await expect(page.getByRole('status')).toHaveText('Branding saved.')
  expect(saves).toBe(1)
  await page.getByRole('link', { name: 'Switch event' }).click()
  await page.locator(`a[href="/events/${secondID}"]`).click()
  await expect(page.getByRole('heading', { name: 'Southern Lights Hack' })).toBeVisible()
  await expect(page.locator('.theme .theme').first()).toHaveCSS('--primary', '#007e67')
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
})

test('unsafe image selection is explained without sending a save', async ({ page }) => {
  await mockWorkspace(page)
  await page.goto(`/events/${firstID}`)
  await openEventSection(page, 'Branding')
  await page.getByRole('button',{name:'Images',exact:true}).click()
  await page.getByLabel('Logo image').setInputFiles({ name: 'unsafe.svg', mimeType: 'image/svg+xml', buffer: Buffer.from('<svg/>') })
  await expect(page.getByRole('alert')).toContainText('PNG or JPEG')
})

test('a stale branding save retains the draft and explains the conflict', async ({ page }, info) => {
  await mockWorkspace(page)
  await page.route(`**/events/${firstID}/branding`, async route => {
    if (route.request().method() !== 'PUT') return route.fallback()
    return route.fulfill({ status: 409, json: { error: { code: 'revision_conflict', message: 'Changed', request_id: 'synthetic' } } })
  })
  await page.goto(`/events/${firstID}`)
  await openEventSection(page, 'Branding')
  await page.getByLabel('Primary colour').fill('#233c78')
  await page.getByRole('button', { name: 'Save branding' }).click()
  await expect(page.getByText('This has changed. Refresh and try again.')).toBeVisible()
  await expect(page.getByLabel('Primary colour')).toHaveValue('#233c78')
  await page.screenshot({ path: info.outputPath('branding-conflict.png'), fullPage: true })
})

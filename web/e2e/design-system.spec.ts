import { expect, test, type Locator } from '@playwright/test'
import { firstID, mockWorkspace } from './fixtures'
import { attendeeForm, questionID, registrationEvent } from './registration-fixtures'
import { defaultBranding } from '../src/branding'

async function contrastOf(locator: Locator, foreground = 'color') {
  return locator.evaluate((element, property) => {
    const canvas = document.createElement('canvas'); canvas.width = canvas.height = 1
    const ctx = canvas.getContext('2d')!
    function rgb(value: string) { ctx.clearRect(0, 0, 1, 1); ctx.fillStyle = value; ctx.fillRect(0, 0, 1, 1); return [...ctx.getImageData(0, 0, 1, 1).data] }
    let background = [255, 255, 255, 255]
    const ancestors: Element[] = []; for (let node: Element | null = element; node; node = node.parentElement) ancestors.unshift(node)
    for (const node of ancestors) { const color = rgb(getComputedStyle(node).backgroundColor), alpha = color[3] / 255; background = background.map((value, i) => i < 3 ? color[i] * alpha + value * (1 - alpha) : 255) }
    function luminance(channels: number[]) { return channels.slice(0, 3).reduce((total, value, i) => { const c = value / 255; return total + [0.2126, 0.7152, 0.0722][i] * (c <= .04045 ? c / 12.92 : ((c + .055) / 1.055) ** 2.4) }, 0) }
    const a = luminance(rgb(getComputedStyle(element).getPropertyValue(property))), b = luminance(background)
    return (Math.max(a, b) + .05) / (Math.min(a, b) + .05)
  }, foreground)
}
const palettes = [
  { name: 'normal', scheme: 'light', branding: defaultBranding },
  { name: 'system dark', scheme: 'dark', branding: defaultBranding },
  { name: 'custom light', scheme: 'light', branding: { ...defaultBranding, primary: '#233c78', foreground: '#283b51', background: '#fff8e9', accent: '#ffedbb' } },
  { name: 'threshold grey', scheme: 'light', branding: { ...defaultBranding, primary: '#767676', foreground: '#767676', background: '#ffffff', accent: '#ffffee' } },
  { name: 'custom dark', scheme: 'dark', branding: { ...defaultBranding, primary: '#b1d2ff', foreground: '#eaf1ff', background: '#172238', accent: '#160f30' } },
] as const
for (const palette of palettes) test(`${palette.name} palette keeps registration fields, actions, portal scope and keyboard focus readable`, async ({ page }, info) => {
  await page.emulateMedia({ colorScheme: palette.scheme, reducedMotion: 'reduce' })
  await mockWorkspace(page)
  await page.route('**/api/v1/registration/events/**', route => route.fulfill({ json: { event: registrationEvent, branding: palette.branding, forms: [attendeeForm], registrations: [] } }))
  await page.goto(`/register/${firstID}`)
  const field = page.getByLabel('What would you like to learn? (required)'), submit = page.getByRole('button', { name: 'Submit application' }), select = page.getByLabel('Team preference', { exact: true })
  await expect(field).toBeVisible()
  await field.focus(); await expect(field).toBeFocused()
  await expect(field).toHaveCSS('border-radius', '8px'); await expect(select).toHaveCSS('border-radius', '8px')
  await expect(field).toHaveCSS('outline-style', 'solid'); await expect(field).toHaveCSS('outline-width', '3px')
  await expect(field).toHaveCSS('transition-duration', '0s')
  expect(await contrastOf(field)).toBeGreaterThanOrEqual(4.5)
  expect(await contrastOf(field, 'border-top-color')).toBeGreaterThanOrEqual(3)
  expect(await contrastOf(select)).toBeGreaterThanOrEqual(4.5)
  expect(await contrastOf(page.locator('.theme .theme .panel p').first())).toBeGreaterThanOrEqual(4.5)
  expect(await contrastOf(submit)).toBeGreaterThanOrEqual(4.5)
  await submit.hover(); expect(await contrastOf(submit)).toBeGreaterThanOrEqual(4.5)
  expect((await submit.boundingBox())!.height).toBeGreaterThanOrEqual(44)
  await field.fill('A synthetic keyboard learning goal'); await select.focus(); await page.keyboard.press('ArrowDown')
  await expect(page.locator('.theme .theme').first()).toHaveCSS('--primary', palette.branding.primary)
  const localPortal = page.locator('.theme .theme [data-theme-portals]').first()
  await expect(localPortal).toHaveCSS('--background', palette.branding.background)
  await page.getByRole('button', { name: 'Synthetic Person', exact: true }).focus(); await page.keyboard.press('Enter')
  const menu = page.getByRole('menu'); await expect(menu).toBeVisible()
  expect(await contrastOf(page.getByRole('menuitem', { name: 'Account settings' }))).toBeGreaterThanOrEqual(4.5)
  expect(await menu.evaluate(el => el.closest('[data-theme-portals]') !== null)).toBe(true)
  await page.keyboard.press('Escape'); await expect(menu).toHaveCount(0)
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await expect(field).toHaveValue('A synthetic keyboard learning goal')
  await page.screenshot({ path: info.outputPath(`design-system-${palette.name.replaceAll(' ', '-')}.png`), fullPage: true })
  await page.route(`**/api/v1/events/${firstID}/registrations`, route => route.fulfill({ status: 422, json: { error: { code: 'invalid', message: 'Review the answer', request_id: 'synthetic', blockers: [{ field: questionID, code: 'invalid', message: 'Explain the learning goal.' }] } } }))
  await submit.click(); await expect(field).toHaveAttribute('aria-invalid', 'true')
  expect(await contrastOf(page.locator('.field-error').first())).toBeGreaterThanOrEqual(4.5)
  expect(await contrastOf(page.locator('.state[role=alert] p').first())).toBeGreaterThanOrEqual(4.5)
})

test('native errors, disabled actions and private drafts keep their existing semantics', async ({ page }) => {
  await mockWorkspace(page)
  await page.route('**/api/v1/registration/events/**', route => route.fulfill({ json: { event: registrationEvent, branding: { ...defaultBranding, background: '#172238', foreground: '#eaf1ff', primary: '#b1d2ff' }, forms: [attendeeForm], registrations: [] } }))
  await page.route(`**/api/v1/events/${firstID}/registrations`, route => route.fulfill({ status: 422, json: { error: { code: 'invalid', message: 'Review the answer', request_id: 'synthetic', blockers: [{ field: questionID, code: 'invalid', message: 'Explain the learning goal.' }] } } }))
  await page.goto(`/register/${firstID}`)
  const field = page.getByLabel('What would you like to learn? (required)')
  await field.fill('PRIVATE-DESIGN-SYSTEM-DRAFT')
  await page.getByRole('button', { name: 'Submit application' }).click()
  await expect(field).toHaveAttribute('aria-invalid', 'true')
  expect(await contrastOf(field, 'border-top-color')).toBeGreaterThanOrEqual(3)
  expect(await contrastOf(page.locator('.field-error').first())).toBeGreaterThanOrEqual(4.5)
  await expect(field).toHaveValue('PRIVATE-DESIGN-SYSTEM-DRAFT')
  expect(await page.evaluate(() => JSON.stringify(localStorage) + JSON.stringify(sessionStorage))).not.toContain('PRIVATE-DESIGN-SYSTEM-DRAFT')
  await page.route('**/api/v1/account/email', route => route.fulfill({ json: { configured: true, verified: false, masked_mailbox: '', provenance: '', pending: false, request_state: '', expires_at: null, retry_after_seconds: 0 } }))
  await page.goto('/account')
  await expect(page.getByRole('button', { name: 'Send confirmation code' })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Send confirmation code' })).toHaveCSS('opacity', '0.6')
  await expect(page.getByLabel('Your account ID')).toHaveAttribute('readonly', '')
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})

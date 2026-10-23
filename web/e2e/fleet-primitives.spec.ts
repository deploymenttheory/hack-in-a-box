import { expect, test } from '@playwright/test'
import { setupTeams } from './teams-workflow-fixtures'
import { firstID, mockWorkspace } from './fixtures'
import { attendeeForm, registrationEvent } from './registration-fixtures'
import { defaultBranding } from '../src/branding'
import { contrastOf } from './design-system-contrast'

test('confirmation traps focus in its actual theme and returns to its controlled opener', async ({ page }, info) => {
 const { requests } = await setupTeams(page)
 const opener = page.getByRole('button', { name: 'Queue shared-channel TEST', exact: true })
 await opener.click()
 const dialog = page.getByRole('alertdialog', { name: 'Queue shared-channel TEST?' })
 await expect(dialog).toBeVisible(); await expect(dialog.getByRole('button', { name: 'Cancel', exact: true })).toBeFocused()
 expect(await dialog.evaluate(el => !!el.closest('[data-theme-portals]'))).toBe(true)
 expect(await contrastOf(dialog)).toBeGreaterThanOrEqual(4.5)
 const close = dialog.getByRole('button', { name: 'Close confirmation' }), confirm = dialog.getByRole('button', { name: 'Queue TEST message', exact: true })
 const box = (await close.boundingBox())!, minimum = await page.evaluate(() => matchMedia('(pointer: coarse)').matches ? 44 : 36)
 expect(box.width).toBeGreaterThanOrEqual(minimum); expect(box.height).toBeGreaterThanOrEqual(minimum)
 await page.keyboard.press('Tab'); await expect(confirm).toBeFocused()
 await page.keyboard.press('Tab'); await expect(close).toBeFocused()
 await page.keyboard.press('Shift+Tab'); await expect(confirm).toBeFocused()
 expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
 await page.screenshot({ path: info.outputPath('fleet-confirmation.png'), fullPage: true })
 await page.keyboard.press('Escape'); await expect(dialog).toHaveCount(0); await expect(opener).toBeFocused(); expect(requests).toHaveLength(0)
})

test('pending confirmation retains dimensions and exact intent, then restores the caller after a definite failure', async ({ page }) => {
 await setupTeams(page)
 let complete: () => void = () => {}, attempts = 0
 const requests: { request_id: string }[] = []
 const blocked = new Promise<void>(resolve => { complete = resolve })
 await page.route(`**/events/${firstID}/teams-workflow/test`, async route => {
  attempts++; requests.push(route.request().postDataJSON())
  expect(route.request().headers()['x-csrf-token']).toBe('synthetic-csrf-token-32-characters')
  expect(route.request().headers()['if-match']).toBe('"1"')
  await blocked
  await route.fulfill({ status: 409, json: { error: { code: 'revision_conflict', message: 'Current state changed', request_id: 'synthetic' } } })
 })
 const opener = page.getByRole('button', { name: 'Queue shared-channel TEST', exact: true })
 await opener.click()
 const confirm = page.getByRole('button', { name: 'Queue TEST message', exact: true }), before = (await confirm.boundingBox())!
 await confirm.click(); await expect(confirm).toHaveAttribute('aria-busy', 'true'); await expect(confirm).toBeDisabled()
 const during = (await confirm.boundingBox())!
 expect(during.width).toBe(before.width); expect(during.height).toBe(before.height)
 await page.keyboard.press('Enter'); await page.keyboard.press('Escape')
 await expect(page.getByRole('alertdialog')).toBeVisible(); expect(attempts).toBe(1)
 expect(Object.keys(requests[0])).toEqual(['request_id']); expect(requests[0].request_id).toMatch(/^[a-f0-9-]{36}$/)
 complete(); await expect(page.getByRole('alertdialog')).toHaveCount(0)
 await expect(page.getByRole('alert')).toContainText('changed'); await expect(opener).toBeFocused()
 await page.getByRole('button', { name: 'Refresh Teams receipts', exact: true }).click()
 expect(attempts).toBe(1)
})

test('an uncertain transport result never replays the confirmed write during reconnect or authoritative refresh', async ({ page }) => {
 await setupTeams(page)
 let attempts = 0
 await page.route(`**/events/${firstID}/teams-workflow/test`, route => { attempts++; return route.abort('failed') })
 await page.getByRole('button', { name: 'Queue shared-channel TEST', exact: true }).click()
 await page.getByRole('button', { name: 'Queue TEST message', exact: true }).click()
 await expect(page.getByRole('alert')).toBeVisible()
 await page.evaluate(() => { window.dispatchEvent(new Event('online')); window.dispatchEvent(new Event('focus')) })
 await page.getByRole('button', { name: 'Refresh Teams receipts', exact: true }).click()
 expect(attempts).toBe(1)
 await expect(page.getByText('it does not prove the workflow posted to Teams', { exact: false })).toBeVisible()
})

test('native Enter submits registration once and server errors keep current field guidance and draft geometry', async ({ page }, info) => {
 await mockWorkspace(page)
 const form = structuredClone(attendeeForm)
 form.definition.fields = [form.definition.fields[0]]
 form.definition.fields[0].help = 'Use a synthetic learning goal.'
 let attempts = 0
 await page.route('**/api/v1/registration/events/**', route => route.fulfill({ json: { event: registrationEvent, branding: defaultBranding, forms: [form], registrations: [] } }))
 await page.route(`**/events/${firstID}/registrations`, route => { attempts++;return route.fulfill({ status: 422, json: { error: { code: 'invalid', message: 'Review the answer', request_id: 'synthetic', blockers: [{ field: form.definition.fields[0].id, code: 'invalid', message: 'Explain the learning goal.' }] } } }) })
 await page.goto(`/register/${firstID}`)
 const field = page.getByLabel('What would you like to learn? (required)'), button = page.getByRole('button', { name: 'Submit application' })
 await field.fill('Recoverable synthetic draft')
 const before = (await field.boundingBox())!
 await field.press('Enter'); await expect(field).toHaveAttribute('aria-invalid', 'true')
 await expect(field).toBeFocused(); await expect(field).toHaveValue('Recoverable synthetic draft')
 const after = (await field.boundingBox())!
 expect(after.width).toBe(before.width); expect(after.height).toBe(before.height); expect(attempts).toBe(1)
 await expect(field).toHaveAccessibleDescription('Use a synthetic learning goal. Explain the learning goal.')
 expect(await field.evaluate(el => el.getAttribute('aria-describedby')!.split(' ').every(id => !!document.getElementById(id)))).toBe(true)
 await button.hover(); expect(await contrastOf(button)).toBeGreaterThanOrEqual(4.5)
 await page.screenshot({ path: info.outputPath('fleet-form-error.png'), fullPage: true })
 expect(await page.evaluate(() => JSON.stringify(localStorage) + JSON.stringify(sessionStorage))).not.toContain('Recoverable synthetic draft')
})

import { expect, test } from '@playwright/test'
import { mkdirSync } from 'node:fs'
import { resolve } from 'node:path'
import { mockWorkspace, firstID } from './fixtures'
import { attendeeForm, registrationEvent } from './registration-fixtures'
import { defaultBranding } from '../src/branding'

// The same deterministic screen can be captured against the frozen foundation
// or the new production build. No demo route or fixture state ships in the app.
test('synthetic registration error evidence is readable at the current viewport', async ({ page }, info) => {
 await mockWorkspace(page)
 const form = structuredClone(attendeeForm)
 form.definition.fields = [form.definition.fields[0]]
 form.definition.fields[0].help = 'Use a synthetic learning goal.'
 await page.route('**/api/v1/registration/events/**', route => route.fulfill({ json: { event: registrationEvent, branding: defaultBranding, forms: [form], registrations: [] } }))
 await page.route(`**/events/${firstID}/registrations`, route => route.fulfill({ status: 422, json: { error: { code: 'invalid', message: 'Review the answer', request_id: 'synthetic', blockers: [{ field: form.definition.fields[0].id, code: 'invalid', message: 'Explain the learning goal.' }] } } }))
 await page.goto(`/register/${firstID}`)
 await page.getByLabel('What would you like to learn? (required)').fill('Synthetic learning goal')
 await page.getByRole('button', { name: 'Review application', exact: true }).click(); await page.getByRole('button', { name: 'Submit application' }).click()
 await expect(page.getByLabel('What would you like to learn? (required)')).toHaveAttribute('aria-invalid', 'true')
 expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
 if (process.env.FLEET_CAPTURE_EVIDENCE) {
  const folder = resolve('../docs/verification/fleet-primitives')
  mkdirSync(folder, { recursive: true })
  await page.screenshot({ path: resolve(folder, `${process.env.FLEET_EVIDENCE_PORTAL}-${info.project.name}-${process.env.FLEET_CAPTURE_EVIDENCE}.png`), fullPage: true })
 }
})

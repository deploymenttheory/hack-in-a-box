// Execute the handbook's synthetic local walkthrough through normal UI/API.
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { dirname, resolve, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { readFileSync, writeFileSync } from 'node:fs'
import { createHash } from 'node:crypto'
const root = resolve(dirname(fileURLToPath(import.meta.url)), '../../..')
const require = createRequire(join(root, 'web/package.json'))
const { chromium, expect } = require('@playwright/test')
const output = process.env.OPERATOR_ARTIFACTS, source = process.env.OPERATOR_SOURCE_REVISION
assert.ok(output); assert.match(source ?? '', /^[a-f0-9]{40}$/)
const origins = { admin: 'http://127.0.0.1:18181', participant: 'http://127.0.0.1:18180', guide: 'http://127.0.0.1:18182' }
const browser = await chromium.launch({ headless: true })
try {
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } })
  const admin = await context.newPage(), errors = []
  admin.on('pageerror', error => errors.push(error.message))
  await admin.goto(`${origins.admin}/auth/login`)
  await expect(admin.getByRole('heading', { name: 'Your events', exact: true })).toBeVisible()
  if (process.argv.includes('--bootstrap-only')) {
    assert.deepEqual(errors, [])
    console.log('Normal initial operator login: PASS')
  } else {
    const participant = await context.newPage()
    participant.on('pageerror', error => errors.push(error.message))
    await participant.goto(`${origins.participant}/auth/login`)
    await expect(participant.getByRole('heading', { name: 'Your events', exact: true })).toBeVisible()
    await admin.getByRole('link', { name: 'Create event', exact: true }).click()
    const title = 'Synthetic operator handbook event'
    for (const [label, value] of [['Event name', title], ['Event slug', 'operator-handbook'], ['Description', 'Synthetic clean installation walkthrough.'], ['Venue', 'Synthetic workshop']]) await admin.getByLabel(label, { exact: true }).fill(value)
    await admin.getByRole('button', { name: 'Attendance and dates', exact: true }).click()
    const year = new Date().getUTCFullYear() + 1
    for (const [label, when] of [['Final admission', '06-05T12:00'], ['Team freeze', '06-06T12:00'], ['Live start', '06-14T09:00'], ['Submission cutoff', '06-15T16:00'], ['Live end', '06-15T17:00']]) await admin.getByLabel(label, { exact: true }).fill(`${year}-${when}`)
    const [previewed] = await Promise.all([
      admin.waitForResponse(r => new URL(r.url()).pathname === '/api/v1/event-preview' && r.request().method() === 'POST'),
      admin.getByRole('button', { name: 'Preview dates and blockers', exact: true }).click(),
    ])
    assert.equal(previewed.status(), 200)
    assert.deepEqual((await previewed.json()).blockers, [])
    await expect(admin.getByText('The date configuration is ready.', { exact: true })).toBeVisible()
    await admin.getByRole('button', { name: 'Organiser responsibility', exact: true }).click()
    await admin.getByRole('checkbox', { name: /becoming this event/ }).check()
    await admin.getByRole('button', { name: 'Save draft event', exact: true }).click()
    const [created] = await Promise.all([
      admin.waitForResponse(r => new URL(r.url()).pathname === '/api/v1/events' && r.request().method() === 'POST'),
      admin.getByRole('button', { name: 'Confirm save draft event', exact: true }).click(),
    ])
    assert.equal(created.status(), 200)
    const event = await created.json(); assert.match(event.id, /^[a-f0-9-]{36}$/)
    await expect(admin.getByRole('heading', { name: title, exact: true })).toBeVisible()
    await admin.getByRole('navigation', { name: 'Event sections', exact: true }).getByRole('button', { name: 'Branding', exact: true }).click()
    await admin.getByRole('button', { name: 'Colours', exact: true }).click()
    await admin.getByLabel('Primary colour', { exact: true }).fill('#1d4ed8')
    await admin.getByRole('button', { name: 'Images', exact: true }).click()
    await admin.getByLabel('Logo image', { exact: true }).setInputFiles({ name: 'synthetic-logo.png', mimeType: 'image/png', buffer: readFileSync(join(root, 'docs/design/fleet-reference/fleet-workspace-admin.png')) })
    await admin.getByRole('button', { name: 'Preview', exact: true }).click()
    await expect(admin.getByRole('region', { name: 'Branding preview', exact: true })).toBeVisible()
    const [saved] = await Promise.all([
      admin.waitForResponse(r => new URL(r.url()).pathname === `/api/v1/events/${event.id}/branding` && r.request().method() === 'PUT'),
      admin.getByRole('button', { name: 'Save branding', exact: true }).click(),
    ])
    assert.equal(saved.status(), 200)
    const brand = await saved.json(), intent = saved.request().postDataJSON()
    assert.equal(brand.branding.primary, '#1d4ed8')
    assert.deepEqual(brand.branding.assets, intent.assets)
    assert.equal(brand.revision, Number(saved.request().headers()['if-match'].replaceAll('"', '')) + 1)
    await expect(admin.getByText('Branding saved.', { exact: true })).toBeVisible()
    const images = []
    for (const [name, page] of [['admin', admin], ['participant', participant]]) {
      await page.goto(`${origins[name]}/events/${event.id}`)
      await expect(page.getByRole('heading', { name: title, exact: true })).toBeVisible()
      const logo = page.getByRole('img', { name: 'Event logo', exact: true })
      await expect(logo).toBeVisible()
      await expect.poll(() => logo.evaluate(img => img.complete && img.naturalWidth > 0)).toBe(true)
      assert.equal(await logo.evaluate(img => getComputedStyle(img).getPropertyValue('--primary').trim()), '#1d4ed8')
      if (name === 'participant') await page.setViewportSize({ width: 390, height: 844 })
      const file = `${name}-branded-event.png`; await page.screenshot({ path: join(output, file), fullPage: true })
      images.push({ name: file, sha256: createHash('sha256').update(readFileSync(join(output, file))).digest('hex') })
    }
    const guide = await context.newPage(); guide.on('pageerror', e => errors.push(e.message))
    await guide.goto(origins.guide)
    await expect(guide.getByRole('heading', { name: 'Principles', exact: true })).toBeVisible()
    await expect(guide.getByText(source.slice(0, 12), { exact: true })).toBeVisible()
    await participant.getByRole('button', { name: 'Synthetic cutover organiser', exact: true }).click()
    const logout = participant.waitForResponse(r => new URL(r.url()).pathname === '/api/v1/logout')
    await participant.getByRole('menuitem', { name: 'Sign out', exact: true }).click()
    assert.equal((await logout).status(), 204)
    await admin.reload(); await expect(admin.getByRole('heading', { name: title, exact: true })).toBeVisible()
    assert.deepEqual(errors, [])
    writeFileSync(join(output, 'branded-event.json'), JSON.stringify({ source, event_id: event.id, event_preview: previewed.status(), event_creation: created.status(), branding_write: saved.status(), branding_revision: brand.revision, primary: brand.branding.primary, logo_count: brand.branding.assets.length, native_logo_decode_both_portals: true, independent_logout: true, guide_revision: source.slice(0, 12), errors, images, scope: 'Fresh isolated local Docker checkout/data; real OIDC with synthetic provider, same organiser in both portals. Not external TLS/live providers/full lifecycle or a separate fresh physical host.' }, null, 2) + '\n', { mode: 0o600 })
    console.log('Branded event, normal sign-ins, matching guide and separate logout: PASS')
  }
} finally { await browser.close() }

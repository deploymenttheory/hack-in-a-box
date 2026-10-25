// Real normal-image browser proof. No API interception, seeded sessions or test clocks.
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { dirname, resolve, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { mkdirSync, writeFileSync, readFileSync } from 'node:fs'
import { execFileSync } from 'node:child_process'
import { createHash } from 'node:crypto'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../../..')
const require = createRequire(join(root, 'web/package.json'))
const { chromium, expect: assertion } = require('@playwright/test')
const AxeBuilder = require('@axe-core/playwright').default
const expect = assertion.configure({ timeout: 15000 })
const project = process.env.FLEET_UI_PROJECT
assert.match(project ?? '', /^hackbox-fleet245-[a-z0-9-]+$/)
const compose = ['compose', '-p', project, '-f', join(root, 'docs/verification/fleet-cutover/compose.native.yaml')]
const output = process.env.FLEET_UI_ARTIFACTS
assert.ok(output, 'Choose a private artifact directory outside the repository')
mkdirSync(output, { recursive: true, mode: 0o700 })
const source = process.env.FLEET_UI_SOURCE_REVISION
assert.match(source ?? '', /^[a-f0-9]{40}$/)
for (const module of ['@playwright/test', '@axe-core/playwright']) {
  const wanted = JSON.parse(readFileSync(join(root, 'web/package.json'), 'utf8')).devDependencies[module]
  assert.equal(JSON.parse(readFileSync(join(root, 'web/node_modules', module, 'package.json'), 'utf8')).version, wanted, `${module} must match the archived lockfile install`)
}
const cases = [], images = []
for (const kind of ['PUBLIC', 'ADMIN', 'GUIDE']) {
  const image = process.env[`FLEET_UI_${kind}_IMAGE`]
  assert.ok(image, `Missing isolated ${kind} image`)
  assert.equal(execFileSync('docker', ['image', 'inspect', '--format', '{{index .Config.Labels "org.opencontainers.image.revision"}}', image], { encoding: 'utf8' }).trim(), source, `${kind} image source differs from browser proof`)
}
const origins = { public: 'http://127.0.0.1:18180', admin: 'http://127.0.0.1:18181', guide: 'http://127.0.0.1:18182' }
async function settled(page) {
  await page.evaluate(async () => {
    await document.fonts.ready
    await Promise.all(document.getAnimations().filter(a => a.effect?.getComputedTiming().iterations !== Infinity).map(a => a.finished.catch(() => {})))
  })
}
async function capture(page, name) {
  await settled(page)
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `${name}: horizontal overflow`)
  const audit = await new AxeBuilder({ page }).analyze()
  assert.deepEqual(audit.violations.filter(v => ['serious', 'critical'].includes(v.impact)).map(v => ({ id: v.id, impact: v.impact })), [], `${name}: accessibility`)
  const path = join(output, `${name}.png`)
  await page.screenshot({ path, fullPage: true })
  images.push({ name: `${name}.png`, sha256: createHash('sha256').update(readFileSync(path)).digest('hex') })
}
async function login(page, origin) {
  await page.goto(`${origin}/auth/login`, { waitUntil: 'domcontentloaded' })
  await expect(page.getByRole('heading', { name: 'Your events', exact: true })).toBeVisible()
  assert.equal(new URL(page.url()).origin, origin)
}
const browser = await chromium.launch({ headless: true })
try {
  for (const viewport of [{ name: 'desktop', width: 1440, height: 1000 }, { name: 'mobile', width: 390, height: 844 }]) {
    for (const colorScheme of ['light', 'dark']) {
      const name = `${viewport.name}-${colorScheme}`
      const context = await browser.newContext({ viewport: { width: viewport.width, height: viewport.height }, colorScheme })
      const guideContext = await browser.newContext({ viewport: { width: viewport.width, height: viewport.height }, colorScheme })
      context.setDefaultTimeout(15000); guideContext.setDefaultTimeout(15000)
      const admin = await context.newPage(), participant = await context.newPage(), guide = await guideContext.newPage()
      const errors = [], guideAPIs = []
      for (const page of [admin, participant, guide]) page.on('pageerror', error => errors.push(error.message))
      guide.on('request', request => { if (new URL(request.url()).pathname.startsWith('/api/')) guideAPIs.push(new URL(request.url()).pathname) })
      try {
        await login(admin, origins.admin)
        await login(participant, origins.public)
        await admin.getByRole('link', { name: 'Create event', exact: true }).click()
        const title = `Synthetic cutover ${name}`
        await admin.getByLabel('Event name', { exact: true }).fill(title)
        await admin.getByLabel('Event slug', { exact: true }).fill(`cutover-${name}`)
        await admin.getByLabel('Description', { exact: true }).fill('Synthetic normal-image UI verification. No real attendee data.')
        await admin.getByLabel('Venue', { exact: true }).fill('Synthetic workshop')
        await admin.getByRole('button', { name: 'Attendance and dates', exact: true }).click()
        const year = new Date().getUTCFullYear() + 1
        for (const [label, when] of [['Final admission', '06-05T12:00'], ['Team freeze', '06-06T12:00'], ['Live start', '06-14T09:00'], ['Submission cutoff', '06-15T16:00'], ['Live end', '06-15T17:00']]) {
          await admin.getByLabel(label, { exact: true }).fill(`${year}-${when}`)
        }
        await admin.getByRole('button', { name: 'Organiser responsibility', exact: true }).click()
        await admin.getByRole('checkbox', { name: /becoming this event/ }).check()
        await admin.getByRole('button', { name: 'Save draft event', exact: true }).click()
        const reviewed = admin.getByRole('button', { name: 'Confirm save draft event', exact: true })
        await expect(reviewed).toBeVisible()
        const [saved] = await Promise.all([
          admin.waitForResponse(response => new URL(response.url()).pathname === '/api/v1/events' && response.request().method() === 'POST'),
          reviewed.click(),
        ])
        assert.equal(saved.status(), 200, 'Native creation must return its accepted event')
        const event = await saved.json()
        assert.match(event.id, /^[a-f0-9-]{36}$/)
        await expect(admin.getByRole('heading', { level: 1, name: title, exact: true })).toBeVisible()
        await capture(admin, `${name}-admin-event`)
        await participant.goto(`${origins.public}/events/${event.id}`, { waitUntil: 'domcontentloaded' })
        await expect(participant.getByRole('heading', { level: 1, name: title, exact: true })).toBeVisible()
        await expect(participant.getByRole('heading', { name: 'Event dates', exact: true })).toBeVisible()
        await capture(participant, `${name}-participant-event`)
        await guide.goto(origins.guide, { waitUntil: 'domcontentloaded' })
        await expect(guide.getByRole('heading', { level: 1, name: 'Principles', exact: true })).toBeVisible()
        await expect(guide.getByText(source.slice(0, 12), { exact: true })).toBeVisible()
        await capture(guide, `${name}-guide`)
        assert.deepEqual(guideAPIs, [], 'The independent guide must not call application APIs')
        assert.deepEqual(await guideContext.cookies(), [], 'Guide does not need an authenticated context')
        // Same browser cookie jar: regular logout must not end the independent admin session.
        await participant.getByRole('button', { name: 'Synthetic cutover organiser', exact: true }).click()
        const logout = participant.waitForResponse(response => new URL(response.url()).pathname === '/api/v1/logout')
        await participant.getByRole('menuitem', { name: 'Sign out', exact: true }).click()
        assert.equal((await logout).status(), 204)
        await admin.reload({ waitUntil: 'domcontentloaded' })
        await expect(admin.getByRole('heading', { level: 1, name: title, exact: true })).toBeVisible()
        if (name === 'mobile-dark') {
          execFileSync('docker', [...compose, 'stop', 'public'], { cwd: root, stdio: 'pipe' })
          const container = execFileSync('docker', [...compose, 'ps', '-aq', 'public'], { cwd: root, encoding: 'utf8' }).trim()
          assert.equal(execFileSync('docker', ['inspect', '--format', '{{.State.Status}}', container], { encoding: 'utf8' }).trim(), 'exited')
          await admin.reload({ waitUntil: 'domcontentloaded' })
          await expect(admin.getByRole('heading', { level: 1, name: title, exact: true })).toBeVisible()
          await guide.reload({ waitUntil: 'domcontentloaded' })
          await expect(guide.getByRole('heading', { level: 1, name: 'Principles', exact: true })).toBeVisible()
          await capture(admin, `${name}-admin-public-stopped`)
          await capture(guide, `${name}-guide-public-stopped`)
        }
        assert.deepEqual(errors, [], 'Normal-image UI JavaScript errors')
        cases.push({ name, event_id: event.id, result: 'pass', native_creation: saved.status(), independent_logout: true, guide_api_requests: guideAPIs.length, public_failure_isolation: name === 'mobile-dark' })
        console.log(`${name}: PASS`)
      } catch (error) {
        // Screenshots contain only the synthetic app/guide, never OAuth pages or cookies.
        for (const [portal, page] of [['admin', admin], ['participant', participant], ['guide', guide]]) {
          const current = new URL(page.url())
          if (Object.values(origins).includes(current.origin) && !current.pathname.startsWith('/auth/')) {
            await page.screenshot({ path: join(output, `${name}-${portal}-failed.png`), fullPage: true }).catch(() => {})
          }
        }
        throw error
      } finally {
        await context.close(); await guideContext.close()
      }
    }
  }
  const immutableImages = Object.fromEntries(['PUBLIC', 'ADMIN', 'GUIDE'].map(kind => [kind.toLowerCase(), execFileSync('docker', ['image', 'inspect', '--format', '{{.Id}}', process.env[`FLEET_UI_${kind}_IMAGE`]], { encoding: 'utf8' }).trim()]))
  writeFileSync(join(output, 'native-normal-ui.json'), JSON.stringify({ source, project, kind: 'Normal production commands with a synthetic external OIDC provider; actual browser/HTTP/PostgreSQL', cases, images, immutable_images: immutableImages, scope: 'Shared shells, reviewed native draft creation, event reading, independent sessions/guide and public-process failure; not every domain route or complete lifecycle certification.' }, null, 2) + '\n', { mode: 0o600 })
} finally {
  await browser.close()
}

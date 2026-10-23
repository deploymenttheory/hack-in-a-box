import { expect, test, type Page } from '@playwright/test'
import { firstID, mockWorkspace, person, secondID } from './fixtures'
import type { components } from '../src/api/schema.gen'
type Item = components['schemas']['MediaItem']
const mediaID = '22222222-2222-4222-8222-222222222222'
const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jL1sAAAAASUVORK5CYII=', 'base64')
const source = (): Item => ({ id: mediaID, version: 1, kind: 'photo', title: 'Reviewed team photograph', caption: 'A team building together', captured_at: null, state: 'released', consent_state: 'reviewed', recording_url: '', can_read_original: true, can_edit: false, can_review: false, can_publish: false, can_withdraw: false, people: [], challenge_related: false })
function state() { return { role: 'attendee' as 'attendee' | 'photographer' | 'organiser', items: [source()], revision: 1, denied: false, consent: 'yes' as 'yes' | 'no' | 'unknown', choiceRevision: 1, archived: false, lostUpload: false, uploads: [] as { key: string; input: unknown }[], writes: [] as string[], receipts: new Map<string, Item>(), sessions: 0, deferSession: undefined as undefined | (() => Promise<void>) } }
async function mockMedia(page: Page, s: ReturnType<typeof state>) {
  await mockWorkspace(page)
  const error = { error: { code: 'forbidden', message: 'Current media authority lost', request_id: 'synthetic' } }
  await page.route('**/api/v1/session', async route => { s.sessions++; if (s.sessions > 1) await s.deferSession?.(); return route.fulfill({ json: { person: { ...person, platform_admin: false }, draft_scope: '33333333-3333-4333-8333-333333333333', csrf_token: 'synthetic-csrf-token-32-characters', can_create_events: false } }) })
  await page.route('**/api/v1/events/*/photo-consent', async route => {
    if (!route.request().url().includes(firstID)) return route.fulfill({ status: 403, json: error })
    if (route.request().method() === 'PUT') { s.consent = route.request().postDataJSON().choice; s.choiceRevision++; if (s.consent !== 'yes') s.items = s.items.map(item => ({ ...item, can_read_original: false })) }
    return route.fulfill({ json: { choice: s.consent, revision: s.choiceRevision, can_opt_in: !s.archived, can_revoke: true } })
  })
  await page.route('**/api/v1/events/*/media**', async route => {
    const request = route.request(), path = new URL(request.url()).pathname, staff = s.role !== 'attendee'
    if (s.denied || !path.includes(firstID)) return route.fulfill({ status: 403, json: error })
    if (path.endsWith('/consent-guidance')) return staff ? route.fulfill({ json: { items: [{ person_id: person.id, display_name: 'Synthetic participant', choice: s.consent, eligible: true }], next_cursor: null } }) : route.fulfill({ status: 403, json: error })
    if (path.endsWith('/original') || path.endsWith('/thumbnail')) return s.consent === 'yes' ? route.fulfill({ contentType: 'application/octet-stream', body: png }) : route.fulfill({ status: 403, json: error })
    if (path.endsWith('/upload')) {
      const key = request.headers()['idempotency-key'], input = request.postDataJSON(); s.uploads.push({ key, input }); expect(request.headers()['x-csrf-token']).toBe('synthetic-csrf-token-32-characters')
      const existing = s.receipts.get(key); if (existing) return route.fulfill({ json: existing })
      const item = { ...source(), id: input.id, title: input.metadata.title, caption: input.metadata.caption, state: 'draft' as const, can_edit: true, can_read_original: s.consent === 'yes', people: input.metadata.people.map((id: string) => ({ id, name: 'Synthetic participant', consent: s.consent, eligible: true })) }; s.items.push(item); s.receipts.set(key, item); s.revision++
      if (s.lostUpload) return route.abort('failed')
      return route.fulfill({ json: item })
    }
    if (request.method() === 'PUT') {
      const input = request.postDataJSON(), id = path.split('/media/')[1], item = s.items.find(i => i.id === id)!; s.writes.push(input.action)
      if (input.action === 'review') { expect(input.review).toEqual({ identification_reviewed: true, no_identifiable_people: true, content_reviewed: true }); item.state = 'reviewed'; item.can_publish = true; item.consent_state = 'reviewed' }
      if (input.action === 'publish') item.state = 'released'
      item.version++; s.revision++; return route.fulfill({ json: item })
    }
    const items = s.role === 'attendee' ? s.items.filter(item => item.state === 'released' && s.consent === 'yes') : s.items
    const id = path.split('/media/')[1]
    if (id) { const item = items.find(i => i.id === id); return item ? route.fulfill({ json: item }) : route.fulfill({ status: 404, json: error }) }
    return route.fulfill({ json: { revision: s.revision, can_collect: staff && !s.archived, can_review: s.role === 'organiser' && !s.archived, items, next_cursor: null } })
  })
}
const gallery = `/events/${firstID}/media`
test('confirmed gallery uses protected images, clears on denial, and never shows photographer guidance', async ({ page }) => {
  const s = state(); await mockMedia(page, s); await page.goto(gallery)
  await expect(page.getByRole('heading', { name: 'Reviewed gallery' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Submit media for organiser review' })).toHaveCount(0)
  await page.getByRole('button', { name: 'View protected photo' }).click(); await expect(page.locator('.media-photo img')).toBeVisible()
  s.denied = true; await page.evaluate(() => window.dispatchEvent(new Event('visibilitychange')))
  await expect(page.getByText('Current media access could not be verified. Images, local forms and private review details have been cleared.')).toBeVisible(); await expect(page.locator('.media-photo img')).toHaveCount(0)
  await expect(page.getByText('Synthetic participant')).toHaveCount(0)
})
test('archived self revocation remains usable and removes future gallery/download controls', async ({ page }) => {
  const s = state(); s.archived = true; await mockMedia(page, s); await page.goto(gallery)
  await expect(page.getByRole('button', { name: 'Allow identifiable event media' })).toBeDisabled()
  await page.getByRole('button', { name: 'Do not allow identifiable media' }).click()
  await expect(page.getByText('No media is available in this view.')).toBeVisible(); await expect(page.getByRole('button', { name: 'Download original' })).toHaveCount(0)
  expect(s.consent).toBe('no')
})
test('photographer sees minimal consent guidance and retries a lost upload with identical intent', async ({ page }) => {
  const s = state(); s.role = 'photographer'; s.items = []; s.lostUpload = true; await mockMedia(page, s); await page.goto(gallery)
  await page.getByLabel('Media title', { exact: true }).fill('Venue photograph')
  await page.getByLabel('Synthetic participant · yes').check()
  await page.getByLabel('Photograph (PNG or JPEG, up to 8 MiB)').setInputFiles({ name: 'venue.png', mimeType: 'image/png', buffer: png })
  await page.getByLabel('Submission reason').fill('Organiser sharing review')
  await page.getByRole('button', { name: 'Submit private media draft' }).click()
  await expect(page.getByText(/The outcome is uncertain/)).toBeVisible(); expect(s.uploads).toHaveLength(1)
  s.lostUpload = false; await page.getByRole('button', { name: 'Retry identical media intent' }).click(); await expect(page.getByText('Saved. Current media has been refreshed.')).toBeVisible()
  expect(s.uploads).toHaveLength(2); expect(s.uploads[1]).toEqual(s.uploads[0]); expect(s.items).toHaveLength(1)
  await expect(page.getByText(/dietary|accessibility|contact_email/i)).toHaveCount(0)
  const viewport = page.viewportSize()!; expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(viewport.width)
})
test('organiser explicitly reviews untagged content before separate release and private controls do not print', async ({ page }) => {
  const s = state(); s.role = 'organiser'; s.items = [{ ...source(), state: 'draft', can_edit: true, can_review: true, can_publish: false, can_withdraw: true, consent_state: 'not_reviewed' }]; await mockMedia(page, s); await page.goto(gallery)
  await page.getByRole('button', { name: 'Open private media review' }).click()
  await page.getByLabel('Reason for this action').fill('Checked venue image')
  await page.emulateMedia({ media: 'print' }); await expect(page.locator('.media-card')).toBeHidden(); await page.emulateMedia({ media: 'screen' })
  await expect(page.getByRole('button', { name: 'Record organiser review' })).toBeDisabled()
  await page.getByLabel('I checked the media and identified every recognisable person.').check(); await page.getByLabel('No identifiable people are present.').check(); await page.getByLabel('I reviewed the content, caption, recording destination and challenge classification.').check()
  await page.getByRole('button', { name: 'Record organiser review' }).click(); await page.getByRole('button', { name: 'Open private media review' }).click()
  await page.getByLabel('Reason for this action').fill('Explicit gallery release'); await page.getByRole('button', { name: 'Release reviewed media to gallery' }).click(); await expect.poll(() => s.writes).toEqual(['review', 'publish'])
  await page.getByRole('button', { name: 'Open private media review' }).click(); await page.emulateMedia({ media: 'print' }); await expect(page.locator('.media-private').last()).toBeHidden(); await expect(page.locator('.media-consent')).toBeHidden()
})
test('an unsent deferred preflight cannot submit after definite authority denial', async ({ page }) => {
  const s = state(); s.role = 'photographer'; s.items = []
  let resume!: () => void; const hold = new Promise<void>(resolve => { resume = resolve }); await mockMedia(page, s); await page.goto(gallery)
  await page.getByLabel('Media type').selectOption('recording'); await page.getByLabel('Media title', { exact: true }).fill('Private recording draft'); await page.getByLabel('Recording HTTPS link').fill('https://recordings.example.test/review'); await page.getByLabel('Submission reason').fill('Review recording')
  s.deferSession = () => hold; await page.getByRole('button', { name: 'Submit private media draft' }).click(); await expect(page.getByText('Checking current access…')).toBeVisible()
  s.denied = true; await page.evaluate(() => window.dispatchEvent(new Event('visibilitychange'))); await expect(page.getByText(/Current media access could not be verified/)).toBeVisible(); resume(); await page.waitForTimeout(100)
  expect(s.writes).toHaveLength(0); await expect(page.locator('input[value="Private recording draft"]')).toHaveCount(0)
})
test('event navigation clears protected image and private unsent metadata', async ({ page }) => {
  const s = state(); s.role = 'photographer'; await mockMedia(page, s); await page.goto(gallery)
  await page.getByLabel('Media title', { exact: true }).fill('UNSENT PRIVATE CAPTION')
  await page.getByRole('button', { name: 'View protected photo' }).click(); await expect(page.locator('.media-photo img')).toBeVisible()
  await page.goto(`/events/${secondID}/media`); await expect(page.locator('.media-photo img')).toHaveCount(0); await expect(page.locator('input[value="UNSENT PRIVATE CAPTION"]')).toHaveCount(0)
})

test('two submits during deferred local file reading retain only one upload intent', async ({ page }) => {
  await page.addInitScript(() => {
    const original = File.prototype.arrayBuffer
    const readers: (() => void)[] = []
    Object.assign(window, { mediaFileReaders: readers })
    File.prototype.arrayBuffer = function () { return new Promise(resolve => { readers.push(() => { void original.call(this).then(resolve) }) }) }
  })
  const s = state(); s.role = 'photographer'; s.items = []; await mockMedia(page, s); await page.goto(gallery)
  await page.getByLabel('Media title', { exact: true }).fill('One deliberate photo')
  await page.getByLabel('Photograph (PNG or JPEG, up to 8 MiB)').setInputFiles({ name: 'venue.png', mimeType: 'image/png', buffer: png })
  await page.getByLabel('Submission reason').fill('One collection intent')
  await page.getByRole('button', { name: 'Submit private media draft' }).click()
  await page.getByRole('button', { name: 'Submit private media draft' }).click()
  await page.evaluate(() => { const readers = (window as unknown as { mediaFileReaders: (() => void)[] }).mediaFileReaders; readers.forEach(read => read()) })
  await expect(page.getByText('Saved. Current media has been refreshed.')).toBeVisible()
  expect(s.uploads).toHaveLength(1)
})

test('own consent remains reachable after gallery denial without granting gallery access', async ({ page }) => {
  const s = state(); s.denied = true; s.archived = true; await mockMedia(page, s); await page.goto(gallery)
  await page.getByRole('link', { name: 'Manage your own photo consent' }).click()
  await expect(page.getByRole('heading', { name: 'Your event photo consent', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Do not allow identifiable media' }).click()
  await expect(page.getByText('Current choice:', { exact: false })).toContainText('no')
  await expect(page.getByRole('button', { name: 'View protected photo' })).toHaveCount(0); expect(s.denied).toBe(true)
})

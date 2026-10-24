import { expect, test, type Page } from '@playwright/test'
import { mockWorkspace, firstID, secondID, person } from './fixtures'
import type { components } from '../src/api/schema.gen'

const notificationID = '019c25e0-15bd-7000-8000-000000000001', resourceID = '7b5cd9cc-86cd-4d4d-bf35-e50184b64b67'
type Detail = components['schemas']['InboxDetail']
async function prepare(page: Page) {
  await mockWorkspace(page)
  await page.route('**/api/v1/session', route => route.fulfill({ json: { person: { ...person, platform_admin: false }, draft_scope: '22222222-2222-4222-8222-222222222222', csrf_token: 'synthetic-csrf-token-32-characters', can_create_events: false } }))
  let allowed = true, read = false
  const requests: unknown[] = []
  const detail = (): Detail => ({ item: { id: notificationID, event_id: firstID, event_name: 'Synthetic event', label: 'Released event resource', created_at: '2027-06-19T09:00:00Z', read_at: read ? '2027-06-19T10:00:00Z' : null }, subject: 'Synthetic message for you', body: 'First line<br>\nSecond line<br>\n&lt;script&gt;window.INBOX_PWNED=true&lt;/script&gt;\n' + 'LongHarmlessText'.repeat(80), format: 'html', destination: { kind: 'content', resource_id: resourceID } })
  await page.route('**/api/v1/inbox**', route => {
    const request = route.request(), url = new URL(request.url()), id = url.pathname.split('/')[4]
    if (!id) return route.fulfill({ json: { items: allowed && url.searchParams.get('event_id') !== secondID ? [detail().item] : [], next_cursor: null, unread: allowed && !read && url.searchParams.get('event_id') !== secondID ? 1 : 0 } })
    if (!allowed) return route.fulfill({ status: 404, json: { error: { code: 'not_found', message: 'This notification is no longer available.', request_id: 'synthetic' } } })
    if (request.method() === 'POST') { requests.push(request.postDataJSON()); read = request.postDataJSON().read }
    return route.fulfill({ json: detail() })
  })
  return { revoke: () => { allowed = false }, requests }
}

test('keyboard opens safe preview and recipient read state toggles without arbitrary target', async ({ page }, testInfo) => {
  const state = await prepare(page)
  await page.goto('/inbox')
  await expect(page.getByRole('status').filter({ hasText: 'unread notifications' })).toContainText('1 unread notifications')
  await expect(page.getByText('Synthetic message for you')).toHaveCount(0)
  const open = page.getByRole('button', { name: 'Open Released event resource' })
  await open.focus(); await page.keyboard.press('Enter')
  await expect(page.getByRole('heading', { name: 'Synthetic message for you' })).toBeFocused()
  await expect(page.locator('.inbox-body')).toContainText('<script>window.INBOX_PWNED=true</script>')
  await expect(page.locator('.inbox-body')).toContainText('First line\nSecond line\n<script>')
  await expect(page.locator('.inbox-body')).not.toContainText('<br>')
  expect(await page.evaluate(() => (window as unknown as Record<string, unknown>).INBOX_PWNED)).toBeUndefined()
  await page.getByRole('button', { name: 'Mark read', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Mark unread', exact: true })).toBeVisible()
  expect(state.requests).toEqual([{ read: true }])
  await page.getByRole('button', { name: 'Mark unread', exact: true }).click()
  expect(state.requests).toEqual([{ read: true }, { read: false }])
  await page.getByRole('button', { name: 'Back to inbox', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Open Released event resource' })).toBeFocused()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  await page.screenshot({ path: testInfo.outputPath('inbox-safe-message.png'), fullPage: true })
})

test('event filters do not show hidden counts and revoked open removes body', async ({ page }) => {
  const state = await prepare(page)
  await page.goto('/inbox')
  await page.getByLabel('Filter by event').selectOption(secondID)
  await expect(page.getByText('No notifications are available in this view.')).toBeVisible()
  await expect(page.getByRole('status').filter({ hasText: 'unread notifications' })).toContainText('0 unread notifications')
  await page.getByLabel('Filter by event').selectOption('')
  await page.getByRole('button', { name: 'Open Released event resource' }).click()
  state.revoke()
  await page.getByRole('button', { name: 'Mark read', exact: true }).click()
  await expect(page.getByText('Synthetic message for you')).toHaveCount(0)
  await expect(page.getByRole('alert')).toContainText('We could not complete the request.')
})

test('released-resource link reaches existing current-authority reader and handles revocation', async ({ page }) => {
  await prepare(page)
  await page.route(`**/events/${firstID}/content/${resourceID}`, route => route.fulfill({ status: 404, json: { error: { code: 'not_found', message: 'The released resource is no longer available.', request_id: 'synthetic' } } }))
  await page.goto('/inbox')
  await page.getByRole('button', { name: 'Open Released event resource' }).click()
  await page.getByRole('link', { name: 'Open released resource' }).click()
  await expect(page).toHaveURL(`/events/${firstID}/resources/${resourceID}`)
  await expect(page.getByRole('alert')).toContainText('We could not complete the request.')
  await expect(page.getByText('Synthetic message for you')).toHaveCount(0)
})


test('returning after a viewport change restores the current responsive inbox action', async ({ page }) => {
  await prepare(page); await page.goto('/inbox')
  await page.getByRole('button', { name: 'Open Released event resource', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Synthetic message for you' })).toBeFocused()
  await page.setViewportSize({ width: 360, height: 800 })
  await page.getByRole('button', { name: 'Back to inbox', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Open Released event resource', exact: true })).toBeFocused()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
})

test('direct resource event denial stays latched and a remount waits for its own current resource authority', async ({ page }) => {
  await page.clock.install()
  const workspace = await mockWorkspace(page)
  let eventAllowed = true, resourceAllowed = true, hold = false, resourceReads = 0, acknowledgements = 0
  let release: () => void = () => {}
  const held = new Promise<void>(resolve => { release = resolve })
  const resource: components['schemas']['ContentPage'] = { id: resourceID, kind: 'page', revision: 1, version: 1, state: 'released', released_version: 1, released_at: '2027-06-13T09:00:00Z', acknowledged_at: null, can_manage: false, message: '', attachment_details: [], document: { title: 'Private direct inbox resource', markdown: 'PRIVATE-DIRECT-RESOURCE-CANARY', audience: { confirmed: true, roles: [], teams: [] }, attachments: [], scheduled_at: null, require_acknowledgement: true } }
  const denied = { error: { code: 'forbidden', message: 'Synthetic current access denied', request_id: 'synthetic' } }
  await page.route(`**/api/v1/events/${firstID}`, route => route.fulfill(eventAllowed ? { json: workspace.events[0] } : { status: 403, json: denied }))
  await page.route(`**/api/v1/events/${firstID}/content/${resourceID}**`, async route => {
    if (route.request().method() !== 'GET') { acknowledgements++; return route.fulfill({ status: 403, json: denied }) }
    resourceReads++
    if (hold) await held
    await route.fulfill(resourceAllowed ? { json: resource } : { status: 403, json: denied })
  })
  try {
    await page.goto(`/events/${firstID}/resources/${resourceID}`)
    await expect(page.getByText('PRIVATE-DIRECT-RESOURCE-CANARY', { exact: true })).toBeVisible()
    eventAllowed = false; hold = true
    await page.clock.fastForward(16_000)
    await expect(page.getByText('PRIVATE-DIRECT-RESOURCE-CANARY', { exact: true })).toHaveCount(0)
    await expect(page.getByRole('alert')).toBeVisible()
    const readsAtDenial = resourceReads
    eventAllowed = true
    await page.clock.fastForward(31_000)
    await expect(page.getByText('PRIVATE-DIRECT-RESOURCE-CANARY', { exact: true })).toHaveCount(0)
    expect(resourceReads).toBe(readsAtDenial)
    await page.getByRole('button', { name: 'Try again', exact: true }).click()
    await expect.poll(() => resourceReads).toBeGreaterThan(readsAtDenial)
    await expect(page.getByText('PRIVATE-DIRECT-RESOURCE-CANARY', { exact: true })).toHaveCount(0)
    await expect(page.getByRole('button', { name: 'I have read version 1', exact: true })).toHaveCount(0)
    resourceAllowed = false; release()
    await expect(page.getByRole('alert')).toBeVisible()
    await expect(page.getByText('PRIVATE-DIRECT-RESOURCE-CANARY', { exact: true })).toHaveCount(0)
    expect(acknowledgements).toBe(0)
  } finally { release() }
})

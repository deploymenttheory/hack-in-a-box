import { expect, test, type Page } from '@playwright/test'
import { mockWorkspace, firstID, secondID, person } from './fixtures'
import type { components } from '../src/api/schema.gen'

const notificationID = '019c25e0-15bd-7000-8000-000000000001', resourceID = '7b5cd9cc-86cd-4d4d-bf35-e50184b64b67'
type Detail = components['schemas']['InboxDetail']
async function prepare(page: Page) {
  await mockWorkspace(page)
  await page.route('**/api/v1/session', route => route.fulfill({ json: { person: { ...person, platform_admin: false }, csrf_token: 'synthetic-csrf-token-32-characters', can_create_events: false } }))
  let allowed = true, read = false
  const requests: unknown[] = []
  const detail = (): Detail => ({ item: { id: notificationID, event_id: firstID, event_name: 'Synthetic event', label: 'Released event resource', created_at: '2027-06-19T09:00:00Z', read_at: read ? '2027-06-19T10:00:00Z' : null }, subject: 'Synthetic message for you', body: '&lt;script&gt;window.INBOX_PWNED=true&lt;/script&gt;\n' + 'LongHarmlessText'.repeat(80), format: 'html', destination: { kind: 'content', resource_id: resourceID } })
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
  await expect(page.getByRole('status')).toContainText('1 unread notifications')
  await expect(page.getByText('Synthetic message for you')).toHaveCount(0)
  const open = page.getByRole('button', { name: 'Released event resource Unread' })
  await open.focus(); await page.keyboard.press('Enter')
  await expect(page.getByRole('heading', { name: 'Synthetic message for you' })).toBeFocused()
  await expect(page.locator('.inbox-body')).toContainText('<script>window.INBOX_PWNED=true</script>')
  expect(await page.evaluate(() => (window as unknown as Record<string, unknown>).INBOX_PWNED)).toBeUndefined()
  await page.getByRole('button', { name: 'Mark read', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Mark unread', exact: true })).toBeVisible()
  expect(state.requests).toEqual([{ read: true }])
  await page.getByRole('button', { name: 'Mark unread', exact: true }).click()
  expect(state.requests).toEqual([{ read: true }, { read: false }])
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  await page.screenshot({ path: testInfo.outputPath('inbox-safe-message.png'), fullPage: true })
})

test('event filters do not show hidden counts and revoked open removes body', async ({ page }) => {
  const state = await prepare(page)
  await page.goto('/inbox')
  await page.getByLabel('Filter by event').selectOption(secondID)
  await expect(page.getByText('No notifications are available in this view.')).toBeVisible()
  await expect(page.getByRole('status')).toContainText('0 unread notifications')
  await page.getByLabel('Filter by event').selectOption('')
  await page.getByRole('button', { name: 'Released event resource Unread' }).click()
  state.revoke()
  await page.getByRole('button', { name: 'Mark read', exact: true }).click()
  await expect(page.getByText('Synthetic message for you')).toHaveCount(0)
  await expect(page.getByRole('alert')).toContainText('We could not complete the request.')
})

test('released-resource link reaches existing current-authority reader and handles revocation', async ({ page }) => {
  await prepare(page)
  await page.route(`**/events/${firstID}/content/${resourceID}`, route => route.fulfill({ status: 404, json: { error: { code: 'not_found', message: 'The released resource is no longer available.', request_id: 'synthetic' } } }))
  await page.goto('/inbox')
  await page.getByRole('button', { name: 'Released event resource Unread' }).click()
  await page.getByRole('link', { name: 'Open released resource' }).click()
  await expect(page).toHaveURL(`/events/${firstID}/resources/${resourceID}`)
  await expect(page.getByRole('alert')).toContainText('We could not complete the request.')
  await expect(page.getByText('Synthetic message for you')).toHaveCount(0)
})

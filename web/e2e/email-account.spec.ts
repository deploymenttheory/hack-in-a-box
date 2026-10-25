import { expect, test, type Page } from '@playwright/test'
import { mockWorkspace, person } from './fixtures'

const email = 'synthetic-private-mailbox@example.test'
const code = '12345678'
const masked = 's***@example.test'
function emptyStatus() { return { configured: true, verified: false, masked_mailbox: '', provenance: '', pending: false, request_state: '', expires_at: null as string | null, retry_after_seconds: 0 } }
async function emailFixture(page: Page, outcome: 'accepted' | 'ambiguous' = 'accepted') {
  await mockWorkspace(page)
  let status = emptyStatus()
  const attempts: object[] = []
  await page.route('**/api/v1/account/email**', async route => {
    const request = route.request()
    if (request.method() === 'GET') return route.fulfill({ json: status })
    const body = request.postDataJSON(); attempts.push(body)
    if (request.url().endsWith('/verification')) {
      expect(body).toEqual({ email })
      status = { ...status, pending: true, request_state: outcome, expires_at: '2027-06-20T10:10:00Z', retry_after_seconds: 60 }
      return route.fulfill({ status: 202, json: status })
    }
    if (body.code !== code) return route.fulfill({ status: 400, json: { error: { code: 'email_code_invalid', message: 'Invalid code', request_id: 'synthetic' } } })
    status = { ...status, verified: true, pending: false, provenance: 'code_confirmed', masked_mailbox: masked, expires_at: null }
    return route.fulfill({ json: status })
  })
  return { attempts, reset: () => { status = emptyStatus() } }
}
async function noPrivatePersistence(page: Page) {
  const browser = await page.evaluate(async () => ({ local: JSON.stringify(localStorage), session: JSON.stringify(sessionStorage), url: location.href, history: JSON.stringify(history.state), resources: performance.getEntriesByType('resource').map(entry => entry.name).join(' '), caches: (await caches.keys()).join(' ') }))
  const serialized = JSON.stringify(browser)
  expect(serialized).not.toContain(email); expect(serialized).not.toContain(code)
  await expect(page.getByText(email, { exact: true })).toHaveCount(0)
  await expect(page.getByText(code, { exact: true })).toHaveCount(0)
}

test('email relay acceptance stays separate from proof and bad codes clear before retry', async ({ page }) => {
  const fixture = await emailFixture(page)
  await page.goto('/account')
  await page.getByLabel('Email address', { exact: true }).fill(email)
  await page.getByRole('button', { name: 'Send confirmation code' }).click()
  await expect(page.getByLabel('Email address', { exact: true })).toHaveValue('')
  await expect(page.getByText('The email relay accepted your confirmation request. Check your inbox for the code.')).toBeVisible()
  await expect(page.getByText('No confirmed email address.')).toBeVisible()
  await page.getByLabel('Eight-digit confirmation code').fill('87654321')
  await page.getByRole('button', { name: 'Confirm email address' }).click()
  await expect(page.getByText(/The confirmation code is invalid or expired/)).toBeVisible()
  await expect(page.getByLabel('Eight-digit confirmation code')).toHaveValue('')
  await noPrivatePersistence(page)
  await page.getByLabel('Eight-digit confirmation code').fill(code)
  await page.getByRole('button', { name: 'Confirm email address' }).click()
  await expect(page.getByText(masked, { exact: true })).toBeVisible()
  await expect(page.getByText('Confirmed by you')).toBeVisible()
  await expect(page.getByLabel('Eight-digit confirmation code')).toHaveCount(0)
  expect(fixture.attempts).toHaveLength(3)
  await noPrivatePersistence(page)
})

test('ambiguous email has no automatic retry and private fields clear on hide and navigation', async ({ page }, info) => {
  const fixture = await emailFixture(page, 'ambiguous')
  await page.goto('/account')
  await page.getByLabel('Email address', { exact: true }).fill(email)
  await page.getByRole('button', { name: 'Send confirmation code' }).click()
  await expect(page.getByText('Confirmation delivery is uncertain. If a code arrives, you can still use it.')).toBeVisible()
  await page.getByLabel('Eight-digit confirmation code').fill(code)
  await page.evaluate(() => window.dispatchEvent(new Event('pagehide')))
  await expect(page.getByLabel('Eight-digit confirmation code')).toHaveValue('')
  await page.getByLabel('Eight-digit confirmation code').fill(code)
  await page.getByRole('link', { name: info.project.metadata.portal === 'administrator' ? 'Hack in a Box · Admin home' : 'Hack in a Box home', exact: true }).click()
  await page.goBack()
  await expect(page.getByLabel('Eight-digit confirmation code')).toHaveValue('')
  expect(fixture.attempts).toHaveLength(1)
  await noPrivatePersistence(page)
})

test('changing the authenticated person discards the previous account private cache and code field', async ({ page }) => {
  const fixture = await emailFixture(page)
  let current = person
  await page.route('**/api/v1/session', route => route.fulfill({ json: { person: current, csrf_token: 'synthetic-csrf-token-32-characters', can_create_events: false } }))
  await page.goto('/account')
  await page.getByLabel('Email address', { exact: true }).fill(email)
  await page.getByRole('button', { name: 'Send confirmation code' }).click()
  await page.getByLabel('Eight-digit confirmation code').fill(code)
  await page.getByRole('button', { name: 'Confirm email address' }).click()
  await expect(page.getByText(masked, { exact: true })).toBeVisible()
  await page.getByLabel('New email address').fill(email)
  current = { ...person, id: 'a1fb08cf-67a6-4e97-98d9-2c19d558288f', display_name: 'Another synthetic person' }
  fixture.reset()
  await page.evaluate(() => window.dispatchEvent(new Event('visibilitychange')))
  await expect(page.getByRole('button', { name: current.display_name })).toBeVisible()
  await expect(page.getByText(masked, { exact: true })).toHaveCount(0)
  await expect(page.getByLabel('Email address', { exact: true })).toHaveValue('')
  await expect(page.getByLabel('Eight-digit confirmation code')).toHaveCount(0)
  await noPrivatePersistence(page)
})

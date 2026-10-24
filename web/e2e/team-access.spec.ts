import { openEventSection } from './event-navigation-fixture'
import { test, expect } from '@playwright/test'
import { accessFixture } from './team-access-fixtures'

async function noStoredValue(page: import('@playwright/test').Page) {
  const stored = await page.evaluate(async () => {
    const stores = [localStorage, sessionStorage].flatMap(storage => Object.keys(storage).map(key => storage.getItem(key) ?? ''))
    for (const name of await caches.keys()) { const cache = await caches.open(name); for (const request of await cache.keys()) stores.push(await (await cache.match(request))!.text()) }
    return stores.join('\n')
  })
  expect(stored).not.toContain('SYNTHETIC-NORTH-VALUE')
  expect(stored).not.toContain('SYNTHETIC-SOUTH-VALUE')
}

test('deliberately reveals and copies available team resources without persisting values', async ({ page }) => {
  const { state, secret } = await accessFixture(page)
  expect(state.reveals).toEqual([])
  await expect(page.getByLabel('Value hidden')).toHaveCount(2)
  await page.evaluate(() => Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: async (value: string) => { (window as unknown as { copiedValue: string }).copiedValue = value } } }))
  await page.getByRole('button', { name: 'Reveal Team account', exact: true }).click(); await page.getByRole('button', { name: 'Confirm reveal', exact: true }).click()
  await expect(page.getByText(secret(), { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Copy Team account', exact: true }).click()
  await expect(page.getByRole('status')).toHaveText('Value copied to your clipboard.')
  expect(await page.evaluate(() => (window as unknown as { copiedValue: string }).copiedValue)).toBe(secret())
  expect(state.reveals).toHaveLength(2)
  expect(state.reveals.map(request => request.revision)).toEqual(['"1"', '"1"'])
  await noStoredValue(page)
  await page.getByRole('button', { name: 'Hide value', exact: true }).click()
  await expect(page.getByText(secret(), { exact: true })).toHaveCount(0)
})

test('reports clipboard failure without claiming a successful copy', async ({ page }) => {
  const { secret } = await accessFixture(page)
  await page.evaluate(() => Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: async () => { throw new DOMException('Synthetic denied permission', 'NotAllowedError') } } }))
  await page.getByRole('button', { name: 'Reveal Team account', exact: true }).click(); await page.getByRole('button', { name: 'Confirm reveal', exact: true }).click()
  await page.getByRole('button', { name: 'Copy Team account', exact: true }).click()
  await expect(page.getByRole('status')).toContainText('Copy failed.')
  await expect(page.getByText('Value copied to your clipboard.', { exact: true })).toHaveCount(0)
  await expect(page.getByText(secret(), { exact: true })).toBeVisible()
})

test('timeout and changing the displayed resource remove old plaintext', async ({ page }) => {
  await page.clock.install({ time: new Date('2027-06-14T09:00:00Z') })
  const { secret, secondResource } = await accessFixture(page)
  await page.getByRole('button', { name: 'Reveal Team account', exact: true }).click(); await page.getByRole('button', { name: 'Confirm reveal', exact: true }).click()
  await expect(page.getByText(secret(), { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Reveal Room access code', exact: true }).click(); await page.getByRole('button', { name: 'Confirm reveal', exact: true }).click()
  await expect(page.getByText(secret(), { exact: true })).toHaveCount(0)
  await expect(page.getByText(secret(secondResource), { exact: true })).toBeVisible()
  await page.clock.runFor(30_100)
  await expect(page.getByText(secret(secondResource), { exact: true })).toHaveCount(0)
  await noStoredValue(page)
})

test('current membership, assignment and version refreshes clear displayed values', async ({ page }) => {
  await page.clock.install({ time: new Date('2027-06-14T09:00:00Z') })
  const { state, secret } = await accessFixture(page)
  await page.getByRole('button', { name: 'Reveal Team account', exact: true }).click(); await page.getByRole('button', { name: 'Confirm reveal', exact: true }).click()
  const oldValue = secret()
  state.items[0].version++; state.revision++
  await page.clock.runFor(5_100)
  await expect(page.getByText(oldValue, { exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: 'Reveal Team account', exact: true }).click(); await page.getByRole('button', { name: 'Confirm reveal', exact: true }).click()
  await expect(page.getByText(secret(), { exact: true })).toBeVisible()
  state.assigned = false
  await page.clock.runFor(5_100)
  await expect(page.getByText(secret(), { exact: true })).toHaveCount(0)
  await expect(page.getByRole('heading', { name: 'No private resources are available yet' })).toBeVisible()
  state.member = false
  await page.clock.runFor(5_100)
  await expect(page.getByRole('heading', { name: 'Waiting for your confirmed team' })).toBeVisible()
})

test('session expiry and rejected reveal cannot retain plaintext', async ({ page }) => {
  const { state, secret } = await accessFixture(page)
  await page.getByRole('button', { name: 'Reveal Team account', exact: true }).click(); await page.getByRole('button', { name: 'Confirm reveal', exact: true }).click()
  await expect(page.getByText(secret(), { exact: true })).toBeVisible()
  state.status = 401
  await page.getByRole('button', { name: 'Copy Team account', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Sign in to continue.' })).toBeVisible()
  await expect(page.getByText(secret(), { exact: true })).toHaveCount(0)
  await noStoredValue(page)
})

test('navigation aborts an outstanding reveal and ignores its late response', async ({ page }) => {
  const { state, secret, firstResource } = await accessFixture(page)
  state.hold = true
  await page.getByRole('button', { name: 'Reveal Team account', exact: true }).click(); await page.getByRole('button', { name: 'Confirm reveal', exact: true }).click()
  await expect.poll(() => !!state.delayed).toBe(true)
  await openEventSection(page, 'Overview')
  await state.delayed!.fulfill({ json: { id: firstResource, version: 1, value: secret() } }).catch(() => undefined)
  await expect(page.getByText(secret(), { exact: true })).toHaveCount(0)
  await openEventSection(page, 'Team access')
  await expect(page.getByLabel('Value hidden')).toHaveCount(2)
  await noStoredValue(page)
})

test('printing and hiding the tab remove private values without export or room controls', async ({ page }) => {
  const { secret } = await accessFixture(page)
  await page.getByRole('button', { name: 'Reveal Team account', exact: true }).click(); await page.getByRole('button', { name: 'Confirm reveal', exact: true }).click()
  await expect(page.getByText(secret(), { exact: true })).toBeVisible()
  await page.emulateMedia({ media: 'print' })
  await expect(page.locator('.private-team-access')).toBeHidden()
  expect(await page.locator('body').innerText()).not.toContain(secret())
  await page.evaluate(() => window.dispatchEvent(new Event('beforeprint')))
  await page.emulateMedia({ media: 'screen' })
  await expect(page.getByText(secret(), { exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: 'Reveal Team account', exact: true }).click(); await page.getByRole('button', { name: 'Confirm reveal', exact: true }).click()
  await page.evaluate(() => window.dispatchEvent(new Event('blur')))
  await expect(page.getByText(secret(), { exact: true })).toHaveCount(0)
  await expect(page.locator('.private-team-access').getByRole('link')).toHaveCount(0)
  await expect(page.locator('.private-team-access').getByRole('button', { name: /export|print|display|room mode/i })).toHaveCount(0)
})

test('separate accounts and signing out cannot reuse another team value', async ({ page, browser }) => {
  const first = await accessFixture(page)
  await page.getByRole('button', { name: 'Reveal Team account', exact: true }).click(); await page.getByRole('button', { name: 'Confirm reveal', exact: true }).click()
  await expect(page.getByText(first.secret(), { exact: true })).toBeVisible()
  const other = await browser.newContext()
  try {
    const secondPage = await other.newPage(), second = await accessFixture(secondPage, 'south')
    expect(second.state.reveals).toEqual([])
    await expect(secondPage.getByText(first.secret(), { exact: true })).toHaveCount(0)
    await secondPage.getByRole('button', { name: 'Reveal Team account', exact: true }).click(); await secondPage.getByRole('button', { name: 'Confirm reveal', exact: true }).click()
    await expect(secondPage.getByText(second.secret(), { exact: true })).toBeVisible()
    await expect(secondPage.getByText(first.secret(), { exact: true })).toHaveCount(0)
    await noStoredValue(secondPage)
  } finally { await other.close() }
  await page.getByRole('button', { name: 'North competitor', exact: true }).click()
  await page.getByRole('menuitem', { name: 'Sign out', exact: true }).click()
  await expect(page.getByRole('link', { name: 'Sign in with your organisation' })).toBeVisible()
  await expect(page.getByText(first.secret(), { exact: true })).toHaveCount(0)
  await noStoredValue(page)
})

for (const copy of [false, true]) test(`a delayed ${copy ? 'copy' : 'reveal'} response crossing resource expiry never discloses its value`, async ({ page }) => {
  await page.clock.install({ time: new Date('2027-06-14T09:00:00Z') })
  const { state, secret, firstResource } = await accessFixture(page)
  state.items[0].expires_at = '2027-06-14T09:00:02Z'
  await page.getByRole('button', { name: 'Refresh team access', exact: true }).click()
  await expect(page.getByText('Available until 14 Jun 2027, 10:00', { exact: false })).toBeVisible()
  await page.evaluate(() => Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: async () => { (window as unknown as { clipboardWrites: number }).clipboardWrites++ } } }))
  if (copy) {
    await page.getByRole('button', { name: 'Reveal Team account', exact: true }).click(); await page.getByRole('button', { name: 'Confirm reveal', exact: true }).click()
    await expect(page.getByText(secret(), { exact: true })).toBeVisible()
  }
  state.hold = true
  await page.getByRole('button', { name: `${copy ? 'Copy' : 'Reveal'} Team account`, exact: true }).click()
  if (!copy) await page.getByRole('button', { name: 'Confirm reveal', exact: true }).click()
  await expect.poll(() => !!state.delayed).toBe(true)
  await page.evaluate(value => {
    const state = window as unknown as { clipboardWrites: number; expiredValueShown: boolean }
    state.clipboardWrites = 0; state.expiredValueShown = false
    new MutationObserver(() => { if (document.body.textContent?.includes(value)) state.expiredValueShown = true }).observe(document.body, { subtree: true, childList: true, characterData: true })
  }, secret())
  await page.clock.runFor(2_100)
  await state.delayed!.fulfill({ json: { id: firstResource, version: 1, value: secret() } })
  await expect(page.getByRole('button', { name: 'Reveal Team account', exact: true })).toBeEnabled()
  await expect(page.getByText(secret(), { exact: true })).toHaveCount(0)
  expect(await page.evaluate(() => ({ shown: (window as unknown as { expiredValueShown: boolean }).expiredValueShown, copied: (window as unknown as { clipboardWrites: number }).clipboardWrites }))).toEqual({ shown: false, copied: 0 })
})

test('a lost reveal response records only unknown metadata and never repeats an audited request automatically', async ({ page }) => {
  const { state, secret } = await accessFixture(page)
  state.status = 503
  await page.getByRole('button', { name: 'Reveal Team account', exact: true }).focus()
  await page.keyboard.press('Enter')
  expect(state.reveals).toHaveLength(0)
  await page.getByRole('button', { name: 'Confirm reveal', exact: true }).click()
  await expect(page.getByText('Reveal outcome unknown.', { exact: false })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Unconfirmed private access audits' })).toContainText('Reveal private resource Team account · Expected revision 1 · Original outcome remains unconfirmed')
  await expect(page.getByText(secret(), { exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: 'Refresh team access', exact: true }).click()
  expect(state.reveals).toHaveLength(1)
  await noStoredValue(page)
  state.status = 200
  await page.getByRole('button', { name: 'Reveal Team account', exact: true }).click()
  expect(state.reveals).toHaveLength(1)
  await page.getByRole('button', { name: 'Confirm reveal', exact: true }).click()
  await expect(page.getByText(secret(), { exact: true })).toBeVisible()
  expect(state.reveals).toHaveLength(2)
  await expect(page.getByRole('region', { name: 'Unconfirmed private access audits' })).toContainText('Original outcome remains unconfirmed')
  await noStoredValue(page)
})

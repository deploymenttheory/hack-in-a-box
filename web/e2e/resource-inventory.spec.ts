import { test, expect, type Page } from '@playwright/test'
import { firstID } from './fixtures'
import { mockInventory, inventoryTeamIDs } from './resource-inventory-fixtures'

async function open(page: Page) { await page.goto(`/events/${firstID}`); await page.getByRole('button', { name: 'Inventory', exact: true }).click() }
async function storedBrowserState(page: Page) {
  return page.evaluate(async () => {
    const databases = await indexedDB.databases(), indexed: unknown[] = []
    for (const info of databases) if (info.name) await new Promise<void>((resolve, reject) => {
      const request = indexedDB.open(info.name!)
      request.onerror = () => reject(request.error)
      request.onsuccess = () => { const db = request.result, stores = [...db.objectStoreNames]; if (!stores.length) { db.close(); resolve(); return }; const tx = db.transaction(stores); for (const name of stores) { const read = tx.objectStore(name).getAll(); read.onsuccess = () => indexed.push(read.result) }; tx.oncomplete = () => { db.close(); resolve() }; tx.onerror = () => reject(tx.error) }
    })
    const cached: unknown[] = []
    for (const name of await caches.keys()) { const cache = await caches.open(name); for (const request of await cache.keys()) { const response = await cache.match(request); cached.push([request.url, response ? await response.clone().text() : '']) } }
    return JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage }, history: history.state, url: location.href, html: document.documentElement.outerHTML, indexed, cached, performance: performance.getEntriesByType('resource').map(r => r.name) })
  })
}

test('masked inventory identifies missing resources and reviews keyboard reassignment without revealing values', async ({ page }, info) => {
  const { state } = await mockInventory(page); await open(page)
  await expect(page.getByRole('heading', { name: 'Resource inventory', exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Teams missing an active resource' }).locator('..')).toContainText('South Stars')
  await expect(page.getByLabel('Stored secret masked')).toHaveCount(3)
  await expect(page.getByRole('table')).toContainText('Expired')
  await page.getByRole('button', { name: 'Assign Cloud account', exact: true }).click()
  await page.getByLabel(/North Stars/).uncheck(); await page.getByLabel(/South Stars/).check()
  await page.getByRole('button', { name: 'Preview resource assignment', exact: true }).click()
  await expect(page.getByRole('region', { name: 'Resource assignment preview' })).toContainText('Before: North Stars')
  await expect(page.getByRole('region', { name: 'Resource assignment preview' })).toContainText('After: South Stars')
  await expect(page.getByRole('region', { name: 'Resource assignment preview' })).toContainText('does not revoke the provider credential')
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
  await page.screenshot({ path: info.outputPath('resource-assignment-review.png'), fullPage: true })
  await page.getByRole('button', { name: 'Confirm resource assignment', exact: true }).focus(); await page.keyboard.press('Enter')
  await expect(page.getByRole('status').filter({ hasText: 'Resource audience updated' })).toBeVisible()
  expect(state.commands[0]).toMatchObject({ action: 'assign', team_ids: [inventoryTeamIDs[1]] })
  expect(state.values).toEqual([])
})

test('new secret is write-only and absent from browser history, storage, cache and unrelated requests', async ({ page }) => {
  const { state } = await mockInventory(page), secret = 'SYNTHETIC-NEW-RESOURCE-SECRET'
  const messages: string[] = [], unrelated: string[] = []
  page.on('console', message => messages.push(message.text())); page.on('pageerror', error => messages.push(error.message))
  page.on('request', request => { if (!new URL(request.url()).pathname.endsWith('/credentials')) unrelated.push(request.url() + (request.postData() ?? '')) })
  await open(page); await page.getByRole('button', { name: 'Add resource', exact: true }).click()
  await page.getByLabel('Resource label', { exact: true }).fill('Synthetic cloud token'); await page.getByLabel('Resource type', { exact: true }).selectOption('credential')
  const input = page.getByLabel('Secret value', { exact: true }); await expect(input).toHaveAttribute('type', 'password'); await input.fill(secret)
  expect(await storedBrowserState(page)).not.toContain(secret)
  await page.getByRole('button', { name: 'Save new resource', exact: true }).click()
  await expect(page.getByText('Resource stored without displaying its value. Review its team assignment next.')).toBeVisible()
  await expect(page.getByText(secret)).toHaveCount(0)
  expect(state.values).toEqual([secret]); expect(await storedBrowserState(page)).not.toContain(secret)
  expect(messages.join(' ')).not.toContain(secret); expect(unrelated.join(' ')).not.toContain(secret)
})

test('failed replacement clears the secret while preserving safe fields and provider follow-up', async ({ page }) => {
  const { state } = await mockInventory(page); state.failure = true; await open(page)
  await page.getByRole('button', { name: 'Replace Cloud account', exact: true }).click()
  const secret = page.getByLabel('New secret value', { exact: true }); await secret.fill('SYNTHETIC-FAILED-REPLACEMENT')
  await expect(page.getByRole('button', { name: 'Save replacement value' })).toBeDisabled()
  await page.getByLabel('I understand the provider follow-up').check(); await page.getByRole('button', { name: 'Save replacement value' }).click()
  await expect(secret).toHaveValue(''); await expect(page.getByText(/The new secret field was cleared/)).toBeVisible()
  await expect(page.getByRole('button', { name: 'Save replacement value' })).toBeDisabled()
  expect(await storedBrowserState(page)).not.toContain('SYNTHETIC-FAILED-REPLACEMENT')
  state.failure = false; await secret.fill('SYNTHETIC-SUCCESSFUL-REPLACEMENT'); await page.getByRole('button', { name: 'Save replacement value' }).click()
  await expect(page.getByText('Replacement value stored. Review the provider-side follow-up separately.')).toBeVisible()
  expect(state.value.items[0].version).toBe(2)
})

test('revocation requires explicit provider acknowledgement and records app-only access change', async ({ page }) => {
  const { state } = await mockInventory(page); await open(page)
  await page.getByRole('button', { name: 'Revoke Cloud account', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Revoke app access', exact: true })).toBeDisabled()
  await page.getByRole('button', { name: 'Cancel revocation', exact: true }).click(); expect(state.commands).toHaveLength(0)
  await page.getByRole('button', { name: 'Revoke Cloud account', exact: true }).click(); await page.getByLabel('I understand the provider follow-up').check()
  await page.getByRole('button', { name: 'Revoke app access', exact: true }).click()
  await expect(page.getByRole('status').filter({ hasText: 'App disclosure access revoked' })).toBeVisible()
  expect(state.commands).toMatchObject([{ action: 'revoke' }]); expect(state.value.items[0].active).toBe(false)
})

test('stale save clears sensitive input and cannot silently apply to the refreshed revision', async ({ page }) => {
  const { state } = await mockInventory(page); await open(page)
  await page.getByRole('button', { name: 'Replace Cloud account', exact: true }).click(); await page.getByLabel('New secret value', { exact: true }).fill('SYNTHETIC-STALE-SECRET')
  await page.getByLabel('I understand the provider follow-up').check(); state.stale = true
  await page.getByRole('button', { name: 'Save replacement value' }).click()
  await expect(page.getByText('The inventory changed while editing. Cancel and review the current inventory before saving again.')).toBeVisible()
  await expect(page.getByLabel('New secret value', { exact: true })).toHaveValue('')
  await expect(page.getByRole('button', { name: 'Save replacement value' })).toBeDisabled()
  expect(state.commands).toHaveLength(1); expect(await storedBrowserState(page)).not.toContain('SYNTHETIC-STALE-SECRET')
})

test('navigation, pagehide and current-role revocation clear secret forms and cached rendering', async ({ page }) => {
  const { state } = await mockInventory(page); await open(page)
  await page.getByRole('button', { name: 'Add resource', exact: true }).click(); await page.getByLabel('Secret value', { exact: true }).fill('SYNTHETIC-NAVIGATION-SECRET')
  await page.evaluate(() => window.dispatchEvent(new Event('pagehide'))); await expect(page.getByLabel('Secret value', { exact: true })).toHaveValue('')
  await page.getByLabel('Secret value', { exact: true }).fill('SYNTHETIC-SECOND-NAV-SECRET')
  page.once('dialog', dialog => dialog.accept()); await page.getByRole('button', { name: 'Overview', exact: true }).click(); await page.getByRole('button', { name: 'Inventory', exact: true }).click()
  await expect(page.getByLabel('Secret value', { exact: true })).toHaveCount(0)
  expect(await storedBrowserState(page)).not.toContain('SYNTHETIC-SECOND-NAV-SECRET')
  state.denied = true; await page.getByRole('button', { name: 'Refresh inventory', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Resource inventory', exact: true })).toHaveCount(0)
  await expect(page.getByText('You do not have access to this resource.')).toBeVisible()
})


test('safe resource metadata records event-local availability and expiry without reading a value', async ({ page }) => {
  const { state } = await mockInventory(page); await open(page)
  await page.getByRole('button', { name: 'Edit metadata for Cloud account', exact: true }).click()
  await expect(page.getByLabel('Secret value', { exact: true })).toHaveCount(0)
  await page.getByLabel('Resource label', { exact: true }).fill('Prepared cloud account')
  await page.getByLabel('Available from (UTC, optional)', { exact: true }).fill('2027-06-19T08:00:00')
  await page.getByLabel('Expiry (UTC, optional)', { exact: true }).fill('2027-06-20T18:00:00')
  await page.getByRole('button', { name: 'Save resource metadata', exact: true }).click()
  await expect(page.getByText('Resource metadata updated. Availability and expiry are enforced by the server.')).toBeVisible()
  expect(state.commands[0]).toMatchObject({ action: 'metadata', label: 'Prepared cloud account', resource_type: 'account', available_at: '2027-06-19T08:00:00Z', expires_at: '2027-06-20T18:00:00Z' })
  expect(state.values).toEqual([])
})

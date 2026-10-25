import { expect, test } from '@playwright/test'

test('production showcase examples keep approved event context and restore the actual opener without APIs or private storage', async ({ page }) => {
 await page.addInitScript(() => {
  Object.defineProperty(Document.prototype, 'cookie', { get() { throw new Error('Guide read cookies') }, set() { throw new Error('Guide wrote cookies') } })
  for (const method of ['getItem', 'setItem', 'removeItem', 'clear', 'key']) Object.defineProperty(Storage.prototype, method, { value() { throw new Error('Guide accessed private storage') } })
 })
 const requests: string[] = [], errors: string[] = []
 page.on('request', request => { if (/^\/(api|auth)\//.test(new URL(request.url()).pathname)) requests.push(request.url()) })
 page.on('pageerror', error => errors.push(error.message))
 await page.goto('/patterns.html')
 const example = page.getByRole('region', { name: 'Reviewed showcase example', exact: true })
 await example.getByLabel('Synthetic showcase event').selectOption({ label: 'Synthetic spring hack' })
 await expect(example.getByRole('link', { name: 'Show useful non-winning work', exact: true })).toBeVisible()
 await expect(example.getByRole('link', { name: 'Make service recovery easier', exact: true })).toHaveCount(0)
 const open = example.getByRole('button', { name: 'Open synthetic project', exact: true })
 await open.press('Enter')
 await expect(example.getByRole('heading', { name: 'Show useful non-winning work', exact: true })).toBeFocused()
 await expect(example.getByRole('article').getByText('Archived · Read-only', { exact: true })).toBeVisible()
 await expect(example.getByRole('article').getByText('No published award shown', { exact: true })).toBeVisible()
 await example.getByRole('button', { name: 'Back to synthetic projects', exact: true }).click()
 await expect(open).toBeFocused()
 await expect(example.getByLabel('Synthetic showcase event')).toHaveValue('12222222-2222-4222-8222-222222222222')
 await example.getByLabel('Synthetic showcase event').selectOption({ label: 'Synthetic autumn hack' })
 await example.getByRole('button', { name: 'Show synthetic published award', exact: true }).click()
 await expect(example.getByText('Synthetic published award', { exact: true })).toBeVisible()
 expect(requests).toEqual([]); expect(errors).toEqual([])
})

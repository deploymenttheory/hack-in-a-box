import { expect, test } from '@playwright/test'

test('independent mentor advisory guide preserves actual focus, corrections and unknown callback separation without private APIs/storage', async ({ page }) => {
 const privateRequests: string[] = [], errors: string[] = []
 await page.addInitScript(() => { Object.defineProperty(Document.prototype, 'cookie', { configurable: true, get() { throw new Error('Guide read cookies') }, set() { throw new Error('Guide wrote cookies') } }); for (const method of ['getItem', 'setItem', 'removeItem', 'clear', 'key'] as const) Object.defineProperty(Storage.prototype, method, { value() { throw new Error('Guide accessed private storage') } }) })
 page.on('request', request => { if (new URL(request.url()).pathname.startsWith('/api/')) privateRequests.push(request.url()) }); page.on('pageerror', error => errors.push(error.message))
 await page.goto('/patterns.html')
 const region = page.getByRole('region', { name: 'Mentor advisory example', exact: true }), opener = region.getByRole('button', { name: 'Review synthetic advisory', exact: true })
 await region.getByLabel('Your advisory views', { exact: true }).fill('A deliberate synthetic observation.')
 await opener.press('Enter')
 const review = page.getByRole('alertdialog', { name: 'Review mentor advisory', exact: true })
 await review.getByRole('button', { name: 'Cancel', exact: true }).click(); await expect(opener).toBeFocused(); await expect(region.getByLabel('Your advisory views', { exact: true })).toHaveValue('A deliberate synthetic observation.')
 await opener.click(); await review.getByRole('button', { name: 'Save advisory views', exact: true }).click(); await page.keyboard.press('Escape'); await expect(review).toBeVisible()
 await page.emulateMedia({ media: 'print' }); await expect(review).not.toBeVisible(); await page.emulateMedia({ media: 'screen' })
 await review.getByRole('button', { name: 'Complete synthetic advisory callback', exact: true }).click(); await expect(region.getByText('Required judge ballots: 0 / 2.', { exact: false })).toBeVisible(); await expect(region.getByText('Contribution version 1', { exact: false })).toBeVisible()
 await region.getByLabel('Your advisory views', { exact: true }).fill('A reasoned synthetic correction.'); await opener.click(); await review.getByRole('button', { name: 'Save advisory views', exact: true }).click(); await review.getByRole('button', { name: 'Complete synthetic advisory callback', exact: true }).click(); await expect(region.getByText('Corrects version 1', { exact: false })).toBeVisible()
 await opener.click(); await review.getByRole('button', { name: 'Save advisory views', exact: true }).click(); await review.getByRole('button', { name: 'Lose synthetic advisory callback', exact: true }).click(); await region.getByRole('button', { name: 'Read synthetic current advisory', exact: true }).click(); await expect(region.getByText('Synthetic outcome unknown.', { exact: false })).toBeVisible(); await region.getByRole('button', { name: 'Reconcile synthetic original advisory', exact: true }).click(); await expect(region.getByText('This is not a native receipt.', { exact: false })).toBeVisible()
 expect(privateRequests).toEqual([]); expect(errors).toEqual([])
})

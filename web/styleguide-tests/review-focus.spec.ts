import { expect, test } from '@playwright/test'
test('published review focus example returns after two reads without API transport', async ({ page }) => {
 const calls: string[] = []
 page.on('request', request => { if (new URL(request.url()).pathname.startsWith('/api/')) calls.push(request.url()) })
 await page.goto('/patterns.html')
 const example = page.getByRole('region', { name: 'Consecutive read focus example', exact: true })
 const opener = example.getByRole('button', { name: 'Review synthetic focus return', exact: true })
 await opener.click()
 await page.getByRole('alertdialog', { name: 'Synthetic focus preview' }).getByRole('button', { name: 'Cancel', exact: true }).click()
 await expect(example.getByRole('status')).toHaveText('Both synthetic reads completed.')
 await expect(opener).toBeFocused()
 expect(calls).toEqual([])
})

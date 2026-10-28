import { test, expect } from '@playwright/test'

test('production setup fields review named assignments and return keyboard focus without requests', async ({ page }) => {
  const calls: string[] = [], errors: string[] = []
  page.on('request', request => { if (/^\/(api|auth)\//.test(new URL(request.url()).pathname)) calls.push(request.url()) })
  page.on('pageerror', error => errors.push(error.message))
  await page.goto('/patterns.html')
  const example = page.getByRole('region', { name: 'Final judging setup example', exact: true })
  await example.getByRole('button', { name: 'Show existing synthetic round', exact: true }).click()
  await example.getByRole('combobox', { name: 'Team to assign', exact: true }).selectOption({ label: 'Aurora' })
  await example.getByRole('checkbox', { name: 'Morgan', exact: true }).check()
  await example.getByRole('textbox', { name: 'Assignment reason', exact: true }).fill('Local deliberate partial coverage')
  const opener = example.getByRole('button', { name: 'Review judge assignment', exact: true })
  await opener.click()
  const gap = example.getByRole('checkbox', { name: 'I understand this leaves 2 required judge places unfilled', exact: true })
  await expect(gap).toBeFocused(); await gap.check(); await opener.focus(); await opener.press('Enter')
  const review = page.getByRole('alertdialog', { name: 'Review final judge assignment', exact: true })
  await expect(review).toContainText('Morgan'); await expect(review).toContainText('Unfilled places explicitly acknowledged')
  await review.getByRole('button', { name: 'Cancel', exact: true }).click(); await expect(opener).toBeFocused()
  await opener.press('Enter'); await review.getByRole('button', { name: 'Save reviewed judge assignment', exact: true }).click()
  await expect(example.getByRole('status', { name: 'Local judging setup result', exact: true })).toContainText('No round or assignment was sent')
  expect(calls).toEqual([]); expect(errors).toEqual([])
})

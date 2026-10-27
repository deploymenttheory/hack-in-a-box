import { test, expect } from '@playwright/test'

test('rubric guide uses production fields and captured review without application requests or private storage', async ({ page }) => {
  const calls: string[] = [], errors: string[] = []
  page.on('request', request => { if (/^\/(api|auth)\//.test(new URL(request.url()).pathname)) calls.push(request.url()) })
  page.on('pageerror', error => errors.push(error.message))
  await page.addInitScript(() => { Object.defineProperty(Document.prototype, 'cookie', { get() { throw new Error('Guide read cookies') }, set() { throw new Error('Guide wrote cookies') } }); for (const method of ['getItem', 'setItem', 'removeItem', 'clear', 'key']) Object.defineProperty(Storage.prototype, method, { value() { throw new Error('Guide accessed private storage') } }) })
  await page.goto('/patterns.html')
  const example = page.getByRole('region', { name: 'Rubric authoring example', exact: true })
  await example.getByLabel('Rubric name', { exact: true }).fill('Local guide review only')
  const opener = example.getByRole('button', { name: 'Review guide rubric', exact: true }); await opener.press('Enter')
  const dialog = page.getByRole('alertdialog', { name: 'Review synthetic rubric', exact: true })
  await expect(dialog).toContainText('Local guide review only'); await expect(dialog).toContainText('Starting revision 0 · Version 0')
  await dialog.getByRole('button', { name: 'Cancel', exact: true }).click(); await expect(opener).toBeFocused(); await expect(example.getByLabel('Rubric name', { exact: true })).toHaveValue('Local guide review only')
  await opener.click(); await dialog.getByRole('button', { name: 'Finish local rubric review', exact: true }).click(); await expect(example.getByRole('status').filter({ hasText: 'Local review finished.' })).toContainText('No rubric was saved, approved or published.')
  expect(calls).toEqual([]); expect(errors).toEqual([])
})

test('rubric guide reveals an invalid category and never reviews duplicate anchor scores', async ({ page }) => {
  await page.goto('/patterns.html')
  const example = page.getByRole('region', { name: 'Rubric authoring example', exact: true }), sections = example.getByRole('navigation', { name: 'Rubric definition sections', exact: true })
  await sections.getByRole('button', { name: 'Categories', exact: true }).click(); await example.getByText('Demo', { exact: true }).click()
  const anchor = example.getByLabel('Anchor 2 score for category 1', { exact: true }); await anchor.fill('1')
  await sections.getByRole('button', { name: 'Model', exact: true }).click(); await example.getByRole('button', { name: 'Review guide rubric', exact: true }).click()
  await expect(example.getByLabel('Anchor 1 score for category 1', { exact: true })).toBeFocused(); await expect(anchor).toHaveAttribute('aria-invalid', 'true')
  await expect(page.getByRole('alertdialog', { name: 'Review synthetic rubric', exact: true })).toHaveCount(0)
})

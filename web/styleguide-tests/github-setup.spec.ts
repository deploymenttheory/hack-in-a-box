import { expect, test } from '@playwright/test'
test('published GitHub setup example explains field errors without API or credential transport', async ({ page }) => {
  const apiCalls: string[] = []
  page.on('request', request => { if (new URL(request.url()).pathname.startsWith('/api/')) apiCalls.push(request.url()) })
  await page.goto('/patterns.html')
  const example = page.getByRole('region', { name: 'GitHub setup validation example', exact: true })
  const id = example.getByLabel('GitHub App numeric ID', { exact: true })
  await example.getByRole('button', { name: 'Review synthetic App details', exact: true }).click()
  await expect(id).toBeFocused(); await expect(id).toHaveAttribute('aria-invalid', 'true')
  await id.fill('42'); await id.press('Tab')
  await expect(id).toHaveAccessibleDescription(/Format looks right; connection not yet verified/)
  expect(apiCalls).toEqual([])
})

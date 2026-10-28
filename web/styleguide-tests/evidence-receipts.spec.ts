import { test, expect } from '@playwright/test'

test('evidence receipt guide compares production values and announces changes without application requests', async ({ page }) => {
  const calls: string[] = [], errors: string[] = []
  page.on('request', request => { if (/^\/(api|auth)\//.test(new URL(request.url()).pathname)) calls.push(request.url()) })
  page.on('pageerror', error => errors.push(error.message))
  await page.goto('/patterns.html')
  const example = page.getByRole('region', { name: 'Team evidence receipt comparison', exact: true })
  await expect(example.getByRole('status')).toHaveText('Evidence fields match the reviewed values.')
  const change = example.getByRole('button', { name: 'Change returned URL', exact: true })
  await change.focus(); await change.press('Enter')
  await expect(change).toBeFocused()
  await expect(example.getByRole('status')).toContainText('Evidence changed. Do not acknowledge')
  const identical = example.getByRole('button', { name: 'Use reordered identical evidence', exact: true })
  await identical.focus(); await identical.press('Enter')
  await expect(identical).toBeFocused()
  await expect(example.getByRole('status')).toHaveText('Evidence fields match the reviewed values.')
  expect(calls).toEqual([]); expect(errors).toEqual([])
})

import { expect, test } from '@playwright/test'
import { mockWorkspace } from './fixtures'

test('GitHub ownership requires verification and explicit invitation impact acknowledgment', async ({ page }, testInfo) => {
  await mockWorkspace(page)
  let linked = false
  let review = 0
  let revision = 0
  await page.route('**/api/v1/account/github', async route => {
    if (route.request().method() === 'DELETE') {
      expect(route.request().postDataJSON()).toEqual({ acknowledge_invitation_impact: true })
      expect(route.request().headers()['if-match']).toBe('"1"')
      linked = false; review = 2; revision++
    }
    await route.fulfill({ json: { enabled: true, revision, account: linked ? { github_id: 123456, login: 'synthetic-builder', verified_at: '2026-10-21T12:00:00Z' } : null, invitations_needing_review: review } })
  })
  await page.route('**/api/v1/account/github/authorize', async route => {
    expect(route.request().postDataJSON()).toEqual({ acknowledge_invitation_impact: true })
    expect(route.request().headers()['x-csrf-token']).toBeTruthy()
    expect(route.request().headers()['if-match']).toBe('"0"')
    linked = true; revision++
    await route.fulfill({ json: { url: new URL('/account?github=linked', page.url()).href } })
  })
  await page.goto('/account')
  await expect(page.getByText('No verified GitHub account linked.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Link GitHub account', exact: true })).toBeDisabled()
  await page.getByRole('checkbox').check()
  await page.getByRole('button', { name: 'Link GitHub account', exact: true }).click()
  await expect(page.getByText('@synthetic-builder')).toBeVisible()
  await expect(page.getByText(/Verified GitHub ID 123456/)).toBeVisible()
  await expect(page.getByRole('button', { name: 'Unlink GitHub account', exact: true })).toBeDisabled()
  await page.screenshot({ path: testInfo.outputPath('github-account.png'), fullPage: true })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  await page.getByRole('checkbox').check()
  await page.getByRole('button', { name: 'Unlink GitHub account', exact: true }).click()
  await expect(page.getByText('No verified GitHub account linked.')).toBeVisible()
  await expect(page.getByText('2 pending invitation(s) need organiser review.')).toBeVisible()
})
test('GitHub configuration and failed callbacks are explained without claiming verification', async ({ page }) => {
  await mockWorkspace(page)
  await page.goto('/account?github=failed')
  await expect(page.getByText('GitHub linking is not configured for this installation.')).toBeVisible()
  await expect(page.getByRole('alert')).toContainText('GitHub verification did not complete')
  await expect(page.getByText('No verified GitHub account linked.')).toBeVisible()
})

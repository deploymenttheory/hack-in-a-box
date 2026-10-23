import { expect, test } from '@playwright/test'
import { deliveryFixture, deliveryIDs, detailPath } from './delivery-history-fixtures'

test('definite rejection retry becoming accepted cannot become a generic repeat send', async ({ page }) => {
  const fixture = await deliveryFixture(page)
  await page.goto(detailPath('retryable'))
  await page.getByLabel('Recovery action').selectOption('retry')
  await expect(page.getByText('Its original approval must remain valid;', { exact: false })).toBeVisible()
  await page.getByLabel('Private operator reason').fill('Review definite rejection under the original approval')
  await page.getByRole('button', { name: 'Record explicit decision' }).dblclick()
  await expect(page.getByText('The reviewed retry was queued.', { exact: false })).toBeVisible()
  expect(fixture.state.posts).toHaveLength(1)
  const row = fixture.rows.find(item => item.id === deliveryIDs.retryable)!
  row.state = 'provider_accepted'; row.operation_state = 'delivered'; row.reason = 'relay_accepted'; row.revision++
  await page.getByRole('button', { name: 'Refresh current delivery evidence' }).click()
  await expect(page.getByText('Provider accepted — delivery not confirmed', { exact: true }).first()).toBeVisible()
  await expect(page.getByLabel('Recovery action').getByRole('option', { name: 'Retry a definite temporary failure', exact: true })).toHaveCount(0)
  await page.getByLabel('Recovery action').selectOption('resend_acknowledged')
  await page.getByLabel('Private operator reason').fill('A different explicit possible-duplicate decision is required')
  await expect(page.getByRole('button', { name: 'Record explicit decision' })).toBeDisabled()
  expect(fixture.state.posts).toHaveLength(1)
})

test('uncertain shared-channel timeout resolves without a new send or invented acceptance', async ({ page }) => {
  const fixture = await deliveryFixture(page)
  const row = fixture.rows.find(item => item.id === deliveryIDs.teams)!
  row.state = 'ambiguous'; row.operation_state = 'ambiguous'; row.reason = 'acceptance_unknown'
  await page.goto(detailPath('teams'))
  await expect(page.getByText('Uncertain outcome', { exact: true }).first()).toBeVisible()
  await expect(page.getByLabel('Recovery action').getByRole('option', { name: 'Retry a definite temporary failure', exact: true })).toHaveCount(0)
  await page.getByLabel('Recovery action').selectOption('resolve_no_resend')
  await page.getByLabel('Private operator reason').fill('Do not repeat a possibly accepted channel message')
  fixture.state.drop = true
  await page.getByRole('button', { name: 'Record explicit decision' }).click()
  await expect(page.getByText('Outcome unknown — no automatic replay', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Refresh current delivery evidence' }).click()
  await expect(page.getByText('The server confirms the exact recorded decision.', { exact: false })).toBeVisible()
  await expect(page.getByText('Uncertain outcome', { exact: true }).first()).toBeVisible()
  await expect(page.getByRole('button', { name: 'Retry the same recovery decision' })).toHaveCount(0)
  expect(fixture.state.posts).toHaveLength(1)
  expect(fixture.state.posts[0].body.action).toBe('resolve_no_resend')
  expect(row.cancelled).toBe(true)
  expect(row.state).toBe('ambiguous')
})

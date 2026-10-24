import { expect, test } from '@playwright/test'
import { detailPath, deliveryFixture, deliveryIDs, historyPath } from './delivery-history-fixtures'
import { secondID } from './fixtures'

test('history distinguishes actual selections, local inbox persistence, acceptance, omission and provider readiness', async ({ page }) => {
  const fixture = await deliveryFixture(page)
  await page.goto(historyPath)
  await expect(page.getByRole('heading', { name: 'Provider health and readiness' })).toBeVisible()
  await expect(page.getByText('Local configuration checks only.', { exact: false })).toBeVisible()
  await expect(page.getByText('Never attempted', { exact: true })).toBeVisible()
  await expect(page.getByText('Provider accepted — delivery not confirmed', { exact: true })).toBeVisible()
  await expect(page.getByText('Uncertain outcome', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'More records' }).click()
  await expect(page.getByText('Recorded in the private in-app inbox', { exact: true })).toBeVisible()
  await expect(page.getByText('Not attempted — recipient unavailable', { exact: true })).toBeVisible()
  await page.getByLabel('Channel', { exact: true }).selectOption('teams')
  const channel = page.getByRole('link', { name: 'Shared event channel', exact: true })
  await channel.focus(); await page.keyboard.press('Enter')
  await expect(page.getByRole('heading', { name: 'Shared event channel', exact: true })).toBeVisible()
  await expect(page.getByText('Campaign version 3', { exact: false })).toBeVisible()
  await expect(page.getByText(/^Prepared [0-9]/)).toBeVisible()
  expect(fixture.state.posts).toHaveLength(0)
})

test('accepted send needs an explicit duplicate-risk decision and can cancel only its queued resend', async ({ page }) => {
  const fixture = await deliveryFixture(page)
  await page.goto(detailPath('accepted'))
  await page.getByLabel('Recovery action').selectOption('resend_acknowledged')
  await page.getByLabel('Private operator reason').fill('PRIVATE-EXPLICIT-RESEND-REASON')
  await expect(page.getByRole('button', { name: 'Record explicit decision' })).toBeDisabled()
  await page.getByRole('checkbox').check()
  await page.getByRole('button', { name: 'Record explicit decision' }).focus(); await page.keyboard.press('Enter')
  await expect(page.getByText('The explicit resend decision was recorded.', { exact: false })).toBeVisible()
  expect(fixture.state.posts).toHaveLength(1)
  expect(fixture.state.posts[0].body.action).toBe('resend_acknowledged')
  await expect(page.getByText('Provider accepted — delivery not confirmed', { exact: true }).first()).toBeVisible()
  await page.getByLabel('Recovery action').selectOption('cancel_resend')
  await page.getByLabel('Private operator reason').fill('Cancel only the unspent additional attempt')
  await page.getByRole('button', { name: 'Record explicit decision' }).click()
  await expect(page.getByText('Only the queued resend was cancelled.', { exact: false })).toBeVisible()
  expect(fixture.state.posts).toHaveLength(2)
  expect(fixture.state.posts[1].revision).toBe('"2"')
  expect(fixture.rows.find(row => row.id === deliveryIDs.accepted)?.state).toBe('provider_accepted')
})

test('uncertain decision retry preserves identical request identity and never grants a second resend', async ({ page }) => {
  const fixture = await deliveryFixture(page); fixture.state.drop = true
  await page.goto(detailPath('ambiguous'))
  await page.getByLabel('Recovery action').selectOption('resend_acknowledged')
  await page.getByLabel('Private operator reason').fill('PRIVATE-UNKNOWN-DECISION-CANARY')
  await page.getByRole('checkbox').check()
  await page.getByRole('button', { name: 'Record explicit decision' }).click()
  await expect(page.getByText('Outcome unknown — no automatic replay', { exact: true })).toBeVisible()
  await expect(page.getByLabel('Private operator reason')).toBeDisabled()
  await page.getByRole('button', { name: 'Retry the same recovery decision' }).dblclick()
  await expect(page.getByText('The explicit resend decision was recorded.', { exact: false })).toBeVisible()
  expect(fixture.commands.size).toBe(1)
  expect(fixture.state.posts).toHaveLength(2)
  expect(fixture.state.posts[0]).toEqual(fixture.state.posts[1])
  expect(await page.evaluate(() => JSON.stringify(localStorage) + JSON.stringify(sessionStorage))).not.toContain('PRIVATE-UNKNOWN-DECISION-CANARY')
  expect(page.url()).not.toContain('PRIVATE-UNKNOWN-DECISION-CANARY')
})

test('read-only reconciliation of an unknown no-resend decision retains uncertain provider evidence after archive', async ({ page }) => {
  await page.clock.install()
  const fixture = await deliveryFixture(page); fixture.state.drop = true
  await page.goto(detailPath('ambiguous'))
  await page.getByLabel('Recovery action').selectOption('resolve_no_resend')
  await page.getByLabel('Private operator reason').fill('PRIVATE-NO-RESEND-CANARY')
  await page.getByRole('button', { name: 'Record explicit decision' }).click()
  await expect(page.getByText('Outcome unknown — no automatic replay', { exact: true })).toBeVisible()
  fixture.state.archived = true
  await page.clock.fastForward(11_000)
  await expect(page.getByRole('button', { name: 'Retry the same recovery decision' })).toBeDisabled()
  await page.getByRole('button', { name: 'Refresh current delivery evidence' }).click()
  await expect(page.getByText('The server confirms the exact recorded decision.', { exact: false })).toBeVisible()
  await expect(page.getByText('Uncertain outcome', { exact: true }).first()).toBeVisible()
  expect(fixture.state.posts).toHaveLength(1)
  expect(fixture.commands.size).toBe(1)
})

test('native conflict and transient evidence failure preserve unsent reason without inventing a saved decision', async ({ page }) => {
  await page.clock.install()
  const fixture = await deliveryFixture(page)
  await page.goto(detailPath('retryable'))
  await page.getByLabel('Recovery action').selectOption('retry')
  await page.getByLabel('Private operator reason').fill('🌍'.repeat(600))
  await expect(page.getByRole('button', { name: 'Record explicit decision' })).toBeDisabled()
  await page.getByLabel('Private operator reason').fill('PRIVATE-UNSENT-CONFLICT')
  fixture.state.reject = true
  await page.getByRole('button', { name: 'Record explicit decision' }).click()
  await expect(page.getByText('No new decision was confirmed.', { exact: false })).toBeVisible()
  await expect(page.getByLabel('Private operator reason')).toHaveValue('PRIVATE-UNSENT-CONFLICT')
  expect(fixture.commands.size).toBe(0)
  fixture.state.transientReads = 1
  await page.clock.fastForward(11_000)
  await expect(page.getByLabel('Private operator reason')).toBeDisabled()
  await expect(page.getByText('Unsent private decision reason', { exact: true })).toBeVisible()
  expect(fixture.state.posts).toHaveLength(1)
})

test('history-page changes require leave acknowledgement and staying keeps the unsent private reason', async ({ page }) => {
  const fixture = await deliveryFixture(page); fixture.state.pagedAttempts = true
  await page.goto(detailPath('ambiguous'))
  await page.getByLabel('Private operator reason').fill('PRIVATE-HISTORY-PAGE-CANARY')
  const stay = page.waitForEvent('dialog'), change = page.getByRole('button', { name: 'Older provider attempts' }).click()
  await (await stay).dismiss(); await change
  await expect(page.getByLabel('Private operator reason')).toHaveValue('PRIVATE-HISTORY-PAGE-CANARY')
  const leave = page.waitForEvent('dialog'), next = page.getByRole('button', { name: 'Older provider attempts' }).click()
  await (await leave).accept(); await next
  await expect(page.getByLabel('Private operator reason')).toHaveValue('')
  expect(fixture.state.posts).toHaveLength(0)
})

test('native role loss clears private reason and recipient state immediately without a leave confirmation', async ({ page }) => {
  await page.clock.install()
  const fixture = await deliveryFixture(page)
  await page.goto(detailPath('accepted'))
  await page.getByLabel('Private operator reason').fill('PRIVATE-REVOKED-REASON')
  let confirmations = 0; page.on('dialog', async dialog => { confirmations++; await dialog.dismiss() })
  fixture.state.allowed = false
  await page.clock.fastForward(11_000)
  await expect(page.getByLabel('Private operator reason')).toHaveCount(0)
  await expect(page.getByRole('heading', { name: 'Synthetic accepted recipient' })).toHaveCount(0)
  await expect(page.getByRole('alert')).toBeVisible()
  expect(confirmations).toBe(0)
  expect(fixture.state.posts).toHaveLength(0)
})

test('staying on an unknown decision during history paging retains its exact explicit retry', async ({ page }) => {
  const fixture = await deliveryFixture(page); fixture.state.drop = true; fixture.state.pagedAttempts = true
  await page.goto(detailPath('ambiguous'))
  await page.getByLabel('Recovery action').selectOption('resolve_no_resend')
  await page.getByLabel('Private operator reason').fill('PRIVATE-UNKNOWN-PAGE-RETRY')
  await page.getByRole('button', { name: 'Record explicit decision' }).click()
  await expect(page.getByText('Outcome unknown — no automatic replay', { exact: true })).toBeVisible()
  const stay = page.waitForEvent('dialog'), change = page.getByRole('button', { name: 'Older provider attempts' }).click()
  await (await stay).dismiss(); await change
  await expect(page.getByLabel('Private operator reason')).toHaveValue('PRIVATE-UNKNOWN-PAGE-RETRY')
  await page.getByRole('button', { name: 'Retry the same recovery decision' }).click()
  await expect(page.getByText('The no-resend decision was recorded.', { exact: false })).toBeVisible()
  expect(fixture.state.posts).toHaveLength(2)
  expect(fixture.state.posts[0]).toEqual(fixture.state.posts[1])
  expect(fixture.commands.size).toBe(1)
})

test('account and event changes clear unsent recovery text instead of persisting it', async ({ page }) => {
  const fixture = await deliveryFixture(page)
  await page.goto(detailPath('accepted'))
  await page.getByLabel('Private operator reason').fill('PRIVATE-ACCOUNT-SWITCH-CANARY')
  fixture.state.actor = { ...fixture.state.actor, id: 'e1000000-0000-4000-8000-000000000001' }
  await page.evaluate(() => window.dispatchEvent(new Event('visibilitychange')))
  await expect(page.getByLabel('Private operator reason')).toHaveValue('')
  await page.getByLabel('Private operator reason').fill('PRIVATE-EVENT-SWITCH-CANARY')
  page.on('dialog', dialog => dialog.accept())
  await page.goto(`/events/${secondID}/communications/deliveries/${deliveryIDs.accepted}`)
  await expect(page.getByLabel('Private operator reason')).toHaveCount(0)
  expect(await page.evaluate(() => JSON.stringify(localStorage) + JSON.stringify(sessionStorage))).not.toContain('PRIVATE-')
  expect(fixture.state.posts).toHaveLength(0)
})

test('expired session clears private recovery text and does not replay an uncertain command', async ({ page }) => {
  const fixture = await deliveryFixture(page); fixture.state.drop = true
  await page.goto(detailPath('ambiguous'))
  await page.getByLabel('Recovery action').selectOption('resolve_no_resend')
  await page.getByLabel('Private operator reason').fill('PRIVATE-EXPIRED-DECISION-CANARY')
  await page.getByRole('button', { name: 'Record explicit decision' }).click()
  await expect(page.getByText('Outcome unknown — no automatic replay', { exact: true })).toBeVisible()
  fixture.state.expired = true
  await page.evaluate(() => window.dispatchEvent(new Event('visibilitychange')))
  await expect(page.getByLabel('Private operator reason')).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Retry the same recovery decision' })).toHaveCount(0)
  expect(fixture.state.posts).toHaveLength(1)
  expect(await page.evaluate(() => JSON.stringify(localStorage) + JSON.stringify(sessionStorage))).not.toContain('PRIVATE-EXPIRED-DECISION-CANARY')
})

test('current pause removes retry permission while preserving unsent reason and allows safe cancellation', async ({ page }) => {
  await page.clock.install()
  const fixture = await deliveryFixture(page)
  await page.goto(detailPath('retryable'))
  await page.getByLabel('Recovery action').selectOption('retry')
  await page.getByLabel('Private operator reason').fill('PRIVATE-PAUSED-REASON')
  fixture.state.paused = true
  await page.clock.fastForward(11_000)
  await expect(page.getByRole('button', { name: 'Record explicit decision' })).toBeDisabled()
  await expect(page.getByLabel('Private operator reason')).toHaveValue('PRIVATE-PAUSED-REASON')
  expect(fixture.state.posts).toHaveLength(0)
  await page.getByLabel('Recovery action').selectOption('cancel')
  await page.getByRole('button', { name: 'Record explicit decision' }).click()
  await expect(page.getByText('Pending work was stopped.', { exact: false })).toBeVisible()
  expect(fixture.state.posts[0].body.action).toBe('cancel')
})

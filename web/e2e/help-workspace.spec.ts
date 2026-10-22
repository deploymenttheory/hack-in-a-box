import { expect, test } from '@playwright/test'
import { firstID, secondID } from './fixtures'
import { helper, secondHelper, organiser, helpState, mockHelp, requestID, otherRequest } from './help-workspace-fixtures'
const queue = `/events/${firstID}/help`, detail = queue + '/' + requestID
async function describe(page: import('@playwright/test').Page, text: string) { await page.getByLabel('What do you need help with?').fill(text); await page.getByLabel('Location (or an approved meeting route required)', { exact: true }).fill('Synthetic team desk') }
async function activate(page: import('@playwright/test').Page, name: string) { await page.getByRole('button', { name, exact: true }).focus(); await page.keyboard.press('Enter') }
const saveState = (page: import('@playwright/test').Page) => page.getByRole('status', { name: 'Help request save state' })

test('team keyboard/mobile request is private, unsent until confirmed, cancellable and reopenable', async ({ page }, info) => {
  const state = helpState(); await mockHelp(page, state); await page.goto(queue)
  await describe(page, 'SYNTHETIC PRIVATE NEW REQUEST <script>')
  await expect(saveState(page)).toContainText('Nothing in this form is saved yet')
  await page.getByRole('button', { name: 'Submit help request', exact: true }).focus(); await page.keyboard.press('Enter')
  await expect(saveState(page)).toContainText('saved on the server')
  await page.getByRole('link', { name: 'Open saved help request' }).click()
  await expect(page.getByRole('heading', { name: 'Waiting', exact: true })).toBeVisible()
  await expect(page.getByLabel('Transfer to a currently eligible helper')).toHaveCount(0)
  await activate(page, 'Cancel request')
  await expect(page.getByRole('heading', { name: 'Cancelled', exact: true })).toBeVisible()
  await activate(page, 'Reopen request')
  await expect(page.getByRole('heading', { name: 'Reopened · Waiting', exact: true })).toBeVisible()
  await expect(page.getByText('reopen → waiting', { exact: false })).toBeVisible()
  expect(state.commands.map(c => c.action)).toEqual(['create', 'cancel', 'reopen'])
  expect(await page.evaluate(() => Object.values(sessionStorage).join('').includes('SYNTHETIC PRIVATE NEW REQUEST'))).toBe(false)
  await page.screenshot({ path: info.outputPath('team-help-current-history.png'), fullPage: true })
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
})

test('helper mobile claim, release, transfer and resolution use only current native targets', async ({ page, context }, info) => {
  const state = helpState(); await mockHelp(page, state, helper); await page.goto(detail)
  await activate(page, 'Claim request')
  await expect(page.getByText('Current helper: Synthetic Helper', { exact: true })).toBeVisible()
  await activate(page, 'Release to waiting'); await expect(page.getByRole('heading', { name: 'Waiting', exact: true })).toBeVisible()
  await activate(page, 'Claim request')
  await page.getByLabel('Transfer to a currently eligible helper').selectOption(secondHelper.id)
  await page.screenshot({ path: info.outputPath('mobile-helper-transfer.png'), fullPage: true })
  await page.getByRole('button', { name: 'Transfer request', exact: true }).focus(); await page.keyboard.press('Enter')
  await expect(page.getByText('Current helper: Second Helper', { exact: true })).toBeVisible()
  const peer = await context.newPage(); await mockHelp(peer, state, secondHelper); await peer.goto(detail)
  await activate(peer, 'Resolve request')
  await expect(peer.getByRole('heading', { name: 'Resolved', exact: true })).toBeVisible()
  expect(state.commands.map(c => c.action)).toEqual(['claim', 'release', 'claim', 'transfer', 'resolve'])
  expect(await page.evaluate(() => 'badHelp' in window)).toBe(false)
})

test('lost claim race refreshes authoritative ownership without clearing unrelated local work', async ({ page, browser }) => {
  const state = helpState(); await mockHelp(page, state, helper); const peerContext = await browser.newContext(); const peer = await peerContext.newPage(); await mockHelp(peer, state, secondHelper)
  await page.goto(detail); await peer.goto(detail)
  await expect(page.getByRole('button', { name: 'Claim request', exact: true })).toBeVisible(); await expect(peer.getByRole('button', { name: 'Claim request', exact: true })).toBeVisible()
  await peer.evaluate(({ actor, event }) => { const target = { personID: actor, sessionScope: '88888888-8888-4888-8888-888888888888', eventID: event, teamID: '11111111-1111-4111-8111-111111111111', noteID: '99999999-9999-4999-8999-999999999999' }; const createdAt = Date.now(); sessionStorage.setItem('hackbox:mentor-text:v1:' + Object.values(target).join(':'), JSON.stringify({ version: 1, target, createdAt, expiresAt: createdAt + 30 * 60 * 1000, baseRevision: 1, noteVersion: 0, text: { category: 'technical', evidence: 'UNRELATED DRAFT', outcome: '' } })) }, { actor: secondHelper.id, event: firstID })
  await activate(page, 'Claim request')
  await activate(peer, 'Claim request')
  await expect(peer.getByRole('status', { name: 'Help action state' })).toContainText('latest owner and state are shown')
  await expect(peer.getByText('Current helper: Synthetic Helper', { exact: true })).toBeVisible()
  await expect(peer.getByRole('button', { name: 'Claim request', exact: true })).toHaveCount(0)
  expect(await peer.evaluate(() => Object.keys(sessionStorage).filter(k => k.startsWith('hackbox:mentor-text:')).map(k => sessionStorage.getItem(k)).join(''))).toContain('UNRELATED DRAFT')
  await peerContext.close()
})

test('organiser filters only authorised summaries and never copies private request text into queue', async ({ page }) => {
  const state = helpState(); await mockHelp(page, state, organiser); await page.goto(queue)
  await expect(page.getByRole('heading', { name: 'Organiser help queue' })).toBeVisible()
  await expect(page.getByText('Showing 2 of 2 currently authorised requests.')).toBeVisible()
  await page.getByLabel('Category', { exact: true }).selectOption('logistics')
  await expect(page.getByText('Showing 1 of 2 currently authorised requests.')).toBeVisible()
  await expect(page.getByRole('link', { name: 'Open request · ' + otherRequest.slice(0, 8) })).toBeVisible()
  await expect(page.getByText('PRIVATE HELP', { exact: false })).toHaveCount(0)
})

test('accepted create with lost response is confirmed by exact identity without automatic duplicate requests', async ({ page }) => {
  const state = helpState(); state.lostCreate = true; await mockHelp(page, state); await page.goto(queue)
  await describe(page, 'UNCERTAIN CREATE PRIVATE BODY')
  await page.getByRole('button', { name: 'Submit help request', exact: true }).click()
  await expect(saveState(page)).toContainText('creation outcome is unconfirmed')
  expect(state.records).toHaveLength(3); expect(state.commands).toHaveLength(1)
  await page.getByRole('button', { name: 'Check whether request was created' }).click()
  await expect(saveState(page)).toContainText('saved on the server'); expect(state.commands).toHaveLength(1)
})

test('interrupted uncommitted create preserves exact intent and deliberate retry uses its original idempotency key', async ({ page }) => {
  const state = helpState(); state.failCreate = true; await mockHelp(page, state); await page.goto(queue)
  await describe(page, 'STILL UNSENT')
  await page.getByRole('button', { name: 'Submit help request', exact: true }).click(); await expect(saveState(page)).toContainText('unconfirmed')
  await page.getByRole('button', { name: 'Check whether request was created' }).click(); await expect(saveState(page)).toContainText('not proof it was never created')
  state.failCreate = false
  await page.getByRole('button', { name: 'Retry the same request deliberately' }).click(); await expect(saveState(page)).toContainText('saved on the server')
  expect(state.commands).toHaveLength(2); expect(state.commands[0].key).toBe(state.commands[1].key); expect(state.commands[0].id).toBe(state.commands[1].id)
})

test('setup remains usable before live start while competition-output creation is rejected without a saved claim', async ({ page }) => {
  const state = helpState(); state.open = false; await mockHelp(page, state); await page.goto(queue)
  await page.getByLabel('Support category').selectOption('competition_output'); await describe(page, 'PRE-LIVE OUTPUT')
  await page.getByRole('button', { name: 'Submit help request', exact: true }).click()
  await expect(saveState(page)).toContainText('server rejected this request'); expect(state.records).toHaveLength(2)
  await page.getByLabel('Support category').selectOption('setup'); await page.getByRole('button', { name: 'Submit help request', exact: true }).click()
  await expect(saveState(page)).toContainText('saved on the server'); expect(state.records).toHaveLength(3)
})

test('another team details remain hidden and active access loss erases request prose and unsent form', async ({ page }) => {
  const state = helpState(); await mockHelp(page, state); await page.goto(queue + '/' + otherRequest)
  await expect(page.getByRole('alert')).toBeVisible(); await expect(page.getByText('PRIVATE HELP', { exact: false })).toHaveCount(0)
  await page.clock.install(); await page.goto(queue); await page.getByLabel('Location (or an approved meeting route required)', { exact: true }).fill('Synthetic team desk'); await describe(page, 'PRIVATE UNSENT FORM')
  state.denied = true; await page.clock.fastForward(16_000)
  await expect(page.getByLabel('What do you need help with?')).toHaveCount(0)
  state.denied = false; await page.clock.fastForward(16_000)
  await expect(page.getByLabel('What do you need help with?')).toHaveValue('')
})

test('account, event and expired session changes clear private in-memory request state', async ({ page }) => {
  const state = helpState(); await mockHelp(page, state); await page.goto(queue); await page.getByLabel('Location (or an approved meeting route required)', { exact: true }).fill('Synthetic team desk'); await describe(page, 'OLD EVENT PRIVATE FORM')
  await page.goto(`/events/${secondID}/help`); await expect(page.getByRole('alert')).toBeVisible(); await expect(page.getByText('OLD EVENT PRIVATE FORM', { exact: false })).toHaveCount(0)
  await page.goto(queue); await expect(page.getByLabel('What do you need help with?')).toHaveValue('')
  await describe(page, 'OLD ACCOUNT PRIVATE FORM'); state.switched = true; await page.reload()
  await expect(page.getByRole('button', { name: 'Different account', exact: true })).toBeVisible(); await expect(page.getByLabel('What do you need help with?')).toHaveCount(0)
  state.switched = false; state.expired = true; await page.reload(); await expect(page.getByRole('heading', { name: 'Sign in to continue.', exact: true })).toBeVisible()
  await expect(page.getByText('OLD ACCOUNT PRIVATE FORM', { exact: false })).toHaveCount(0)
})

test('location or an approved meeting route is explained and validated before keyboard submission', async ({ page }) => {
  const state = helpState(); await mockHelp(page, state); await page.goto(queue)
  await page.getByLabel('What do you need help with?').fill('Need setup support')
  await page.getByRole('button', { name: 'Submit help request', exact: true }).focus(); await page.keyboard.press('Enter')
  await expect(page.getByRole('alert')).toContainText('Add a location or an approved HTTPS meeting route')
  await expect(page.getByLabel('Location (or an approved meeting route required)', { exact: true })).toBeFocused(); expect(state.commands).toHaveLength(0)
  await page.getByLabel('Location (or an approved meeting route required)', { exact: true }).fill('Synthetic desk')
  await page.getByRole('button', { name: 'Submit help request', exact: true }).click()
  await expect(saveState(page)).toContainText('saved on the server'); expect(state.commands).toHaveLength(1)
})
test('cached two-event same-request UUID navigation clears old uncertain action and transfer intent', async ({ page }) => {
  const state = helpState(); state.allowOtherEvent = true; await mockHelp(page, state, helper)
  const other = `/events/${secondID}/help/${requestID}`
  await page.goto(other); await expect(page.getByText('SECOND EVENT PROBLEM', { exact: true })).toBeVisible()
  await page.evaluate(url => { history.pushState({}, '', url); window.dispatchEvent(new PopStateEvent('popstate')) }, detail)
  await expect(page.getByText('PRIVATE HELP', { exact: false })).toBeVisible()
  await activate(page, 'Claim request')
  await page.getByLabel('Transfer to a currently eligible helper').selectOption(secondHelper.id); state.lostAction = true
  await page.getByRole('button', { name: 'Transfer request', exact: true }).click()
  await expect(page.getByRole('status', { name: 'Help action state' })).toContainText('outcome is unconfirmed')
  expect(state.commands).toHaveLength(2); state.lostAction = false
  await page.evaluate(url => { history.pushState({}, '', url); window.dispatchEvent(new PopStateEvent('popstate')) }, other)
  await expect(page.getByText('SECOND EVENT PROBLEM', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Retry the same action deliberately' })).toHaveCount(0)
  await expect(page.getByRole('status', { name: 'Help action state' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Claim request', exact: true })).toBeEnabled(); expect(state.commands).toHaveLength(2)
})

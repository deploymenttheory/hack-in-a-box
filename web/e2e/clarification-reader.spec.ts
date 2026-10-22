import { expect, test, type Page } from '@playwright/test'
import { mockWorkspace, firstID, person } from './fixtures'
import type { components } from '../src/api/schema.gen'

const answerID = '8d4158fd-3ef9-42eb-9c0c-a064f34db9cd', notificationID = '019c25e0-15bd-7000-8000-000000000011'
const answerPath = `/events/${firstID}/clarifications/${answerID}`
type AnswerPage = components['schemas']['ClarificationPage']
async function prepare(page: Page) {
  await mockWorkspace(page)
  await page.route('**/api/v1/session', route => route.fulfill({ json: { person: { ...person, platform_admin: false }, csrf_token: 'synthetic-csrf-token-32-characters', can_create_events: false } }))
  let allowed = true
  const current: AnswerPage['answer'] = { id: answerID, version: 2, document: { title: 'Reviewed common interpretation', body: 'SYNTHETIC-COMMON-CORRECTION\n\n<script>window.CLARIFICATION_PWNED=true</script>', kind: 'clarification' }, source: { challenge_id: '64bb2e85-04a2-4715-aab0-8f50030d804a', challenge_version: 1, rubric: { id: '122700b0-acf4-4e13-ae54-d339af614225', version: 1, digest: 'a'.repeat(64) } }, author_name: 'Synthetic organiser', authored_at: '2027-06-19T08:00:00Z', approved_at: '2027-06-19T08:30:00Z', published_at: '2027-06-19T09:00:00Z', current: true, source_current: false }
  current.version = 3
  const detail: AnswerPage = { answer: current, history: [current, { ...current, version: 2, current: false, document: { ...current.document, body: 'SYNTHETIC-SUPERSEDED-ANSWER' } }], archived: false, history_has_more: true }
  await page.route(`**/api/v1/events/${firstID}/clarifications/${answerID}`, route => allowed ? route.fulfill({ json: detail }) : route.fulfill({ status: 404, json: { error: { code: 'not_found', message: 'The common answer is not currently available.', request_id: 'synthetic' } } }))
  await page.route(`**/api/v1/events/${firstID}/clarifications/${answerID}/history**`, route => {
    expect(new URL(route.request().url()).searchParams.get('cursor')).toBe('2')
    return route.fulfill({ json: { items: [{ ...current, version: 1, current: false, document: { ...current.document, body: 'SYNTHETIC-EARLIER-PUBLISHED-REVISION' } }], next_cursor: null } })
  })
  const item = { id: notificationID, event_id: firstID, event_name: 'Synthetic event', label: 'Reviewed common clarification', created_at: current.published_at, read_at: null }
  await page.route('**/api/v1/inbox**', route => {
    if (new URL(route.request().url()).pathname === '/api/v1/inbox') return route.fulfill({ json: { items: allowed ? [item] : [], next_cursor: null, unread: allowed ? 1 : 0 } })
    return route.fulfill({ json: { item, subject: 'Reviewed common clarification', body: '', format: 'text', destination: { kind: 'clarification', resource_id: answerID } } })
  })
  return { revoke: () => { allowed = false } }
}

test('safe native inbox reference opens canonical answer and explicit published history', async ({ page }) => {
  await prepare(page)
  await page.goto('/inbox')
  await page.getByRole('button', { name: 'Reviewed common clarification Unread' }).click()
  const link = page.getByRole('link', { name: 'Open reviewed common answer' })
  await link.focus(); await page.keyboard.press('Enter')
  await expect(page).toHaveURL(answerPath)
  await expect(page.getByRole('heading', { name: 'Reviewed common interpretation' })).toBeVisible()
  await expect(page.getByText('Published', { exact: false }).first()).toContainText('Synthetic organiser')
  await expect(page.getByRole('status')).toContainText('earlier published challenge or rubric')
  await expect(page.getByText('SYNTHETIC-SUPERSEDED-ANSWER')).not.toBeVisible()
  await page.getByText('Published correction history', { exact: true }).click()
  await expect(page.getByText('SYNTHETIC-SUPERSEDED-ANSWER')).toBeVisible()
  await page.getByRole('button', { name: 'More published revisions' }).click()
  await expect(page.getByText('SYNTHETIC-EARLIER-PUBLISHED-REVISION')).toBeVisible()
  await expect(page.getByRole('button', { name: 'More published revisions' })).toHaveCount(0)
  expect(await page.evaluate(() => (window as unknown as Record<string, unknown>).CLARIFICATION_PWNED)).toBeUndefined()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  expect(await page.evaluate(() => JSON.stringify(localStorage) + JSON.stringify(sessionStorage))).not.toContain('SYNTHETIC-COMMON')
})

test('copied canonical link fails closed before release and after current access revocation', async ({ page }) => {
  const state = await prepare(page)
  state.revoke()
  await page.goto(answerPath)
  await expect(page.getByRole('alert')).toContainText('We could not complete the request.')
  await expect(page.getByText('SYNTHETIC-COMMON-CORRECTION')).toHaveCount(0)
  await expect(page.getByText('Published correction history', { exact: true })).toHaveCount(0)
})

test('current-authority poll clears answer and history instead of retaining stale content', async ({ page }) => {
  const state = await prepare(page)
  await page.clock.install()
  await page.goto(answerPath)
  await expect(page.getByRole('heading', { name: 'Reviewed common interpretation' })).toBeVisible()
  state.revoke()
  await page.clock.fastForward(16_000)
  await expect(page.getByRole('alert')).toContainText('We could not complete the request.')
  await expect(page.getByRole('heading', { name: 'Reviewed common interpretation' })).toHaveCount(0)
  await expect(page.getByText('SYNTHETIC-SUPERSEDED-ANSWER')).toHaveCount(0)
})

import { expect, test, type Page } from '@playwright/test'
import { mockWorkspace, firstID, person } from './fixtures'
import type { components } from '../src/api/schema.gen'
const surveyID = '8d4158fd-3ef9-42eb-9c0c-a064f34db9cd', questionID = '122700b0-acf4-4e13-ae54-d339af614225', notificationID = '019c25e0-15bd-7000-8000-000000000011'
const path = `/events/${firstID}/surveys/${surveyID}`, secret = 'PRIVATE-SURVEY-SAVED-RESPONSE <script>window.SURVEY_PWNED=true</script>'
async function prepare(page: Page) {
  await mockWorkspace(page)
  await page.route('**/api/v1/session', route => route.fulfill({ json: { person: { ...person, platform_admin: false }, csrf_token: 'synthetic-csrf-token-32-characters', can_create_events: false, draft_scope: '48e4056e-ce21-4b45-8fc1-6dc7dde63572' } }))
  let allowed = true, open = true
  const survey: components['schemas']['SurveyParticipant'] = { id: surveyID, version: 1, timezone: 'Pacific/Auckland', form: { id: surveyID, name: 'Learning after the hack', audience: 'attendee', fields: [{ id: questionID, label: 'Private explanation', type: 'text', required: false, help: 'Only you and current organisers can read this response.', choices: [] }] }, opens_at: '2027-06-21T09:00:00Z', closes_at: '2027-06-28T09:00:00Z', response_visibility: 'Responses are identifiable to current event organisers, not anonymous. Other participants cannot read your response.', measures: [], can_respond: true, response: { revision: 2, form_version: 1, answers: { [questionID]: secret }, submitted_at: '2027-06-22T09:00:00Z' } }
  await page.route(`**/api/v1/events/${firstID}/surveys/${surveyID}`, route => allowed ? route.fulfill({ json: { ...survey, can_respond: open } }) : route.fulfill({ status: 404, json: { error: { code: 'not_found', message: 'Survey unavailable.', request_id: 'synthetic' } } }))
  const item = { id: notificationID, event_id: firstID, event_name: 'Synthetic event', label: 'Event announcement', created_at: survey.opens_at, read_at: null }
  await page.route('**/api/v1/inbox**', route => new URL(route.request().url()).pathname === '/api/v1/inbox' ? route.fulfill({ json: { items: allowed ? [item] : [], next_cursor: null, unread: allowed ? 1 : 0 } }) : route.fulfill({ json: { item, subject: 'Reviewed survey invitation', body: 'Approved invitation body remains unchanged.', format: 'text', destination: { kind: 'survey', resource_id: surveyID } } }))
  return { revoke: () => { allowed = false }, close: () => { open = false } }
}
test('approved invitation opens own private reader by keyboard', async ({ page }) => {
  await prepare(page); await page.goto('/inbox'); await page.getByRole('button', { name: 'Open Event announcement', exact: true }).click()
  await expect(page.getByText('Approved invitation body remains unchanged.')).toBeVisible()
  const link = page.getByRole('link', { name: 'Open your survey' }); await link.focus(); await page.keyboard.press('Enter')
  await expect(page).toHaveURL(path); await expect(page.getByRole('heading', { name: 'Learning after the hack' })).toBeVisible()
  await expect(page.getByRole('note')).toContainText('not anonymous'); await expect(page.getByText(secret, { exact: true })).toBeVisible()
  await expect(page.getByText('Your saved response: revision 2', { exact: false })).toBeVisible()
  expect(await page.evaluate(() => (window as unknown as Record<string, unknown>).SURVEY_PWNED)).toBeUndefined()
  expect(await page.evaluate(() => JSON.stringify(localStorage) + JSON.stringify(sessionStorage))).not.toContain(secret)
  await expect(page.getByText('Opens 21 Jun 2027, 21:00 · Closes 28 Jun 2027, 21:00 · Pacific/Auckland', { exact: true })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(page.viewportSize()!.width)
})
test('close retains own history while stopping response capability', async ({ page }) => {
  const state = await prepare(page); await page.clock.install(); await page.goto(path)
  await expect(page.getByText('This survey is open.', { exact: false })).toBeVisible(); state.close(); await page.clock.fastForward(16_000)
  await expect(page.getByText('Response editing is currently closed or unavailable.')).toBeVisible(); await expect(page.getByText(secret, { exact: true })).toBeVisible()
})
test('current native denial clears private answer on poll', async ({ page }) => {
  const state = await prepare(page); await page.clock.install(); await page.goto(path)
  await expect(page.getByText(secret, { exact: true })).toBeVisible(); state.revoke(); await page.clock.fastForward(16_000)
  await expect(page.getByRole('alert')).toBeVisible(); await expect(page.getByText(secret, { exact: true })).toHaveCount(0)
  await expect(page.getByRole('heading', { name: 'Private explanation' })).toHaveCount(0)
})
test('copied private link fails closed without invitation', async ({ page }) => {
  const state = await prepare(page); state.revoke(); await page.goto(path)
  await expect(page.getByRole('alert')).toBeVisible(); await expect(page.getByText(secret, { exact: true })).toHaveCount(0)
  await expect(page.getByRole('heading', { name: 'Learning after the hack' })).toHaveCount(0)
})

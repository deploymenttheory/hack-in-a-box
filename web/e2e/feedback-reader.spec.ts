import { expect, test, type Page } from '@playwright/test'
import { firstID, mockWorkspace, person } from './fixtures'
import type { components } from '../src/api/schema.gen'
const feedbackID = '33333333-3333-4333-8333-333333333333', teamID = '44444444-4444-4444-8444-444444444444', privateSource = '55555555-5555-4555-8555-555555555555', actionID = '66666666-6666-4666-8666-666666666666'
type Document = components['schemas']['SharedFeedbackDocument']
async function fixture(page: Page) {
 await mockWorkspace(page)
 const document: Document = { id: feedbackID, team_id: teamID, version: 3, category: 'setup', title: 'Reviewed replacement kit guidance', body: 'Shared advice <script>window.feedbackLeak=true</script>\nCheck the reviewed checklist.', state: 'published', sources: [], help_request_id: null, author_id: person.id, reviewer_id: person.id, reviewed_digest: '', published_at: '2027-06-20T09:00:00Z', changed_at: '2027-06-20T09:00:00Z' }
 const state = { denied: false, writes: 0, followupReads: 0, feedbackReads: 0 }
 await page.route(`**/api/v1/events/${firstID}/feedback**`, async route => {
  const request = route.request(), url = new URL(request.url()), error = () => route.fulfill({ status: 404, json: { error: { code: 'not_found', request_id: 'synthetic' } } })
  if (request.method() !== 'GET') { state.writes++; throw new Error('Read-only destination attempted a mutation') }
  if (state.denied) return error()
  if (url.pathname.includes('/actions')) return route.fulfill({ json: { items: [{ command_version: null, action: { id: actionID, feedback_id: feedbackID, version: 1, title: 'Reviewed kit check', body: 'Confirm replacement cables are available', owner_id: person.id, due_at: '2027-06-20T10:00:00Z', status: 'open', cycle: 1, changed_by: person.id, changed_at: '2027-06-20T09:00:00Z' }, can_edit: false, can_update_status: true, owner_available: true, overdue: false }], next_cursor: null } })
  if (url.pathname.endsWith('/history')) return route.fulfill({ json: { items: [{ ...document, version: 4, state: 'draft', body: 'PRIVATE UNRELEASED HISTORY', sources: [{ kind: 'question', id: privateSource, version: 1 }] }, document], next_cursor: null } })
  if (url.pathname.includes('/feedback-followups/')) state.followupReads++; else state.feedbackReads++
  return route.fulfill({ json: { command_version: null, current_version: 4, published: document, draft: { ...document, version: 4, state: 'draft', body: 'PRIVATE STAFF DRAFT NOT PART OF RELEASED READER', sources: [{ kind: 'question', id: privateSource, version: 1 }] }, can_manage: false, can_acknowledge: true, acknowledgements: [], escalations: [] } })
 })
 return state
}
test('actual shared-feedback destination reads only released prose/actions with keyboard publication history', async ({ page }, info) => {
 const state = await fixture(page); await page.goto(`/events/${firstID}/feedback/${feedbackID}`)
 await expect(page.getByRole('heading', { name: 'Reviewed team feedback', exact: true })).toBeVisible(); await expect(page.getByText('Reviewed kit check', { exact: true })).toBeVisible(); await expect(page.getByText('Shared advice <script>', { exact: false })).toBeVisible()
 await expect(page.locator('script').filter({ hasText: 'feedbackLeak' })).toHaveCount(0); await expect(page.locator('body')).not.toContainText('PRIVATE STAFF DRAFT'); await expect(page.locator('body')).not.toContainText(privateSource)
 await page.getByRole('button', { name: 'Show feedback publication history' }).focus(); await page.keyboard.press('Enter'); await expect(page.getByRole('heading', { name: 'Reviewed replacement kit guidance · Published version 3' })).toBeVisible(); await expect(page.locator('body')).not.toContainText('PRIVATE UNRELEASED HISTORY')
 expect(state.writes).toBe(0); expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true); await page.screenshot({ path: info.outputPath('reviewed-team-feedback.png'), fullPage: true })
})
test('feedback and fixed-recipient followup with equal UUIDs use distinct native resource requests', async ({ page }) => {
 const state = await fixture(page), first = '019c25e0-15bd-7000-8000-000000000001', second = '019c25e0-15bd-7000-8000-000000000002'
 const item = (id: string) => ({ id, event_id: firstID, event_name: 'Synthetic feedback event', label: id === first ? 'Reviewed team feedback' : 'Support-action follow-up', created_at: '2027-06-20T09:00:00Z', read_at: null })
 await page.route('**/api/v1/inbox**', route => {
  const id = new URL(route.request().url()).pathname.split('/')[4]
  if (!id) return route.fulfill({ json: { items: [item(first), item(second)], next_cursor: null, unread: 2 } })
  return route.fulfill({ json: { item: item(id), subject: 'Safe native notice', body: '', format: 'text', destination: { kind: id === first ? 'feedback' : 'feedback_followup', resource_id: feedbackID } } })
 })
 await page.goto('/inbox'); await page.getByRole('button', { name: 'Open Reviewed team feedback', exact: true }).click(); await page.getByRole('link', { name: 'Open reviewed team feedback', exact: true }).click(); await expect(page.getByRole('heading', { name: 'Reviewed team feedback', exact: true })).toBeVisible()
 await page.getByRole('link', { name: /^Inbox/ }).first().click(); await page.getByRole('button', { name: 'Open Support-action follow-up', exact: true }).click(); await page.getByRole('link', { name: 'Open support-action follow-up', exact: true }).click(); await expect(page.getByRole('heading', { name: 'Support-action follow-up', exact: true })).toBeVisible(); await expect.poll(() => state.followupReads).toBeGreaterThan(0); expect(state.feedbackReads).toBeGreaterThan(0); expect(state.writes).toBe(0)
})
test('currently revoked resource hides released text and support actions without tab focus', async ({ page }) => {
 const state = await fixture(page); await page.goto(`/events/${firstID}/feedback/${feedbackID}`); await expect(page.getByRole('heading', { name: 'Reviewed team feedback', exact: true })).toBeVisible(); state.denied = true
 await expect(page.getByRole('heading', { name: 'Reviewed team feedback', exact: true })).toHaveCount(0, { timeout: 20_000 }); await expect(page.locator('body')).not.toContainText('Confirm replacement cables'); await expect(page.locator('body')).not.toContainText('Check the reviewed checklist'); expect(state.writes).toBe(0)
})

import { test, expect, type Page } from '@playwright/test'
import type { components } from '../src/api/schema.gen'
import { firstID, mockWorkspace, person } from './fixtures'
const teamID = '3f8af336-a168-47ed-b67c-cba51d530413', path = `/events/${firstID}/teams/${teamID}/feedback-report`
async function prepare(page: Page) {
 const workspace = await mockWorkspace(page); workspace.events[0].stage = 'completed'
 const state = { authorised: true, temporary: false, binding: 'a'.repeat(64), writes: 0, hold: undefined as Promise<void> | undefined }
 await page.route(`**/api/v1/events/${firstID}/teams`, route => route.fulfill({ json: { revision: 1, can_manage: true, can_create: false, frozen: true, freeze_at: workspace.events[0].configuration.team_freeze, teams: [{ id: teamID, name: 'Synthetic Aurora', state: 'published', captain: null, published_captain: null, invitation_code: null, frozen: true, draft: [], published: [{ id: person.id, name: person.display_name }] }], people: [], requests: [] } satisfies components['schemas']['TeamPage'] }))
 await page.route(`**/api/v1/events/${firstID}/teams/${teamID}/feedback-report`, async route => {
  if (route.request().method() === 'POST') { state.writes++; return route.fulfill({ status: 408, json: { error: { code: 'unavailable', message: 'Synthetic transport fault', request_id: 'synthetic' } } }) }
  if (state.hold) await state.hold
  if (!state.authorised || state.temporary) return route.fulfill({ status: state.authorised ? 503 : 403, json: { error: { code: state.authorised ? 'unavailable' : 'forbidden', message: 'Synthetic current read fault', request_id: 'synthetic' } } })
  return route.fulfill({ json: { version: 0, latest: null, receipt: null, current_source: { binding: state.binding, team_id: teamID, scores_available: false, final_scores: null } } satisfies components['schemas']['TeamFeedbackPage'] })
 })
 await page.goto(path); await expect(page.getByRole('heading', { name: 'Team feedback reports' })).toBeVisible(); await expect(page.getByLabel('Report title', { exact: true })).toBeVisible()
 return state
}
async function fill(page: Page) {
 await page.getByLabel('Report title', { exact: true }).fill('Synthetic private title')
 await page.getByLabel('Team feedback', { exact: true }).fill('PRIVATE-UNSENT-REPORT-CANARY')
 await page.getByLabel('Reason for this version or release', { exact: true }).fill('Deliberate synthetic reason')
}
test('published-team prose needs no score confirmation and changed current source prevents a reviewed write', async ({ page }) => {
 const state = await prepare(page); await fill(page); const submit = page.getByRole('button', { name: 'Review draft save', exact: true }); await submit.click(); const review = page.getByRole('alertdialog', { name: 'Review team feedback draft', exact: true }); await expect(review).toContainText('PRIVATE-UNSENT-REPORT-CANARY'); state.binding = 'b'.repeat(64); await review.getByRole('button', { name: 'Save reviewed draft', exact: true }).click(); await expect(review).toContainText('published-team report source changed'); expect(state.writes).toBe(0); await review.getByRole('button', { name: 'Cancel', exact: true }).click(); await expect(submit).toBeFocused(); await expect(page.getByLabel('Team feedback', { exact: true })).toHaveValue('PRIVATE-UNSENT-REPORT-CANARY')
})
test('temporary source failure retains unsent wording but definite denial clears private preview and later restoration starts fresh', async ({ page }) => {
 const state = await prepare(page); await fill(page); state.temporary = true; await page.getByRole('button', { name: 'Read current report', exact: true }).click(); await expect(page.getByLabel('Team feedback', { exact: true })).toHaveValue('PRIVATE-UNSENT-REPORT-CANARY'); await expect(page.getByRole('button', { name: 'Review draft save', exact: true })).toBeDisabled(); state.temporary = false; await page.getByRole('button', { name: 'Try again', exact: true }).click(); await expect(page.getByRole('button', { name: 'Review draft save', exact: true })).toBeEnabled(); await page.getByRole('button', { name: 'Preview team-facing draft', exact: true }).click(); state.authorised = false; await page.getByRole('button', { name: 'Read current report', exact: true }).click(); await expect(page.getByText('Current report access is unavailable.', { exact: false })).toBeVisible(); await expect(page.getByText('PRIVATE-UNSENT-REPORT-CANARY', { exact: true })).toHaveCount(0); await expect(page.getByLabel('Team feedback', { exact: true })).toHaveCount(0); state.authorised = true; await page.getByRole('button', { name: 'Check current report access', exact: true }).click(); await expect(page.getByLabel('Team feedback', { exact: true })).toHaveValue(''); expect(state.writes).toBe(0)
})
test('session ending during held current preflight fences every later report write', async ({ page }) => {
 const state = await prepare(page); await fill(page); await page.getByRole('button', { name: 'Review draft save', exact: true }).click(); let release!: () => void; state.hold = new Promise<void>(resolve => { release = resolve })
 try { await page.getByRole('alertdialog').getByRole('button', { name: 'Save reviewed draft', exact: true }).click(); await page.evaluate(() => window.dispatchEvent(new Event('session-ending'))); release(); await expect(page.getByLabel('Team feedback', { exact: true })).toHaveCount(0); expect(state.writes).toBe(0) } finally { release() }
})

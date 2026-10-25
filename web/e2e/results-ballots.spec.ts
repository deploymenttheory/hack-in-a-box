import { test, expect, type Page } from '@playwright/test'
import type { components } from '../src/api/schema.gen'
import { firstID, mockWorkspace, person } from './fixtures'
import { criteria, judgeFixture, otherJudge, runID, teamA, teamB } from './judge-fixture'
async function prepare(page: Page) {
 const workspace = await mockWorkspace(page); workspace.events[0].stage = 'assessment'
 const briefs = judgeFixture().briefs
 for (const brief of Object.values(briefs)) brief.criteria = { ...brief.criteria, mode: 'all_teams' }
 const state = { authorised: true, writes: 0, hold: undefined as Promise<void> | undefined, ballots: { judge_run_id: runID, revision: workspace.events[0].revision, version: 1, source_binding: 'a'.repeat(64), receipt: null, ties: ['final', 'category'].map((kind, index) => ({ state: { tie: { id: (index ? 'c' : 'b').repeat(64), kind: kind as 'final' | 'category', category_id: index ? criteria.categories[0].id : null, team_ids: [teamA, teamB] }, required_judge_ids: [person.id, otherJudge], votes_received: 0, winner_id: null, blocker: 'required_judge_votes_missing' }, own_vote: null })) } satisfies components['schemas']['ResultBallotPage'] }
 await page.route(`**/api/v1/events/${firstID}/results/ballots`, async route => {
  if (route.request().method() === 'POST') { state.writes++; return route.fulfill({ status: 408, json: { error: { code: 'unavailable', message: 'Synthetic unexpected write', request_id: 'synthetic' } } }) }
  if (state.hold) await state.hold
  return route.fulfill(state.authorised ? { json: state.ballots } : { status: 403, json: { error: { code: 'forbidden', message: 'Synthetic current assignment denied', request_id: 'synthetic' } } })
 })
 await page.route(`**/api/v1/events/${firstID}/judging/${runID}/teams/*`, route => route.fulfill({ json: briefs[new URL(route.request().url()).pathname.split('/').at(-1)!] }))
 await page.goto(`/events/${firstID}/results/ballots`)
 await expect(page.getByRole('heading', { name: 'Review assigned ties', exact: true })).toBeVisible()
 return state
}
async function review(page: Page) {
 await page.getByRole('button', { name: 'Review Demo category', exact: true }).press('Enter')
 await page.getByLabel('Your selected winner', { exact: true }).selectOption(teamA)
 await page.getByLabel('Your observed evidence for Synthetic Aurora build', { exact: true }).fill('PRIVATE-CATEGORY-EVIDENCE')
 await page.getByLabel('Reason for your tie vote', { exact: true }).fill('Reviewed specific published category')
 await page.getByRole('button', { name: 'Review tie vote', exact: true }).press('Enter')
}
test('category selection is explicit and captured current source drift prevents every ballot write', async ({ page }) => {
 const state = await prepare(page)
 await expect(page.getByLabel('Your selected winner', { exact: true })).toHaveCount(0)
 await review(page)
 const dialog = page.getByRole('alertdialog', { name: 'Review your tie vote', exact: true })
 await expect(dialog).toContainText('Selected tie: Demo category')
 state.ballots.source_binding = 'd'.repeat(64)
 await dialog.getByRole('button', { name: 'Record reviewed tie vote', exact: true }).click()
 await expect(dialog).toContainText('Nothing was sent.')
 expect(state.writes).toBe(0)
 await dialog.getByRole('button', { name: 'Cancel', exact: true }).click()
 await expect(page.getByRole('button', { name: 'Review tie vote', exact: true })).toBeFocused()
 await expect(page.getByLabel('Your observed evidence for Synthetic Aurora build', { exact: true })).toHaveValue('PRIVATE-CATEGORY-EVIDENCE')
})
test('category held preflight cannot survive session teardown', async ({ page }) => {
 const state = await prepare(page); await review(page)
 let release!: () => void; state.hold = new Promise<void>(resolve => { release = resolve })
 try {
  await page.getByRole('alertdialog').getByRole('button', { name: 'Record reviewed tie vote', exact: true }).click()
  await page.evaluate(() => window.dispatchEvent(new Event('session-ending')))
  release()
  await expect(page.getByText('PRIVATE-CATEGORY-EVIDENCE', { exact: true })).toHaveCount(0)
  await expect(page.getByLabel('Your selected winner', { exact: true })).toHaveCount(0)
  expect(state.writes).toBe(0)
 } finally { release() }
})
test('current category assignment denial clears candidates and held reviews; restoration starts with explicit selection', async ({ page }) => {
 const state = await prepare(page); await review(page)
 await page.getByRole('alertdialog').getByRole('button', { name: 'Cancel', exact: true }).click()
 state.authorised = false
 await page.getByRole('button', { name: 'Read current tie ballots', exact: true }).click()
 await expect(page.getByRole('heading', { name: 'Tie ballots unavailable', exact: true })).toBeVisible()
 await expect(page.getByText('PRIVATE-CATEGORY-EVIDENCE', { exact: true })).toHaveCount(0)
 await expect(page.getByRole('region', { name: 'Assigned tie tasks', exact: true })).toHaveCount(0)
 state.authorised = true
 await page.getByRole('button', { name: 'Check current ballot access', exact: true }).click()
 await expect(page.getByRole('region', { name: 'Assigned tie tasks', exact: true })).toBeVisible()
 await expect(page.getByLabel('Your selected winner', { exact: true })).toHaveCount(0)
 expect(state.writes).toBe(0)
})

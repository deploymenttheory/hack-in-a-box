import { expect, type Page } from '@playwright/test'
import { firstID, mockWorkspace, person } from './fixtures'
import type { components } from '../src/api/schema.gen'
import type { Card, Checkpoint, Command } from '../src/scorecard-model'
const team = '33333333-3333-4333-8333-333333333333', cpID = '44444444-4444-4444-8444-444444444444', category = '55555555-5555-4555-8555-555555555555', nextCategory = '66666666-6666-4666-8666-666666666666'
function checkpoint(): Checkpoint { return { definition: { id: cpID, run_id: cpID, name: 'Technical checkpoint', opens_wall_time: '2020-01-01T09:00:00', closes_wall_time: '2030-01-01T17:00:00', dst_choice: '' }, version: 1, purpose: 'checkpoint', rubric: { id: cpID, version: 2, digest: 'published-safe-digest' }, criteria: { name: 'Published rubric', mode: 'mentor_qualification', required_judge_count: 1, finalist_count: 1, reserve_count: 0, prizes: [], side_quests: [], categories: [{ id: category, label: 'Technical merit', min_score: 0, max_score: 5, weight: 1, guidance: 'Inspect the demonstrated working output.', anchors: [{ score: 0, description: 'Explicitly observed no working output.' }, { score: 5, description: 'Strong demonstrated output.' }] }, { id: nextCategory, label: 'Teamwork', min_score: 0, max_score: 5, weight: 1, guidance: 'Observe collaboration; leave unobserved work pending.', anchors: [] }] }, timezone: 'Europe/London', opens_at: '2020-01-01T09:00:00Z', closes_at: '2030-01-01T17:00:00Z' } }
async function fixture(page: Page) {
 await mockWorkspace(page)
 const state = { cp: checkpoint(), revision: 10, cards: [] as Card[], history: [] as Card[], writes: [] as Command[], denied: false, readFailure: '' as '' | 'checkpoint' | 'card', denyCard: false, lost: false, failed: false, reject: 0, ownReads: 0, observationReads: [] as string[], observations: [] as components['schemas']['MentorObservation'][], secondPage: false, frozen: true }
 await page.route(`**/events/${firstID}/teams/${team}/**`, async route => {
  const request = route.request(), url = new URL(request.url()), path = url.pathname
  const error = (status: number) => route.fulfill({ status, json: { error: { code: status === 409 ? 'conflict' : 'forbidden', request_id: 'synthetic' } } })
  if (state.denied || (state.denyCard && path.includes('/scorecards'))) return error(403)
  if (request.method() === 'GET' && ((state.readFailure === 'checkpoint' && path.endsWith('/scorecard-checkpoints')) || (state.readFailure === 'card' && path.endsWith('/scorecards')))) return route.abort('failed')
  if (path.endsWith('/scorecard-checkpoints')) return route.fulfill({ json: { revision: state.revision, items: [state.cp], next_cursor: null } })
  if (path.includes('/scorecards/') && path.endsWith('/history')) { const before = Number(url.searchParams.get('cursor') ?? 9999); return route.fulfill({ json: { items: state.history.filter(c => c.reference.version < before).sort((a,b) => b.reference.version-a.reference.version).slice(0, Number(url.searchParams.get('limit') ?? 50)), next_cursor: null } }) }
  if (path.endsWith('/scorecards')) {
   if (request.method() === 'POST') {
    const command = request.postDataJSON() as Command; state.writes.push(command)
    if (state.failed) return route.abort('failed')
    if (state.reject) return error(state.reject)
    if (request.headers()['if-match'] !== `"${state.revision}"` || command.version !== (state.cards[0]?.reference.version ?? 0)) return error(409)
    const card: Card = { revision: ++state.revision, reference: { id: cpID, version: command.version+1 }, team_id: team, checkpoint_id: cpID, checkpoint_version: state.cp.version, author_id: person.id, run_id: state.cp.definition.run_id, rubric: state.cp.rubric, criteria: state.cp.criteria, purpose: state.cp.purpose, submission_revision: state.cp.purpose === 'mentor_final' ? 7 : null, state: command.action === 'submit' ? 'submitted' : 'draft', scores: command.scores, created_at: '2026-10-23T09:00:00Z', changed_at: '2026-10-23T09:00:00Z' }
    state.cards = [card]; state.history.push(card)
    if (state.lost) return route.abort('failed')
    return route.fulfill({ json: card })
   }
   state.ownReads++
   if (state.secondPage && !url.searchParams.has('cursor')) return route.fulfill({ json: { revision: state.revision, items: [], next_cursor: cpID } })
   return route.fulfill({ json: { revision: state.revision, items: state.cards, next_cursor: null } })
  }
  if (path.endsWith('/submission')) return route.fulfill({ json: { outcome: state.frozen ? 'frozen' : 'missing', frozen_at: state.frozen ? '2026-10-23T08:00:00Z' : null, final: state.frozen ? { revision: 7, evidence: [] } : null } })
  if (path.endsWith('/mentor-observations/history')) { state.observationReads.push(url.search); return route.fulfill({ json: { observations: state.observations.filter(n => n.id === url.searchParams.get('observation_id') && n.version < Number(url.searchParams.get('observation_cursor'))), statuses: [], next_observation_cursor: null, next_status_cursor: null } }) }
  if (path.endsWith('/mentor-observations')) return route.fulfill({ json: { observations: state.observations, next_cursor: null } })
  throw new Error(`Unexpected scorecard fixture path ${path}`)
 })
 return state
}
async function leaveCard(page: Page, operation: () => Promise<unknown>) { const accept = (dialog: import('@playwright/test').Dialog) => void dialog.accept(); page.on('dialog', accept); try { await operation() } finally { page.off('dialog', accept) } }
async function open(page: Page) { await page.goto(`/events/${firstID}/mentor/${team}/scorecards`); await page.getByRole('button', { name: 'Open Technical checkpoint', exact: true }).click(); await expect(page.getByRole('region', { name: 'Your private scorecard' })).toBeVisible() }
async function score(page: Page, value = '0') { await page.getByRole('checkbox').first().uncheck(); await page.getByLabel('Technical merit score (explicit zero is allowed when within range)', { exact: true }).fill(value) }
async function reviewCard(page: Page, name: string) {
 const task = page.getByRole('region', { name: 'Your private scorecard' }), source = task.getByRole('button', { name, exact: true })
 await source.click()
 const confirm = page.getByRole('button', { name: name.startsWith('Save') ? 'Confirm private draft' : 'Confirm independent submission', exact: true })
 await expect.poll(async () => await confirm.isVisible() || (await task.getAttribute('aria-busy') === 'false' && await source.count() > 0)).toBe(true)
 if (await confirm.isVisible()) await confirm.click()
}
const status = (page: Page) => page.getByRole('status', { name: 'Scorecard save state' })


export { fixture, open, score, reviewCard, leaveCard, status, team, cpID, category, nextCategory }

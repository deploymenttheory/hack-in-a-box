import { expect, type Page } from '@playwright/test'
import { firstID } from './fixtures'
import { mockMentors } from './mentor-fixtures'
import { mentorCheckpointGuideSource } from '../src/styleguide/mentor-checkpoint-fixture'
import { publicChallengeRubric } from '../src/challenge-rubric-model'
import { rubricGuideConfiguration } from '../src/styleguide/rubric-fixture'
import type { components } from '../src/api/schema.gen'

type Schema = components['schemas']
export const plannerRegion = (page: Page) => page.getByRole('region', { name: 'Organiser mentor checkpoint planning', exact: true })
export async function plannerFixture(page: Page, withRun = true) {
  const base = await mockMentors(page), source = mentorCheckpointGuideSource(withRun)
  Object.assign(base.events[0], source.event, { id: firstID, name: 'Synthetic mentor planning', can_read_review_coverage: true })
  source.event = base.events[0]
  const configuration = { ...rubricGuideConfiguration(), ...source.configuration, approved: true, definition: { ...source.approved!.definition, private_judge_guidance: 'PRIVATE PLANNER GUIDANCE CANARY' } }
  const state = { source, configuration, writes: [] as { path: string; body: unknown; revision: string }[], reads: [] as string[], denied: false, failRead: false, loseNext: false, badReceipt: false, ambiguous: false, readBarrier: undefined as Promise<void> | undefined, postBarrier: undefined as Promise<void> | undefined }
  await page.route('**/api/v1/**', async route => {
    const request = route.request(), path = new URL(request.url()).pathname
    const reject = (status: number) => route.fulfill({ status, json: { error: { code: status === 403 ? 'forbidden' : status === 409 ? 'revision_conflict' : 'synthetic_failure', message: 'Synthetic native boundary', request_id: 'synthetic' } } })
    if (path === `/api/v1/events/${firstID}`) return state.denied ? reject(403) : route.fulfill({ json: state.source.event })
    if (path === '/api/v1/time/resolve') {
      const body = request.postDataJSON()
      if (state.ambiguous && body.wall_time === '2027-10-31T01:30:00') {
        if (!body.dst_choice) return reject(400)
        return route.fulfill({ json: { instant: body.dst_choice === 'earlier' ? '2027-10-31T00:30:00Z' : '2027-10-31T01:30:00Z' } })
      }
      return route.fulfill({ json: { instant: new Date(body.wall_time + 'Z').toISOString() } })
    }
    if (path.endsWith('/rubric/configuration')) {
      if (state.denied) return reject(403)
      if (request.method() === 'GET') { state.reads.push(path); if (state.readBarrier) await state.readBarrier; if (state.failRead) return reject(503); return route.fulfill({ json: state.configuration }).catch(() => {}) }
      const body = request.postDataJSON(); state.writes.push({ path, body, revision: request.headers()['if-match'] })
      expect(request.headers()['if-match']).toBe(`"${state.configuration.revision}"`)
      expect(body.action).toBe('create_run')
      state.configuration.runs.push({ id: '88888888-8888-4888-8888-888888888888', purpose: body.purpose, reference: state.configuration.reference, previous_id: null, started_at: null })
      state.configuration.revision++; state.source.event.revision++
      if (state.postBarrier) await state.postBarrier
      if (state.loseNext) { state.loseNext = false; return route.abort('failed') }
      return route.fulfill({ json: state.badReceipt ? { ...state.configuration, revision: state.configuration.revision + 1 } : state.configuration })
    }
    if (path.endsWith('/scorecard-checkpoints')) {
      if (state.denied) return reject(403)
      if (request.method() === 'GET') { state.reads.push(path); return route.fulfill({ json: { ...state.source.checkpoints, revision: state.source.event.revision } }) }
      const body = request.postDataJSON() as Schema['ScorecardCheckpointCommand']; state.writes.push({ path, body, revision: request.headers()['if-match'] })
      expect(request.headers()['if-match']).toBe(`"${state.source.event.revision}"`)
      const run = state.configuration.runs.find(value => value.id === body.definition.run_id)!
      const current = state.source.checkpoints.items.find(value => value.definition.id === body.definition.id)
      expect(body.version).toBe(current?.version ?? 0)
      const checkpoint: Schema['ScorecardCheckpoint'] = { definition: body.definition, version: body.version + 1, purpose: run.purpose as 'checkpoint' | 'mentor_final', rubric: run.reference, criteria: publicChallengeRubric(state.configuration).definition, timezone: state.source.event.configuration.timezone, opens_at: new Date(body.definition.opens_wall_time + 'Z').toISOString(), closes_at: new Date(body.definition.closes_wall_time + 'Z').toISOString() }
      state.source.checkpoints.items = state.source.checkpoints.items.filter(value => value.definition.id !== checkpoint.definition.id).concat(checkpoint)
      state.source.event.revision++
      if (state.postBarrier) await state.postBarrier
      if (state.loseNext) { state.loseNext = false; return route.abort('failed') }
      return route.fulfill({ json: state.badReceipt ? { ...checkpoint, version: checkpoint.version + 1 } : checkpoint })
    }
    return route.fallback()
  })
  return state
}
export async function openPlanner(page: Page) {
  await page.goto(`/events/${firstID}?section=mentors`)
  await expect(plannerRegion(page)).toBeVisible()
  await expect(plannerRegion(page).getByRole('combobox', { name: 'Planning task', exact: true })).toBeVisible()
}
export async function fillCheckpoint(page: Page, name = 'Independent provisional observation') {
  const planner = plannerRegion(page)
  await planner.getByRole('combobox', { name: 'Mentor assessment run', exact: true }).selectOption({ index: 1 })
  await planner.getByRole('textbox', { name: 'Checkpoint name', exact: true }).fill(name)
  await planner.getByLabel('Checkpoint opens', { exact: true }).fill('2027-06-20T10:00')
  await planner.getByLabel('Checkpoint closes', { exact: true }).fill('2027-06-20T15:00')
  await planner.getByRole('textbox', { name: 'Checkpoint configuration reason', exact: true }).fill('PRIVATE UNSENT PLANNING REASON')
}
export async function reviewCheckpoint(page: Page) {
  await plannerRegion(page).getByRole('button', { name: 'Review checkpoint window', exact: true }).click()
  const review = page.getByRole('alertdialog', { name: 'Review mentor checkpoint window', exact: true })
  await expect(review).toBeVisible()
  return review
}

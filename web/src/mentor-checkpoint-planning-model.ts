import { api, APIError, unwrap, type EventRecord } from './api/client'
import type { components } from './api/schema.gen'
import type { AssessmentConfiguration } from './assessment-run-model'
import { publicRubricDefinition, readChallengeRubric, sameRubric, type ChallengeRubricSnapshot } from './challenge-rubric-model'
import { rubricReferenceKey } from './rubric-planning-model'

type Schema = components['schemas']
export type MentorAssessmentPurpose = 'checkpoint' | 'mentor_final'
export type MentorCheckpointPlanning = {
  event: EventRecord
  configuration: AssessmentConfiguration
  approved?: ChallengeRubricSnapshot
  checkpoints: Schema['ScorecardCheckpointPage']
}
export type CheckpointInput = { runID: string; name: string; opens: string; closes: string; dst: '' | 'earlier' | 'later'; reason: string }
export type ResolvedCheckpointWindow = { opens: string; closes: string }
export type CheckpointIntent = { command: Schema['ScorecardCheckpointCommand']; run: Schema['RubricRun']; criteria: Schema['PublicRubricDefinition']; window: ResolvedCheckpointWindow; timezone: string }

export async function loadMentorCheckpointPlanning(eventID: string, signal: AbortSignal, assertCurrent: () => void): Promise<MentorCheckpointPlanning> {
  const params = { signal, cache: 'no-store' as const, params: { path: { event_id: eventID } } }
  const event = unwrap(await api.GET('/api/v1/events/{event_id}', params)); assertCurrent()
  if (event.id !== eventID || !event.can_edit) throw new APIError(403, 'forbidden')
  const configuration = unwrap(await api.GET('/api/v1/events/{event_id}/rubric/configuration', params)); assertCurrent()
  const checkpoints: Schema['ScorecardCheckpointPage'] = { revision: event.revision, items: [], next_cursor: null }, cursors = new Set<string>()
  let cursor: string | undefined
  for (;;) {
    const page = unwrap(await api.GET('/api/v1/events/{event_id}/scorecard-checkpoints', { ...params, params: { path: { event_id: eventID }, query: { cursor, limit: 100 } } })); assertCurrent()
    if (page.revision !== checkpoints.revision || page.items.some(item => checkpoints.items.some(old => old.definition.id === item.definition.id))) throw new APIError(409, 'checkpoint_changed')
    checkpoints.items.push(...page.items)
    if (!page.next_cursor) break
    if (cursors.has(page.next_cursor)) throw new Error('Checkpoint cursor did not advance.')
    cursors.add(page.next_cursor); cursor = page.next_cursor
  }
  const rubric = await readChallengeRubric(eventID, undefined, signal, assertCurrent); assertCurrent()
  // Public criteria and coordination only; no private judge guidance/cards.
  return { event, configuration: { revision: configuration.revision, reference: configuration.reference, runs: configuration.runs }, approved: rubric.approved, checkpoints }
}
export function mentorPlanningIdentity(source: MentorCheckpointPlanning) {
  return JSON.stringify([source.event.id, source.event.can_edit, source.event.stage, source.event.configuration.timezone, source.event.configuration.live_start, source.event.configuration.submission_cutoff])
}
export class CheckpointPlanningResponseIssue extends Error {}
export class CheckpointTimeIssue extends Error {
  constructor(readonly field: 'opens' | 'closes') { super('This local time could not be resolved. Correct it or choose an earlier/later occurrence when the clock repeats.') }
}
export function mentorPlanningVersion(source: MentorCheckpointPlanning) {
  return JSON.stringify([source.event.id, source.event.can_edit, source.event.revision, source.event.stage, source.event.configuration.timezone, source.event.configuration.live_start, source.event.configuration.submission_cutoff, source.configuration, source.approved?.reference, source.checkpoints])
}
export function mentorAssessmentRuns(source: MentorCheckpointPlanning) {
  return source.configuration.runs.filter(run => run.purpose === 'checkpoint' || run.purpose === 'mentor_final' && source.approved?.definition.mode === 'mentor_qualification')
}
export function mentorRunProblems(source: MentorCheckpointPlanning, purpose: MentorAssessmentPurpose, chosen: string, reason: string) {
  const issues: Record<string, string> = {}
  if (!source.approved || chosen !== rubricReferenceKey(source.approved.reference)) issues.rubric = 'Select the exact approved criteria.'
  if (purpose === 'mentor_final' && source.approved?.definition.mode !== 'mentor_qualification') issues.purpose = 'Final mentor qualification applies only to mentor-qualification judging.'
  if (!reason.trim()) issues.reason = 'Enter the reason for this assessment run.'
  else if (new TextEncoder().encode(reason.trim()).length > 2000) issues.reason = 'Keep the reason within 2,000 UTF-8 bytes.'
  return issues
}
export function checkpointProblems(source: MentorCheckpointPlanning, input: CheckpointInput) {
  const issues: Record<string, string> = {}
  if (!mentorAssessmentRuns(source).some(run => run.id === input.runID)) issues.run = 'Choose a current applicable mentor assessment run.'
  if (!input.name.trim()) issues.name = 'Enter a checkpoint name.'
  else if (new TextEncoder().encode(input.name.trim()).length > 160) issues.name = 'Keep the name within 160 UTF-8 bytes.'
  if (!input.opens) issues.opens = 'Enter the local opening time.'
  if (!input.closes) issues.closes = 'Enter the local closing time.'
  if (!input.reason.trim()) issues.reason = 'Enter the reason for this checkpoint.'
  else if (new TextEncoder().encode(input.reason.trim()).length > 2000) issues.reason = 'Keep the reason within 2,000 UTF-8 bytes.'
  return issues
}
export function checkpointWindowProblems(event: EventRecord, purpose: MentorAssessmentPurpose, window: ResolvedCheckpointWindow) {
  const issues: Record<string, string> = {}, start = Date.parse(window.opens), end = Date.parse(window.closes), live = Date.parse(event.configuration.live_start), cutoff = Date.parse(event.configuration.submission_cutoff)
  if (!Number.isFinite(start)) issues.opens = 'The opening instant could not be resolved.'
  if (!Number.isFinite(end) || end <= start) issues.closes = 'Close after the opening instant.'
  if (purpose === 'checkpoint' && start < live) issues.opens = 'Live observations cannot open before live start.'
  if (purpose === 'checkpoint' && end > cutoff) issues.closes = 'Live observations must close by the submission cutoff.'
  if (purpose === 'mentor_final' && start < cutoff) issues.opens = 'Final mentor qualification cannot open before the submission cutoff.'
  return issues
}
export async function resolveCheckpointWindow(source: MentorCheckpointPlanning, input: CheckpointInput, csrf: string, signal: AbortSignal, assertCurrent: () => void): Promise<ResolvedCheckpointWindow> {
  const resolve = async (wall: string) => {
    try {
      const value = unwrap(await api.POST('/api/v1/time/resolve', { signal, params: { header: { 'X-CSRF-Token': csrf } }, body: { timezone: source.event.configuration.timezone, wall_time: wall.length === 16 ? wall + ':00' : wall, dst_choice: input.dst } })); assertCurrent(); return value.instant
    } catch (error) { assertCurrent(); if (error instanceof APIError && error.status === 400) throw new CheckpointTimeIssue(wall === input.opens ? 'opens' : 'closes'); throw error }
  }
  const opens = await resolve(input.opens), closes = await resolve(input.closes)
  return { opens, closes }
}
const definitionKey = (definition: Schema['ScorecardCheckpointDefinition']) => JSON.stringify([definition.id, definition.name, definition.run_id, definition.opens_wall_time, definition.closes_wall_time, definition.dst_choice])
export function matchesCheckpointResponse(value: Schema['ScorecardCheckpoint'], intent: CheckpointIntent) {
  return definitionKey(value.definition) === definitionKey(intent.command.definition) && value.version === intent.command.version + 1
    && value.purpose === intent.run.purpose && sameRubric(value.rubric, intent.run.reference) && value.timezone === intent.timezone
    && JSON.stringify(publicRubricDefinition(value.criteria)) === JSON.stringify(publicRubricDefinition(intent.criteria))
    && Number.isFinite(Date.parse(value.opens_at)) && Date.parse(value.opens_at) === Date.parse(intent.window.opens)
    && Number.isFinite(Date.parse(value.closes_at)) && Date.parse(value.closes_at) === Date.parse(intent.window.closes)
}

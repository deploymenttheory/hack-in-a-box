import { createdAssessmentRun } from './assessment-run-model'
import { api, APIError, unwrap, type EventRecord } from './api/client'
import type { components } from './api/schema.gen'
import { readChallengeRubric, sameRubric, type ChallengeRubricSnapshot } from './challenge-rubric-model'
import { rubricReferenceKey } from './rubric-planning-model'

type Schema = components['schemas']
export type JudgingSetup = {
  event: EventRecord
  configuration: Pick<Schema['RubricConfiguration'], 'revision' | 'reference' | 'runs'>
  approved?: ChallengeRubricSnapshot
  criteria?: ChallengeRubricSnapshot
  teams: Schema['TeamPage']
  staff: Schema['StaffPage']
  run?: Schema['RubricRun']
  round?: Schema['JudgeRound']
}
export type JudgeAssignmentCommand = Schema['JudgeRoundCommand'] & { action: 'assign'; team_id: string; judge_ids: string[] }
export class JudgingSetupReceiptIssue extends Error {}
export type AssignmentIntent = { requestID: string; runID: string; teamID: string; revision: number; fingerprint: string }

export async function loadJudgingSetup(eventID: string, signal: AbortSignal, assertCurrent: () => void): Promise<JudgingSetup> {
  const params = { signal, cache: 'no-store' as const, params: { path: { event_id: eventID } } }
  // A known authority loss must not wait behind a held domain read.
  const event = unwrap(await api.GET('/api/v1/events/{event_id}', params))
  assertCurrent()
  if (event.id !== eventID || !event.can_edit) throw new APIError(403, 'forbidden')
  const [value, teams, staff] = await Promise.all([
    api.GET('/api/v1/events/{event_id}/rubric/configuration', params).then(unwrap),
    api.GET('/api/v1/events/{event_id}/teams', params).then(unwrap),
    api.GET('/api/v1/events/{event_id}/staff', params).then(unwrap),
  ])
  assertCurrent()
  if (!teams.can_manage) throw new APIError(403, 'forbidden')
  const runs = value.runs.filter(run => run.purpose === 'judge_final')
  if (runs.length > 1) throw new Error('More than one current final round was returned. Read current setup before continuing.')
  const run = runs[0], rubric = await readChallengeRubric(eventID, run?.reference, signal, assertCurrent)
  const round = run ? unwrap(await api.GET('/api/v1/events/{event_id}/judging/{run_id}', { signal, cache: 'no-store', params: { path: { event_id: eventID, run_id: run.id } } })) : undefined
  assertCurrent()
  if (round && round.run_id !== run?.id || run && !sameRubric(run.reference, rubric.selected?.reference)) throw new Error('The final round does not match its exact rubric source.')
  // Keep only public criteria and coordination metadata in this task. Private
  // judge guidance/cards from organiser configuration are not copied here.
  return { event, configuration: { revision: value.revision, reference: value.reference, runs: value.runs }, approved: rubric.approved, criteria: rubric.selected, teams, staff, run, round }
}
export function currentJudges(source: JudgingSetup) {
  return source.staff.items.filter(person => person.active && person.role === 'judge')
}
export function publishedJudgingTeams(source: JudgingSetup) {
  return source.teams.teams.filter(team => team.state === 'published' && team.frozen)
}
export function judgingSetupIdentity(source: JudgingSetup) {
  return JSON.stringify([source.event.id, source.event.can_edit, source.teams.can_manage, source.event.stage,
    source.approved?.reference, source.run?.id, source.run?.reference,
    publishedJudgingTeams(source).map(team => [team.id, team.name]), currentJudges(source).map(judge => [judge.person_id, judge.display_name])])
}
export function judgingSetupVersion(source: JudgingSetup) {
  return JSON.stringify([judgingSetupIdentity(source), source.event.revision, source.configuration.revision, source.configuration.reference,
    source.configuration.runs, source.round])
}
export function roundCreationProblems(source: JudgingSetup, chosen: string, reason: string) {
  const errors: Record<string, string> = {}
  if (source.run) errors.rubric = 'A current final round already exists.'
  else if (!source.approved || chosen !== rubricReferenceKey(source.approved.reference)) errors.rubric = 'Choose the exact approved rubric version.'
  if (!reason.trim()) errors.reason = 'Enter the reason for creating this round.'
  else if (new TextEncoder().encode(reason.trim()).length > 2000) errors.reason = 'Keep the reason within 2,000 UTF-8 bytes.'
  return errors
}
export function assignmentProblems(source: JudgingSetup, teamID: string, judges: string[], reason: string, acknowledgeGap: boolean) {
  const errors: Record<string, string> = {}
  if (!source.run || !source.round) errors.team = 'Create the final round first.'
  else if (!publishedJudgingTeams(source).some(team => team.id === teamID)) errors.team = 'Choose a current frozen team.'
  if (judges.length > 50 || new Set(judges).size !== judges.length || judges.some(id => !currentJudges(source).some(judge => judge.person_id === id))) errors.judges = 'Choose distinct current judges, up to 50.'
  const missing = Math.max(0, (source.round?.required_judges ?? 0) - judges.length)
  if (missing && !acknowledgeGap) errors.gap = `Acknowledge the ${missing} unfilled required judge ${missing === 1 ? 'place' : 'places'}.`
  if (!reason.trim()) errors.reason = 'Enter the reason for this assignment.'
  else if (new TextEncoder().encode(reason.trim()).length > 4000) errors.reason = 'Keep the reason within 4,000 UTF-8 bytes.'
  return errors
}
export function createdFinalRound(before: JudgingSetup, after: Schema['RubricConfiguration'], chosen: Schema['RubricReference']) {
  return createdAssessmentRun(before.configuration, after, chosen, 'judge_final')
}

export async function assignmentFingerprint(runID: string, revision: number, body: JudgeAssignmentCommand) {
  const nil = '00000000-0000-0000-0000-000000000000'
  // Exact Go RoundCommand layout, including defaults and HTML/Unicode escaping.
  const command = { request_id: body.request_id, action: body.action, team_id: body.team_id, judge_ids: body.judge_ids,
    judge_id: body.judge_id ?? nil, competitor_id: body.competitor_id ?? nil, conflict_kind: body.conflict_kind ?? '', enabled: body.enabled ?? false, reason: body.reason }
  const value = JSON.stringify({ Run: runID, Version: revision, Command: command }).replace(/[<>&\u2028\u2029]/g, c => `\\u${c.charCodeAt(0).toString(16).padStart(4, '0')}`)
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(value))
  return [...new Uint8Array(digest)].map(byte => byte.toString(16).padStart(2, '0')).join('')
}
export function matchesAssignmentReceipt(receipt: Schema['JudgeReceipt'], intent: AssignmentIntent, actorID: string) {
  return receipt.request_id === intent.requestID && receipt.actor_id === actorID && receipt.run_id === intent.runID
    && receipt.team_id === intent.teamID && receipt.action === 'assign' && receipt.revision === intent.revision + 1
    && receipt.state === 'recorded' && receipt.fingerprint === intent.fingerprint
}

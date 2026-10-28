import { afterEach, expect, test, vi } from 'vitest'
import { api } from './api/client'
import { createdAssessmentRun } from './assessment-run-model'
import { rubricReferenceKey } from './rubric-planning-model'
import { judgingSetupGuideSource } from './styleguide/judging-setup-fixture'
import { rubricGuideConfiguration } from './styleguide/rubric-fixture'
import { checkpointProblems, checkpointWindowProblems, loadMentorCheckpointPlanning, matchesCheckpointResponse, mentorAssessmentRuns, mentorRunProblems, type CheckpointInput, type CheckpointIntent, type MentorCheckpointPlanning } from './mentor-checkpoint-planning-model'

function source(): MentorCheckpointPlanning {
  const guide = judgingSetupGuideSource(true)
  return { event: guide.event, configuration: { ...guide.configuration, runs: [{ ...guide.run!, purpose: 'checkpoint' }] }, approved: guide.approved, checkpoints: { revision: guide.event.revision, items: [], next_cursor: null } }
}
const input: CheckpointInput = { runID: '22222222-2222-4222-8222-222222222222', name: 'Independent live observation', opens: '2027-06-20T10:00:00', closes: '2027-06-20T16:00:00', dst: '', reason: 'Reviewed live checkpoint window' }
afterEach(() => vi.restoreAllMocks())

test('live and final mentor purposes remain separate and final qualification requires its actual judging mode', () => {
  const value = source(), chosen = rubricReferenceKey(value.approved!.reference)
  expect(mentorRunProblems(value, 'checkpoint', chosen, input.reason)).toEqual({})
  expect(mentorRunProblems(value, 'mentor_final', chosen, input.reason)).toHaveProperty('purpose')
  value.configuration.runs.push({ ...value.configuration.runs[0], id: '55555555-5555-4555-8555-555555555555', purpose: 'mentor_final' })
  expect(mentorAssessmentRuns(value)).toHaveLength(1)
  value.approved!.definition.mode = 'mentor_qualification'
  expect(mentorAssessmentRuns(value)).toHaveLength(2)
  expect(mentorRunProblems(value, 'mentor_final', chosen, input.reason)).toEqual({})
  expect(mentorRunProblems(value, 'checkpoint', '', input.reason)).toHaveProperty('rubric')
})
test('checkpoint fields enforce actual UTF-8 limits and current run identity before native time resolution', () => {
  const value = source()
  expect(checkpointProblems(value, input)).toEqual({})
  expect(checkpointProblems(value, { ...input, runID: 'former-run' })).toHaveProperty('run')
  expect(checkpointProblems(value, { ...input, name: '界'.repeat(54) })).toHaveProperty('name')
  expect(checkpointProblems(value, { ...input, reason: '界'.repeat(667) })).toHaveProperty('reason')
  expect(checkpointProblems(value, { ...input, opens: '', closes: '' })).toMatchObject({ opens: expect.any(String), closes: expect.any(String) })
})
test('resolved live and final windows preserve the native live-start/cutoff boundary and require forward time', () => {
  const { event } = source(), live = { opens: event.configuration.live_start, closes: event.configuration.submission_cutoff }
  expect(checkpointWindowProblems(event, 'checkpoint', live)).toEqual({})
  expect(checkpointWindowProblems(event, 'checkpoint', { ...live, opens: '2027-06-20T08:59:59Z' })).toHaveProperty('opens')
  expect(checkpointWindowProblems(event, 'checkpoint', { ...live, closes: '2027-06-20T16:00:01Z' })).toHaveProperty('closes')
  expect(checkpointWindowProblems(event, 'mentor_final', { opens: live.closes, closes: '2027-06-20T18:00:00Z' })).toEqual({})
  expect(checkpointWindowProblems(event, 'mentor_final', live)).toHaveProperty('opens')
  expect(checkpointWindowProblems(event, 'checkpoint', { opens: live.opens, closes: live.opens })).toHaveProperty('closes')
})
test('only an exact checkpoint response settles its version/run/rubric/definition and resolved instants', () => {
  const value = source(), run = value.configuration.runs[0]
  const definition = { id: '66666666-6666-4666-8666-666666666666', name: input.name, run_id: run.id, opens_wall_time: input.opens, closes_wall_time: input.closes, dst_choice: input.dst }
  const intent: CheckpointIntent = { command: { definition, version: 0, reason: input.reason }, run, timezone: value.event.configuration.timezone, window: { opens: '2027-06-20T09:00:00Z', closes: '2027-06-20T15:00:00Z' } }
  const receipt = { definition, version: 1, purpose: 'checkpoint' as const, rubric: run.reference, criteria: value.approved!.definition, timezone: intent.timezone, opens_at: '2027-06-20T10:00:00+01:00', closes_at: '2027-06-20T16:00:00+01:00' }
  expect(matchesCheckpointResponse(receipt, intent)).toBe(true)
  expect(matchesCheckpointResponse({ ...receipt, version: 2 }, intent)).toBe(false)
  expect(matchesCheckpointResponse({ ...receipt, purpose: 'mentor_final' }, intent)).toBe(false)
  expect(matchesCheckpointResponse({ ...receipt, definition: { ...definition, run_id: 'other-run' } }, intent)).toBe(false)
  expect(matchesCheckpointResponse({ ...receipt, rubric: { ...run.reference, digest: 'b'.repeat(64) } }, intent)).toBe(false)
  expect(matchesCheckpointResponse({ ...receipt, opens_at: '2027-06-20T09:00:01Z' }, intent)).toBe(false)
  expect(matchesCheckpointResponse({ ...receipt, closes_at: 'invalid' }, intent)).toBe(false)
})
test('all assessment purposes retain earlier exact runs and require the original unstarted reviewed addition', () => {
  const value = source()
  for (const purpose of ['checkpoint', 'mentor_final', 'judge_final'] as const) {
    const added = { ...value.configuration.runs[0], id: '77777777-7777-4777-8777-777777777777', purpose }
    const after = { ...rubricGuideConfiguration(), ...value.configuration, revision: value.configuration.revision + 1, runs: [...value.configuration.runs, added] }
    expect(createdAssessmentRun(value.configuration, after, value.approved!.reference, purpose)).toEqual(added)
    expect(createdAssessmentRun(value.configuration, { ...after, runs: [{ ...after.runs[0], started_at: '2027-06-20T09:00:00Z' }, added] }, value.approved!.reference, purpose)).toBeUndefined()
  }
})
test('current organiser authorization is checked before reading privileged setup and private guidance never leaves the boundary', async () => {
  const value = source(), configuration = { ...rubricGuideConfiguration(), ...value.configuration, approved: true }
  configuration.definition.private_judge_guidance = 'PRIVATE CHECKPOINT GUIDANCE CANARY'
  let allowed = false
  const get = vi.spyOn(api, 'GET').mockImplementation((async (path: string) => {
    const data = path.endsWith('/configuration') ? configuration : path.endsWith('/scorecard-checkpoints') ? value.checkpoints : { ...value.event, can_edit: allowed }
    return { data, response: new Response(null, { status: 200 }) }
  }) as typeof api.GET)
  await expect(loadMentorCheckpointPlanning(value.event.id, new AbortController().signal, () => {})).rejects.toMatchObject({ status: 403 })
  expect(get).toHaveBeenCalledTimes(1)
  allowed = true
  const result = await loadMentorCheckpointPlanning(value.event.id, new AbortController().signal, () => {})
  expect(result.approved?.reference).toEqual(value.approved!.reference)
  expect(JSON.stringify(result)).not.toContain('PRIVATE CHECKPOINT GUIDANCE CANARY')
  value.checkpoints.revision++
  await expect(loadMentorCheckpointPlanning(value.event.id, new AbortController().signal, () => {})).rejects.toMatchObject({ status: 409 })
})

import { afterEach, expect, test, vi } from 'vitest'
import { api } from './api/client'
import { judgingSetupFingerprint, assignmentProblems, deliberationProblems, createdFinalRound, currentJudges, loadJudgingSetup, matchesJudgingSetupReceipt, roundCreationProblems, type JudgeAssignmentCommand, type JudgeDeliberationCommand } from './judging-setup-model'
import { judgingSetupGuideSource } from './styleguide/judging-setup-fixture'
import { rubricGuideConfiguration } from './styleguide/rubric-fixture'
import { rubricReferenceKey } from './rubric-planning-model'

const runID = '22222222-2222-4222-8222-222222222222'
const body: JudgeAssignmentCommand = { request_id: '11111111-1111-4111-8111-111111111111', action: 'assign', team_id: '33333333-3333-4333-8333-333333333333', judge_ids: ['44444444-4444-4444-8444-444444444444', '55555555-5555-4555-8555-555555555555'], reason: 'Review <named> judges & source \u2028' }
afterEach(() => vi.restoreAllMocks())

test('assignment digest matches the actual Go RoundCommand vector including default UUIDs and escaping', async () => {
  // Generated independently using json.Marshal of the real Go RoundCommand type.
  expect(await judgingSetupFingerprint(runID, 7, body)).toBe('68eb6f7ed0386b6669f98b66a7ffda35ce11a0291f4458ff69f39210e7281c57')
  expect(await judgingSetupFingerprint(runID, 8, body)).not.toBe(await judgingSetupFingerprint(runID, 7, body))
})
test('only the exact original actor/request/run/team/version/fingerprint assignment receipt settles an outcome', () => {
  const intent = { requestID: body.request_id, runID, teamID: body.team_id, revision: 7, fingerprint: 'a'.repeat(64) }
  const receipt = { request_id: body.request_id, actor_id: 'actor', run_id: runID, team_id: body.team_id, action: 'assign', revision: 8, state: 'recorded', fingerprint: intent.fingerprint }
  expect(matchesJudgingSetupReceipt(receipt, intent, 'actor')).toBe(true)
  for (const key of ['request_id', 'actor_id', 'run_id', 'team_id', 'action', 'state', 'fingerprint'] as const) expect(matchesJudgingSetupReceipt({ ...receipt, [key]: 'wrong' }, intent, 'actor')).toBe(false)
  expect(matchesJudgingSetupReceipt({ ...receipt, revision: 9 }, intent, 'actor')).toBe(false)
})
test('creation needs explicit exact approved selection and a bounded reason, never an inferred or duplicate round', () => {
  const source = judgingSetupGuideSource(), chosen = rubricReferenceKey(source.approved!.reference)
  expect(roundCreationProblems(source, chosen, 'Reviewed final round')).toEqual({})
  expect(roundCreationProblems(source, '', 'Reviewed final round')).toHaveProperty('rubric')
  expect(roundCreationProblems(source, chosen, ' ')).toHaveProperty('reason')
  expect(roundCreationProblems(source, chosen, '界'.repeat(667))).toHaveProperty('reason')
  expect(roundCreationProblems(judgingSetupGuideSource(true), chosen, 'Another')).toHaveProperty('rubric')
})
test('creation response preserves existing runs and acknowledges one distinct unstarted exact final reference', () => {
  const source = judgingSetupGuideSource(), planned = judgingSetupGuideSource(true).run!
  const before = { ...source, configuration: { ...source.configuration, runs: [{ ...planned, id: '66666666-6666-4666-8666-666666666666', purpose: 'checkpoint' as const }] } }
  const result = { ...rubricGuideConfiguration(), ...before.configuration, revision: before.configuration.revision + 1, runs: [...before.configuration.runs, planned] }
  expect(createdFinalRound(before, result, planned.reference)).toEqual(planned)
  expect(createdFinalRound(before, { ...result, runs: [planned] }, planned.reference)).toBeUndefined()
  expect(createdFinalRound(before, { ...result, runs: [...before.configuration.runs, { ...planned, started_at: '2027-06-20T10:00:00Z' }] }, planned.reference)).toBeUndefined()
  expect(createdFinalRound(before, result, { ...planned.reference, digest: 'b'.repeat(64) })).toBeUndefined()
  expect(createdFinalRound(before, { ...result, revision: result.revision + 1 }, planned.reference)).toBeUndefined()
})
test('partial judge coverage is explicit while inactive, duplicate, non-frozen and oversized inputs are rejected', () => {
  const source = judgingSetupGuideSource(true), team = source.teams.teams[0].id, judges = currentJudges(source).map(person => person.person_id)
  expect(assignmentProblems(source, team, judges, 'Reviewed all three', false)).toEqual({})
  expect(assignmentProblems(source, team, [], 'Deliberately clear the cohort', false)).toHaveProperty('gap')
  expect(assignmentProblems(source, team, [], 'Deliberately clear the cohort', true)).toEqual({})
  expect(assignmentProblems(source, team, [judges[0], judges[0]], 'Duplicate', true)).toHaveProperty('judges')
  source.staff.items[0].active = false
  expect(assignmentProblems(source, team, judges, 'Revoked', false)).toHaveProperty('judges')
  expect(assignmentProblems(source, 'other-team', [], 'Wrong team', true)).toHaveProperty('team')
  expect(assignmentProblems(source, team, [], '界'.repeat(1334), true)).toHaveProperty('reason')
})
test('source loader returns coordination/public criteria without private judge guidance and rejects revoked authority', async () => {
  const source = judgingSetupGuideSource(), configuration = { ...rubricGuideConfiguration(), ...source.configuration, approved: true }
  configuration.definition.private_judge_guidance = 'PRIVATE JUDGE CANARY'
  let authorised = true
  vi.spyOn(api, 'GET').mockImplementation((async (path: string) => {
    const data = path.endsWith('/configuration') ? configuration : path.endsWith('/teams') ? source.teams : path.endsWith('/staff') ? source.staff : { ...source.event, can_edit: authorised }
    return { data, response: new Response(null, { status: 200 }) }
  }) as typeof api.GET)
  const loaded = await loadJudgingSetup(source.event.id, new AbortController().signal, () => {})
  expect(JSON.stringify(loaded)).not.toContain('PRIVATE JUDGE CANARY')
  expect(loaded.approved?.reference).toEqual(source.approved!.reference)
  authorised = false
  await expect(loadJudgingSetup(source.event.id, new AbortController().signal, () => {})).rejects.toMatchObject({ status: 403 })
})


test('deliberation digest matches real Go RoundCommand defaults and cannot match assignment receipts', async () => {
  const command: JudgeDeliberationCommand = { request_id: body.request_id, action: 'open_deliberation', reason: 'Review <final> submissions & exact source \u2028' }
  const fingerprint = await judgingSetupFingerprint(runID, 7, command)
  expect(fingerprint).toBe('1073cb2d5deb6ed5d7984e9503bfa2f05b973d37fe9d7692823946127f05d7a1')
  const intent = { requestID: command.request_id, runID, teamID: '00000000-0000-0000-0000-000000000000', revision: 7, fingerprint, action: 'open_deliberation' as const }
  const receipt = { request_id: command.request_id, actor_id: 'actor', run_id: runID, team_id: intent.teamID, action: 'open_deliberation', revision: 8, state: 'recorded', fingerprint }
  expect(matchesJudgingSetupReceipt(receipt, intent, 'actor')).toBe(true)
  for (const key of ['request_id', 'actor_id', 'run_id', 'team_id', 'action', 'state', 'fingerprint'] as const) expect(matchesJudgingSetupReceipt({ ...receipt, [key]: 'wrong' }, intent, 'actor')).toBe(false)
  expect(matchesJudgingSetupReceipt({ ...receipt, revision: 9 }, intent, 'actor')).toBe(false)
})
test('deliberation requires assessment and an unopened round; coverage never establishes completion', () => {
  const source = judgingSetupGuideSource(true)
  expect(deliberationProblems(source, 'Reviewed final cards')).toHaveProperty('operation')
  source.event.stage = 'assessment'
  expect(deliberationProblems(source, 'Reviewed final cards')).toEqual({})
  expect(deliberationProblems(source, '')).toHaveProperty('reason')
  expect(deliberationProblems(source, '界'.repeat(1334))).toHaveProperty('reason')
  source.round!.deliberation = true
  expect(deliberationProblems(source, 'Open again')).toHaveProperty('operation')
})

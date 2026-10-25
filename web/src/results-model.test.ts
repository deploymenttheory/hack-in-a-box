import { matchesBallotReceipt } from './result-ballot-model'
import { describe, expect, it } from 'vitest'
import { matchesResultReceipt, nilUUID } from './results-model'
import { matchesTeamFeedbackReceipt } from './team-feedback-model'
import type { components } from './api/schema.gen'
const actor = '10000000-0000-4000-8000-000000000001', request = '10000000-0000-4000-8000-000000000002', team = '10000000-0000-4000-8000-000000000003'
const intent = { actorID: actor, requestID: request, action: 'publish_awards', revision: 7, version: 2, fingerprint: 'a'.repeat(64) }
const receipt: components['schemas']['ResultReceipt'] = { actor_id: actor, request_id: request, action: 'publish_awards', expected_revision: 7, expected_version: 2, version: 3, fingerprint: intent.fingerprint, run_id: nilUUID, tie_id: '', team_ids: null, publication_id: '10000000-0000-4000-8000-000000000004' }
describe('original result acknowledgement ownership', () => {
 it('requires an exact actor, intent and real publication identity', () => {
  expect(matchesResultReceipt(receipt, intent)).toBe(true)
  for (const change of [{ actor_id: team }, { request_id: team }, { action: 'confirm' as const }, { expected_revision: 8 }, { expected_version: 1 }, { version: 4 }, { fingerprint: 'b'.repeat(64) }, { publication_id: null }, { publication_id: nilUUID }, { run_id: team }, { tie_id: 'different-tie' }, { team_ids: [team] }]) expect(matchesResultReceipt({ ...receipt, ...change }, intent)).toBe(false)
 })
 it('cannot confuse a current confirmation or another team receipt with the original report release', () => {
  const original = { request_id: request, actor_id: actor, team_id: team, action: 'release' as const, expected_version: 3, version: 4, fingerprint: 'a'.repeat(64) }, target = { ...intent, action: 'release', version: 3 }
  expect(matchesTeamFeedbackReceipt(original, target, team)).toBe(true)
  for (const change of [{ actor_id: team }, { team_id: actor }, { request_id: team }, { action: 'save' as const }, { expected_version: 4 }, { version: 5 }, { fingerprint: 'b'.repeat(64) }]) expect(matchesTeamFeedbackReceipt({ ...original, ...change }, target, team)).toBe(false)
 })
})

it('final vote receipts bind the exact native run, tie and candidate set without retaining vote prose', () => {
 const target = { ...intent, action: 'vote', resourceID: actor, subjectID: team, intentID: JSON.stringify({ tie: 'native-tie', teams: [team, request] }) }
 const value = { ...receipt, action: 'vote' as const, run_id: actor, tie_id: 'native-tie', team_ids: [team, request], publication_id: null }
 expect(matchesBallotReceipt(value, target)).toBe(true)
 for (const change of [{ actor_id: team }, { run_id: team }, { tie_id: 'another-tie' }, { team_ids: [team] }, { team_ids: [request, team] }, { publication_id: request }, { expected_version: 8 }, { fingerprint: 'b'.repeat(64) }]) expect(matchesBallotReceipt({ ...value, ...change }, target)).toBe(false)
})

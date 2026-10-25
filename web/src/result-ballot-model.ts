import type { components } from './api/schema.gen'
import { nativeFingerprint, nilUUID } from './results-model'
import type { DecisionMetadata } from './results-decision'
export type BallotCommand = components['schemas']['ResultVoteCommand']
type BallotReceipt = components['schemas']['ResultReceipt']
export function ballotFingerprint(body: BallotCommand, revision: number) {
 return nativeFingerprint({ Command: { request_id: body.request_id.toLowerCase(), version: body.version, action: body.action, source_binding: body.source_binding, tie_id: body.tie_id, team_id: body.team_id.toLowerCase(), evidence: body.evidence.map(e => ({ team_id: e.team_id.toLowerCase(), deliverable_id: e.deliverable_id?.toLowerCase() ?? null, note: e.note })), awards: null, reason: body.reason }, ExpectedRevision: revision })
}
export function matchesBallotReceipt(receipt: BallotReceipt, intent: DecisionMetadata) {
 return receipt.request_id === intent.requestID && receipt.actor_id === intent.actorID && receipt.action === 'vote' && receipt.expected_revision === intent.revision && receipt.expected_version === intent.version && receipt.version === intent.version + 1 && receipt.fingerprint === intent.fingerprint && receipt.run_id === intent.resourceID && receipt.run_id !== nilUUID && receipt.publication_id === null && JSON.stringify({ tie: receipt.tie_id, teams: receipt.team_ids }) === intent.intentID
}

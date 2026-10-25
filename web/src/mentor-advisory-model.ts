import type { components } from './api/schema.gen'
import { nativeFingerprint, nilUUID } from './results-model'
import type { DecisionMetadata } from './results-decision'
export type AdvisoryCommand = components['schemas']['ResultAdvisoryCommand']
type Command = AdvisoryCommand
type Receipt = components['schemas']['ResultAdvisoryReceipt']
export function advisoryFingerprint(body: Command, revision: number) {
 return nativeFingerprint({ Command: { request_id: body.request_id.toLowerCase(), action: body.action, version: body.version, source_binding: body.source_binding, tie_id: body.tie_id, team_id: body.team_id.toLowerCase(), submission_revision: body.submission_revision, views: body.views, reason: body.reason }, ExpectedRevision: revision })
}
export function matchesAdvisoryReceipt(receipt: Receipt, intent: DecisionMetadata) {
 return receipt.request_id === intent.requestID && receipt.actor_id === intent.actorID && receipt.action === 'advise' && receipt.expected_revision === intent.revision && receipt.expected_version === intent.version && receipt.version === intent.version + 1 && receipt.fingerprint === intent.fingerprint && receipt.run_id === intent.resourceID && receipt.run_id !== nilUUID && receipt.team_id === intent.subjectID && receipt.tie_id === intent.intentID
}

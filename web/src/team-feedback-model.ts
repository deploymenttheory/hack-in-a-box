import type { components } from './api/schema.gen'
import { nativeFingerprint } from './results-model'
import type { DecisionMetadata } from './results-decision'
type S = components['schemas']
export type TeamFeedbackBody = S['TeamFeedbackBody']
export type TeamFeedbackPage = S['TeamFeedbackPage']
export type TeamFeedbackCommand = S['TeamFeedbackCommand']
export type TeamFeedbackRecord = S['TeamFeedbackRecord']
export type TeamFeedbackScores = S['TeamFeedbackScores']
export function teamFeedbackFingerprint(teamID: string, expected: number, command: TeamFeedbackCommand) {
 return nativeFingerprint({ TeamID: teamID.toLowerCase(), ExpectedVersion: expected, Command: { request_id: command.request_id.toLowerCase(), action: command.action, source_binding: command.source_binding, score_binding: command.score_binding ?? '', body: { title: command.body?.title ?? '', summary: command.body?.summary ?? '', highlights: command.body?.highlights ?? null, next_steps: command.body?.next_steps ?? null }, include_final_scores: command.include_final_scores ?? false, reason: command.reason } })
}
export function matchesTeamFeedbackReceipt(receipt: S['TeamFeedbackReceipt'], intent: DecisionMetadata, teamID: string) {
 return receipt.team_id === teamID && receipt.actor_id === intent.actorID && receipt.request_id === intent.requestID && receipt.action === intent.action && receipt.expected_version === intent.version && receipt.version === intent.version + 1 && receipt.fingerprint === intent.fingerprint
}
export const feedbackLines = (value: string) => value.split('\n').map(s => s.trim()).filter(Boolean)
export function feedbackErrors(body: TeamFeedbackBody, reason: string) {
 const bytes = (value: string) => new TextEncoder().encode(value).length, errors: Record<string, string> = {}
 if (!body.title.trim() || bytes(body.title) > 160) errors.title = 'Use a title from 1 to 160 bytes.'
 if (!body.summary.trim() || bytes(body.summary) > 8000) errors.summary = 'Use a summary from 1 to 8,000 bytes.'
 for (const key of ['highlights', 'next_steps'] as const) if (body[key].length > 10 || body[key].some(value => !value.trim() || bytes(value) > 1000)) errors[key] = 'Use at most 10 items, each from 1 to 1,000 bytes.'
 if (!reason.trim() || bytes(reason) > 2000) errors.reason = 'Record a reason from 1 to 2,000 bytes.'
 return errors
}

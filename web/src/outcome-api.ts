import { api, unwrap } from './api/client'
import type { components } from './api/schema.gen'
export type Outcome = components['schemas']['OutcomeView']
export type OutcomeCommand = components['schemas']['OutcomeCommand']
export type OutcomeDefinition = components['schemas']['OutcomeDefinition']
export type Evidence = components['schemas']['OutcomeEvidence']
export type Publication = components['schemas']['OutcomePublication']
export type PublicationCommand = components['schemas']['OutcomePublicationCommand']
export type Review = components['schemas']['OutcomeReview']
export type ReviewCommand = components['schemas']['OutcomeReviewCommand']
export type MeasureReference = components['schemas']['OutcomeMeasureReference']
export const readOutcome = async (id: string, signal?: AbortSignal, command?: string) => unwrap(await api.GET('/api/v1/outcomes/{outcome_id}', { signal, params: { path: { outcome_id: id }, query: { command_id: command } } }))
export const readPublication = async (id: string, signal?: AbortSignal, command?: string) => unwrap(await api.GET('/api/v1/outcomes/{outcome_id}/publication', { signal, params: { path: { outcome_id: id }, query: { command_id: command } } }))
export const readReview = async (id: string, signal?: AbortSignal, command?: string) => unwrap(await api.GET('/api/v1/outcome-reviews/{review_id}', { signal, params: { path: { review_id: id }, query: { command_id: command } } }))
export type NativeResult = Outcome | Publication | Review
export type NativeIntent = { kind: 'outcome'; outcomeID: string; expected: number; command: OutcomeCommand } | { kind: 'publication'; outcomeID: string; expected: number; command: PublicationCommand } | { kind: 'review'; outcomeID: string; reviewID: string; expected: number; command: ReviewCommand }
export async function sendIntent(intent: NativeIntent, csrf: string): Promise<NativeResult> {
  const header = { 'X-CSRF-Token': csrf, 'If-Match': `"${intent.expected}"` }
  switch (intent.kind) {
    case 'outcome': return unwrap(await api.PUT('/api/v1/outcomes/{outcome_id}', { params: { path: { outcome_id: intent.outcomeID }, header }, body: intent.command }))
    case 'publication': return unwrap(await api.PUT('/api/v1/outcomes/{outcome_id}/publication', { params: { path: { outcome_id: intent.outcomeID }, header }, body: intent.command }))
    case 'review': return unwrap(await api.PUT('/api/v1/outcomes/{outcome_id}/reviews/{review_id}', { params: { path: { outcome_id: intent.outcomeID, review_id: intent.reviewID }, header }, body: intent.command }))
  }
}
export async function intentReceipt(intent: NativeIntent, signal?: AbortSignal): Promise<NativeResult> {
  switch (intent.kind) {
    case 'outcome': return readOutcome(intent.outcomeID, signal, intent.command.id)
    case 'publication': return readPublication(intent.outcomeID, signal, intent.command.id)
    case 'review': return readReview(intent.reviewID, signal, intent.command.id)
  }
}

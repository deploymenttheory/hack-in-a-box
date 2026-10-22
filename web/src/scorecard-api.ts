import { api, APIError, unwrap } from './api/client'
import type { Card, Checkpoint, Intent } from './scorecard-model'
import { matchesReceipt } from './scorecard-model'
export function checkpointPage(eventID: string, teamID: string, cursor?: string, signal?: AbortSignal) { return api.GET('/api/v1/events/{event_id}/teams/{team_id}/scorecard-checkpoints', { signal, params: { path: { event_id: eventID, team_id: teamID }, query: { cursor, limit: 50 } } }).then(unwrap) }
export function cardPage(eventID: string, teamID: string, cursor?: string, signal?: AbortSignal) { return api.GET('/api/v1/events/{event_id}/teams/{team_id}/scorecards', { signal, params: { path: { event_id: eventID, team_id: teamID }, query: { cursor, limit: 50 } } }).then(unwrap) }
export function submissionPage(eventID: string, teamID: string, signal?: AbortSignal) { return api.GET('/api/v1/events/{event_id}/teams/{team_id}/submission', { signal, params: { path: { event_id: eventID, team_id: teamID } } }).then(unwrap) }
export async function findCard(eventID: string, teamID: string, checkpoint: Checkpoint, authorID: string, signal?: AbortSignal) {
  let cursor: string | undefined, revision: number | undefined
  do {
    const page = await cardPage(eventID, teamID, cursor, signal)
    if (revision !== undefined && revision !== page.revision) throw new APIError(409, 'revision_conflict')
    revision = page.revision
    if (page.items.some(c => c.author_id !== authorID || c.team_id !== teamID)) throw new APIError(403, 'unexpected_private_author')
    const card = page.items.find(c => c.checkpoint_id === checkpoint.definition.id && c.checkpoint_version === checkpoint.version && c.run_id === checkpoint.definition.run_id)
    if (card) return { revision, card }
    cursor = page.next_cursor ?? undefined
  } while (cursor)
  return { revision: revision!, card: null }
}
export async function currentCard(eventID: string, teamID: string, checkpoint: Checkpoint, authorID: string, signal?: AbortSignal) {
  if (!navigator.onLine) throw new Error('A current server connection is required; scores cannot be submitted offline.')
  let cursor: string | undefined, revision: number | undefined, found: Checkpoint | undefined
  do {
    const page = await checkpointPage(eventID, teamID, cursor, signal)
    if (revision !== undefined && revision !== page.revision) throw new APIError(409, 'revision_conflict')
    revision = page.revision
    found = page.items.find(cp => cp.definition.id === checkpoint.definition.id)
    if (found) break
    cursor = page.next_cursor ?? undefined
  } while (cursor)
  if (!found || found.version !== checkpoint.version || found.rubric.digest !== checkpoint.rubric.digest || found.definition.run_id !== checkpoint.definition.run_id) throw new APIError(409, 'checkpoint_changed')
  const own = await findCard(eventID, teamID, checkpoint, authorID, signal)
  let frozenRevision: number | null = null
  if (checkpoint.purpose === 'mentor_final') {
    const submission = await submissionPage(eventID, teamID, signal)
    if (submission.outcome !== 'frozen' || !submission.frozen_at || !submission.final) throw new APIError(409, 'final_entry_not_frozen')
    frozenRevision = submission.final.revision
  }
  return { ...own, frozenRevision }
}
// The native identity and exact card/event versions prevent duplicate effects;
// there is no invented Idempotency-Key endpoint or automatic version rebasing.
export async function reconcileCard(eventID: string, intent: Intent, signal?: AbortSignal): Promise<Card | null> {
  const current = await currentCard(eventID, intent.teamID, intent.checkpoint, intent.authorID, signal)
  if (!current.card || current.card.reference.version < intent.expectedVersion) return null
  const history = unwrap(await api.GET('/api/v1/events/{event_id}/teams/{team_id}/scorecards/{card_id}/history', { signal, params: { path: { event_id: eventID, team_id: intent.teamID, card_id: current.card.reference.id }, query: { cursor: intent.expectedVersion + 1, limit: 1 } } }))
  const exact = history.items[0]
  return exact && matchesReceipt(exact, intent) ? exact : null
}

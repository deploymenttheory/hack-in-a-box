import { api, APIError, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import type { CurrentDraftTarget, DraftTarget } from './mentor-drafts'

export type Observation = components['schemas']['MentorObservation']
export type ObservationPage = components['schemas']['MentorObservationPage']
export function readObservations(eventID: string, teamID: string, cursor?: string) {
  return api.GET('/api/v1/events/{event_id}/teams/{team_id}/mentor-observations', { params: { path: { event_id: eventID, team_id: teamID }, query: { cursor, limit: 50 } } }).then(unwrap)
}
export async function exactObservation(eventID: string, teamID: string, noteID: string, allowMissing: boolean) {
  try {
    const history = unwrap(await api.GET('/api/v1/events/{event_id}/teams/{team_id}/mentor-observations/history', { params: { path: { event_id: eventID, team_id: teamID }, query: { observation_id: noteID, limit: 1 } } }))
    const note = history.observations[0]
    if (!note || note.id !== noteID || note.team_id !== teamID) throw new APIError(404, 'not_found')
    return note
  } catch (error) { if (allowMissing && error instanceof APIError && error.status === 404) return null; throw error }
}
// A missing note is permitted only for an explicitly new client UUID, never
// inferred from a truncated current-notes page or an existing-note denial.
export async function currentObservation(target: DraftTarget, isNew: boolean): Promise<CurrentDraftTarget> {
  if (!navigator.onLine) throw new Error('A current server connection is required.')
  const page = await readObservations(target.eventID, target.teamID)
  if (!page.can_record) return { eventID: target.eventID, teamID: target.teamID, noteID: target.noteID, revision: page.revision, noteVersion: 0, canWrite: false, reviewOpen: false }
  const note = await exactObservation(target.eventID, target.teamID, target.noteID, isNew)
  return { eventID: target.eventID, teamID: target.teamID, noteID: target.noteID, revision: page.revision, noteVersion: note?.version ?? 0, canWrite: !note || note.author_id === target.personID, reviewOpen: page.can_record }
}

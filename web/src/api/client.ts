import createClient from 'openapi-fetch'
import { mentorSessionGeneration, retainMentorDraftScope, invalidateMentorDrafts } from '../mentor-drafts'
import type { paths, components } from './schema.gen'

export type Session = components['schemas']['Session']
export type EventRecord = components['schemas']['Event']
export type Blocker = components['schemas']['Blocker']
export class APIError extends Error {
  constructor(public status: number, public code: string, public requestID?: string, public blockers: Blocker[] = []) {
    const messages: Record<string, string> = {
      email_code_invalid: 'The confirmation code is invalid or expired. Request a new code when available.',
      email_rate_limited: 'Wait before requesting another confirmation code.',
      email_unavailable: 'Personal email is not available for this installation.',
      withdrawal_required: 'This person must explicitly withdraw their active or waitlisted attendee registration before receiving this role.',
      competition_ineligible: 'Previous organiser or early brief access permanently prevents competition in this event.',
      assessment_conflict: 'A person cannot compete and assess in the same event.',
      last_organiser: 'Assign another organiser and transfer ownership before removing this organiser.',
      mentor_coverage_invalid: 'Resolve mentor conflicts, capacity and availability. Any remaining coverage gap needs an explicit recorded exception before publication.',
      team_locked: 'Explicitly unlock the source team before moving its members.',
      team_capacity: 'The destination team has reached its hard capacity. Choose another team or review its capacity.',
      teams_frozen: 'Teams are frozen. Joining and moving members are no longer available.',
      teams_unready: 'Assign every confirmed competitor, resolve teams below three and choose captains before freezing. Changes remain drafts until published.',
      registration_closed: 'New applications are closed. Your existing status remains available.',
      registration_exists: 'You already have an active application of this type. Your status is shown below.',
      capacity_unavailable: 'All attendee places are currently held. No place was allocated.',
      form_invalid: 'Resolve the form fields before continuing.',
      event_blocked: 'Resolve the event blockers before continuing.',
    }
    super(messages[code] ?? (status === 401 ? 'Your session has expired. Sign in to continue.' :
      status === 403 ? 'You do not have access to this resource.' :
      status === 409 ? 'This has changed. Refresh and try again.' :
      'We could not complete the request. Please try again.'))
  }
}
export function unwrap<T>(result: { data?: T; error?: components['schemas']['ErrorEnvelope']; response: Response }): T {
  if (result.error || result.data === undefined) throw new APIError(result.response.status, result.error?.error.code ?? 'unavailable', result.error?.error.request_id, result.error?.error.blockers)
  return result.data
}
export const api = createClient<paths>({ credentials: 'same-origin' })
const requestSessions = new WeakMap<Request, number>()
api.use({ onRequest({ request }) { requestSessions.set(request, mentorSessionGeneration()) }, onError() { window.dispatchEvent(new Event('workspace-network-unavailable')) }, onResponse({ request, response }) {
  window.dispatchEvent(new Event('workspace-response'))
  if (response.status === 401 && requestSessions.get(request) === mentorSessionGeneration()) { window.dispatchEvent(new Event('session-expired')); invalidateMentorDrafts() }
} })
export const privateKey = (person: string, event: string, resource: string) => ['private', person, event, resource] as const
export async function getSession(context?: { signal?: AbortSignal }): Promise<Session> {
  const { data, error, response } = await api.GET('/api/v1/session', { signal: context?.signal })
  if (error || !data) throw new APIError(response.status, error?.error.code ?? 'unavailable', error?.error.request_id)
  if (context?.signal?.aborted) throw context.signal.reason
  retainMentorDraftScope(data.person.id, data.draft_scope)
  return data
}

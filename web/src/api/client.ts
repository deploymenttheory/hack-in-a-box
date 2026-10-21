import createClient from 'openapi-fetch'
import type { paths, components } from './schema.gen'

export type Session = components['schemas']['Session']
export type EventRecord = components['schemas']['Event']
export type Blocker = components['schemas']['Blocker']
export class APIError extends Error {
  constructor(public status: number, public code: string, public requestID?: string, public blockers: Blocker[] = []) {
    const messages: Record<string, string> = {
      withdrawal_required: 'This person must explicitly withdraw their active or waitlisted attendee registration before receiving this role.',
      competition_ineligible: 'Previous organiser or early brief access permanently prevents competition in this event.',
      assessment_conflict: 'A person cannot compete and assess in the same event.',
      last_organiser: 'Assign another organiser and transfer ownership before removing this organiser.',
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
api.use({ onResponse({ response }) {
  if (response.status === 401) window.dispatchEvent(new Event('session-expired'))
} })
export const privateKey = (person: string, event: string, resource: string) => ['private', person, event, resource] as const
export async function getSession(): Promise<Session> {
  const { data, error, response } = await api.GET('/api/v1/session')
  if (error || !data) throw new APIError(response.status, error?.error.code ?? 'unavailable', error?.error.request_id)
  return data
}

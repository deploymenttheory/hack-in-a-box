import { useParams } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { api, APIError, privateKey, unwrap } from './api/client'
import { useIdentity } from './session'
import { Failure } from './components/states'
import { localDate } from './event-time'
import { HelpAttentionQueue } from './help-attention-queue'
import { HelpRequestTask } from './help-request-task'
import { CollectionFrame, CollectionHeading, ContextLink } from './components/patterns'
import { PlanningOperations } from './planning-operations'
import { usePrivateRequestScope } from './private-request-scope'

function nativeDenial(error: unknown) { return error instanceof APIError && [401, 403, 404, 409].includes(error.status) }
export function HelpWorkspace() {
  const { eventId } = useParams({ strict: false }), identity = useIdentity()
  const key = `${identity.person.id}:${identity.draft_scope}:${eventId}:help`
  return eventId ? <PlanningOperations key={key}><Workspace eventID={eventId} /></PlanningOperations> : <Failure error={new Error('Missing event')} />
}
function Workspace({ eventID }: { eventID: string }) {
  const identity = useIdentity(), scopeKey = `${identity.person.id}:${identity.draft_scope}:${eventID}:help`, scope = usePrivateRequestScope(scopeKey)
  const event = useQuery({ queryKey: privateKey(identity.person.id, eventID, 'event'), queryFn: async ({ signal }) => { const ticket = scope.capture(); const value = unwrap(await api.GET('/api/v1/events/{event_id}', { signal, params: { path: { event_id: eventID } } })); ticket.assert(); return value }, refetchInterval: 15_000, refetchOnWindowFocus: 'always' })
  const queue = useQuery({ queryKey: privateKey(identity.person.id, eventID, 'help-queue'), queryFn: async ({ signal }) => { const ticket = scope.capture(); const value = unwrap(await api.GET('/api/v1/events/{event_id}/help', { signal, params: { path: { event_id: eventID } } })); ticket.assert(); return value }, refetchInterval: 15_000, refetchOnWindowFocus: 'always' })
  const denied = nativeDenial(event.error) || nativeDenial(queue.error)
  const state = denied ? { kind: 'denied' as const, message: 'Current help access is unavailable. Private tasks have been removed.' } : event.isPending || queue.isPending ? { kind: 'loading' as const } : event.error || queue.error ? { kind: event.data && queue.data ? 'paused' as const : 'failed' as const, message: 'The current help queue could not be verified. Changes are paused.', read: { label: 'Read current queue', onSelect: () => { void event.refetch(); void queue.refetch() } } } : { kind: 'ready' as const }
  return <section className="fleet-help-workspace"><ContextLink back target={{ label: 'Back to event', href: `/events/${eventID}` }} /><CollectionFrame scopeKey={scopeKey} state={state} heading={<CollectionHeading title="Team help" scope={{ entity: event.data?.name ?? 'Current event' }} description={event.data && <>Setup and logistics support are operational. Competition-output help opens at {localDate(event.data.configuration.live_start, event.data.configuration.timezone)}, after actual common challenge publication. Help never completes a formal review or changes a score.</>} />}>
    {event.data && queue.data && !denied && <><HelpAttentionQueue key={`${scopeKey}:${queue.data.can_manage}`} eventID={eventID} actorID={identity.person.id} timezone={event.data.configuration.timezone} value={queue.data} /><HelpRequestTask key={`${scopeKey}:${queue.data.request_team?.id}:${queue.data.can_request}`} event={event.data} context={queue.data} refresh={() => queue.refetch()} /></>}
  </CollectionFrame></section>
}

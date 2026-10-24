import { HelpActions } from './help-actions'
import { useParams } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { api, APIError, privateKey, unwrap } from './api/client'
import { useIdentity } from './session'
import { Failure } from './components/states'
import { CollectionFrame, CollectionHeading, ContextLink } from './components/patterns'
import { StatusBadge } from './components/ui/feedback'
import { PlanningOperations } from './planning-operations'
import { usePrivateRequestScope } from './private-request-scope'

const when = (value: string | null, timezone: string) => value ? new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short', timeZone: timezone }).format(new Date(value)) : 'Not recorded'
function safeMeeting(raw: string) { try { const url = new URL(raw); return url.protocol === 'https:' && !url.username && !url.password && !url.port ? raw : undefined } catch { return undefined } }
export function HelpPage() {
  const { eventId, requestId } = useParams({ strict: false }), identity = useIdentity()
  return eventId && requestId ? <PlanningOperations key={`${identity.person.id}:${identity.draft_scope}:${eventId}:${requestId}`}><HelpReader eventID={eventId} requestID={requestId} /></PlanningOperations> : <Failure error={new Error('Missing help request')} />
}
function HelpReader({ eventID, requestID }: { eventID: string; requestID: string }) {
  const identity = useIdentity(), scopeKey = `${identity.person.id}:${identity.draft_scope}:${eventID}:${requestID}`, scope = usePrivateRequestScope(scopeKey)
  const query = useQuery({ queryKey: [...privateKey(identity.person.id, eventID, 'help-request'), requestID], gcTime: 0, refetchOnMount: 'always', queryFn: async ({ signal }) => { const ticket = scope.capture(); const value = unwrap(await api.GET('/api/v1/events/{event_id}/help/{request_id}', { signal, params: { path: { event_id: eventID, request_id: requestID } } })); ticket.assert(); return value }, refetchInterval: 15_000, refetchOnWindowFocus: 'always' })
  const value = query.data, request = value?.request
  const denied = query.error instanceof APIError && [401, 403, 404].includes(query.error.status)
  const state = denied ? { kind: 'denied' as const, message: 'Current access to this private request is unavailable.' } : query.isPending || !query.isFetchedAfterMount ? { kind: 'loading' as const } : query.error ? { kind: query.data ? 'paused' as const : 'failed' as const, message: 'The current request could not be verified. Actions are paused.', read: { label: 'Refresh request', onSelect: () => void query.refetch() } } : { kind: 'ready' as const }
  const status = request && (request.status === 'waiting' && value.history.at(-1)?.action === 'reopen' ? 'Reopened · Waiting' : request.status[0].toUpperCase() + request.status.slice(1))
  return <section className="fleet-help-detail"><nav aria-label="Help navigation" className="fleet-pattern-actions"><ContextLink back target={{ label: 'Back to event', href: `/events/${eventID}` }} /><ContextLink target={{ label: 'Help queue and new request', href: `/events/${eventID}/help` }} /></nav><CollectionFrame scopeKey={scopeKey} state={state} heading={<CollectionHeading title="Team help request" scope={{ entity: value?.event_name ?? 'Current event' }} description="Operational support is separate from formal observations, reviews and scoring." secondary={[{ label: 'Refresh request', icon: 'refresh', onSelect: () => void query.refetch() }]} />}>
    {request && value && !denied && <><header className="py-6 border-b border-border"><StatusBadge tone={request.status === 'resolved' ? 'success' : request.status === 'waiting' ? 'warning' : 'neutral'}>{request.category.replaceAll('_', ' ')}</StatusBadge><h2>{status}</h2><p>Team reference: {request.team_id} · Revision {request.revision}</p><p>Current helper: {value.assignee_name || request.assignee_id || 'Not claimed'}</p></header><section className="grid gap-4 py-6" aria-label="Private request context"><h3>Request context</h3><p className="whitespace-pre-wrap">{request.description}</p>{request.location && <p className="whitespace-pre-wrap">Location: {request.location}</p>}{request.meeting_url && (safeMeeting(request.meeting_url) ? <a href={request.meeting_url} target="_blank" rel="noopener noreferrer">Open approved meeting route</a> : <p>Meeting route unavailable</p>)}<dl className="fleet-admission-facts"><div><dt>Requested</dt><dd>{when(request.requested_at, value.timezone)}</dd></div><div><dt>First claimed</dt><dd>{when(request.first_claimed_at, value.timezone)}</dd></div><div><dt>Resolved</dt><dd>{when(request.resolved_at, value.timezone)}</dd></div><div><dt>Authoritative update</dt><dd>{when(request.updated_at, value.timezone)}</dd></div></dl></section>
      <HelpActions key={scopeKey} value={value} refresh={() => query.refetch()} /><section className="py-6 border-t border-border"><h3>Transition history</h3><ol>{value.history.map(change => <li className="py-3 border-b border-border" key={change.revision}>{change.action.replaceAll('_', ' ')} → {change.status} · {when(change.at, value.timezone)}{change.assignee_id && ` · Helper ${change.assignee_id}`}</li>)}</ol><p>These timestamps describe operational support. They do not change a score or mark a formal review complete.</p></section></>}
  </CollectionFrame></section>
}

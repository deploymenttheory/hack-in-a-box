import { Link, useParams } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { api, privateKey, unwrap } from './api/client'
import { useIdentity } from './session'
import { Failure, Loading } from './components/states'
import { Button } from './components/ui/button'

const when = (value: string | null, timezone: string) => value ? new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short', timeZone: timezone }).format(new Date(value)) : 'Not recorded'
function safeMeeting(raw: string) {
  try { const url = new URL(raw); return url.protocol === 'https:' && !url.username && !url.password && !url.port ? raw : undefined } catch { return undefined }
}
export function HelpPage() {
  const { eventId, requestId } = useParams({ strict: false })
  return eventId && requestId ? <HelpReader eventID={eventId} requestID={requestId} /> : <Failure error={new Error('Missing help request')} />
}
function HelpReader({ eventID, requestID }: { eventID: string; requestID: string }) {
  const identity = useIdentity()
  const query = useQuery({ queryKey: [...privateKey(identity.person.id, eventID, 'help-request'), requestID], queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/help/{request_id}', { params: { path: { event_id: eventID, request_id: requestID } } })), refetchInterval: 15_000, refetchOnWindowFocus: 'always' })
  const value = query.data, request = value?.request
  return <section className="help-reader"><Link to="/events/$eventId" params={{ eventId: eventID }}>← Back to event</Link><h1>Team help request</h1><p>Operational support is separate from formal observations, reviews and scoring.</p><Button variant="outline" onClick={() => void query.refetch()}>Refresh request</Button>
    {query.isPending ? <Loading /> : query.error ? <Failure error={query.error} retry={() => void query.refetch()} /> : request && <article className="panel"><p>{value.event_name} · Times in {value.timezone}</p><p className="eyebrow">{request.category.replaceAll('_', ' ')} · Revision {request.revision}</p><h2>{request.status[0].toUpperCase() + request.status.slice(1)}</h2><p>Team reference: {request.team_id}</p><p>Current helper: {request.assignee_id ?? 'Not claimed'}</p><dl className="dates"><div><dt>Requested</dt><dd>{when(request.requested_at, value.timezone)}</dd></div><div><dt>First claimed</dt><dd>{when(request.first_claimed_at, value.timezone)}</dd></div><div><dt>Resolved</dt><dd>{when(request.resolved_at, value.timezone)}</dd></div><div><dt>Authoritative update</dt><dd>{when(request.updated_at, value.timezone)}</dd></div></dl>
      <h3>Request context</h3><p className="help-problem">{request.description}</p>{request.location && <p className="help-problem">Location: {request.location}</p>}{request.meeting_url && (safeMeeting(request.meeting_url) ? <a href={request.meeting_url} target="_blank" rel="noopener noreferrer">Open approved meeting route</a> : <p>Meeting route unavailable</p>)}
      <h3>Transition history</h3><ol>{value.history.map(change => <li key={change.revision}>{change.action.replaceAll('_', ' ')} → {change.status} · {when(change.at, value.timezone)}{change.assignee_id && ` · Helper ${change.assignee_id}`}</li>)}</ol><p className="muted">These timestamps describe operational support. They do not change a score or mark a formal review complete.</p>
    </article>}
  </section>
}

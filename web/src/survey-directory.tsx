import { useEffect, useMemo } from 'react'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from '@tanstack/react-router'
import { api, privateKey, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { Failure, Loading } from './components/states'
import { localDate } from './event-time'
import { Button } from './components/ui/button'

export function SurveyDirectoryPage() {
  const { eventId } = useParams({ strict: false }) as { eventId: string }, identity = useIdentity()
  return <Directory key={`${identity.person.id}:${identity.draft_scope}:${eventId}`} eventId={eventId} />
}
function Directory({ eventId }: { eventId: string }) {
  const identity = useIdentity(), cache = useQueryClient()
  const key = useMemo(() => [...privateKey(identity.person.id, eventId, 'survey-directory'), identity.draft_scope], [identity.person.id, identity.draft_scope, eventId])
  const event = useQuery({ queryKey: [...key, 'event'], gcTime: 0, retry: false, refetchOnWindowFocus: 'always', refetchInterval: 15_000, queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}', { params: { path: { event_id: eventId } } })) })
  const own = useInfiniteQuery({ queryKey: [...key, 'own'], gcTime: 0, retry: false, enabled: !!event.data && !event.error, refetchOnWindowFocus: 'always', refetchInterval: 15_000, initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => unwrap(await api.GET('/api/v1/events/{event_id}/surveys', { params: { path: { event_id: eventId }, query: { after: pageParam, limit: 30 } } })), getNextPageParam: page => page.next_id ?? undefined })
  useEffect(() => () => { cache.removeQueries({ queryKey: key }) }, [cache, key])
  if (event.error) return <Failure error={event.error} retry={() => void event.refetch()} />
  if (event.isPending) return <Loading />
  return <section className="workspace survey-workspace"><Link to="/events/$eventId" params={{ eventId }}>← Back to event</Link><p className="eyebrow">{event.data.name}</p><h1>Surveys and feedback</h1><p>Responses are identifiable to current event organisers, not anonymous. Other participants cannot read your response. Wider reports use suppressed counts and explicitly reviewed text.</p><h2>Your invitations</h2>
    {own.error ? <Failure error={own.error} retry={() => void own.refetch()} /> : own.isPending ? <Loading /> : <><ul className="inbox-list">{own.data.pages.flatMap(page => page.items).map(item => <li className="panel" key={item.id}><h3>{item.name}</h3><p>Question version {item.version} · {item.has_responded ? 'Response saved' : 'No response yet'} · {item.can_respond ? 'Open for edits' : 'Read-only'}</p><p>Closes {localDate(item.closes_at, event.data.configuration.timezone)}</p><Link to="/events/$eventId/surveys/$surveyId" params={{ eventId, surveyId: item.id }}>Open your survey</Link>{' · '}<Link to="/events/$eventId/surveys/$surveyId/results" params={{ eventId, surveyId: item.id }}>Reviewed report</Link></li>)}</ul>{own.data.pages.every(page => page.items.length === 0) && <p>No current invitations. Later role grants do not silently add an invitation to an already published survey.</p>}{own.hasNextPage && <Button variant="outline" disabled={own.isFetchingNextPage} onClick={() => void own.fetchNextPage()}>More invitations</Button>}</>}
    {event.data.can_read_staff && <StaffDirectory event={event.data} />}
  </section>
}
function StaffDirectory({ event }: { event: EventRecord }) {
  const identity = useIdentity(), cache = useQueryClient()
  const key = useMemo(() => [...privateKey(identity.person.id, event.id, 'survey-configurations'), identity.draft_scope], [identity.person.id, identity.draft_scope, event.id])
  const query = useInfiniteQuery({ queryKey: key, gcTime: 0, retry: false, refetchOnWindowFocus: 'always', refetchInterval: 15_000, initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => unwrap(await api.GET('/api/v1/events/{event_id}/surveys/configurations', { params: { path: { event_id: event.id }, query: { after: pageParam, limit: 30 } } })), getNextPageParam: page => page.next_id ?? undefined })
  useEffect(() => () => { cache.removeQueries({ queryKey: key }) }, [cache, key])
  return <section><h2>Organiser survey administration</h2>{query.error ? <Failure error={query.error} retry={() => void query.refetch()} /> : query.isPending ? <Loading /> : <>{query.data.pages[0].can_manage && <Link className="primary-link" to="/events/$eventId/surveys/new" params={{ eventId: event.id }} search={{ fromEvent: undefined, fromSurvey: undefined }}>Create survey</Link>}<ul className="inbox-list">{query.data.pages.flatMap(page => page.items).map(item => <li className="panel" key={item.id}><h3>{item.name}</h3><p>Version {item.version} · {item.audience} · {item.cancelled ? 'Cancelled' : item.published ? 'Published, fixed questions' : item.approved ? 'Approved, not published' : 'Draft'}</p><Link to="/events/$eventId/surveys/$surveyId/manage" params={{ eventId: event.id, surveyId: item.id }}>Manage survey and restricted results</Link></li>)}</ul>{query.hasNextPage && <Button variant="outline" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>More survey configurations</Button>}</>}</section>
}

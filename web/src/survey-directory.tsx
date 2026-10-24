import { useEffect, useMemo } from 'react'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from '@tanstack/react-router'
import { api, privateKey, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { Failure, Loading } from './components/states'
import { localDate } from './event-time'
import { ActionLink } from './components/ui/button'
import { StatusBadge } from './components/ui/feedback'
import { CollectionHeading, CollectionFrame, CollectionToolbar, CollectionRecords, CollectionPagination, type CollectionState } from './components/patterns'
import { usePrivateRequestScope } from './private-request-scope'
import { nativeDenied } from './survey-context'

export function SurveyDirectoryPage() {
  const { eventId } = useParams({ strict: false }) as { eventId: string }, identity = useIdentity()
  return <Directory key={`${identity.person.id}:${identity.draft_scope}:${eventId}`} eventId={eventId} />
}
function Directory({ eventId }: { eventId: string }) {
  const identity = useIdentity(), cache = useQueryClient()
  const scope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${eventId}:survey-directory`)
  const key = useMemo(() => [...privateKey(identity.person.id, eventId, 'survey-directory'), identity.draft_scope], [identity.person.id, identity.draft_scope, eventId])
  const event = useQuery({ queryKey: [...key, 'event'], gcTime: 0, retry: false, refetchOnWindowFocus: 'always', refetchInterval: 15_000, queryFn: async ({ signal }) => { const ticket = scope.capture(); const value = unwrap(await api.GET('/api/v1/events/{event_id}', { signal, params: { path: { event_id: eventId } } })); ticket.assert(); return value } })
  const own = useInfiniteQuery({ queryKey: [...key, 'own'], gcTime: 0, retry: false, enabled: !!event.data && !event.error, refetchOnWindowFocus: 'always', refetchInterval: 15_000, initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam, signal }) => { const ticket = scope.capture(); const value = unwrap(await api.GET('/api/v1/events/{event_id}/surveys', { signal, params: { path: { event_id: eventId }, query: { after: pageParam, limit: 30 } } })); ticket.assert(); return value }, getNextPageParam: page => page.next_id ?? undefined })
  useEffect(() => () => { cache.removeQueries({ queryKey: key }) }, [cache, key])
  if (event.error) return <Failure error={event.error} retry={() => void event.refetch()} />
  if (event.isPending) return <Loading />
  const rows = own.data?.pages.flatMap(page => page.items) ?? []
  const state = directoryState(own, rows.length, 'No current invitations. Later role grants do not silently add an invitation to an already published survey.')
  return <section className="fleet-survey-workspace"><Link to="/events/$eventId" params={{ eventId }}>Back to event</Link>
    <CollectionHeading title="Surveys and feedback" scope={{ event: event.data.name, entity: 'Your invitations and reviewed reports' }} description="Responses are identifiable to current event organisers, not anonymous. Other attendees cannot read your response. Wider reports use suppressed counts and explicitly reviewed text." secondary={[{ label: 'Refresh invitations', icon: 'refresh', onSelect: () => void own.refetch() }]} />
    <CollectionFrame heading={<h2>Your invitations</h2>} scopeKey={scope.key} state={state} toolbar={<CollectionToolbar source="cursor" summary="Only invitations available to your current event role. Open a survey to review its fixed questions and your own response." />}>
      <CollectionRecords label="Your survey invitations" rows={rows} rowKey={item => item.id} rowLabel={item => item.name} destination={item => ({ label: item.name, href: `/events/${eventId}/surveys/${item.id}` })} columns={[
        { id: 'version', label: 'Questions', cell: item => `Version ${item.version}` },
        { id: 'response', label: 'Your response', cell: item => <StatusBadge tone={item.has_responded ? 'success' : 'neutral'}>{item.has_responded ? 'Response saved' : 'No response yet'}</StatusBadge> },
        { id: 'access', label: 'Response access', cell: item => item.can_respond ? 'Open for edits' : 'Read-only' },
        { id: 'closes', label: 'Closes', cell: item => localDate(item.closes_at, event.data.configuration.timezone) },
      ]} actions={item => <><ActionLink icon="book" href={`/events/${eventId}/surveys/${item.id}`}>Open your survey</ActionLink><ActionLink icon="columns" variant="ghost" href={`/events/${eventId}/surveys/${item.id}/results`}>Reviewed report</ActionLink></>} />
      <CollectionPagination label="Survey invitations" source={{ kind: 'cursor', loaded: rows.length, busy: own.isFetchingNextPage, next: own.hasNextPage ? () => void own.fetchNextPage() : undefined }} />
    </CollectionFrame>
    {event.data.can_read_staff && <StaffDirectory event={event.data} />}
  </section>
}
function StaffDirectory({ event }: { event: EventRecord }) {
  const identity = useIdentity(), cache = useQueryClient()
  const key = useMemo(() => [...privateKey(identity.person.id, event.id, 'survey-configurations'), identity.draft_scope], [identity.person.id, identity.draft_scope, event.id])
  const query = useInfiniteQuery({ queryKey: key, gcTime: 0, retry: false, refetchOnWindowFocus: 'always', refetchInterval: 15_000, initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => unwrap(await api.GET('/api/v1/events/{event_id}/surveys/configurations', { params: { path: { event_id: event.id }, query: { after: pageParam, limit: 30 } } })), getNextPageParam: page => page.next_id ?? undefined })
  useEffect(() => () => { cache.removeQueries({ queryKey: key }) }, [cache, key])
  const rows = query.data?.pages.flatMap(page => page.items) ?? []
  return <section className="fleet-survey-section"><CollectionHeading title="Organiser survey administration" scope={{ event: event.name, entity: 'Current survey configurations' }} description="Author fixed questions, review current participation and publish a separately reviewed report." primary={query.data?.pages[0].can_manage ? { label: 'Create survey', icon: 'add', href: `/events/${event.id}/surveys/new` } : undefined} />
    <CollectionFrame heading={null} scopeKey={key.join(':')} state={directoryState(query, rows.length, 'No survey configurations have been created.')} toolbar={<CollectionToolbar source="cursor" summary="Native cursor paging. Publication fixes the questions and invitation cohort." />}>
      <CollectionRecords label="Survey configurations" rows={rows} rowKey={item => item.id} rowLabel={item => item.name} destination={item => ({ label: item.name, href: `/events/${event.id}/surveys/${item.id}/manage` })} columns={[
        { id: 'version', label: 'Questions', cell: item => `Version ${item.version}` },
        { id: 'audience', label: 'Audience', cell: item => item.audience },
        { id: 'state', label: 'Publication', cell: item => <StatusBadge tone={item.cancelled ? 'neutral' : item.published ? 'success' : 'warning'}>{item.cancelled ? 'Cancelled' : item.published ? 'Published, fixed questions' : item.approved ? 'Approved, not published' : 'Draft'}</StatusBadge> },
        { id: 'opens', label: 'Opens', cell: item => localDate(item.opens_at, event.configuration.timezone) },
        { id: 'closes', label: 'Closes', cell: item => localDate(item.closes_at, event.configuration.timezone) },
      ]} actions={item => <ActionLink icon="book" href={`/events/${event.id}/surveys/${item.id}/manage`}>Manage survey and restricted results</ActionLink>} />
      <CollectionPagination label="Survey configurations" source={{ kind: 'cursor', loaded: rows.length, busy: query.isFetchingNextPage, next: query.hasNextPage ? () => void query.fetchNextPage() : undefined }} />
    </CollectionFrame>
  </section>
}
function directoryState(query: { error: unknown; isPending: boolean; data: unknown; refetch: () => unknown }, count: number, empty: string): CollectionState {
  if (nativeDenied(query.error)) return { kind: 'denied', message: 'Current survey access is unavailable.' }
  if (query.error) return { kind: query.data ? 'paused' : 'failed', message: 'Current survey access could not be refreshed.', read: { label: 'Check survey access', onSelect: () => { void query.refetch() } } }
  if (query.isPending) return { kind: 'loading' }
  return count ? { kind: 'ready' } : { kind: 'empty', message: empty }
}

import { communicationRead } from './communication-private'
import { usePrivateRequestScope } from './private-request-scope'
import { useEffect, useRef, useState } from 'react'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from '@tanstack/react-router'
import { APIError, api, privateKey, unwrap } from './api/client'
import { useIdentity } from './session'
import { Icon } from './components/ui/icon'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { SafeMarkdown } from './components/safe-markdown'
import { ResourceReader } from './resource-library'
import { EventBrand } from './branding'
import { CollectionHeading, CollectionPagination, CollectionRecords, CollectionToolbar } from './components/patterns/collection'
import { StatusBadge } from './components/ui/feedback'

export function InboxLink({ active = false }: { active?: boolean }) {
  const identity = useIdentity()
  const inbox = useQuery({ queryKey: privateKey(identity.person.id, 'personal', 'inbox-badge'), queryFn: async () => unwrap(await api.GET('/api/v1/inbox', { params: { query: { limit: 1 } } })), refetchInterval: 30_000, refetchOnWindowFocus: 'always', retry: false })
  return <Link className="fleet-shell-inbox" data-active={active || undefined} to="/inbox" aria-current={active ? 'page' : undefined}><Icon name="list" /><span>Inbox{inbox.data && !inbox.error && inbox.data.unread > 0 ? ` (${inbox.data.unread} unread)` : ''}</span></Link>
}
export function InboxPage() {
  const identity = useIdentity()
  return <InboxWorkspace key={`${identity.person.id}:${identity.draft_scope}`} />
}
function InboxWorkspace() {
  const identity = useIdentity(), cache = useQueryClient(), [eventID, setEventID] = useState(''), [selected, setSelected] = useState<string>(), [detailFailure, setDetailFailure] = useState<unknown>()
  const scope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:inbox`), filterScope = usePrivateRequestScope(eventID, scope)
  const events = useQuery({ queryKey: [...privateKey(identity.person.id, 'personal', 'inbox-events'),identity.draft_scope], queryFn: () => communicationRead(scope, async signal => unwrap(await api.GET('/api/v1/events', { signal, params: { query: { limit: 100 } } }))), retry: false })
  const notifications = useInfiniteQuery({ queryKey: [...privateKey(identity.person.id, 'personal', 'inbox'),identity.draft_scope,eventID], initialPageParam: undefined as string | undefined, queryFn: ({ pageParam }) => communicationRead(filterScope, async signal => unwrap(await api.GET('/api/v1/inbox', { signal, params: { query: { event_id: eventID || undefined, cursor: pageParam, limit: 30 } } }))), getNextPageParam: page => page.next_cursor ?? undefined, refetchInterval: 30_000, refetchOnWindowFocus: 'always', retry: false })
  const items = notifications.data?.pages.flatMap(page => page.items) ?? []
  const collection = useRef<HTMLDivElement>(null), opener = useRef<string | undefined>(undefined)
  useEffect(() => {
    if (!selected && opener.current) {
      [...(collection.current?.querySelectorAll<HTMLElement>('[data-record-focus]') ?? [])].find(node => node.dataset.recordFocus === opener.current)?.focus()
      opener.current = undefined
    }
  }, [selected])
  return <section className="workspace fleet-inbox">
    {!!detailFailure && <Failure error={detailFailure} />}
    <CollectionHeading title="Inbox" scope={{ entity: 'Your notifications' }} description="Announcements and reminders available to your current event access." secondary={[{ label: 'Refresh inbox', icon: 'refresh', disabled: notifications.isFetching, onSelect: () => void notifications.refetch() }]} />
    {!selected && <CollectionToolbar source="cursor" summary={notifications.data && !notifications.error ? <p role="status" aria-live="polite">{notifications.data.pages[0].unread} unread notifications in this view.</p> : undefined} filters={<div className="fleet-inbox-filter"><label htmlFor="inbox-event">Filter by event</label><select id="inbox-event" value={eventID} onChange={e => { setEventID(e.target.value); setSelected(undefined) }}><option value="">All your events</option>{events.data?.items.map(event => <option key={event.id} value={event.id}>{event.name}</option>)}</select>{events.error && <p role="status">Event choices are unavailable. All authorised notifications remain accessible.</p>}{events.data?.next_cursor && <p>Only the first 100 event choices are listed. All your events includes every authorised notification.</p>}</div>} />}
    {selected ? <Notification key={`${identity.draft_scope}:${selected}`} id={selected} back={() => setSelected(undefined)} deny={error => { setDetailFailure(error); setSelected(undefined); cache.removeQueries({ queryKey: privateKey(identity.person.id, 'personal', 'inbox-detail') }); void notifications.refetch(); void cache.invalidateQueries({ queryKey: privateKey(identity.person.id, 'personal', 'inbox-badge') }) }} /> : notifications.isPending ? <Loading /> : notifications.error ? <Failure error={notifications.error} retry={() => void notifications.refetch()} /> : <div ref={collection}>
      <CollectionRecords label="Your notifications" rows={items} rowKey={item => item.id} rowLabel={item => item.label}
        columns={[{ id: 'event', label: 'Event', cell: item => item.event_name || 'Personal notification' }, { id: 'received', label: 'Received', cell: item => <time dateTime={item.created_at}>{new Date(item.created_at).toLocaleString()}</time> }, { id: 'read', label: 'Status', essential: true, cell: item => <StatusBadge tone={item.read_at ? 'neutral' : 'success'}>{item.read_at ? 'Read' : 'Unread'}</StatusBadge> }]}
        actions={item => <Button icon="next" variant="ghost" data-record-focus={item.id} aria-label={`Open ${item.label}`} onClick={() => { opener.current = item.id; setDetailFailure(undefined); setSelected(item.id) }}>Open</Button>} />
      {!items.length && <p className="empty-state">No notifications are available in this view.</p>}
      <CollectionPagination label="Inbox pages" source={{ kind: 'cursor', loaded: items.length, busy: notifications.isFetchingNextPage, next: notifications.hasNextPage ? () => void notifications.fetchNextPage() : undefined }} />
    </div>}
  </section>
}

function literalHTML(source: string) {
  return source.split('<br>\n').join('\n').replace(/&(amp|lt|gt|#34|#39);/g, (_, entity: string) => ({ amp: '&', lt: '<', gt: '>', '#34': '"', '#39': "'" })[entity] ?? '')
}
function Notification({ id, back, deny }: { id: string; back: () => void; deny: (error: unknown) => void }) {
  const identity = useIdentity(), cache = useQueryClient(), heading = useRef<HTMLHeadingElement>(null), [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>()
  const key = [...privateKey(identity.person.id, 'personal', 'inbox-detail'),identity.draft_scope,id]
  const scope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${id}:message`), writing = useRef(false)
  const message = useQuery({ queryKey: key, gcTime: 0, queryFn: () => communicationRead(scope, async signal => unwrap(await api.GET('/api/v1/inbox/{notification_id}', { signal, params: { path: { notification_id: id } } }))), refetchOnWindowFocus: 'always', refetchInterval: 15_000, retry: false })
  useEffect(() => { heading.current?.focus() }, [message.data?.item.id])
  async function mark(read: boolean) {
    if (writing.current) return
    const ticket = scope.capture(); if (!ticket.current()) return
    writing.current = true
    setPending(true); setFailure(undefined)
    try {
      const result = unwrap(await api.POST('/api/v1/inbox/{notification_id}/read', { params: { path: { notification_id: id }, header: { 'X-CSRF-Token': identity.csrf_token } }, body: { read } }))
      if (!ticket.current()) return
      cache.setQueryData(key, result)
      await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, 'personal', 'inbox') })
      await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, 'personal', 'inbox-badge') })
    } catch (error) { if (ticket.current()) { if (error instanceof APIError && [401,403,404].includes(error.status)) { scope.invalidate(); deny(error); return } setFailure(error); cache.removeQueries({ queryKey: key }); void message.refetch() } } finally { writing.current = false; if (ticket.current()) setPending(false) }
  }
  const inaccessible = message.error instanceof APIError && [401,403,404].includes(message.error.status)
  useEffect(() => { if (inaccessible) { scope.invalidate(); deny(message.error) } }, [inaccessible, message.error, scope, deny])
  const detail = message.data
  return <section className="inbox-detail"><Button icon="back" variant="ghost" disabled={pending} onClick={back}>Back to inbox</Button>{message.isPending ? <Loading /> : message.error ? <Failure error={message.error} retry={() => void message.refetch()} /> : detail && <article className="fleet-inbox-message"><p className="fleet-scope-label">{detail.item.event_name} · {detail.item.label}</p><h2 ref={heading} tabIndex={-1}>{detail.subject}</h2><div className="fleet-pattern-actions"><time dateTime={detail.item.created_at}>{new Date(detail.item.created_at).toLocaleString()}</time><StatusBadge>{detail.item.read_at ? 'Read' : 'Unread'}</StatusBadge></div>{detail.format === 'markdown' ? <SafeMarkdown>{detail.body}</SafeMarkdown> : <div className="inbox-body">{detail.format === 'html' ? literalHTML(detail.body) : detail.body}</div>}{detail.destination?.kind === 'content' && <Link className="primary-link" to="/events/$eventId/resources/$resourceId" params={{ eventId: detail.item.event_id, resourceId: detail.destination.resource_id }}>Open released resource</Link>}{detail.destination?.kind === 'outcome_review' && <Link className="primary-link" to="/ideas/reviews/$reviewId" params={{ reviewId: detail.destination.resource_id }}>Open your outcome follow-through review</Link>}{detail.destination?.kind === 'survey' && <Link className="primary-link" to="/events/$eventId/surveys/$surveyId" params={{ eventId: detail.item.event_id, surveyId: detail.destination.resource_id }}>Open your survey</Link>}{detail.destination?.kind === 'help' && <Link className="primary-link" to="/events/$eventId/help/$requestId" params={{ eventId: detail.item.event_id, requestId: detail.destination.resource_id }}>Open help request</Link>}{detail.destination?.kind === 'feedback' && <Link className="primary-link" to="/events/$eventId/feedback/$feedbackId" params={{ eventId: detail.item.event_id, feedbackId: detail.destination.resource_id }}>Open reviewed team feedback</Link>}{detail.destination?.kind === 'feedback_followup' && <Link className="primary-link" to="/events/$eventId/feedback-followups/$followupId" params={{ eventId: detail.item.event_id, followupId: detail.destination.resource_id }}>Open support-action follow-up</Link>}{detail.destination?.kind === 'clarification' && <Link className="primary-link" to="/events/$eventId/clarifications/$answerId" params={{ eventId: detail.item.event_id, answerId: detail.destination.resource_id }}>Open reviewed common answer</Link>}<Button icon="check" variant="outline" pending={pending} onClick={() => void mark(!detail.item.read_at)}>{detail.item.read_at ? 'Mark unread' : 'Mark read'}</Button>{!!failure && <Failure error={failure} />}</article>}</section>
}
export function ResourcePage() {
  const { eventId, resourceId } = useParams({ from: '/events/$eventId/resources/$resourceId' }), identity = useIdentity()
  const event = useQuery({ queryKey: privateKey(identity.person.id, eventId, 'event'), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}', { params: { path: { event_id: eventId } } })), refetchOnWindowFocus: 'always', retry: false })
  return <section className="workspace"><Link to="/inbox">← Your inbox</Link>{event.isPending ? <Loading /> : event.error ? <Failure error={event.error} /> : <EventBrand eventID={eventId}><ResourceReader id={resourceId} event={event.data!} back={() => window.history.back()} /></EventBrand>}</section>
}

import { useEffect, useRef, useState } from 'react'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from '@tanstack/react-router'
import { api, privateKey, unwrap } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { SafeMarkdown } from './components/safe-markdown'
import { ResourceReader } from './resource-library'
import { EventBrand } from './branding'

export function InboxLink() {
  const identity = useIdentity()
  const inbox = useQuery({ queryKey: privateKey(identity.person.id, 'personal', 'inbox-badge'), queryFn: async () => unwrap(await api.GET('/api/v1/inbox', { params: { query: { limit: 1 } } })), refetchInterval: 30_000, refetchOnWindowFocus: 'always', retry: false })
  return <Link to="/inbox">Inbox{inbox.data && !inbox.error && inbox.data.unread > 0 ? ` (${inbox.data.unread} unread)` : ''}</Link>
}
export function InboxPage() {
  const identity = useIdentity(), [eventID, setEventID] = useState(''), [selected, setSelected] = useState<string>()
  const events = useQuery({ queryKey: privateKey(identity.person.id, 'personal', 'inbox-events'), queryFn: async () => unwrap(await api.GET('/api/v1/events', { params: { query: { limit: 100 } } })), retry: false })
  const notifications = useInfiniteQuery({ queryKey: [...privateKey(identity.person.id, 'personal', 'inbox'),eventID], initialPageParam: undefined as string | undefined, queryFn: async ({ pageParam }) => unwrap(await api.GET('/api/v1/inbox', { params: { query: { event_id: eventID || undefined, cursor: pageParam, limit: 30 } } })), getNextPageParam: page => page.next_cursor ?? undefined, refetchInterval: 30_000, refetchOnWindowFocus: 'always', retry: false })
  const items = notifications.data?.pages.flatMap(page => page.items) ?? []
  return <section className="workspace inbox"><div className="page-heading"><div><p className="eyebrow">Your notifications</p><h1>Inbox</h1><p>Announcements and reminders available to your current event access.</p></div><Button variant="outline" onClick={() => void notifications.refetch()}>Refresh inbox</Button></div>
    <label htmlFor="inbox-event">Filter by event</label><select id="inbox-event" value={eventID} onChange={e => { setEventID(e.target.value); setSelected(undefined) }}><option value="">All your events</option>{events.data?.items.map(event => <option key={event.id} value={event.id}>{event.name}</option>)}</select>
    {events.error && <p role="status">Event choices are unavailable. All authorised notifications remain accessible.</p>}
    {selected ? <Notification key={selected} id={selected} back={() => setSelected(undefined)} /> : notifications.isPending ? <Loading /> : notifications.error ? <Failure error={notifications.error} retry={() => void notifications.refetch()} /> : <><p role="status" aria-live="polite">{notifications.data?.pages[0].unread ?? 0} unread notifications in this view.</p><ul className="inbox-list">{items.map(item => <li key={item.id} className="panel"><p className="eyebrow">{item.event_name}</p><h2><button className="inbox-open" onClick={() => setSelected(item.id)}>{item.label}<span className="badge">{item.read_at ? 'Read' : 'Unread'}</span></button></h2><p>{new Date(item.created_at).toLocaleString()}</p></li>)}</ul>{!items.length && <p className="empty-state">No notifications are available in this view.</p>}{notifications.hasNextPage && <Button disabled={notifications.isFetchingNextPage} onClick={() => void notifications.fetchNextPage()}>More notifications</Button>}</>}
  </section>
}
function literalHTML(source: string) {
  return source.split('<br>\n').join('\n').replace(/&(amp|lt|gt|#34|#39);/g, (_, entity: string) => ({ amp: '&', lt: '<', gt: '>', '#34': '"', '#39': "'" })[entity] ?? '')
}
function Notification({ id, back }: { id: string; back: () => void }) {
  const identity = useIdentity(), cache = useQueryClient(), heading = useRef<HTMLHeadingElement>(null), [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>()
  const key = [...privateKey(identity.person.id, 'personal', 'inbox-detail'),id]
  const message = useQuery({ queryKey: key, queryFn: async () => unwrap(await api.GET('/api/v1/inbox/{notification_id}', { params: { path: { notification_id: id } } })), refetchOnWindowFocus: 'always', refetchInterval: 15_000, retry: false })
  useEffect(() => { heading.current?.focus() }, [message.data?.item.id])
  async function mark(read: boolean) {
    setPending(true); setFailure(undefined)
    try {
      const result = unwrap(await api.POST('/api/v1/inbox/{notification_id}/read', { params: { path: { notification_id: id }, header: { 'X-CSRF-Token': identity.csrf_token } }, body: { read } }))
      cache.setQueryData(key, result)
      await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, 'personal', 'inbox') })
      await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, 'personal', 'inbox-badge') })
    } catch (error) { setFailure(error); cache.removeQueries({ queryKey: key }); void message.refetch() } finally { setPending(false) }
  }
  const detail = message.data
  return <section className="inbox-detail"><Button variant="outline" onClick={back}>Back to inbox</Button>{message.isPending ? <Loading /> : message.error ? <Failure error={message.error} retry={() => void message.refetch()} /> : detail && <article className="panel"><p className="eyebrow">{detail.item.event_name} · {detail.item.label}</p><h2 ref={heading} tabIndex={-1}>{detail.subject}</h2><p>{new Date(detail.item.created_at).toLocaleString()} · {detail.item.read_at ? 'Read' : 'Unread'}</p>{detail.format === 'markdown' ? <SafeMarkdown>{detail.body}</SafeMarkdown> : <div className="inbox-body">{detail.format === 'html' ? literalHTML(detail.body) : detail.body}</div>}{detail.destination?.kind === 'content' && <Link className="primary-link" to="/events/$eventId/resources/$resourceId" params={{ eventId: detail.item.event_id, resourceId: detail.destination.resource_id }}>Open released resource</Link>}<Button variant="outline" disabled={pending} onClick={() => void mark(!detail.item.read_at)}>{detail.item.read_at ? 'Mark unread' : 'Mark read'}</Button>{!!failure && <Failure error={failure} />}</article>}</section>
}
export function ResourcePage() {
  const { eventId, resourceId } = useParams({ from: '/events/$eventId/resources/$resourceId' }), identity = useIdentity()
  const event = useQuery({ queryKey: privateKey(identity.person.id, eventId, 'event'), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}', { params: { path: { event_id: eventId } } })), refetchOnWindowFocus: 'always', retry: false })
  return <section className="workspace"><Link to="/inbox">← Your inbox</Link>{event.isPending ? <Loading /> : event.error ? <Failure error={event.error} /> : <EventBrand eventID={eventId}><ResourceReader id={resourceId} event={event.data!} back={() => window.history.back()} /></EventBrand>}</section>
}

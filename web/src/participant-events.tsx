import { useState } from 'react'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { Link, useParams } from '@tanstack/react-router'
import { api, privateKey, unwrap } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { EventBrand } from './branding'
import { localDate } from './event-time'
import { ResourceLibrary } from './resource-library'
import { TeamDeliverables } from './team-deliverables'

export function EventDirectory() {
  const identity = useIdentity()
  const query = useInfiniteQuery({ queryKey: privateKey(identity.person.id, 'directory', 'events'), initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => unwrap(await api.GET('/api/v1/events', { params: { query: { after: pageParam, limit: 50 } } })), getNextPageParam: page => page.next_cursor ?? undefined })
  return <section className="workspace"><div className="page-heading"><div><p className="eyebrow">Your workspace</p><h1>Your events</h1><p>Meet your team and take part in the next hack.</p></div></div>
    {query.isPending ? <Loading /> : query.error ? <Failure error={query.error} retry={() => void query.refetch()} /> : <>
      <div className="event-grid">{query.data.pages.flatMap(page => page.items).map(event => <Link className="event-card" key={event.id} to="/events/$eventId" params={{ eventId: event.id }}><span className="badge">{event.stage}</span><h2>{event.name}</h2><p>{localDate(event.configuration.live_start, event.configuration.timezone)}</p></Link>)}</div>
      {!query.data.pages[0].items.length && <div className="empty-state"><h2>No events yet</h2><p>Find an event to register for, or wait for your organiser to grant access.</p></div>}
      {query.hasNextPage && <Button onClick={() => void query.fetchNextPage()} disabled={query.isFetchingNextPage}>Show more events</Button>}
    </>}
  </section>
}
export function EventWorkspace() {
  const { eventId } = useParams({ from: '/events/$eventId' })
  return <EventBrand key={eventId} eventID={eventId}><EventContent eventID={eventId} /></EventBrand>
}
function EventContent({ eventID }: { eventID: string }) {
  const identity = useIdentity()
  const [tab, setTab] = useState('overview')
  const [unsaved, setUnsaved] = useState(false)
  const query = useQuery({ queryKey: privateKey(identity.person.id, eventID, 'event'), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}', { params: { path: { event_id: eventID } } })), refetchOnWindowFocus: 'always' })
  if (query.isPending) return <Loading />
  if (query.error) return <Failure error={query.error} retry={() => void query.refetch()} />
  const event = query.data
  return <section className="workspace"><Link to="/">← Switch event</Link><div className="page-heading"><div><p className="eyebrow">{event.stage} · {event.configuration.timezone}</p><h1>{event.name}</h1><p>{event.configuration.description}</p></div></div>
    <nav className="tabs" aria-label="Event sections">{['overview', 'resources', 'deliverables'].map(name => <button key={name} className={tab === name ? 'active' : ''} aria-current={tab === name ? 'page' : undefined} onClick={() => { if (!unsaved || window.confirm('Leave this section with unsaved changes?')) setTab(name) }}>{name[0].toUpperCase() + name.slice(1)}</button>)}</nav>
    {tab === 'resources' ? <ResourceLibrary event={event} /> : tab === 'deliverables' ? <TeamDeliverables event={event} onDirty={setUnsaved} /> : <section className="panel"><h2>Event dates</h2><dl className="dates">{[['Challenge publication', event.challenge_publication], ['Live start', event.configuration.live_start], ['Submission cutoff', event.configuration.submission_cutoff], ['Live end', event.configuration.live_end]].map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{localDate(value, event.configuration.timezone)}</dd></div>)}</dl></section>}
  </section>
}

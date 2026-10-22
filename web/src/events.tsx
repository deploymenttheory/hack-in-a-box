import { MCWorkspace } from './mc-console'
import { ExportDownloads } from './export-downloads'
import { TeamDeliverables } from './team-deliverables'
import { ResourceLibrary } from './resource-library'
import { ResourceInventory } from './resource-inventory'
import { ContentWorkspace } from './content-workspace'
import { VenueReadiness } from './venue-readiness'
import { PreparationBoard } from './preparation-board'
import { AttendanceWorkspace } from './attendance'
import { LogisticsWorkspace } from './logistics'
import { localDate } from './event-time'
import { TeamWorkspace } from './teams'
import { MentorWorkspace } from './mentors'
import { ApplicantReview } from './applicant-review'
import { useState } from 'react'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from '@tanstack/react-router'
import { api, APIError, privateKey, unwrap, type EventRecord } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { EventForm } from './event-form'
import { Staff } from './staff'
import { RegistrationForms } from './registration-forms'
import { EventBrand, BrandingEditor } from './branding'

export { localDate } from './event-time'

export function EventDirectory() {
  const identity = useIdentity()
  const query = useInfiniteQuery({ queryKey: privateKey(identity.person.id, 'directory', 'events'), initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => unwrap(await api.GET('/api/v1/events', { params: { query: { after: pageParam, limit: 50 } } })), getNextPageParam: page => page.next_cursor ?? undefined })
  return <section className="workspace"><div className="page-heading"><div><p className="eyebrow">Your workspace</p><h1>Your events</h1><p>Prepare the next hack, or return to an event already in motion.</p></div>{identity.can_create_events && <div><Link className="primary-link" to="/events/new">Create event</Link><p><Link to="/templates">Event templates</Link></p></div>}</div>
    {query.isPending ? <Loading /> : query.error ? <Failure error={query.error} retry={() => void query.refetch()} /> : <>
      <div className="event-grid">{query.data.pages.flatMap(page => page.items).map(event => <Link className="event-card" key={event.id} to="/events/$eventId" params={{ eventId: event.id }}><span className="badge">{event.stage}</span><h2>{event.name}</h2><p>{localDate(event.configuration.live_start, event.configuration.timezone)}</p><p className="muted">{event.configuration.timezone} · {event.confirmed_attendees} confirmed competitors</p></Link>)}</div>
      {query.data.pages[0].items.length === 0 && <div className="empty-state"><h2>No events yet</h2><p>{identity.can_create_events ? 'Create your first event to set its dates and appoint the organising team.' : 'Events will appear here when your organiser grants you access.'}</p></div>}
      {query.hasNextPage && <Button onClick={() => void query.fetchNextPage()} disabled={query.isFetchingNextPage}>Show more events</Button>}
    </>}
  </section>
}

export function NewEvent() {
  const identity = useIdentity()
  if (!identity.can_create_events) return <Failure error={new APIError(403, 'forbidden')} />
  return <section className="workspace"><Link to="/">← Your events</Link><div className="page-heading"><div><p className="eyebrow">Plan the next hack</p><h1>Create an event</h1><p>Save a draft, check the dates, then explicitly open registration when you are ready.</p></div></div><EventForm /></section>
}

export function EventWorkspace() {
  const { eventId } = useParams({ from: '/events/$eventId' })
  return <EventBrand key={eventId} eventID={eventId}><EventContent eventId={eventId} /></EventBrand>
}

function EventContent({ eventId }: { eventId: string }) {
  const identity = useIdentity()
  const [tab, setTab] = useState('overview')
  const [unsaved, setUnsaved] = useState(false)
  const key = privateKey(identity.person.id, eventId, 'event')
  const query = useQuery({ queryKey: key, queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}', { params: { path: { event_id: eventId } } })), refetchOnWindowFocus: 'always' })
  if (query.isPending) return <Loading />
  if (query.error) return <Failure error={query.error} retry={() => void query.refetch()} />
  const event = query.data
  return <section className="workspace"><Link to="/">← Switch event</Link><div className="page-heading"><div><p className="eyebrow">{event.stage} · {event.configuration.timezone}</p><h1>{event.name}</h1><p>{event.configuration.description || 'Your shared event workspace.'}</p></div><div className="count"><strong>{event.confirmed_attendees}</strong><span>confirmed competitors</span><small>Minimum {event.configuration.minimum_attendance} · Capacity {event.configuration.capacity}</small></div></div>
    {event.can_edit && <ExportDownloads eventID={event.id} audience="organiser" />}
    <nav className="tabs" aria-label="Event sections">{['overview', 'resources', 'deliverables', 'mentors', ...(event.can_present ? ['mc console'] : []), ...(event.can_edit ? ['setup', 'schedule', 'branding', 'forms'] : []), ...(event.can_read_staff ? ['staff', 'applicants', 'teams', 'preparation', 'content', 'inventory'] : []), ...(event.can_read_attendance ? ['attendance', 'operations'] : []), ...(event.can_read_logistics ? ['logistics'] : [])].map(name => <button key={name} className={tab === name ? 'active' : ''} onClick={() => { if (!unsaved || window.confirm('Leave this section with unsaved changes?')) setTab(name) }} aria-current={tab === name ? 'page' : undefined}>{name[0].toUpperCase() + name.slice(1)}</button>)}</nav>
    {tab === 'mc console' && event.can_present ? <MCWorkspace event={event} onDirty={setUnsaved} /> : tab === 'inventory' && event.can_read_staff ? <ResourceInventory event={event} onDirty={setUnsaved} /> : tab === 'mentors' ? <MentorWorkspace event={event} onDirty={setUnsaved} /> : tab === 'deliverables' ? <TeamDeliverables event={event} onDirty={setUnsaved} /> : tab === 'resources' ? <ResourceLibrary event={event} /> : tab === 'content' && event.can_read_staff ? <ContentWorkspace event={event} onDirty={setUnsaved} openSection={setTab} /> : tab === 'operations' && event.can_read_attendance ? <VenueReadiness event={event} onDirty={setUnsaved} openSection={setTab} /> : tab === 'attendance' && event.can_read_attendance ? <AttendanceWorkspace event={event} onDirty={setUnsaved} openSection={setTab} /> : tab === 'logistics' && event.can_read_logistics ? <LogisticsWorkspace event={event} back={() => setTab('attendance')} /> : tab === 'preparation' && event.can_read_staff ? <PreparationBoard event={event} onDirty={setUnsaved} openSection={setTab} /> : tab === 'teams' && event.can_read_staff ? <TeamWorkspace eventID={event.id} timezone={event.configuration.timezone} /> : tab === 'applicants' && event.can_read_staff ? <ApplicantReview event={event} openSetup={() => setTab('setup')} /> : tab === 'forms' && event.can_edit ? <RegistrationForms event={event} onDirty={setUnsaved} /> : tab === 'branding' && event.can_edit ? <BrandingEditor event={event} onDirty={setUnsaved} /> : tab === 'setup' && event.can_edit ? <EventForm key={event.id} event={event} onDirty={setUnsaved} /> : tab === 'staff' && event.can_read_staff ? <Staff event={event} /> : tab === 'schedule' && event.can_edit ? <Schedule event={event} /> : <>
      <div className="detail-grid"><div><h2>Event dates</h2><dl className="dates">{[
        ['Final admission', event.configuration.admission_cutoff], ['Team freeze', event.configuration.team_freeze], ['Challenge publication', event.challenge_publication], ['Live start', event.configuration.live_start], ['Submission cutoff', event.configuration.submission_cutoff], ['Live end', event.configuration.live_end],
      ].map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{localDate(value, event.configuration.timezone)}</dd></div>)}</dl></div><div><h2>Readiness</h2><Blockers items={event.blockers} /><p className="muted">Only confirmed competitors count toward the minimum. Reaching it makes the go decision available; an organiser still chooses to proceed.</p></div></div>
      {event.can_read_staff && <VenueReadiness event={event} summary openSection={setTab} />}
      {event.can_transition && <StageControls event={event} />}
    </>}
  </section>
}

export function Blockers({ items }: { items: components['schemas']['Blocker'][] }) {
  return items.length ? <ul className="blockers">{items.map((item, index) => <li key={`${item.field}-${index}`}><strong>{item.field.replaceAll('_', ' ')}</strong><span>{item.message}</span></li>)}</ul> : <p className="notice">No blockers for the next stage.</p>
}

function StageControls({ event }: { event: EventRecord }) {
  const identity = useIdentity(); const cache = useQueryClient()
  const [reason, setReason] = useState(''); const [exception, setException] = useState(false); const [pending, setPending] = useState(false); const [failure, setFailure] = useState<unknown>()
  const next: Record<string, components['schemas']['Event']['stage']> = { draft: 'registration', registration: 'preparation', preparation: 'live', live: 'assessment', assessment: 'completed', completed: 'archived', cancelled: 'archived' }
  async function transition(stage: Exclude<components['schemas']['Event']['stage'], 'draft'>) {
    setPending(true); setFailure(undefined)
    try {
      unwrap(await api.POST('/api/v1/events/{event_id}/transition', { params: { path: { event_id: event.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${event.revision}"` } }, body: { stage, reason, attendance_exception: exception } }))
      await cache.invalidateQueries({ queryKey: ['private', identity.person.id] }); setReason(''); setException(false)
    } catch (error) { setFailure(error) } finally { setPending(false) }
  }
  return <section className="panel"><h2>Organiser decision</h2><p>Stage changes are explicit and checked against the latest attendance, team and submission facts.</p><label htmlFor="decision-reason">Reason for an exception, cancellation or reopening</label><textarea id="decision-reason" value={reason} onChange={e => setReason(e.target.value)} maxLength={2000} />
    {event.published_at && event.stage === 'preparation' && <label className="check"><input type="checkbox" checked={exception} onChange={e => setException(e.target.checked)} />Approve proceeding below minimum attendance after post-publication withdrawals</label>}
    <div className="actions">{next[event.stage] && <Button disabled={pending} onClick={() => void transition(next[event.stage] as Exclude<components['schemas']['Event']['stage'], 'draft'>)}>Advance to {next[event.stage]}</Button>}{event.stage === 'completed' && <Button variant="outline" disabled={pending || !reason.trim()} onClick={() => void transition('assessment')}>Reopen assessment</Button>}{event.stage !== 'cancelled' && <Button variant="outline" disabled={pending || !reason.trim()} onClick={() => void transition('cancelled')}>Cancel event</Button>}</div>
    {!!failure && <Failure error={failure} />}
  </section>
}

function Schedule({ event }: { event: EventRecord }) {
  const identity = useIdentity(); const cache = useQueryClient(); const [failure, setFailure] = useState<unknown>(); const [pending, setPending] = useState(false)
  const key = privateKey(identity.person.id, event.id, 'schedule')
  const query = useQuery({ queryKey: key, queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/schedule', { params: { path: { event_id: event.id } } })) })
  async function approve(item: string) {
    setPending(true); setFailure(undefined)
    try { unwrap(await api.POST('/api/v1/events/{event_id}/schedule/{item_id}/approve', { params: { path: { event_id: event.id, item_id: item }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${event.revision}"` } } })); await cache.invalidateQueries({ queryKey: key }) } catch (error) { setFailure(error) } finally { setPending(false) }
  }
  return <section className="panel"><h2>Schedule review</h2><p>Check the event-local times before approving. Edits require fresh approval; completed actions retain their history.</p>{query.isPending ? <Loading /> : query.error ? <Failure error={query.error} /> : <ul className="item-list">{query.data.items.map(row => <li key={row.item.id}><div><strong>{row.item.title}</strong><p>{localDate(row.due_at, event.configuration.timezone)} · {row.status.replaceAll('_', ' ')}</p></div>{row.status === 'draft' && <Button disabled={pending} onClick={() => void approve(row.item.id)}>Approve {row.item.title}</Button>}</li>)}</ul>}{!!failure && <Failure error={failure} />}</section>
}

import { RepositoryWorkspace } from './repository-workspace'
import { PlanningOperations, planningDepartureAllowed } from './planning-operations'
import { RulesPlanningWorkspace } from './rules-planning'
import { PlanningHeader, PlanningNavigation, eventPlanningDestinations } from './planning-layout'
import { ActionQueue } from './components/patterns'
import { StatusBadge } from './components/ui/feedback'
import { Textarea, Checkbox, Label } from './components/ui/fields'
import { EventDirectoryView } from './event-directory-view'
import { MCWorkspace } from './mc-console'
import { ObjectivesWorkspace } from './objective-workspace'
import { CampaignWorkspace } from './campaign-composer'
import { ExportDownloads } from './export-downloads'
import { TeamDeliverables } from './team-deliverables'
import { EventGuidance } from './event-guidance'
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
  const items = query.data?.pages.flatMap(page => page.items) ?? []
  const state = query.isPending ? { kind: 'loading' as const } : query.error ? { kind: 'failed' as const, message: 'Your events could not be loaded.', read: { label: 'Try again', onSelect: () => void query.refetch() } } : !items.length ? { kind: 'empty' as const, message: identity.can_create_events ? 'Create your first event to set its dates and appoint the organising team.' : 'Events will appear here when your organiser grants you access.' } : { kind: 'ready' as const }
  return <EventDirectoryView scopeKey={identity.person.id} events={items} state={state} participant={false} canCreate={identity.can_create_events} more={query.hasNextPage ? () => void query.fetchNextPage() : undefined} busy={query.isFetchingNextPage} />
}

export function NewEvent() {
  const identity = useIdentity()
  if (!identity.can_create_events) return <Failure error={new APIError(403, 'forbidden')} />
  return <section className="fleet-planning"><Link to="/">← Your events</Link><PlanningHeader title="Create an event" context="Event planning · New draft" description="Set the event’s identity and dates, review the native validation, then save a draft before opening registration." /><EventForm /></section>
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
  if (!query.data || (query.error instanceof APIError && [401,403,404].includes(query.error.status))) return <Failure error={query.error} retry={() => void query.refetch()} />
  const event = query.data
  const sections = eventPlanningDestinations(['overview', 'today', ...(event.can_read_review_coverage ? ['review coverage'] : []), 'resources', 'deliverables', 'mentors', ...(event.can_present ? ['mc console'] : []), ...(event.can_edit ? ['setup', 'schedule', 'branding', 'rules', 'forms'] : []), ...(event.can_read_staff ? ['objectives', 'campaigns', 'staff', 'applicants', 'teams', 'preparation', 'content', 'inventory', 'repositories'] : []), ...(event.can_read_attendance ? ['attendance', 'operations'] : []), ...(event.can_read_logistics ? ['logistics'] : [])])
  return <PlanningOperations key={`${identity.person.id}:${identity.draft_scope}:${eventId}:${event.can_edit}`}><section className="fleet-planning"><Link to="/">← Switch event</Link><PlanningHeader title={event.name} context={`${event.stage} · ${event.configuration.timezone} · Revision ${event.revision}`} description={event.configuration.description || 'Your shared event workspace.'} actions={<StatusBadge tone="neutral">{event.confirmed_attendees} confirmed competitors · Minimum {event.configuration.minimum_attendance}</StatusBadge>} />
    <details className="fleet-planning-section"><summary>Related event workspaces and private downloads</summary><div className="fleet-pattern-actions"><Link to="/events/$eventId/surveys" params={{ eventId: event.id }}>Surveys and feedback</Link><Link to="/ideas" search={{ event: eventId }}>Organiser idea review and outcome follow-through</Link><Link to="/events/$eventId/media" params={{ eventId: event.id }}>Event media and photographer submissions</Link><Link to="/events/$eventId/photo-consent" params={{ eventId: event.id }}>Your photo consent</Link></div>{event.can_edit && <ExportDownloads eventID={event.id} audience="organiser" />}</details>
    <div className="fleet-planning-layout"><PlanningNavigation items={sections} current={tab} choose={name => { if (planningDepartureAllowed() && (!unsaved || window.confirm('Leave this section with unsaved changes?'))) setTab(name) }} /><div className="fleet-planning-canvas">{query.error && <div role="status"><p>Current event authority could not be checked. Your unsaved inputs remain; changes are paused.</p><Button icon="refresh" variant="outline" onClick={()=>void query.refetch()}>Check current event access</Button></div>}<fieldset className="fleet-planning-authority" disabled={!!query.error}>

    {tab === 'rules' && event.can_edit ? <RulesPlanningWorkspace event={event} onDirty={setUnsaved} openSection={setTab}/> : tab === 'review coverage' && event.can_read_review_coverage ? <section><h2>Support and review coverage</h2><Link to="/events/$eventId/review" params={{ eventId: event.id }}>Open current support and assessment dashboard</Link></section> : tab === 'today' ? <section><p className="eyebrow">Your event</p><h2>Today</h2><EventGuidance event={event} /></section> : tab === 'objectives' && event.can_read_staff ? <ObjectivesWorkspace key={event.id} event={event} onDirty={setUnsaved} /> : tab === 'campaigns' && event.can_read_staff ? <CampaignWorkspace event={event} onDirty={setUnsaved} /> : tab === 'mc console' && event.can_present ? <MCWorkspace event={event} onDirty={setUnsaved} /> : tab === 'repositories' && event.can_read_staff ? <RepositoryWorkspace event={event} /> : tab === 'inventory' && event.can_read_staff ? <ResourceInventory event={event} onDirty={setUnsaved} /> : tab === 'mentors' ? <MentorWorkspace event={event} onDirty={setUnsaved} /> : tab === 'deliverables' ? <TeamDeliverables event={event} onDirty={setUnsaved} /> : tab === 'resources' ? <ResourceLibrary event={event} /> : tab === 'content' && event.can_read_staff ? <ContentWorkspace event={event} onDirty={setUnsaved} openSection={setTab} /> : tab === 'operations' && event.can_read_attendance ? <VenueReadiness event={event} onDirty={setUnsaved} openSection={setTab} /> : tab === 'attendance' && event.can_read_attendance ? <AttendanceWorkspace event={event} onDirty={setUnsaved} openSection={setTab} /> : tab === 'logistics' && event.can_read_logistics ? <LogisticsWorkspace event={event} back={() => setTab('attendance')} /> : tab === 'preparation' && event.can_read_staff ? <PreparationBoard event={event} onDirty={setUnsaved} openSection={setTab} /> : tab === 'teams' && event.can_read_staff ? <TeamWorkspace eventID={event.id} timezone={event.configuration.timezone} canEdit={event.can_edit} onDirty={setUnsaved} /> : tab === 'applicants' && event.can_read_staff ? <ApplicantReview event={event} openSetup={() => setTab('setup')} onDirty={setUnsaved} /> : tab === 'forms' && event.can_edit ? <RegistrationForms event={event} onDirty={setUnsaved} /> : tab === 'branding' && event.can_edit ? <BrandingEditor event={event} onDirty={setUnsaved} /> : tab === 'setup' && event.can_edit ? <EventForm key={event.id} event={event} onDirty={setUnsaved} onReadiness={()=>setTab('overview')} /> : tab === 'staff' && event.can_read_staff ? <Staff event={event} /> : tab === 'schedule' && event.can_edit ? <Schedule event={event} /> : <>
      <div className="fleet-planning-detail"><section className="fleet-planning-section"><h2>Event dates</h2><dl className="dates">{[
        ['Final admission', event.configuration.admission_cutoff], ['Team freeze', event.configuration.team_freeze], ['Challenge publication', event.challenge_publication], ['Live start', event.configuration.live_start], ['Submission cutoff', event.configuration.submission_cutoff], ['Live end', event.configuration.live_end],
      ].map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{localDate(value, event.configuration.timezone)}</dd></div>)}</dl></section><section className="fleet-planning-section"><h2>Readiness</h2><Blockers items={event.blockers} /><p className="muted">Only confirmed competitors count toward the minimum. Reaching it makes the go decision available; an organiser still chooses to proceed.</p></section></div>
      <ActionQueue title="Next planning actions" items={event.blockers.map((blocker,index)=>({id:String(index),title:blocker.message,description:'Review current native facts before making a stage decision.',icon:'warning',scope:{event:event.name,entity:blocker.field.replaceAll('_',' ')},status:{label:'Requires review',tone:'warning'},action:{label:'Review '+blocker.field.replaceAll('_',' '),icon:'next',onSelect:()=>setTab(blocker.field.includes('team')&&event.can_read_staff?'teams':['attendance','minimum_attendance','confirmed_attendees'].includes(blocker.field)&&event.can_read_staff?'applicants':event.can_edit?'setup':'overview')}}))} emptyMessage="No blockers reported for the next stage. The organiser still makes an explicit go decision." />
      {event.can_read_staff && <VenueReadiness event={event} summary openSection={setTab} />}
      {event.can_transition && <StageControls event={event} />}
    </>}
  </fieldset></div></div></section></PlanningOperations>
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
  return <section className="fleet-planning-section"><h2>Organiser decision</h2><p>Stage changes are explicit and checked against the latest attendance, team and submission facts.</p><fieldset className="fleet-planning-authority" disabled={pending}><Label htmlFor="decision-reason">Reason for an exception, cancellation or reopening</Label><Textarea id="decision-reason" value={reason} onChange={e => setReason(e.target.value)} maxLength={2000} />
    {event.published_at && event.stage === 'preparation' && <Label className="check"><Checkbox checked={exception} onChange={e => setException(e.target.checked)} />Approve proceeding below minimum attendance after post-publication withdrawals</Label>}
    <div className="actions">{next[event.stage] && <Button icon="next" disabled={pending} onClick={() => void transition(next[event.stage] as Exclude<components['schemas']['Event']['stage'], 'draft'>)}>Advance to {next[event.stage]}</Button>}{event.stage === 'completed' && <Button variant="outline" disabled={pending || !reason.trim()} onClick={() => void transition('assessment')}>Reopen assessment</Button>}{event.stage !== 'cancelled' && <Button variant="outline" disabled={pending || !reason.trim()} onClick={() => void transition('cancelled')}>Cancel event</Button>}</div></fieldset>
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
  return <section className="fleet-planning-section"><h2>Schedule review</h2><p>Check the event-local times before approving. Edits require fresh approval; completed actions retain their history.</p>{query.isPending ? <Loading /> : query.error ? <Failure error={query.error} /> : <ul className="fleet-planning-records">{query.data.items.map(row => <li key={row.item.id}><div><strong>{row.item.title}</strong><p>{localDate(row.due_at, event.configuration.timezone)} · {row.status.replaceAll('_', ' ')}</p></div>{row.status === 'draft' && <Button icon="check" disabled={pending} onClick={() => void approve(row.item.id)}>Approve {row.item.title}</Button>}</li>)}</ul>}{!!failure && <Failure error={failure} />}</section>
}

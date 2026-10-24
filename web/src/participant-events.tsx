import { RepositoryWorkspace } from './repository-workspace'
import { initialEventSection } from './today-navigation'
import { EventToday } from './today'
import { PlanningOperations, planningDepartureAllowed } from './planning-operations'
import { PlanningHeader, PlanningNavigation, eventPlanningDestinations } from './planning-layout'
import { EventDirectoryView } from './event-directory-view'
import { ExportDownloads } from './export-downloads'
import { MentorRoster } from './mentor-roster'
import { useState } from 'react'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { Link, useParams } from '@tanstack/react-router'
import { api, privateKey, unwrap } from './api/client'
import { useIdentity } from './session'
import { Failure, Loading } from './components/states'
import { EventBrand } from './branding'
import { localDate } from './event-time'
import { ResourceLibrary } from './resource-library'
import { TeamAccess } from './team-access'
import { TeamDeliverables } from './team-deliverables'

export function EventDirectory() {
  const identity = useIdentity()
  const query = useInfiniteQuery({ queryKey: privateKey(identity.person.id, 'directory', 'events'), initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => unwrap(await api.GET('/api/v1/events', { params: { query: { after: pageParam, limit: 50 } } })), getNextPageParam: page => page.next_cursor ?? undefined })
  const items = query.data?.pages.flatMap(page => page.items) ?? []
  const state = query.isPending ? { kind: 'loading' as const } : query.error ? { kind: 'failed' as const, message: 'Your events could not be loaded.', read: { label: 'Try again', onSelect: () => void query.refetch() } } : !items.length ? { kind: 'empty' as const, message: "Find an event to register for, or wait for your organiser to grant access." } : { kind: 'ready' as const }
  return <EventDirectoryView scopeKey={identity.person.id} events={items} state={state} participant={true} canCreate={false} more={query.hasNextPage ? () => void query.fetchNextPage() : undefined} busy={query.isFetchingNextPage} />
}
export function EventWorkspace() {
  const { eventId } = useParams({ from: '/events/$eventId' })
  const identity = useIdentity()
  return <EventBrand key={eventId} eventID={eventId}><PlanningOperations key={`${identity.person.id}:${identity.draft_scope}:${eventId}`}><EventContent eventID={eventId} /></PlanningOperations></EventBrand>
}
function EventContent({ eventID }: { eventID: string }) {
  const identity = useIdentity()
  const [requestedTab, setTab] = useState(() => initialEventSection(window.location.search))
  const [unsaved, setUnsaved] = useState(false)
  const query = useQuery({ queryKey: privateKey(identity.person.id, eventID, 'event'), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}', { params: { path: { event_id: eventID } } })), refetchOnWindowFocus: 'always' })
  if (query.isPending) return <Loading />
  if (query.error) return <Failure error={query.error} retry={() => void query.refetch()} />
  const event = query.data
  const sections = eventPlanningDestinations(['overview','today','resources','deliverables','mentors','team access','repositories'])
  const tab = sections.some(section => section.id === requestedTab) ? requestedTab : 'overview'
  return <section className="fleet-planning"><Link to="/">← Switch event</Link><PlanningHeader title={event.name} context={`${event.stage} · ${event.configuration.timezone}`} description={event.configuration.description} />
    <details className="fleet-planning-section"><summary>Related event workspaces</summary><div className="fleet-pattern-actions"><Link to="/events/$eventId/surveys" params={{ eventId: event.id }}>Surveys and feedback</Link><Link to="/ideas" search={{ event: eventID }}>Propose an idea or record current team follow-through</Link><Link to="/events/$eventId/media" params={{ eventId: event.id }}>Event media</Link><Link to="/events/$eventId/photo-consent" params={{ eventId: event.id }}>Your photo consent</Link><Link to="/events/$eventId/help" params={{ eventId: eventID }}>Ask for help or open the helper queue</Link></div></details>
    <div className="fleet-planning-layout"><PlanningNavigation items={sections} current={tab} choose={name=>{if(planningDepartureAllowed() && (!unsaved || window.confirm('Leave this section with unsaved changes?')))setTab(name)}} /><div className="fleet-planning-canvas">

    {tab === 'today' ? <EventToday event={event} openSection={name => { if (planningDepartureAllowed() && (!unsaved || window.confirm('Leave this section with unsaved changes?'))) setTab(name) }} availableSections={sections.map(section => section.id)} /> : tab === 'repositories' ? <RepositoryWorkspace event={event} onDirty={setUnsaved} /> : tab === 'team access' ? <TeamAccess event={event} /> : tab === 'mentors' ? <MentorRoster event={event} /> : tab === 'resources' ? <ResourceLibrary event={event} /> : tab === 'deliverables' ? <TeamDeliverables event={event} onDirty={setUnsaved} /> : <section className="fleet-planning-section"><h2>Event dates</h2><ExportDownloads eventID={event.id} audience="participant" /><dl className="dates">{[['Challenge publication', event.challenge_publication], ['Live start', event.configuration.live_start], ['Submission cutoff', event.configuration.submission_cutoff], ['Live end', event.configuration.live_end]].map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{localDate(value, event.configuration.timezone)}</dd></div>)}</dl></section>}
  </div></div></section>
}

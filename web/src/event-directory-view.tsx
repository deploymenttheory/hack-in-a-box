import { useNavigate } from '@tanstack/react-router'
import type { EventRecord } from './api/client'
import { CollectionFrame, CollectionHeading, CollectionPagination, CollectionRecords, type CollectionState } from './components/patterns'
import { StatusBadge } from './components/ui/feedback'
import { localDate } from './event-time'

export function EventDirectoryView({ scopeKey, events, state, canCreate, participant, more, busy }: { scopeKey: string; events: EventRecord[]; state: CollectionState; canCreate: boolean; participant: boolean; more?: () => void; busy: boolean }) {
  const navigate = useNavigate()
  return <CollectionFrame scopeKey={scopeKey} state={state} heading={<CollectionHeading title="Your events" scope={{ entity: participant ? 'Participant workspace' : 'Administration workspace' }} description={participant ? 'Meet your team and take part in the next hack.' : 'Prepare the next hack, or return to an event already in motion.'} primary={canCreate ? { label: 'Create event', icon: 'add', href: '/events/new' } : undefined} secondary={canCreate ? [{ label: 'Event templates', icon: 'layout', href: '/templates' }] : participant ? [{ label: 'Find events', icon: 'search', href: '/register' }] : []} />}>
    <CollectionRecords label="Your events" rows={events} rowKey={event => event.id} rowLabel={event => event.name} destination={event => ({ label: event.name, href: `/events/${event.id}`, onNavigate: () => void navigate({ to: '/events/$eventId', params: { eventId: event.id } }) })} columns={[
      { id: 'stage', label: 'Stage', essential: true, cell: event => <StatusBadge>{event.stage}</StatusBadge> },
      { id: 'start', label: 'Live start', essential: true, cell: event => localDate(event.configuration.live_start, event.configuration.timezone) },
      { id: 'timezone', label: 'Timezone', cell: event => event.configuration.timezone },
      ...(!participant ? [{ id: 'confirmed', label: 'Confirmed competitors', cell: (event: EventRecord) => event.confirmed_attendees }] : []),
    ]} />
    <CollectionPagination source={{ kind: 'cursor', loaded: events.length, next: more, busy }} label="Event directory pages" />
  </CollectionFrame>
}

import { Link } from '@tanstack/react-router'
import { ExportDownloads } from './export-downloads'
import { useQuery } from '@tanstack/react-query'
import { api, privateKey, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { Failure, Loading } from './components/states'
import { localDate } from './event-time'
import { type MentorPage, slotLabel } from './mentor-model'

export function MentorRoster({ event }: { event: EventRecord }) {
  const identity = useIdentity()
  const query = useQuery({ queryKey: privateKey(identity.person.id, event.id, 'mentors'), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/mentor-coverage', { params: { path: { event_id: event.id } } })), refetchOnWindowFocus: 'always', refetchOnMount: 'always' })
  if (query.isPending) return <Loading />
  if (query.error) return <Failure error={query.error} retry={() => void query.refetch()} />
  return <section className="panel"><h2>Your mentor coverage</h2><Link to="/events/$eventId/mentor" params={{ eventId: event.id }}>Open mentor workspace</Link><ExportDownloads eventID={event.id} audience="mentor" /><PublishedMentorRoster event={event} value={query.data} /></section>
}
export function PublishedMentorRoster({ event, value }: { event: EventRecord; value: MentorPage }) {
  const names = new Map(value.profiles.map(p => [p.person_id, p.name])), teamNames = new Map(value.teams.map(t => [t.id, t.name]))
  const format = (instant: string) => localDate(instant, event.configuration.timezone)
  return <section aria-label="Published mentor roster"><h3>Published mentor roster</h3>{!value.published.length ? <p>No published mentor assignments are available to you.</p> : <ul>{value.published.map(a => <li key={`${a.team_id}-${a.slot}`}>{teamNames.get(a.team_id) ?? a.team_id} · {a.team_id.slice(0, 8)}…{a.team_id.slice(-4)} · {slotLabel(a.slot)}: {names.get(a.person_id) ?? a.person_id} · {format(a.window.start)}–{format(a.window.end)} ({event.configuration.timezone})</li>)}</ul>}</section>
}

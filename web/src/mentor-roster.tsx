import { useRef, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { ExportDownloads } from './export-downloads'
import { api, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { localDate } from './event-time'
import { type MentorPage, slotLabel } from './mentor-model'
import { Button } from './components/ui/button'
import { StatusBadge } from './components/ui/feedback'
import { CollectionFrame, CollectionHeading, CollectionRecords, DetailWorkspace } from './components/patterns'
import { useCoverageFocus, type CoverageFocusTarget } from './coverage-navigation'
import { CoverageDetail } from './coverage-detail'
import { useCoveragePage } from './coverage-command'
export function MentorRoster({ event }: { event: EventRecord }) {
  const identity = useIdentity()
  return <RosterScope key={`${identity.person.id}:${identity.draft_scope}:${event.id}`} event={event} />
}
function RosterScope({ event }: { event: EventRecord }) {
  const owner = useCoveragePage<MentorPage>(event.id, 'mentors', signal => api.GET('/api/v1/events/{event_id}/mentor-coverage', { params: { path: { event_id: event.id } }, signal }).then(unwrap), async () => { throw new Error('Published roster is read only') })
  return <CollectionFrame scopeKey={owner.scopeKey} state={owner.state} heading={<CollectionHeading title="Your mentor coverage" scope={{ event: event.name, entity: 'Published roster' }} description={<p>Only currently permitted published assignments are shown. A coverage window does not grant access outside its current permissions.</p>} />}><Link to="/events/$eventId/mentor" params={{ eventId: event.id }}>Open mentor workspace</Link><ExportDownloads eventID={event.id} audience="mentor" />{owner.query.data && <PublishedMentorRoster event={event} value={owner.query.data} />}</CollectionFrame>
}
export function PublishedMentorRoster({ event, value }: { event: EventRecord; value: MentorPage }) {
  const [selected, setSelected] = useState<string>(), focusRoot = useRef<HTMLElement>(null), focus = useCoverageFocus(focusRoot), origin = useRef<CoverageFocusTarget | undefined>(undefined)
  const names = new Map(value.profiles.map(p => [p.person_id, p.name])), teamNames = new Map(value.teams.map(t => [t.id, t.name])), format = (instant: string) => localDate(instant, event.configuration.timezone)
  const team = value.teams.find(t => t.id === selected), assignments = team ? value.published.filter(a => a.team_id === team.id) : []
  const collection = <section className="py-4" aria-label="Published mentor roster"><h2>Published mentor roster</h2><StatusBadge>{value.published.length} assignments visible to you</StatusBadge>{!value.published.length ? <p>No published mentor assignments are available to you.</p> : <CollectionRecords label="Published mentor assignments" rows={value.published} rowKey={a => `${a.team_id}-${a.slot}`} rowLabel={a => teamNames.get(a.team_id) ?? a.team_id} columns={[{ id: 'slot', label: 'Coverage', cell: a => slotLabel(a.slot) }, { id: 'mentor', label: 'Mentor', cell: a => names.get(a.person_id) ?? a.person_id }, { id: 'window', label: `Window (${event.configuration.timezone})`, cell: a => `${format(a.window.start)}–${format(a.window.end)}` }]} actions={a => <Button icon="next" variant="ghost" data-record-focus={`${a.team_id}-${a.slot}`} onClick={() => { origin.current = focus.capture(); setSelected(a.team_id) }}>View coverage for {teamNames.get(a.team_id) ?? a.team_id}</Button>} />}{value.can_manage && value.published_gaps.length > 0 && <section className="py-4 border-t border-border" aria-label="Published coverage gaps"><h3>Published coverage gaps</h3><ul>{value.published_gaps.map((gap, index) => <li key={index}>{gap.hard ? 'Blocked: ' : ''}{gap.message}</li>)}</ul></section>}</section>
  return <section ref={focusRoot} tabIndex={-1} aria-label="Published coverage workspace"><DetailWorkspace collection={collection} detail={team && <CoverageDetail key={team.id} title={`Published coverage: ${team.name}`} close={() => { setSelected(undefined); focus.restore(origin.current) }}><p className="fleet-reference">Team {team.id}</p><p>Declared team skills: {team.skills.join(', ') || 'Unknown'}</p>{assignments.map(a => <section className="py-4 border-t border-border" key={a.slot}><h3>{slotLabel(a.slot)}</h3><p>{names.get(a.person_id) ?? a.person_id}</p><p>{format(a.window.start)}–{format(a.window.end)} ({event.configuration.timezone})</p></section>)}<p>This is the published coverage available to your current role. Private assessments and other teams’ details are not included.</p></CoverageDetail>} /></section>
}

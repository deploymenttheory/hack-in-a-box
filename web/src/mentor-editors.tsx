import { useState } from 'react'
import { api, unwrap, APIError, type EventRecord } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { localDate, wallTime } from './event-time'
import { Failure } from './components/states'
import { Button } from './components/ui/button'
import { type MentorPage, type MentorProfile, type MentorCommand, type MentorWindow, type MentorAssignment, replaceSlot, workload, slotLabel, declaredAvailabilityCovers } from './mentor-model'

type WallWindow = { start: string; end: string; startDST: '' | 'earlier' | 'later'; endDST: '' | 'earlier' | 'later' }
type Save = (command: Omit<MentorCommand, 'reason'>) => Promise<void>
function initialWindow(value: MentorWindow, timezone: string): WallWindow { return { start: wallTime(value.start, timezone), end: wallTime(value.end, timezone), startDST: '', endDST: '' } }
async function resolveWindow(value: WallWindow, timezone: string, csrf: string): Promise<MentorWindow> {
  const resolve = async (wall: string, dst: WallWindow['startDST']) => unwrap(await api.POST('/api/v1/time/resolve', { params: { header: { 'X-CSRF-Token': csrf } }, body: { wall_time: wall.length === 16 ? wall + ':00' : wall, timezone, dst_choice: dst } })).instant
  const [start, end] = await Promise.all([resolve(value.start, value.startDST), resolve(value.end, value.endDST)])
  if (new Date(start).getTime() >= new Date(end).getTime()) throw new Error('Coverage must end after it starts.')
  return { start, end }
}
function WindowFields({ value, change, id, timezone }: { value: WallWindow; change: (value: WallWindow) => void; id: string; timezone: string }) {
  return <div className="form-grid">{(['start', 'end'] as const).map(part => <div key={part}><label htmlFor={`${id}-${part}`}>Coverage {part} ({timezone})</label><input id={`${id}-${part}`} type="datetime-local" step="1" value={value[part]} onChange={e => change({ ...value, [part]: e.target.value })} /><label htmlFor={`${id}-${part}-dst`}>Repeated {part} time</label><select id={`${id}-${part}-dst`} value={part === 'start' ? value.startDST : value.endDST} onChange={e => change({ ...value, [part === 'start' ? 'startDST' : 'endDST']: e.target.value })}><option value="">Not ambiguous</option><option value="earlier">Earlier occurrence</option><option value="later">Later occurrence</option></select></div>)}</div>
}
export function MentorProfileEditor({ initial, event, disabled, cancel, save }: { initial: MentorProfile; event: EventRecord; disabled: boolean; cancel: () => void; save: Save }) {
  const identity = useIdentity(), [skills, setSkills] = useState(initial.skills.join(', ')), [maximum, setMaximum] = useState(initial.maximum_teams?.toString() ?? '')
  const [windows, setWindows] = useState(initial.availability.map(w => initialWindow(w, event.configuration.timezone))), [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>()
  async function submit() {
    setPending(true); setFailure(undefined)
    try { const availability = await Promise.all(windows.map(w => resolveWindow(w, event.configuration.timezone, identity.csrf_token))); await save({ action: 'profile', person_id: initial.person_id, skills: skills.split(','), availability, maximum_teams: maximum ? Number(maximum) : null }) } catch (error) { setFailure(error) } finally { setPending(false) }
  }
  const invalid = !!maximum && (!Number.isInteger(Number(maximum)) || Number(maximum) < 1 || Number(maximum) > 10000)
  return <form className="panel section-heading" aria-label="Mentor profile editor" onSubmit={e => { e.preventDefault(); void submit() }}><h3>Declared availability for {initial.name}</h3><fieldset disabled={disabled || pending}><label htmlFor="mentor-skills">Declared skills</label><input id="mentor-skills" value={skills} onChange={e => setSkills(e.target.value)} /><label htmlFor="mentor-personal-capacity">Personal maximum assigned teams (optional)</label><input id="mentor-personal-capacity" type="number" min={1} max={10000} value={maximum} onChange={e => setMaximum(e.target.value)} /><p>Empty uses the event default. Record actual available intervals; missing availability is unknown.</p>{windows.map((w, i) => <section key={i}><WindowFields id={`availability-${i}`} timezone={event.configuration.timezone} value={w} change={next => setWindows(windows.map((old, j) => j === i ? next : old))} /><Button type="button" variant="outline" onClick={() => setWindows(windows.filter((_, j) => j !== i))}>Remove interval {i + 1}</Button></section>)}<Button type="button" variant="outline" disabled={windows.length >= 1000} onClick={() => setWindows([...windows, initialWindow(event.configuration.live_start && event.configuration.live_end ? { start: event.configuration.live_start, end: event.configuration.live_end } : { start: '', end: '' }, event.configuration.timezone)])}>Add availability interval</Button><p>Nonexistent or repeated local times must be resolved before saving.</p><Button type="submit" disabled={invalid || windows.some(w => !w.start || !w.end)}>Save declared availability</Button></fieldset><Button type="button" variant="outline" disabled={pending} onClick={cancel}>Cancel mentor edit</Button>{!!failure && (failure instanceof Error && !(failure instanceof APIError) ? <p role="alert">{failure.message}</p> : <Failure error={failure} />)}</form>
}
export function MentorAssignmentEditor({ event, value, people, disabled, cancel, save }: { event: EventRecord; value: MentorPage; people: components['schemas']['HackTeam'][]; disabled: boolean; cancel: () => void; save: Save }) {
  const identity = useIdentity(), [team, setTeam] = useState(''), [person, setPerson] = useState(''), [slot, setSlot] = useState<MentorAssignment['slot']>('primary')
  const [window, setWindow] = useState(initialWindow(value.live, event.configuration.timezone)), [preview, setPreview] = useState<{ assignment?: MentorAssignment }>(), [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>()
  async function inspect() {
    setPending(true); setFailure(undefined)
    try { setPreview(person ? { assignment: { team_id: team, person_id: person, slot, window: await resolveWindow(window, event.configuration.timezone, identity.csrf_token) } } : {}) } catch (error) { setFailure(error) } finally { setPending(false) }
  }
  const proposed = preview ? replaceSlot(value.draft, team, slot, preview.assignment) : value.draft
  const selectedTeam = people.find(t => t.id === team), selectedMentor = value.profiles.find(p => p.person_id === person)
  const memberIDs = new Set([...(selectedTeam?.draft ?? []), ...(selectedTeam?.published ?? [])].map(p => p.id))
  const conflict = !!person && (memberIDs.has(person) || value.conflicts.some(c => c.mentor_id === person && memberIDs.has(c.competitor_id)))
  const other = value.draft.find(a => a.team_id === team && a.slot !== slot)
  const duplicate = !!person && other?.person_id === person
  const overCapacity = !!selectedMentor && workload(proposed, person) > (selectedMentor.maximum_teams ?? value.default_capacity)
  const outsideLive = !!preview?.assignment && (new Date(preview.assignment.window.start).getTime() < new Date(value.live.start).getTime() || new Date(preview.assignment.window.end).getTime() > new Date(value.live.end).getTime())
  const unavailable = !!preview?.assignment && !!selectedMentor && !declaredAvailabilityCovers(selectedMentor.availability, preview.assignment.window)
  const partial = slot === 'primary' && !!preview?.assignment && (new Date(preview.assignment.window.start).getTime() > new Date(value.live.start).getTime() || new Date(preview.assignment.window.end).getTime() < new Date(value.live.end).getTime())
  const names = new Map(value.profiles.map(p => [p.person_id, p.name]))
  return <form className="panel section-heading" aria-label="Mentor assignment editor" onSubmit={e => { e.preventDefault(); void inspect() }}><h3>Review a mentor slot change</h3><fieldset disabled={disabled || pending}>
    <label htmlFor="mentor-team">Team receiving coverage</label><select id="mentor-team" value={team} onChange={e => { setTeam(e.target.value); setPreview(undefined) }}><option value="">Choose a team</option>{value.teams.map(t => <option key={t.id} value={t.id}>{t.name} · {t.id.slice(0, 8)}…{t.id.slice(-4)}</option>)}</select>
    <label htmlFor="mentor-slot">Coverage slot</label><select id="mentor-slot" value={slot} onChange={e => { setSlot(e.target.value as typeof slot); setPreview(undefined) }}><option value="primary">Primary mentor</option><option value="second">Second reviewer</option></select>
    <label htmlFor="assigned-mentor">Mentor for this slot</label><select id="assigned-mentor" value={person} onChange={e => { setPerson(e.target.value); setPreview(undefined) }}><option value="">Uncovered / clear slot</option>{value.profiles.filter(p => p.active).map(p => <option key={p.person_id} value={p.person_id}>{p.name} · {p.person_id.slice(0, 8)}…{p.person_id.slice(-4)}</option>)}</select>
    {person && <WindowFields id="assignment" value={window} change={next => { setWindow(next); setPreview(undefined) }} timezone={event.configuration.timezone} />}
    {conflict && <p role="alert">A declared reporting or own-team conflict blocks this assignment.</p>}{duplicate && <p role="alert">Choose a second reviewer distinct from the primary mentor.</p>}
    <Button type="submit" disabled={!team || conflict || duplicate || (!!person && (!window.start || !window.end))}>Preview mentor change</Button>
    {preview && <section aria-label="Mentor change preview"><h4>Workload and coverage preview</h4><p>{value.teams.find(t => t.id === team)?.name} · {slotLabel(slot)}: {names.get(person) ?? 'Uncovered'}{preview.assignment && ` · ${localDate(preview.assignment.window.start, event.configuration.timezone)}–${localDate(preview.assignment.window.end, event.configuration.timezone)}`}</p><ul>{value.profiles.map(p => <li key={p.person_id}>{p.name}: {workload(value.draft, p.person_id)} → {workload(proposed, p.person_id)} / {p.maximum_teams ?? value.default_capacity} teams</li>)}</ul>{overCapacity && <p role="alert">Over capacity. Choose another mentor or review their maximum workload.</p>}{outsideLive && <p role="alert">Coverage must stay within the live hack dates.</p>}{unavailable && <p role="alert">Declared availability does not cover the complete selected interval.</p>}{partial && <p role="status">Partial primary coverage leaves a gap. Publication requires a recorded organiser exception.</p>}{!person && <p>Clearing this slot creates a coverage gap. Publication requires a recorded exception.</p>}<Button type="button" disabled={overCapacity || conflict || duplicate || outsideLive || unavailable} onClick={() => void save({ action: 'assign', team_id: team, slot, ...(preview.assignment ? { person_id: person, window: preview.assignment.window } : {}) })}>Save mentor draft change</Button></section>}
  </fieldset><Button type="button" variant="outline" disabled={pending} onClick={cancel}>Cancel mentor edit</Button>{!!failure && (failure instanceof Error && !(failure instanceof APIError) ? <p role="alert">{failure.message}</p> : <Failure error={failure} />)}</form>
}

export function MentorCapacityEditor({ initial, disabled, cancel, save }: { initial: number; disabled: boolean; cancel: () => void; save: Save }) {
  const [capacity, setCapacity] = useState(initial.toString())
  return <form className="panel section-heading" aria-label="Mentor capacity editor" onSubmit={e => { e.preventDefault(); void save({ action: 'settings', default_capacity: Number(capacity) }) }}><h3>Event mentor capacity</h3><label htmlFor="mentor-capacity">Default maximum assigned teams per mentor</label><input id="mentor-capacity" type="number" min={1} max={10000} value={capacity} disabled={disabled} onChange={e => setCapacity(e.target.value)} /><p>A personal override takes precedence. Reducing capacity below published workload is blocked.</p><Button type="submit" disabled={disabled || !Number.isInteger(Number(capacity)) || Number(capacity) < 1 || Number(capacity) > 10000}>Save mentor capacity</Button><Button type="button" variant="outline" onClick={cancel}>Cancel mentor edit</Button></form>
}
export function MentorTeamSkillsEditor({ initial, disabled, cancel, save }: { initial: MentorPage['teams'][number]; disabled: boolean; cancel: () => void; save: Save }) {
  const [skills, setSkills] = useState(initial.skills.join(', '))
  return <form className="panel section-heading" aria-label="Team skill editor" onSubmit={e => { e.preventDefault(); void save({ action: 'team_skills', team_id: initial.id, skills: skills.split(',') }) }}><h3>Declared team skills for {initial.name}</h3><label htmlFor="mentor-team-skills">Skills requested for team coverage</label><input id="mentor-team-skills" value={skills} disabled={disabled} onChange={e => setSkills(e.target.value)} /><p>Use declared needs. Names and private operational information do not determine these skills.</p><Button type="submit" disabled={disabled}>Save team skill needs</Button><Button type="button" variant="outline" onClick={cancel}>Cancel mentor edit</Button></form>
}

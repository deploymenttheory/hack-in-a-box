import { taskStatuses, taskRoles, words, ownerName, type PreparationTask, type Owner } from './preparation-model'
import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, privateKey, unwrap, type EventRecord } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { localDate } from './event-time'
import { TaskEditor } from './preparation-editor'

function newTask(): PreparationTask {
  return { definition: { id: crypto.randomUUID(), title: '', description: '', owner_id: null, owner_role: '', status: 'not_started', schedule_id: null, fixed_due_at: null, depends_on: [], checklist: [], evidence: [] }, reason: '', override_dependencies: false, revision: 0, due_at: null, scheduling_issue: false, blocked: false, overdue: false, unowned: true, can_edit: true, restricted_evidence_count: 0, hidden_dependencies: 0 }
}

export function PreparationBoard({ event, onDirty, openSection }: { event: EventRecord; onDirty: (dirty: boolean) => void; openSection: (section: string) => void }) {
  const identity = useIdentity()
  const [view, setView] = useState('board'), [status, setStatus] = useState(''), [owner, setOwner] = useState(''), [role, setRole] = useState(''), [flag, setFlag] = useState('')
  const [selected, setSelected] = useState<{ task: PreparationTask; revision: number; selection: string }>(), [dirty, setDirty] = useState(false)
  const query = useQuery({ queryKey: privateKey(identity.person.id, event.id, 'preparation'), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/preparation/tasks', { params: { path: { event_id: event.id } } })), refetchOnWindowFocus: 'always' })
  const staff = useQuery({ queryKey: privateKey(identity.person.id, event.id, 'staff'), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/staff', { params: { path: { event_id: event.id } } })) })
  const teams = useQuery({ queryKey: privateKey(identity.person.id, event.id, 'teams'), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/teams', { params: { path: { event_id: event.id } } })) })
  useEffect(() => { onDirty(dirty); return () => onDirty(false) }, [dirty, onDirty])
  if (query.isPending) return <Loading />
  if (query.error) return <Failure error={query.error} retry={() => void query.refetch()} />
  const page = query.data
  const owners: Owner[] = [...new Map([...(staff.data?.items.filter(p => p.active).map(p => ({ id: p.person_id, name: p.display_name })) ?? []), ...(teams.data?.people ?? [])].map(p => [p.id, p])).values()]
  const items = page.items.filter(t => (!status || t.definition.status === status) && (!owner || t.definition.owner_id === owner) && (!role || t.definition.owner_role === role) && (!flag || (flag === 'overdue' ? t.overdue : flag === 'blocked' ? t.blocked : t.unowned)))
  function choose(task: PreparationTask) {
    if (!dirty || window.confirm('Leave this task with unsaved changes?')) { setSelected({ task, revision: page.revision, selection: crypto.randomUUID() }); setDirty(false) }
  }
  function navigate(section: string) { if (!dirty || window.confirm('Leave this task with unsaved changes?')) openSection(section) }
  return <section className="preparation-workspace" aria-label="Event preparation"><div className="preparation-heading"><div><h2>Preparation</h2><p>Assign the work, resolve dependencies and keep the event ready.</p></div>{page.can_manage && <Button onClick={() => choose(newTask())}>New task</Button>}</div>
    <Readiness event={event} openSection={navigate} />
    <div className="actions" aria-label="Preparation views">{['board', 'list', 'run sheet'].map(v => <Button key={v} variant={view === v ? 'default' : 'outline'} aria-pressed={view === v} onClick={() => setView(v)}>{v[0].toUpperCase() + v.slice(1)}</Button>)}</div>
    <div className="preparation-filters"><div><label htmlFor="task-filter-status">Filter by status</label><select id="task-filter-status" value={status} onChange={e => setStatus(e.target.value)}><option value="">All statuses</option>{taskStatuses.map(s => <option key={s} value={s}>{words(s)}</option>)}</select></div>
      <div><label htmlFor="task-filter-owner">Filter by owner</label><select id="task-filter-owner" value={owner} onChange={e => setOwner(e.target.value)}><option value="">All owners</option>{owners.map(p => <option key={p.id} value={p.id}>{p.name}</option>)}</select></div>
      <div><label htmlFor="task-filter-role">Filter by role</label><select id="task-filter-role" value={role} onChange={e => setRole(e.target.value)}><option value="">All roles</option>{taskRoles.map(r => <option key={r}>{r}</option>)}</select></div>
      <div><label htmlFor="task-filter-attention">Needs attention</label><select id="task-filter-attention" value={flag} onChange={e => setFlag(e.target.value)}><option value="">All tasks</option><option value="overdue">Overdue</option><option value="blocked">Blocked</option><option value="unowned">Unowned</option></select></div></div>
    {(staff.error || teams.error) && <p role="alert">Some owner names could not be loaded. Existing assignments are retained.</p>}
    <p role="status">{items.length} of {page.items.length} tasks · Times in {page.timezone}</p>
    {view === 'run sheet' ? <RunSheet event={event} tasks={items} owners={owners} choose={choose} /> : view === 'board' ? <div className="preparation-columns">{taskStatuses.map(s => <section key={s} aria-label={`${words(s)} tasks`}><h3>{words(s)} <span className="badge">{items.filter(t => t.definition.status === s).length}</span></h3>{items.filter(t => t.definition.status === s).map(t => <TaskCard key={t.definition.id} task={t} all={page.items} owners={owners} timezone={page.timezone} choose={choose} />)}</section>)}</div> : <div className="preparation-list">{items.map(t => <TaskCard key={t.definition.id} task={t} all={page.items} owners={owners} timezone={page.timezone} choose={choose} />)}</div>}
    {!page.items.length && <p className="notice">No preparation tasks yet. Create a task to assign the next piece of work.</p>}
    {selected && <TaskEditor key={selected.selection} event={event} initial={selected.task} revision={selected.revision} page={page} owners={owners} onDirty={setDirty} close={() => { if (!dirty || window.confirm('Leave this task with unsaved changes?')) setSelected(undefined) }} />}
  </section>
}

function TaskCard({ task, all, owners, timezone, choose }: { task: PreparationTask; all: PreparationTask[]; owners: Owner[]; timezone: string; choose: (t: PreparationTask) => void }) {
  return <article className="preparation-card"><h4>{task.definition.title}</h4><p>{ownerName(task, owners)}</p><p>{localDate(task.due_at, timezone)}</p><div className="task-flags"><span className="badge">{words(task.definition.status)}</span>{task.overdue && <span className="badge">Overdue</span>}{task.blocked && <span className="badge">Blocked</span>}{task.unowned && <span className="badge">Unowned</span>}{task.scheduling_issue && <span className="badge">Date needs attention</span>}</div>
    {!!task.definition.depends_on.length && <p>Depends on: {task.definition.depends_on.map(id => all.find(t => t.definition.id === id)?.definition.title ?? 'Unavailable task').join(', ')}</p>}{task.hidden_dependencies > 0 && <p>{task.hidden_dependencies} restricted dependencies</p>}
    <Button variant="outline" onClick={() => choose(task)}>{task.can_edit ? 'Edit' : 'View'} {task.definition.title}</Button></article>
}

function Readiness({ event, openSection }: { event: EventRecord; openSection: (section: string) => void }) {
  const c = event.configuration
  const destination = (field: string) => field.includes('team') ? 'teams' : ['minimum_attendance', 'confirmed_attendees', 'attendance'].includes(field) ? 'applicants' : ['admission_cutoff', 'team_freeze', 'live_start', 'submission_cutoff', 'challenge_publication'].includes(field) && event.can_edit ? 'setup' : 'overview'
  return <details className="panel readiness" open><summary>Event readiness and milestones</summary><p><strong>{event.confirmed_attendees} confirmed competitors · Minimum {c.minimum_attendance}</strong></p><p>Task completion and waivers do not change the go decision or event gates.</p><dl className="preparation-milestones">{[
    ['Final admission cutoff', c.admission_cutoff], ['Team freeze', c.team_freeze], ['Common challenge publication', event.challenge_publication], ['Live start', c.live_start], ['Submission cutoff', c.submission_cutoff],
  ].map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{localDate(value, c.timezone)}</dd></div>)}</dl>
    {event.blockers.length ? <ul className="blockers">{event.blockers.map((b, i) => <li key={i}><span>{b.message}</span><Button variant="outline" onClick={() => openSection(destination(b.field))}>Review {words(b.field)}</Button></li>)}</ul> : <p>No blockers reported for the next stage.</p>}<Button variant="outline" onClick={() => openSection('overview')}>Review go decision and stage</Button></details>
}

function RunSheet({ event, tasks, owners, choose }: { event: EventRecord; tasks: PreparationTask[]; owners: Owner[]; choose: (t: PreparationTask) => void }) {
  const identity = useIdentity()
  const schedule = useQuery({ queryKey: privateKey(identity.person.id, event.id, 'schedule'), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/schedule', { params: { path: { event_id: event.id } } })) })
  const rows = [...tasks.map(t => ({ id: t.definition.id, title: t.definition.title, due: t.due_at, owner: ownerName(t, owners), state: words(t.definition.status), task: t })), ...(schedule.data?.items.map(s => ({ id: s.item.id, title: s.item.title, due: s.due_at, owner: `Audience: ${s.item.audience.join(', ')}`, state: words(s.status), task: undefined })) ?? [])].sort((a, b) => (a.due || '9999').localeCompare(b.due || '9999'))
  return <section className="section-heading" aria-label="Run sheet"><h3>Run sheet · {event.configuration.timezone}</h3><p>Task filters apply to tasks; the event agenda and milestones remain visible.</p>{schedule.isPending ? <Loading /> : schedule.error ? <Failure error={schedule.error} retry={() => void schedule.refetch()} /> : null}<ol className="run-sheet">{rows.map(row => <li key={row.id}><time>{localDate(row.due, event.configuration.timezone)}</time><div><strong>{row.title}</strong><p>{row.owner} · {row.state}</p>{row.task && <><Button variant="outline" onClick={() => choose(row.task!)}>Open {row.title}</Button><EvidenceLinks eventID={event.id} items={row.task.definition.evidence} /></>}</div></li>)}</ol></section>
}

export function EvidenceLinks({ eventID, items }: { eventID: string; items: components['schemas']['PreparationEvidence'][] }) {
  return <ul className="evidence-links">{items.map(e => <li key={e.id}><a href={e.kind === 'file' ? `/api/v1/events/${eventID}/files/${e.file_id}` : e.url} target="_blank" rel="noopener noreferrer">{e.label}</a> <small>({e.visibility === 'organisers' ? 'organisers only' : 'assignees'})</small></li>)}</ul>
}

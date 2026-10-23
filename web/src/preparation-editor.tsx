import { PlanningEditorSections, PlanningStep, revealPlanningField, firstPlanningError } from './planning-layout'
import { Input, Textarea, Select, Label } from './components/ui/fields'
import { useEffect, useRef, useState } from 'react'
import { useForm, useFieldArray, useWatch } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useBlocker } from '@tanstack/react-router'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, APIError, privateKey, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure } from './components/states'
import { localDate, wallTime } from './event-time'
import { taskRoles, taskStatuses, words, ownerName, type PreparationTask, type PreparationPage, type Owner } from './preparation-model'
import { TaskEvidence } from './preparation-evidence'

const schema = z.object({
  definition: z.object({ id: z.uuid(), title: z.string().trim().min(1, 'Enter a task title.').max(160), description: z.string().max(4000), owner_id: z.uuid().nullable(), owner_role: z.enum(['', ...taskRoles]), status: z.enum(taskStatuses), schedule_id: z.uuid().nullable(), fixed_due_at: z.string().nullable(), depends_on: z.array(z.uuid()).max(200), checklist: z.array(z.object({ id: z.uuid(), title: z.string().trim().min(1).max(300), complete: z.boolean() })).max(100), evidence: z.array(z.object({ id: z.uuid(), kind: z.enum(['url', 'repository', 'file']), label: z.string().trim().min(1).max(300), url: z.string().max(2048), file_id: z.uuid().nullable(), visibility: z.enum(['organisers', 'assignees']) })).max(30) }),
  reason: z.string().max(2000), override_dependencies: z.boolean(), due: z.string(), dst_choice: z.enum(['', 'earlier', 'later']),
}).superRefine((v, ctx) => {
  if ((['blocked', 'complete', 'waived'].includes(v.definition.status) || v.override_dependencies) && !v.reason.trim()) ctx.addIssue({ code: 'custom', path: ['reason'], message: 'Record a reason for blocking, completion, a waiver or an override.' })
  v.definition.evidence.forEach((e, i) => {
    if (e.kind === 'file') return
    try { const url = new URL(e.url); if (!['https:', 'http:'].includes(url.protocol) || url.username || url.password) throw new Error() } catch { ctx.addIssue({ code: 'custom', path: ['definition', 'evidence', i, 'url'], message: 'Use an HTTP or HTTPS link without embedded credentials.' }) }
  })
})
export type TaskValues = z.infer<typeof schema>
function values(task: PreparationTask, timezone: string): TaskValues {
  return { definition: task.definition, reason: '', override_dependencies: false, due: wallTime(task.definition.fixed_due_at ?? undefined, timezone), dst_choice: '' }
}

export function TaskEditor({ event, initial, revision, page, owners, onDirty, close, requireAssignment = false }: { requireAssignment?: boolean; event: EventRecord; initial: PreparationTask; revision: number; page: PreparationPage; owners: Owner[]; onDirty: (dirty: boolean) => void; close: () => void }) {
  const identity = useIdentity(), cache = useQueryClient()
  const [baseRevision, setBaseRevision] = useState(revision), [saved, setSaved] = useState(initial), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState('')
  const [latest, setLatest] = useState<PreparationPage>(), [ack, setAck] = useState(false), [uploading, setUploading] = useState(false), [reviewing, setReviewing] = useState(false)
  const form = useForm<TaskValues>({ defaultValues: values(initial, page.timezone), resolver: zodResolver(schema) })
  const definition = useWatch({ control: form.control, name: 'definition' })
  const checklist = useFieldArray({ control: form.control, name: 'definition.checklist', keyName: 'key' })
  const dirty = form.formState.isDirty
  const current = page.items.find(t => t.definition.id === initial.definition.id)
  const canEdit = event.can_edit && (current?.can_edit ?? page.can_manage)
  const conflict = failure instanceof APIError && failure.code === 'revision_conflict'
  const busy = form.formState.isSubmitting || uploading || reviewing
  const editor = useRef<HTMLFormElement>(null)
  useEffect(() => { editor.current?.focus(); editor.current?.scrollIntoView({ block: 'start' }) }, [])
  useEffect(() => { onDirty(dirty || uploading); return () => onDirty(false) }, [dirty, uploading, onDirty])
  useBlocker({ shouldBlockFn: () => (dirty || uploading) && !window.confirm('Leave this task with unsaved changes?'), enableBeforeUnload: dirty || uploading })
  async function reviewLatest() {
    setReviewing(true)
    try {
      const fresh = unwrap(await api.GET('/api/v1/events/{event_id}/preparation/tasks', { params: { path: { event_id: event.id } } }))
      cache.setQueryData(privateKey(identity.person.id, event.id, 'preparation'), fresh)
      setLatest(fresh); setAck(false)
    } catch (error) { setFailure(error) } finally { setReviewing(false) }
  }
  async function save(v: TaskValues) {
    if (!canEdit || (conflict && (!latest || !ack))) return
    setNotice('')
    if ((requireAssignment || (v.definition.owner_role === 'logistics' && v.definition.status === 'blocked')) && (!v.definition.owner_id || (!v.definition.schedule_id && !v.due))) { form.setError('root', { message: 'Assign a named owner and a deadline for this operational checklist.' }); return }
    try {
      let due = v.definition.fixed_due_at
      if (!v.definition.schedule_id && v.due !== wallTime(v.definition.fixed_due_at ?? undefined, page.timezone)) {
        due = v.due ? unwrap(await api.POST('/api/v1/time/resolve', { params: { header: { 'X-CSRF-Token': identity.csrf_token } }, body: { timezone: page.timezone, wall_time: v.due.length === 16 ? v.due + ':00' : v.due, dst_choice: v.dst_choice } })).instant : null
      }
      const result = unwrap(await api.PUT('/api/v1/events/{event_id}/preparation/tasks/{task_id}', { params: { path: { event_id: event.id, task_id: initial.definition.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${conflict && latest ? latest.revision : baseRevision}"` } }, body: { definition: { ...v.definition, fixed_due_at: v.definition.schedule_id ? null : due }, reason: v.reason, override_dependencies: v.definition.status === 'complete' && v.override_dependencies } }))
      const updated = result.items.find(t => t.definition.id === initial.definition.id)!
      setSaved(updated); setBaseRevision(result.revision); form.reset(values(updated, result.timezone)); setFailure(undefined); setLatest(undefined); setAck(false); setNotice('Task saved.')
      cache.setQueryData(privateKey(identity.person.id, event.id, 'preparation'), result)
      await Promise.all([cache.invalidateQueries({ queryKey: privateKey(identity.person.id, event.id, 'event') }), cache.invalidateQueries({ queryKey: privateKey(identity.person.id, event.id, `preparation-history-${initial.definition.id}`) })])
    } catch (error) { setFailure(error); setLatest(undefined); setAck(false); if (error instanceof APIError && error.status === 403) await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, event.id, 'preparation') }) }
  }
  return <form ref={editor} tabIndex={-1} className="fleet-planning-form fleet-planning-section" aria-label="Task editor" onSubmit={event=>void form.handleSubmit(save,errors=>revealPlanningField(editor.current,firstPlanningError(errors)))(event)}><div className="preparation-heading"><h3>{saved.revision ? `Task · ${saved.definition.title}` : 'New preparation task'}</h3><Button type="button" variant="outline" disabled={busy} onClick={close}>Close task</Button></div>{!canEdit && <p className="notice">This task is read-only. Your draft remains visible.</p>}
    <PlanningEditorSections label="Preparation task sections">
    <PlanningStep title="Task and ownership"><fieldset disabled={!canEdit || busy}><legend>Task and ownership</legend><div className="form-grid">
      <div className="span-all"><Label htmlFor="task-title">Task title</Label><Input id="task-title" maxLength={160} {...form.register('definition.title')} /></div><div className="span-all"><Label htmlFor="task-description">Description</Label><Textarea id="task-description" maxLength={4000} {...form.register('definition.description')} /></div>
      <div><Label htmlFor="task-owner">Named owner</Label><Select id="task-owner" {...form.register('definition.owner_id', { setValueAs: v => v || null })}><option value="">No named owner</option>{definition.owner_id && !owners.some(p => p.id === definition.owner_id) && <option value={definition.owner_id}>Previous assignee (retained)</option>}{owners.map(p => <option value={p.id} key={p.id}>{p.name}</option>)}</Select></div>
      <div><Label htmlFor="task-role">Assigned role</Label><Select id="task-role" {...form.register('definition.owner_role')}><option value="">No role requirement</option>{taskRoles.map(r => <option key={r}>{r}</option>)}</Select></div>
      <div><Label htmlFor="task-status">Task status</Label><Select id="task-status" {...form.register('definition.status')}>{taskStatuses.map(s => <option key={s} value={s}>{words(s)}</option>)}</Select></div></div><p className="muted">A named owner must currently hold the assigned role. Attendees must have a confirmed place.</p></fieldset></PlanningStep>
    <PlanningStep title="Due time"><fieldset disabled={!canEdit || busy}><legend>Due time · {page.timezone}</legend><Label htmlFor="task-schedule">Follow a schedule item</Label><Select id="task-schedule" {...form.register('definition.schedule_id', { setValueAs: v => v || null })}><option value="">Use a fixed date or leave unscheduled</option>{definition.schedule_id && !event.configuration.schedule.some(s => s.id === definition.schedule_id) && <option value={definition.schedule_id}>Missing schedule item — choose a replacement</option>}{event.configuration.schedule.map(s => <option value={s.id} key={s.id}>{s.title}</option>)}</Select>
      {!definition.schedule_id && <div className="form-grid"><div><Label htmlFor="task-due">Fixed due date</Label><Input id="task-due" type="datetime-local" step="1" {...form.register('due')} /></div><div><Label htmlFor="task-dst">When this time occurs twice</Label><Select id="task-dst" {...form.register('dst_choice')}><option value="">Ask me to choose</option><option value="earlier">Earlier occurrence</option><option value="later">Later occurrence</option></Select></div></div>}<p className="muted">Schedule-linked dates follow event changes. Fixed dates stay fixed.</p></fieldset></PlanningStep>
    <PlanningStep title="Dependencies"><fieldset disabled={!canEdit || busy}><legend>Dependencies</legend>{page.items.filter(t => t.definition.id !== initial.definition.id).map(t => <Label className="check" key={t.definition.id}><Input type="checkbox" value={t.definition.id} {...form.register('definition.depends_on')} />{t.definition.title} · {words(t.definition.status)}</Label>)}{page.items.filter(t => t.definition.id !== initial.definition.id).length === 0 && <p>No other tasks yet.</p>}</fieldset></PlanningStep>
    <PlanningStep title="Checklist"><fieldset disabled={!canEdit || busy}><legend>Checklist</legend>{checklist.fields.map((item, i) => <div className="checklist-row" key={item.key}><div><Label htmlFor={`check-title-${item.id}`}>Checklist item {i + 1}</Label><Input id={`check-title-${item.id}`} maxLength={300} {...form.register(`definition.checklist.${i}.title`)} /></div><Label className="check"><Input type="checkbox" {...form.register(`definition.checklist.${i}.complete`)} />Done</Label><Button type="button" variant="outline" aria-label={`Remove checklist item ${i + 1}`} onClick={() => checklist.remove(i)}>Remove</Button></div>)}<Button type="button" variant="outline" disabled={checklist.fields.length >= 100} onClick={() => checklist.append({ id: crypto.randomUUID(), title: '', complete: false })}>Add checklist item</Button></fieldset></PlanningStep>
    <PlanningStep title="Evidence"><TaskEvidence form={form} eventID={event.id} revision={conflict && latest && ack ? latest.revision : baseRevision} reportFailure={setFailure} disabled={!canEdit || form.formState.isSubmitting || reviewing} uploading={uploading} setUploading={setUploading} /></PlanningStep>
    <PlanningStep title="Decision record"><fieldset disabled={!canEdit || busy}><legend>Decision record</legend><Label htmlFor="task-reason">Reason for this change</Label><Textarea id="task-reason" maxLength={2000} {...form.register('reason')} /><p>Required for blocking, completion, waivers and overrides. A task waiver does not change an event gate.</p>{definition.status === 'complete' && <Label className="check"><Input type="checkbox" {...form.register('override_dependencies')} />Override unfinished dependencies or checklist items with this reason</Label>}</fieldset></PlanningStep>
    </PlanningEditorSections>
    {saved.restricted_evidence_count > 0 && <p>{saved.restricted_evidence_count} restricted evidence items are hidden.</p>}
    {Object.keys(form.formState.errors).length > 0 && <div role="alert" className="field-error">{form.formState.errors.root?.message ?? form.formState.errors.reason?.message ?? 'Check the task title, checklist labels and evidence links before saving.'}</div>}
    {conflict ? <section className="fleet-planning-section" role="alert"><h3>Another change was saved</h3><p>Your draft is preserved. Review the latest saved version before applying it again.</p><Button type="button" variant="outline" disabled={busy} onClick={() => void reviewLatest()}>Review latest version</Button>{latest && <><SavedComparison all={latest.items} task={latest.items.find(t => t.definition.id === initial.definition.id)} event={event} owners={owners} /><Label className="check"><Input type="checkbox" checked={ack} onChange={e => setAck(e.target.checked)} />I reviewed the latest version and want to apply my draft.</Label></>}</section> : !!failure && <Failure error={failure} />}
    <div className="actions"><Button type="submit" disabled={!canEdit || busy || (conflict && (!latest || !ack))}>Save task</Button></div>{notice && <p role="status">{notice}</p>}
    {saved.revision > 0 && <TaskHistory event={event} taskID={initial.definition.id} owners={owners} />}
  </form>
}

function SavedComparison({ task, all, event, owners }: { task?: PreparationTask; all: PreparationTask[]; event: EventRecord; owners: Owner[] }) {
  if (!task) return <p>This task has not been saved; other event data changed.</p>
  return <div aria-label="Latest saved task"><h4>{task.definition.title}</h4><p>{task.definition.description}</p><p>{ownerName(task, owners)} · {words(task.definition.status)} · {localDate(task.due_at, event.configuration.timezone)}</p><p>Reason: {task.reason || 'None recorded'} · Override: {task.override_dependencies ? 'Yes' : 'No'}</p>
    <p>Due rule: {task.definition.schedule_id ? event.configuration.schedule.find(s => s.id === task.definition.schedule_id)?.title ?? 'Missing schedule item' : task.definition.fixed_due_at ? 'Fixed date' : 'Unscheduled'}</p>
    <p>Dependencies: {task.definition.depends_on.map(id => all.find(t => t.definition.id === id)?.definition.title ?? 'Unavailable task').join(', ') || 'None'}</p>
    <ul>{task.definition.checklist.map(c => <li key={c.id}>{c.complete ? 'Done' : 'To do'}: {c.title}</li>)}</ul>
    <ul>{task.definition.evidence.map(e => <li key={e.id}>{e.label} · {e.kind} · {e.visibility === 'organisers' ? 'Organisers only' : 'Assignees'} {e.url && <span>{e.url}</span>}</li>)}</ul>
  </div>
}

function TaskHistory({ event, taskID, owners }: { event: EventRecord; taskID: string; owners: Owner[] }) {
  const identity = useIdentity(), [open, setOpen] = useState(false)
  const query = useQuery({ queryKey: privateKey(identity.person.id, event.id, `preparation-history-${taskID}`), enabled: open, queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/preparation/tasks/{task_id}/history', { params: { path: { event_id: event.id, task_id: taskID } } })) })
  return <details onToggle={e => setOpen(e.currentTarget.open)}><summary>Task history</summary>{query.error ? <Failure error={query.error} /> : query.isPending ? <p>Loading history…</p> : <ol className="history">{query.data.items.map(c => <li key={c.revision}><strong>{words(c.definition.status)}</strong> · {localDate(c.created_at, event.configuration.timezone)} · {owners.find(p => p.id === c.actor_id)?.name ?? 'Previous staff member'}<p>{c.reason || 'Task details updated.'}</p></li>)}</ol>}</details>
}

import { CollectionRecords, DetailWorkspace, Inspector } from './components/patterns'
import { Input, Textarea, Select, Label } from './components/ui/fields'
import { useEffect, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useBlocker } from '@tanstack/react-router'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { api, APIError, privateKey, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { localDate } from './event-time'
import { ownerName, words, type PreparationPage, type PreparationTask } from './preparation-model'

export function VenueReadiness({ event, summary = false, openSection, onDirty }: { event: EventRecord; summary?: boolean; openSection: (section: string) => void; onDirty?: (dirty: boolean) => void }) {
  const identity = useIdentity(), [selected, setSelected] = useState<{ task: PreparationTask; revision: number; key: string }>(), [dirty, setDirty] = useState(false)
  const query = useQuery({ queryKey: privateKey(identity.person.id, event.id, 'preparation'), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/preparation/tasks', { params: { path: { event_id: event.id } } })), refetchOnWindowFocus: 'always' })
  const staff = useQuery({ queryKey: privateKey(identity.person.id, event.id, 'staff'), enabled: event.can_read_staff, queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/staff', { params: { path: { event_id: event.id } } })) })
  useEffect(() => { onDirty?.(dirty); return () => onDirty?.(false) }, [dirty, onDirty])
  const owners = [{ id: identity.person.id, name: identity.person.display_name }, ...(staff.data?.items.filter(p => p.active).map(p => ({ id: p.person_id, name: p.display_name })) ?? [])]
  function leave() { return !dirty || window.confirm('Leave this checklist with unsaved changes?') }
  return <section className="fleet-planning-section venue-readiness" aria-label="Operational readiness"><h2>Venue, AV and catering</h2><p>Operational checklists retain their owners, deadlines and blockers in the preparation board.</p>
    {query.isPending ? <Loading /> : query.error ? <Failure error={query.error} retry={() => void query.refetch()} /> : <>{query.data.items.filter(t => t.definition.owner_role === 'logistics').length === 0 && <p>No logistics checklists are visible to you yet.</p>}{summary ? <ul className="fleet-planning-records">{query.data.items.filter(t => t.definition.owner_role === 'logistics').map(t => <li key={t.definition.id}><div><strong>{t.definition.title}</strong><p>Owner: {ownerName(t, owners)}</p><p>Due: {localDate(t.due_at, query.data.timezone)} ({query.data.timezone})</p><div className="task-flags"><span className="badge">{words(t.definition.status)}</span>{t.blocked && t.definition.status !== 'blocked' && <span className="badge">Blocked</span>}{t.overdue && <span className="badge">Overdue</span>}{!t.definition.owner_id && <span className="badge">Named owner required</span>}{!t.due_at && <span className="badge">Deadline required</span>}</div>{t.reason && <p>{t.reason}</p>}<p>{t.definition.checklist.filter(c => c.complete).length} of {t.definition.checklist.length} checks complete</p></div>{!summary && t.can_edit && <Button variant="outline" onClick={() => { if (leave()) { setSelected({ task: t, revision: query.data.revision, key: crypto.randomUUID() }); setDirty(false) } }}>Update {t.definition.title}</Button>}</li>)}</ul> : <DetailWorkspace collection={<CollectionRecords label="Operational readiness checklists" rows={query.data.items.filter(t=>t.definition.owner_role==='logistics')} rowKey={t=>t.definition.id} rowLabel={t=>t.definition.title} destination={t=>({label:t.definition.title,href:'#operational-status',onNavigate:()=>{if(leave()){setSelected({task:t,revision:query.data.revision,key:crypto.randomUUID()});setDirty(false)}}})} columns={[{id:'owner',label:'Owner',essential:true,cell:t=>ownerName(t,owners)},{id:'due',label:'Due',essential:true,cell:t=>localDate(t.due_at,query.data.timezone)},{id:'status',label:'Status',essential:true,cell:t=>words(t.definition.status)},{id:'attention',label:'Needs attention',cell:t=>[t.blocked&&'Blocked',t.overdue&&'Overdue',!t.definition.owner_id&&'Named owner required',!t.due_at&&'Deadline required',t.reason].filter(Boolean).join(' · ')||'No flags'},{id:'checks',label:'Checklist',cell:t=>t.definition.checklist.filter(c=>c.complete).length+' of '+t.definition.checklist.length+' checks complete'}]} actions={t=>t.can_edit&&<Button variant="ghost" icon="check" onClick={()=>{if(leave()){setSelected({task:t,revision:query.data.revision,key:crypto.randomUUID()});setDirty(false)}}}>Update {t.definition.title}</Button>} />} detail={selected&&<Inspector identity={selected.key} title={selected.task.definition.title} scope={{event:event.name,entity:'Operational checklist'}} back={{label:'Back to operational checklists',href:'#operations',onNavigate:()=>{if(leave()){setSelected(undefined);setDirty(false)}}}}><ChecklistProgress key={selected.key} event={event} initial={selected.task} revision={selected.revision} page={query.data} onDirty={setDirty} /></Inspector>} />}
</>}
    <div className="actions">{summary ? <Button variant="outline" onClick={() => openSection('operations')}>Review operational checklists</Button> : event.can_read_staff && <Button variant="outline" onClick={() => { if (leave()) openSection('preparation') }}>Assign owners, deadlines and checklists</Button>}</div>
  </section>
}

const progressSchema = z.object({ status: z.enum(['not_started', 'in_progress', 'blocked', 'complete', 'waived']), reason: z.string().max(2000), checklist: z.array(z.object({ id: z.uuid(), title: z.string(), complete: z.boolean() })) }).superRefine((v, ctx) => { if (['blocked', 'complete', 'waived'].includes(v.status) && !v.reason.trim()) ctx.addIssue({ code: 'custom', path: ['reason'], message: 'Record a reason for blocking, completion or a waiver.' }) })
type Progress = z.infer<typeof progressSchema>
function defaults(t: PreparationTask): Progress { return { status: t.definition.status, reason: '', checklist: t.definition.checklist } }
function ChecklistProgress({ event, initial, revision, page, onDirty }: { event: EventRecord; initial: PreparationTask; revision: number; page: PreparationPage; onDirty: (dirty: boolean) => void }) {
  const identity = useIdentity(), cache = useQueryClient(), editor = useRef<HTMLFormElement>(null)
  const [saved, setSaved] = useState(initial), [baseRevision, setBaseRevision] = useState(revision), [failure, setFailure] = useState<unknown>(), [latest, setLatest] = useState<{ task: PreparationTask; revision: number }>(), [ack, setAck] = useState(false), [notice, setNotice] = useState('')
  const form = useForm<Progress>({ defaultValues: defaults(initial), resolver: zodResolver(progressSchema) }), dirty = form.formState.isDirty
  const current = page.items.find(t => t.definition.id === initial.definition.id), canEdit = !!current?.can_edit
  const configured = !!current?.definition.owner_id && !!current?.due_at && !current?.scheduling_issue
  const conflict = failure instanceof APIError && failure.code === 'revision_conflict'
  useEffect(() => { editor.current?.focus(); editor.current?.scrollIntoView({ block: 'start' }) }, [])
  useEffect(() => { onDirty(dirty); return () => onDirty(false) }, [dirty, onDirty])
  useBlocker({ shouldBlockFn: () => dirty && !window.confirm('Leave this checklist with unsaved changes?'), enableBeforeUnload: dirty })
  async function refresh() {
    const fresh = unwrap(await api.GET('/api/v1/events/{event_id}/preparation/tasks', { params: { path: { event_id: event.id } } }))
    cache.setQueryData(privateKey(identity.person.id, event.id, 'preparation'), fresh)
    return fresh
  }
  async function review() { try { const fresh = await refresh(), task = fresh.items.find(t => t.definition.id === initial.definition.id); if (!task) throw new APIError(403, 'forbidden'); setLatest({ task, revision: fresh.revision }); setAck(false) } catch (error) { setFailure(error) } }
  async function save(v: Progress) {
    if (!canEdit || !configured || (conflict && (!latest || !ack))) return
    setNotice('')
    const base = conflict && latest ? latest.task : saved, expected = conflict && latest ? latest.revision : baseRevision
    const checks = new Map(v.checklist.map(c => [c.id, c.complete]))
    try {
      const result = unwrap(await api.PUT('/api/v1/events/{event_id}/preparation/tasks/{task_id}', { params: { path: { event_id: event.id, task_id: initial.definition.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${expected}"` } }, body: { definition: { ...base.definition, status: v.status, checklist: base.definition.checklist.map(c => ({ ...c, complete: checks.get(c.id) ?? c.complete })) }, reason: v.reason, override_dependencies: false } }))
      const task = result.items.find(t => t.definition.id === initial.definition.id)!
      setSaved(task); setBaseRevision(result.revision); form.reset(defaults(task)); setFailure(undefined); setLatest(undefined); setAck(false); setNotice('Checklist progress saved.'); cache.setQueryData(privateKey(identity.person.id, event.id, 'preparation'), result); await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, event.id, 'event') })
    } catch (error) { setFailure(error); setLatest(undefined); setAck(false); if (error instanceof APIError && error.status === 403) await refresh().catch(setFailure) }
  }
  return <form ref={editor} tabIndex={-1} className="fleet-planning-section" aria-label="Checklist progress" onSubmit={form.handleSubmit(save)}><h3>{saved.definition.title}</h3>{!configured && <p role="alert">An organiser must assign a named owner and a valid deadline before this checklist can progress.</p>}{!canEdit && <p className="notice">You can no longer update this checklist.</p>}<fieldset disabled={!canEdit || !configured || form.formState.isSubmitting}><legend>Progress</legend><Label htmlFor="operational-status">Checklist status</Label><Select id="operational-status" {...form.register('status')}>{['not_started', 'in_progress', 'blocked', 'complete', ...(page.can_manage || saved.definition.status === 'waived' ? ['waived'] : [])].map(status => <option key={status} value={status} disabled={status === 'waived' && !page.can_manage}>{words(status)}</option>)}</Select>{form.getValues('checklist').map((c, i) => <Label className="check" key={c.id}><Input type="checkbox" {...form.register(`checklist.${i}.complete`)} />{c.title}</Label>)}<Label htmlFor="operational-reason">Progress or blocker reason</Label><Textarea id="operational-reason" maxLength={2000} {...form.register('reason')} />{form.formState.errors.reason && <p role="alert">{form.formState.errors.reason.message}</p>}</fieldset>
    {conflict ? <div role="alert"><p>This checklist changed. Your progress edits are preserved.</p><Button type="button" variant="outline" onClick={() => void review()}>Review latest checklist</Button>{latest && <><p>Latest status: {words(latest.task.definition.status)} · Due {localDate(latest.task.due_at, page.timezone)}</p><ul>{latest.task.definition.checklist.map(c => <li key={c.id}>{c.title}: {c.complete ? 'Done' : 'To do'}</li>)}</ul><Label className="check"><Input type="checkbox" checked={ack} onChange={e => setAck(e.target.checked)} />Apply my progress after reviewing the latest checklist</Label></>}</div> : !!failure && <Failure error={failure} />}
    <div className="actions"><Button type="submit" disabled={!canEdit || !configured || form.formState.isSubmitting || (conflict && (!latest || !ack))}>Save checklist progress</Button></div>{notice && <p role="status">{notice}</p>}
  </form>
}

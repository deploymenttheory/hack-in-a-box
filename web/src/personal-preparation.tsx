import { useCommunicationCommands } from './communication-tests'
import { useTodayRead } from './today-read'
import type { PrivateRequestScope } from './private-request-scope'
import { useCallback, useEffect, useId, useLayoutEffect, useRef, useState } from 'react'
import { useBlocker } from '@tanstack/react-router'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, privateKey, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Input, Textarea, Select, Label, Checkbox } from './components/ui/fields'
import { ConfirmationDialog } from './components/ui/dialog'
import { StatusBadge } from './components/ui/feedback'
import { CollectionRecords, DetailWorkspace } from './components/patterns'
import { Failure, Loading } from './components/states'
import { EventGuidance } from './event-guidance'
import { localDate } from './event-time'
import { referenceURL, words } from './deliverable-model'
import { usePlanningActionFocus } from './planning-layout'
import { usePrivateRequestScope } from './private-request-scope'
import { resourceDenied, ResourceOperationResult, useResourceOperation } from './resource-operation'
import type { PreparationPage, PreparationTask } from './preparation-model'

const personallyAssigned = (task: PreparationTask, personID: string, roles: readonly string[]) => (!task.definition.owner_role || roles.includes(task.definition.owner_role)) && (task.definition.owner_id ? task.definition.owner_id === personID : !!task.definition.owner_role)
const preparationRoleSignature = (roles: readonly string[]) => roles.join(':')

export function PersonalPreparation({ event, onDirty }: { event: EventRecord; onDirty: (dirty: boolean) => void }) {
  const identity = useIdentity()
  return <PreparationAuthority key={`${identity.person.id}:${identity.draft_scope}:${event.id}`} event={event} onDirty={onDirty} />
}
function PreparationAuthority({ event, onDirty }: { event: EventRecord; onDirty: (dirty: boolean) => void }) {
  const identity = useIdentity(), cache = useQueryClient(), previousRoles = useRef<string | undefined>(undefined)
  const load = useCallback(async (signal: AbortSignal) => {
    const page = unwrap(await api.GET('/api/v1/events/{event_id}/today', { signal, cache: 'no-store', params: { path: { event_id: event.id } } }))
    if (page.event.id !== event.id) throw new Error('Preparation event identity changed')
    return [...page.roles, ...(page.participation_state === 'confirmed' ? ['attendee'] : [])].sort()
  }, [event.id])
  const authority = useTodayRead(`${identity.person.id}:${identity.draft_scope}:${event.id}:preparation-roles`, load, undefined, preparationRoleSignature)
  const signature = authority.data ? preparationRoleSignature(authority.data) : undefined
  useLayoutEffect(() => {
    const prefix = privateKey(identity.person.id, event.id, 'personal-preparation')
    if (authority.denied) { void cache.cancelQueries({ queryKey: prefix }); cache.removeQueries({ queryKey: prefix }) }
    else if (previousRoles.current !== undefined && signature !== undefined && previousRoles.current !== signature) {
      const key = [...prefix, previousRoles.current]; void cache.cancelQueries({ queryKey: key }); cache.removeQueries({ queryKey: key })
    }
    previousRoles.current = signature
  }, [authority.denied, signature, cache, identity.person.id, event.id])
  if (authority.denied || !authority.data && authority.error) return <Failure error={authority.error} retry={() => void authority.refresh()} />
  if (!authority.data) return <Loading />
  return <>{!!authority.error && <Failure error={authority.error} retry={() => void authority.refresh()} />}<PreparationLifetime key={authority.data.join(':')} event={event} onDirty={onDirty} roles={authority.data} parentScope={authority.authority} authorityPaused={!!authority.error || authority.pending} /></>
}
function PreparationLifetime({ event, onDirty, roles, parentScope, authorityPaused }: { event: EventRecord; onDirty: (dirty: boolean) => void; roles: readonly string[]; parentScope: PrivateRequestScope; authorityPaused: boolean }) {
  const identity = useIdentity(), cache = useQueryClient(), [selected, setSelected] = useState(''), [dirty, setDirty] = useState(false), [locked, setLocked] = useState(false), [denied, setDenied] = useState<unknown>(), focus = usePlanningActionFocus()
  const mount = useId()
  const assignmentSignature = useRef<string | undefined>(undefined)
  const scope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${event.id}:personal-preparation`, parentScope)
  useEffect(() => { onDirty(dirty || locked); return () => onDirty(false) }, [dirty, locked, onDirty])
  const preparationKey = [...privateKey(identity.person.id, event.id, 'personal-preparation'), preparationRoleSignature(roles), mount]
  const query = useQuery({ queryKey: preparationKey, gcTime: 0, retry: false, queryFn: async ({ signal }) => { const ticket = scope.capture(); let value: PreparationPage; try { value = unwrap(await api.GET('/api/v1/events/{event_id}/preparation/tasks', { signal, params: { path: { event_id: event.id } } })); ticket.assert() } catch (error) { if (resourceDenied(error)) scope.invalidate(); throw error }; const signature = JSON.stringify(value.items.filter(task => personallyAssigned(task, identity.person.id, roles)).map(task => [task.definition.id, task.definition.owner_id, task.definition.owner_role, task.can_edit]).sort()); if (assignmentSignature.current !== undefined && assignmentSignature.current !== signature) scope.renew(); assignmentSignature.current = signature; return value }, refetchOnWindowFocus: 'always', refetchInterval: 15_000 })
  useLayoutEffect(() => {
    if (!denied && !resourceDenied(query.error)) return
    const key = [...privateKey(identity.person.id, event.id, 'personal-preparation'), preparationRoleSignature(roles), mount]
    void cache.cancelQueries({ queryKey: key, exact: true }); cache.removeQueries({ queryKey: key, exact: true })
  }, [denied, query.error, identity.person.id, event.id, cache, roles, mount])
  const tasks = query.data?.items.filter(task => personallyAssigned(task, identity.person.id, roles)) ?? [], task = tasks.find(item => item.definition.id === selected)
  function choose(id: string) { if (!locked && (!dirty || window.confirm('Leave this preparation task with unsaved changes?'))) { setSelected(id); if (!id) focus.restore(selected) } }
  if (denied || resourceDenied(query.error)) return <Failure error={denied ?? query.error} />
  return <section className="fleet-resource-section" aria-label="Your preparation"><h2>Your preparation</h2><EventGuidance event={event} /><p>Your assigned prerequisites and shared role tasks. Completion here does not submit team evidence.</p>{query.isPending ? <Loading /> : !query.data ? <Failure error={query.error} retry={() => void query.refetch()} /> : <>{!!query.error && <Failure error={query.error} retry={() => void query.refetch()} />}<DetailWorkspace collection={<><CollectionRecords label="Your preparation tasks" rows={tasks} rowKey={item => item.definition.id} rowLabel={item => item.definition.title} columns={[{ id: 'state', label: 'Progress', cell: item => <StatusBadge tone={item.definition.status === 'complete' ? 'success' : item.blocked ? 'warning' : 'neutral'}>{words(item.definition.status)}</StatusBadge> }, { id: 'due', label: 'Due', cell: item => localDate(item.due_at, event.configuration.timezone) }, { id: 'owner', label: 'Owner', cell: item => item.definition.owner_id ? 'You' : `Shared ${words(item.definition.owner_role)} task` }, { id: 'attention', label: 'Attention', cell: item => item.blocked ? 'Blocked' : item.overdue ? 'Overdue' : 'On schedule' }]} actions={item => <Button ref={focus.bind(item.definition.id)} data-record-focus={item.definition.id} icon="next" variant="ghost" disabled={locked || !!query.error || authorityPaused} onClick={() => choose(item.definition.id)}>Open {item.definition.title}</Button>} />{!tasks.length && <p>No preparation tasks are currently assigned to you.</p>}</>} detail={task ? <PersonalTask key={`${task.definition.id}:${task.definition.owner_id}:${task.definition.owner_role}:${task.can_edit}`} event={event} task={task} page={query.data} cacheKey={preparationKey} parentScope={scope} paused={!!query.error || authorityPaused} onDirty={setDirty} onLocked={setLocked} onDeny={error => { scope.invalidate(); setDenied(error) }} close={() => choose('')} /> : undefined} /></>}</section>
}
function PersonalTask({ event, task, page, cacheKey, parentScope, paused, onDirty, onLocked, onDeny, close }: { cacheKey: readonly unknown[]; parentScope: PrivateRequestScope; event: EventRecord; task: PreparationTask; page: PreparationPage; paused: boolean; onDirty: (dirty: boolean) => void; onLocked: (locked: boolean) => void; onDeny: (error: unknown) => void; close: () => void }) {
  const heading = useRef<HTMLHeadingElement>(null)
  useEffect(() => { heading.current?.focus() }, [])
  const identity = useIdentity(), cache = useQueryClient(), [base, setBase] = useState(task), [revision, setRevision] = useState(page.revision), [definition, setDefinition] = useState(task.definition), [reason, setReason] = useState('')
  const [latest, setLatest] = useState<PreparationPage>(), [reviewed, setReviewed] = useState(false), [review, setReview] = useState<{ definition: PreparationTask['definition']; reason: string; revision: number }>()
  const owner = `${identity.person.id}:${identity.draft_scope}:${event.id}:preparation:${task.definition.id}`, safeCommands = useCommunicationCommands(owner)
  const operation = useResourceOperation(owner, onDeny, { parentScope, safeCommands, resourceID: task.definition.id })
  const dirty = JSON.stringify(definition) !== JSON.stringify(base.definition) || !!reason || !!review, locked = !task.can_edit || paused || operation.blocked
  useEffect(() => { onDirty(dirty); return () => onDirty(false) }, [dirty, onDirty])
  useEffect(() => { onLocked(operation.blocked); return () => onLocked(false) }, [operation.blocked, onLocked])
  useBlocker({ shouldBlockFn: () => operation.isBlocked() || (dirty && !window.confirm('Leave this preparation task with unsaved changes?')), enableBeforeUnload: dirty || operation.blocked })
  const invalid = definition.evidence.some(item => !item.label.trim() || (item.kind !== 'file' && !referenceURL(item.url))) || (['blocked', 'complete'].includes(definition.status) && !reason.trim()) || definition.status === 'waived'
  async function save() {
    if (!review || paused || !task.can_edit) return
    const captured = review, ticket = operation.capture()
    await operation.run({ operation: 'Save personal preparation', revision: captured.revision, command: { definition: captured.definition, reason: captured.reason, override_dependencies: false } }, async () => {
      const result = unwrap(await api.PUT('/api/v1/events/{event_id}/preparation/tasks/{task_id}', { params: { path: { event_id: event.id, task_id: definition.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${captured.revision}"` } }, body: { definition: captured.definition, reason: captured.reason, override_dependencies: false } }))
      if (result.revision <= captured.revision || !result.items.some(item => item.definition.id === definition.id)) throw new Error('Unexpected preparation response')
      return result
    }, async (result, current) => { const updated = result.items.find(item => item.definition.id === definition.id)!; setBase(updated); setDefinition(updated.definition); setRevision(result.revision); setReason(''); setReview(undefined); cache.setQueryData(cacheKey, result); await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, event.id, 'event') }); current.assert(); return 'Preparation progress saved.' })
    if (ticket.current()) setReview(undefined)
  }
  function readCurrent() { void operation.read(async signal => unwrap(await api.GET('/api/v1/events/{event_id}/preparation/tasks', { signal, params: { path: { event_id: event.id } } })), fresh => { setLatest(fresh); setReviewed(false) }) }
  function keepEdits() {
    const current = latest?.items.find(item => item.definition.id === definition.id)
    if (!current || !latest || !reviewed || !current.can_edit) return
    const complete = new Map(definition.checklist.map(item => [item.id, item.complete]))
    setDefinition({ ...current.definition, status: definition.status, checklist: current.definition.checklist.map(item => ({ ...item, complete: complete.get(item.id) ?? item.complete })), evidence: definition.evidence }); setBase(current); setRevision(latest.revision); setLatest(undefined); setReviewed(false); operation.useCurrent()
  }
  return <section className="fleet-inspector" aria-label="Personal preparation task"><Button icon="back" variant="ghost" disabled={operation.blocked} onClick={close}>Close preparation task</Button><h3 ref={heading} tabIndex={-1}>{definition.title}</h3><p>{definition.description}</p><p>Owner: {definition.owner_id ? identity.person.display_name : `Shared ${words(definition.owner_role)} task`} · Due {localDate(task.due_at, event.configuration.timezone)} · {task.overdue ? 'Overdue' : words(task.definition.status)}</p>{!definition.owner_id && <p>This task has shared progress for its current role assignees.</p>}{task.blocked && <p>Dependencies or scheduling block completion. Ask an organiser to resolve the blocker; you cannot waive it.</p>}
    <form className="fleet-resource-form" onSubmit={event => { event.preventDefault(); if (!locked && !invalid) setReview({ definition: structuredClone(definition), reason, revision }) }}><fieldset className="fleet-resource-fields" disabled={locked}><legend>Progress</legend><Label htmlFor="personal-status">Preparation status</Label><Select id="personal-status" value={definition.status} onChange={event => setDefinition({ ...definition, status: event.target.value as typeof definition.status })}>{['not_started', 'in_progress', 'blocked', 'complete', ...(definition.status === 'waived' ? ['waived'] : [])].map(status => <option key={status} value={status} disabled={status === 'waived'}>{words(status)}</option>)}</Select>{definition.checklist.map(item => <Label className="check" key={item.id}><Checkbox checked={item.complete} onChange={event => setDefinition({ ...definition, checklist: definition.checklist.map(value => value.id === item.id ? { ...value, complete: event.target.checked } : value) })} />{item.title}</Label>)}<Label htmlFor="personal-reason">Progress reason</Label><Textarea id="personal-reason" value={reason} onChange={event => setReason(event.target.value)} maxLength={2000} /><p>Record a reason for blocking or completing a task.</p></fieldset>
    <fieldset className="fleet-resource-fields" disabled={locked}><legend>Preparation references</legend>{definition.evidence.map((item, index) => <section className="fleet-resource-section" key={item.id}>{item.kind === 'file' ? <a href={`/api/v1/events/${event.id}/files/${item.file_id}`}>{item.label}</a> : <><Label htmlFor={`personal-label-${item.id}`}>Reference label {index + 1}</Label><Input id={`personal-label-${item.id}`} value={item.label} maxLength={300} onChange={event => setDefinition({ ...definition, evidence: definition.evidence.map(value => value.id === item.id ? { ...value, label: event.target.value } : value) })} /><Label htmlFor={`personal-url-${item.id}`}>Preparation reference URL {index + 1}</Label><Input id={`personal-url-${item.id}`} type="url" value={item.url} maxLength={2048} onChange={event => setDefinition({ ...definition, evidence: definition.evidence.map(value => value.id === item.id ? { ...value, url: event.target.value } : value) })} /></>}<Button type="button" icon="remove" variant="ghost" onClick={() => setDefinition({ ...definition, evidence: definition.evidence.filter(value => value.id !== item.id) })}>Remove reference {index + 1}</Button></section>)}<Button type="button" icon="add" variant="outline" disabled={definition.evidence.length >= 30} onClick={() => setDefinition({ ...definition, evidence: [...definition.evidence, { id: crypto.randomUUID(), kind: 'url', label: '', url: '', file_id: null, visibility: 'assignees' }] })}>Add preparation reference</Button></fieldset>{task.restricted_evidence_count > 0 && <p>Restricted organiser evidence is retained by the server.</p>}{!task.can_edit && <p>This task is read-only.</p>}<Button type="submit" icon="save" disabled={locked || invalid || !dirty}>Save preparation progress</Button></form>
    {latest && <section className="fleet-resource-section" aria-label="Latest preparation task">{latest.items.filter(item => item.definition.id === definition.id).map(item => <div key={item.definition.id}><h4>{item.definition.title}</h4><p>{words(item.definition.status)} · Due {localDate(item.due_at, event.configuration.timezone)}</p><ul>{item.definition.checklist.map(value => <li key={value.id}>{value.title} · {value.complete ? 'Done' : 'To do'}</li>)}</ul></div>)}{!latest.items.some(item => item.definition.id === definition.id) && <p>This task is no longer assigned to you.</p>}<Label className="check"><Checkbox checked={reviewed} onChange={event => setReviewed(event.target.checked)} />I reviewed the latest preparation task</Label></section>}
    <ResourceOperationResult operation={operation} read={readCurrent} acceptCurrent={{ label: 'Keep my preparation progress', disabled: !reviewed || !latest?.items.some(item => item.definition.id === definition.id && item.can_edit), onSelect: keepEdits }} />
    <ConfirmationDialog printExcluded open={!!review} onOpenChange={open => { if (!open && !operation.pending) setReview(undefined) }} title="Review preparation progress" description="This updates only your assigned preparation progress and permitted references. It does not submit team competition evidence." confirmLabel="Confirm preparation progress" pending={operation.pending} disabled={locked || !review} onConfirm={() => void save()}><p>{review?.definition.title} · {words(review?.definition.status ?? '')} · Expected revision {review?.revision}</p><p>{review?.reason}</p></ConfirmationDialog>
  </section>
}

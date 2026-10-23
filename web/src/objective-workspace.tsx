import { useCallback, useEffect, useMemo, useState } from 'react'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { useBlocker } from '@tanstack/react-router'
import { api, APIError, privateKey, unwrap, type EventRecord } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { ObjectiveFields, emptyObjective, objectiveProblems, objectiveOwnerLabel, type ObjectiveDefinition, type ObjectiveOwner } from './objective-fields'
import { TextField } from './definition-editor'
import { localDate } from './event-time'
import { usePrivateRequestScope, type PrivateRequestScope, type PrivateRequestTicket } from './private-request-scope'

type View = components['schemas']['ObjectiveView']
type Command = components['schemas']['ObjectiveCommand']
type Intent = { expected: number; command: Command }
const denied = (error: unknown) => error instanceof APIError && [401, 403, 404].includes(error.status)
function canonical(value: unknown): unknown { if (Array.isArray(value)) return value.map(canonical); if (value && typeof value === 'object') return Object.fromEntries(Object.entries(value).sort(([a], [b]) => a.localeCompare(b)).map(([key, item]) => [key, canonical(item)])); return value }
const same = (a: unknown, b: unknown) => JSON.stringify(canonical(a)) === JSON.stringify(canonical(b))
const gapText: Record<string, string> = { owner_missing: 'No owner selected', owner_unavailable: 'Selected owner is no longer available', baseline_unknown: 'A baseline is unknown', target_missing: 'A target is unknown', unit_missing: 'A unit is missing', method_missing: 'A measurement method is missing', evidence_source_missing: 'Evidence instructions are missing', review_horizon_missing: 'A review horizon is missing' }

export function ObjectivesWorkspace({ event, onDirty }: { event: EventRecord; onDirty: (value: boolean) => void }) {
  const identity = useIdentity(), cache = useQueryClient()
  const prefix = useMemo(() => [...privateKey(identity.person.id, event.id, 'objective-planning')], [identity.person.id, event.id])
  const scope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${event.id}`)
  const [selected, setSelected] = useState<{ id: string; fresh: boolean }>()
  const [blocked, setBlocked] = useState<unknown>()
  const purge = useCallback((error: unknown) => { scope.invalidate(); setBlocked(error); onDirty(false); void cache.cancelQueries({ queryKey: prefix }); cache.removeQueries({ queryKey: prefix }) }, [cache, onDirty, prefix, scope])
  async function checked<T>(load: () => Promise<T>): Promise<T> { const ticket = scope.capture(); ticket.assert(); try { const value = await load(); ticket.assert(); return value } catch (error) { if (ticket.current() && denied(error)) purge(error); throw error } }
  const options = { refetchInterval: 15_000, refetchOnWindowFocus: 'always' as const, retry: false, enabled: !blocked }
  const authority = useQuery({ ...options, queryKey: [...prefix, 'authority'], queryFn: ({ signal }) => checked(async () => { const value = unwrap(await api.GET('/api/v1/events/{event_id}', { signal, params: { path: { event_id: event.id } } })); if (!value.can_read_staff) throw new APIError(403, 'forbidden'); return value }) })
  const owners = useQuery({ ...options, queryKey: [...prefix, 'owners'], queryFn: ({ signal }) => checked(async () => unwrap(await api.GET('/api/v1/events/{event_id}/objective-owners', { signal, params: { path: { event_id: event.id } } }))) })
  const listing = useInfiniteQuery({ ...options, queryKey: [...prefix, 'list'], initialPageParam: undefined as string | undefined, queryFn: ({ pageParam, signal }) => checked(async () => unwrap(await api.GET('/api/v1/events/{event_id}/objectives', { signal, params: { path: { event_id: event.id }, query: { cursor: pageParam, limit: 25 } } }))), getNextPageParam: p => p.next_cursor ?? undefined })
  const unavailable = authority.error ?? owners.error ?? listing.error
  const available = !unavailable && !!authority.data && !!owners.data && !!listing.data
  if (blocked) return <section><h2>Objectives unavailable</h2><p>Current access changed. Private inputs and cached planning data have been cleared from this workspace.</p><Failure error={blocked} /><Button onClick={() => { scope.renew(); setSelected(undefined); setBlocked(undefined) }}>Check current access</Button></section>
  return <section className="objective-workspace"><h2>Objectives and success measures</h2><Button variant="outline" onClick={() => void cache.invalidateQueries({ queryKey: prefix })}>Refresh planning</Button><p>Optional organiser planning. Targets are goals, not achieved results. Missing owners or baselines are planning gaps and do not block the event.</p><p>Private planning inputs stay in memory in this tab. They are not recovered after leaving, reloading or signing out.</p>
    {unavailable && <div role="status"><p>Current planning access could not be checked. Unsent edits remain in this tab; saving is disabled until a fresh check succeeds.</p><Button variant="outline" onClick={() => { void cache.invalidateQueries({ queryKey: prefix }) }}>Check connection and current access</Button></div>}
    {selected ? <ObjectiveController key={`${event.id}:${selected.id}`} event={event} id={selected.id} fresh={selected.fresh} scope={scope} owners={owners.data?.items ?? []} available={available} writable={!!authority.data?.can_edit} onDeny={purge} onDirty={onDirty} back={() => setSelected(undefined)} /> : listing.isPending ? <Loading /> : <>
      {!!listing.error && <Failure error={listing.error} />}
      {!listing.error && <div className="objective-directory">{listing.data?.pages.flatMap(p => p.items).map(item => <article className="panel" key={item.reference.id}><h3>{item.definition.description}</h3><p>Definition version {item.current_version} · {item.retired ? 'Retired' : 'Active planning'} · {item.definition.theme_sensitive ? 'Theme-sensitive: embargo applies' : 'Organiser-only planning'}</p><p>{item.definition.measures.length} success measure(s)</p><PlanningGaps values={item.planning_gaps} /><Button variant="outline" onClick={() => setSelected({ id: item.reference.id, fresh: false })}>Open objective</Button></article>)}</div>}
      {listing.hasNextPage && <Button variant="outline" disabled={listing.isFetchingNextPage} onClick={() => void listing.fetchNextPage()}>More objectives</Button>}
      {available && authority.data?.can_edit && <Button onClick={() => setSelected({ id: crypto.randomUUID(), fresh: true })}>Add objective</Button>}
      {available && !authority.data?.can_edit && <p>Planning is read-only for this event. Retained definitions and history remain available to current organisers.</p>}
    </>}
  </section>
}

function ObjectiveController({ event, id, fresh, scope, owners, available, writable, onDeny, onDirty, back }: { event: EventRecord; id: string; fresh: boolean; scope: PrivateRequestScope; owners: ObjectiveOwner[]; available: boolean; writable: boolean; onDeny: (error: unknown) => void; onDirty: (dirty: boolean) => void; back: () => void }) {
  const identity = useIdentity()
  const [created, setCreated] = useState(!fresh)
  const key = [...privateKey(identity.person.id, event.id, 'objective-planning'), id, 'current']
  const current = useQuery({ queryKey: key, queryFn: async ({ signal }) => { const ticket = scope.capture(); ticket.assert(); try { const value = unwrap(await api.GET('/api/v1/events/{event_id}/objectives/{objective_id}', { signal, params: { path: { event_id: event.id, objective_id: id } } })); ticket.assert(); return value } catch (error) { if (ticket.current() && denied(error)) onDeny(error); throw error } }, enabled: created, refetchInterval: 15_000, refetchOnWindowFocus: 'always', retry: false })
  useEffect(() => { if (denied(current.error)) onDeny(current.error) }, [current.error, onDeny])
  if (!fresh && !current.data) return current.error ? <Failure error={current.error} retry={() => void current.refetch()} /> : <Loading />
  if (denied(current.error)) return <Failure error={current.error} />
  return <ObjectiveEditor parentScope={scope} event={event} id={id} initial={fresh ? undefined : current.data} current={current.data} currentKey={key} owners={owners} available={available && !current.error} writable={writable} onDeny={onDeny} onDirty={onDirty} onSaved={() => setCreated(true)} back={back} />
}

function PlanningGaps({ values }: { values: string[] }) { return values.length ? <div className="notice"><strong>Advisory planning gaps</strong><ul>{[...new Set(values)].map(value => <li key={value}>{gapText[value] ?? 'Planning information is missing'}</li>)}</ul><p>These are not event lifecycle gates.</p></div> : null }

function ObjectiveEditor({ parentScope, event, id, initial, current, currentKey, owners, available, writable, onDeny, onDirty, onSaved, back }: { parentScope: PrivateRequestScope; event: EventRecord; id: string; initial?: View; current?: View; currentKey: readonly string[]; owners: ObjectiveOwner[]; available: boolean; writable: boolean; onDeny: (error: unknown) => void; onDirty: (dirty: boolean) => void; onSaved: () => void; back: () => void }) {
  const identity = useIdentity(), cache = useQueryClient()
  const scope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${event.id}:${id}`, parentScope)
  const [base, setBase] = useState(initial)
  const [definition, setDefinition] = useState<ObjectiveDefinition>(() => structuredClone(initial?.definition ?? emptyObjective()))
  const [reason, setReason] = useState(''), [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState('')
  const [conflict, setConflict] = useState(false), [outcomeMismatch, setOutcomeMismatch] = useState(false)
  const [intent, setIntent] = useState<Intent>(), [retryAllowed, setRetryAllowed] = useState(false), [comparison, setComparison] = useState<View>(), [historyOpen, setHistoryOpen] = useState(false)
  const expected = base?.reference.version ?? 0
  const dirty = !same(definition, base?.definition ?? emptyObjective()) || !!reason || !!intent
  const stale = !!current && current.current_version !== expected
  const problems = objectiveProblems(definition)
  useEffect(() => { onDirty(dirty); return () => onDirty(false) }, [dirty, onDirty])
  useBlocker({ shouldBlockFn: () => dirty && !window.confirm('Leave objective planning with unsent edits or an unconfirmed save?'), enableBeforeUnload: dirty })
  async function freshCurrent(ticket: PrivateRequestTicket) {
    ticket.assert()
    const eventNow = unwrap(await api.GET('/api/v1/events/{event_id}', { signal: ticket.signal, params: { path: { event_id: event.id } } }))
    ticket.assert()
    if (!eventNow.can_read_staff) throw new APIError(403, 'forbidden')
    cache.setQueryData([...privateKey(identity.person.id, event.id, 'objective-planning'), 'authority'], eventNow)
    let latest: View | undefined
    try { ticket.assert(); latest = unwrap(await api.GET('/api/v1/events/{event_id}/objectives/{objective_id}', { signal: ticket.signal, params: { path: { event_id: event.id, objective_id: id } } })) } catch (error) { if (!(expected === 0 && error instanceof APIError && error.status === 404)) throw error }
    ticket.assert()
    if (latest) cache.setQueryData(currentKey, latest)
    return { latest, canWrite: eventNow.can_edit && (!latest || latest.can_manage) }
  }
  function accessError(error: unknown) { if (denied(error)) onDeny(error) }
  function accepted(ticket: PrivateRequestTicket, value: View, message: string) {
    ticket.assert()
    if (value.current_version === value.reference.version) cache.setQueryData(currentKey, value); onSaved(); setConflict(false); setOutcomeMismatch(false); setBase(value); setDefinition(structuredClone(value.definition)); setReason(''); setIntent(undefined); setRetryAllowed(false); setComparison(undefined); setFailure(undefined); setNotice(message)
    void cache.invalidateQueries({ queryKey: [...privateKey(identity.person.id, event.id, 'objective-planning'), 'list'] })
    void cache.invalidateQueries({ queryKey: [...privateKey(identity.person.id, event.id, 'objective-planning'), id, 'history'] })
  }
  async function write(next: Intent, retry = false) {
    if (pending || !available || !writable || !navigator.onLine) return
    const ticket = scope.capture(); if (!ticket.current()) return
    setPending(true); setFailure(undefined); setNotice('')
    let sent = false
    try {
      const checked = await freshCurrent(ticket)
      ticket.assert()
      if (!checked.canWrite) { setNotice('This event no longer accepts planning changes. Your unsent edits remain in this tab.'); return }
      if ((checked.latest?.current_version ?? 0) !== next.expected) { setConflict(true); setComparison(checked.latest); setNotice('A newer definition exists. Review it before choosing a new base.'); return }
      ticket.assert()
      sent = true
      const value = unwrap(await api.PUT('/api/v1/events/{event_id}/objectives/{objective_id}', { params: { path: { event_id: event.id, objective_id: id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${next.expected}"` } }, body: next.command }))
      accepted(ticket, value, next.command.action === 'retire' ? `Definition version ${value.reference.version} records this objective as retired.` : `Definition version ${value.reference.version} saved. No achieved results were recorded.`)
    } catch (error) {
      if (!ticket.current()) return
      if (denied(error)) { accessError(error); return }
      setFailure(error)
      if (error instanceof APIError && error.status === 400) setNotice('Validation rejected the request. No new version was saved; your unsent inputs remain in this tab.')
      else if (error instanceof APIError && error.status === 409 && !retry) { setConflict(true); setNotice('The revision was rejected. Your unsent edits remain; check and compare the current definition.'); setIntent(undefined) }
      else if (sent && !denied(error)) { setIntent(next); setRetryAllowed(false); setNotice('Save outcome is unconfirmed. Keep this tab open and check the exact retained version before any retry. Nothing is retried automatically.') }
      else if (!denied(error)) setNotice('Current access could not be checked. No write was sent; your unsent edits remain in this tab.')
    } finally { if (ticket.current()) setPending(false) }
  }
  async function compare() {
    const ticket = scope.capture(); if (!ticket.current()) return
    setPending(true); setFailure(undefined)
    try { const checked = await freshCurrent(ticket); ticket.assert(); if (checked.latest) setComparison(checked.latest) } catch (error) { if (ticket.current()) { setFailure(error); accessError(error) } } finally { if (ticket.current()) setPending(false) }
  }
  async function checkOutcome() {
    if (!intent) return
    const ticket = scope.capture(); if (!ticket.current()) return
    setPending(true); setFailure(undefined); setRetryAllowed(false); setOutcomeMismatch(false)
    try {
      const checked = await freshCurrent(ticket)
      let retained: View | undefined
      try { ticket.assert(); retained = unwrap(await api.GET('/api/v1/events/{event_id}/objectives/{objective_id}', { signal: ticket.signal, params: { path: { event_id: event.id, objective_id: id }, query: { version: intent.expected + 1 } } })) } catch (error) { if (!(error instanceof APIError && error.status === 404)) throw error }
      ticket.assert()
      const matches = retained && retained.actor_id === identity.person.id && retained.reason === intent.command.reason && retained.retired === (intent.command.action === 'retire') && same(retained.definition, intent.command.definition ?? base?.definition)
      if (matches && retained) {
        accepted(ticket, retained, `Exact retained definition version ${retained.reference.version} matches the attempted change. Current definition version is ${retained.current_version}.`)
      } else if ((checked.latest?.current_version ?? 0) === intent.expected && checked.canWrite) {
        setRetryAllowed(true); setNotice('No matching next version is present, and the original base is still current. You may explicitly retry the same unchanged request.')
      } else {
        setComparison(checked.latest); setOutcomeMismatch(true); setConflict(true); setNotice('The current definition differs from the attempted change. Review the current version; no automatic write will occur.')
      }
    } catch (error) { if (ticket.current()) { setFailure(error); accessError(error) } } finally { if (ticket.current()) setPending(false) }
  }
  function edit(next: ObjectiveDefinition) { setDefinition(next); setNotice('Unsent planning changes. No achieved result has been recorded.'); setFailure(undefined) }
  const canEdit = available && writable && !pending && !intent
  return <section className="objective-editor"><Button variant="outline" onClick={() => { if (!dirty || window.confirm('Leave this objective with unsent edits or an unconfirmed save?')) back() }}>Back to objectives</Button><h3>{base ? `Definition version ${base.reference.version}${base.retired ? ' · Retired' : ''}` : 'New objective'}</h3>
    {!available && <p role="status">Current access is unavailable. Unsent input is retained in this tab; writes are disabled.</p>}
    {!writable && <p>Planning is read-only for this event. No new version can be saved.</p>}
    {base && available && <PlanningGaps values={base.planning_gaps} />}
    <form onSubmit={e => { e.preventDefault(); if (!problems.length && reason.trim()) void write({ expected, command: { action: 'save', definition: structuredClone(definition), reason } }) }}>
      <fieldset disabled={!canEdit}><legend>Private planning inputs</legend><ObjectiveFields value={definition} change={edit} owners={owners} /><TextField label="Reason for this definition version" value={reason} change={setReason} multiline /></fieldset>
      {!!problems.length && <ul aria-label="Planning validation">{problems.map(message => <li key={message}>{message}</li>)}</ul>}
      <Button type="submit" disabled={!canEdit || stale || conflict || !dirty || !reason.trim() || !!problems.length}>Save definition version</Button>
      {base && !base.retired && <Button type="button" variant="outline" disabled={!canEdit || stale || conflict || !reason.trim() || !same(definition, base.definition)} onClick={() => void write({ expected, command: { action: 'retire', definition: null, reason } })}>Retire objective</Button>}
    </form>
    {(stale || comparison || conflict) && available && <section className="panel" aria-label="Current definition comparison"><h3>Review the current definition</h3><p>Your unsent inputs have not been replaced. Choosing a new base is explicit and does not save anything.</p><Button variant="outline" disabled={pending} onClick={() => void compare()}>Load current definition for comparison</Button>{comparison && <><DefinitionComparison current={comparison.definition} unsent={definition} owners={owners} /><p>Compared with definition version {comparison.current_version}.</p><Button disabled={pending || !!intent || comparison.current_version !== current?.current_version} onClick={() => { setBase(comparison); setComparison(undefined); setConflict(false); setNotice('Current version selected as the new base. Your unsent inputs are retained; review them before an explicit save.') }}>Use version {comparison.current_version} as base and keep my edits</Button></>}</section>}
    {intent && <section className="notice" aria-label="Unconfirmed definition save"><p>No write is queued offline or retried automatically.</p>{outcomeMismatch && <Button variant="outline" disabled={pending} onClick={() => { setIntent(undefined); setRetryAllowed(false); setOutcomeMismatch(false); setConflict(true); setNotice('Unmatched attempt left behind explicitly. Review the current definition before choosing a new base; your unsent edits are retained.') }}>Leave unmatched attempt and review current definition</Button>}<Button disabled={pending || !available} onClick={() => void checkOutcome()}>Check exact save outcome</Button>{retryAllowed && <Button variant="outline" disabled={pending || !available || !writable} onClick={() => void write(intent, true)}>Retry the same request</Button>}</section>}
    {notice && <p role="status" aria-label="Objective save state">{notice}</p>}{!!failure && <Failure error={failure} />}
    {base && available && <><Button variant="outline" onClick={() => setHistoryOpen(!historyOpen)}>{historyOpen ? 'Hide' : 'Show'} definition history</Button>{historyOpen && <ObjectiveHistory scope={scope} event={event} id={id} onDeny={onDeny} />}</>}
  </section>
}

function DefinitionComparison({ current, unsent, owners }: { current: ObjectiveDefinition; unsent: ObjectiveDefinition; owners: ObjectiveOwner[] }) {
  return <div className="objective-comparison">{[['Current saved definition', current], ['Your unsent definition', unsent]].map(([heading, value]) => { const d = value as ObjectiveDefinition; return <section key={heading as string}><h4>{heading as string}</h4><p className="preserve-lines">{d.description}</p><p>{objectiveOwnerLabel(d.owner_id, owners)} · {d.theme_sensitive ? 'Theme-sensitive' : 'Not marked theme-sensitive'}</p>{d.measures.map(m => <div key={m.id}><h5>{m.description}</h5><p>Target: {m.target ?? 'Unknown'} {m.unit}</p><p>Baseline: {m.baseline ? `${m.baseline.value} (${m.baseline.basis === 'estimate' ? 'Estimate' : 'Self-report'})` : 'Unknown'}</p><p>Method: {m.method || 'Not specified'}</p><p>Evidence source: {m.evidence_source || 'Not specified'}</p><p>Review horizon: {m.review_horizon || 'Not specified'}</p></div>)}</section> })}</div>
}

function ObjectiveHistory({ scope, event, id, onDeny }: { scope: PrivateRequestScope; event: EventRecord; id: string; onDeny: (error: unknown) => void }) {
  const identity = useIdentity()
  const history = useInfiniteQuery({ queryKey: [...privateKey(identity.person.id, event.id, 'objective-planning'), id, 'history'], initialPageParam: undefined as number | undefined, queryFn: async ({ pageParam, signal }) => { const ticket = scope.capture(); ticket.assert(); try { const value = unwrap(await api.GET('/api/v1/events/{event_id}/objectives/{objective_id}/history', { signal, params: { path: { event_id: event.id, objective_id: id }, query: { before_version: pageParam, limit: 25 } } })); ticket.assert(); return value } catch (error) { if (ticket.current() && denied(error)) onDeny(error); throw error } }, getNextPageParam: p => p.next_cursor ?? undefined, refetchInterval: 15_000, retry: false })
  useEffect(() => { if (denied(history.error)) onDeny(history.error) }, [history.error, onDeny])
  if (history.isPending) return <Loading />
  if (history.error) return <Failure error={history.error} retry={() => void history.refetch()} />
  return <section aria-label="Objective definition history"><h3>Retained definition versions</h3>{history.data.pages.flatMap(p => p.items).map(v => <article key={v.reference.version} className="panel"><h4>Version {v.reference.version} · {v.retired ? 'Retired' : 'Planning definition'}</h4><p>{localDate(v.at, event.configuration.timezone)} · {v.reason}</p><p>{v.definition.description}</p>{v.definition.measures.map(m => <p key={m.id}>{m.description}: target {m.target ?? 'Unknown'} {m.unit}; baseline {m.baseline ? `${m.baseline.value} (${m.baseline.basis})` : 'Unknown'}</p>)}</article>)}{history.hasNextPage && <Button disabled={history.isFetchingNextPage} onClick={() => void history.fetchNextPage()}>Older definition versions</Button>}</section>
}

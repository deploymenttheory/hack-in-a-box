import { usePlanningActionFocus } from './planning-layout'
import { CollectionRecords, DetailWorkspace, Inspector } from './components/patterns'
import { Input, Textarea, Select, Label } from './components/ui/fields'
import { useEffect, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useBlocker } from '@tanstack/react-router'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { api, APIError, privateKey, unwrap, type EventRecord } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { localDate } from './event-time'

type Entry = components['schemas']['AttendanceEntry']
type AttendancePage = components['schemas']['AttendancePage']
const dateLabel = (day: string) => new Intl.DateTimeFormat('en-GB', { dateStyle: 'full', timeZone: 'UTC' }).format(new Date(`${day}T12:00:00Z`))

export function AttendanceWorkspace({ event, onDirty, openSection }: { event: EventRecord; onDirty: (dirty: boolean) => void; openSection: (name: string) => void }) {
  const identity = useIdentity(), cache = useQueryClient(), actionFocus=usePlanningActionFocus()
  const [day, setDay] = useState(''), [search, setSearch] = useState(''), [filter, setFilter] = useState('all'), [busy, setBusy] = useState(''), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState('')
  const [editing, setEditing] = useState<Entry>(), [bufferOpen, setBufferOpen] = useState(false), [selection, setSelection] = useState(''), [dirty, setDirty] = useState(false)
  const retries = useRef(new Map<string, { request_id: string; revision: number }>())
  const key = privateKey(identity.person.id, event.id, `attendance:${day}`)
  const query = useQuery({ queryKey: key, queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/attendance', { params: { path: { event_id: event.id }, query: { day: day || undefined } } })), refetchOnWindowFocus: 'always' })
  useEffect(() => { onDirty(dirty); return () => onDirty(false) }, [dirty, onDirty])
  function leave() { return !dirty || window.confirm('Leave these attendance changes without saving?') }
  function close() { if (leave()) { actionFocus.restore(editing?editing.person_id:'catering-buffer'); setEditing(undefined); setBufferOpen(false); setDirty(false) } }
  async function checkIn(person: Entry, selectedDay: string) {
    if (busy) return
    setBusy(person.person_id); setFailure(undefined); setNotice('')
    const retryKey = `${selectedDay}:${person.person_id}`, previous = retries.current.get(retryKey) ?? { request_id: crypto.randomUUID(), revision: person.revision }
    retries.current.set(retryKey, previous)
    try {
      const result = unwrap(await api.PUT('/api/v1/events/{event_id}/attendance/{person_id}/{day}', { params: { path: { event_id: event.id, person_id: person.person_id, day: selectedDay }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${previous.revision}"` } }, body: { request_id: previous.request_id, present: true, reason: '' } }))
      retries.current.delete(retryKey); setNotice(`${result.display_name}: ${result.present ? 'checked in' : 'not checked in'}.`)
      cache.setQueryData<AttendancePage>(key, current => current && ({ ...current, items: current.items.map(p => p.person_id === result.person_id ? result : p) }))
    } catch (error) { setFailure(error); if (error instanceof APIError && error.status < 500) retries.current.delete(retryKey) }
    finally { await query.refetch(); setBusy('') }
  }
  if (query.isPending) return <Loading />
  if (query.error) return <Failure error={query.error} retry={() => void query.refetch()} />
  const value = query.data, counts = value.counts
  const items = value.items.filter(p => p.display_name.toLocaleLowerCase().includes(search.trim().toLocaleLowerCase()) && (filter === 'all' || (filter === 'arrived' ? p.present : !p.present)))
  return <section id="attendance" className="attendance-workspace" aria-label="Attendance workspace"><h2>Arrivals and headcounts</h2><p>Record confirmed competitors’ physical arrivals for each hack day.</p>
    <dl className="fleet-planning-metrics"><div><dt>Confirmed competitors</dt><dd>{counts.confirmed_competitors}</dd><small>Minimum {counts.minimum_competitors} · {counts.confirmed_competitors >= counts.minimum_competitors ? 'Threshold reached; organiser go decision still required' : 'Below the event minimum'}</small></div><div><dt>Checked in on this day</dt><dd>{counts.checked_in_competitors}</dd><small>Physical arrivals; separate from admission</small></div><div><dt>Active staff</dt><dd>{counts.staff}</dd><small>{counts.staff_also_competing} also competing</small></div><div><dt>Catering total</dt><dd>{counts.catering_total}</dd><small>Includes {counts.catering_buffer} buffer places; counts each person once</small></div></dl>
    <p className="muted">Source roster cutoff: {localDate(value.roster_as_of, value.timezone)} ({value.timezone}). Based on the current confirmed roster and active staff.</p>
    <div className="actions"><Button variant="outline" disabled={!!busy || query.isFetching} onClick={() => void query.refetch()}>Refresh headcounts</Button>{value.can_configure && <Button ref={actionFocus.bind('catering-buffer')} variant="outline" onClick={() => { if (leave()) { setEditing(undefined); setBufferOpen(true); setSelection(crypto.randomUUID()); setDirty(false) } }}>Configure catering buffer</Button>}<Button variant="outline" onClick={() => { if (leave()) openSection('operations') }}>Operational checklists</Button><Button variant="outline" onClick={() => { if (leave()) openSection('overview') }}>Review event go decision</Button>{value.can_read_logistics && <Button variant="outline" onClick={() => { if (leave()) openSection('logistics') }}>Open restricted logistics</Button>}</div>
    <div className="form-grid section-heading"><div><Label htmlFor="attendance-day">Hack day</Label><Select id="attendance-day" value={value.day} disabled={!!busy || !value.days.length} onChange={e => { if (leave()) { setDay(e.target.value); setEditing(undefined); setBufferOpen(false); setDirty(false); setNotice(''); setFailure(undefined) } }}>{value.days.length ? value.days.map(d => <option value={d} key={d}>{dateLabel(d)}</option>) : <option value="">Set live dates before check-in</option>}</Select></div><div><Label htmlFor="arrival-filter">Arrival status</Label><Select id="arrival-filter" value={filter} onChange={e => setFilter(e.target.value)}><option value="all">All confirmed competitors</option><option value="arrived">Checked in</option><option value="absent">Not checked in</option></Select></div></div>
    <Label htmlFor="attendance-search">Search confirmed competitors</Label><Input id="attendance-search" type="search" value={search} onChange={e => setSearch(e.target.value)} autoComplete="off" />
    <p role="status">{items.length} matching competitors{value.day ? ` · ${dateLabel(value.day)}` : ''}</p>
    <DetailWorkspace collection={<CollectionRecords label="Confirmed competitor arrivals" rows={items} rowKey={p=>p.person_id} rowLabel={p=>p.display_name} columns={[{id:'arrival',label:'Arrival status',essential:true,cell:p=>p.present?'Checked in':'Not checked in'},{id:'account',label:'Account',cell:p=>p.active?'Active':'Account inactive'},{id:'updated',label:'Updated',cell:p=>p.updated_at?localDate(p.updated_at,value.timezone):'No recorded arrival'}]} actions={p=>p.revision===0&&!p.present?<Button ref={actionFocus.bind(p.person_id)} data-record-focus={p.person_id} icon="check" disabled={!!busy||!value.can_check_in||!p.active} onClick={()=>void checkIn(p,value.day)}>Check in {p.display_name}</Button>:<Button ref={actionFocus.bind(p.person_id)} data-record-focus={p.person_id} icon="user" variant="ghost" disabled={!!busy||!value.can_check_in} onClick={()=>{if(leave()){setBufferOpen(false);setEditing(p);setSelection(crypto.randomUUID());setDirty(false)}}}>Correct {p.display_name}</Button>} />} detail={(editing||bufferOpen)&&<Inspector identity={selection} title={editing?'Correct '+editing.display_name:'Catering buffer'} scope={{event:event.name,entity:dateLabel(value.day)}} back={{label:'Back to arrivals',href:'#attendance',onNavigate:close}}>{editing && <ArrivalEditor key={selection} initial={editing} eventID={event.id} day={value.day} timezone={value.timezone} canEdit={value.can_check_in && value.items.some(p => p.person_id === editing.person_id)} refresh={async () => { const latest = await query.refetch(); if (latest.error) throw latest.error; return latest.data! }} onDirty={setDirty} close={close} />}
    {bufferOpen && <CateringEditor key={selection} eventID={event.id} initial={value} onDirty={setDirty} close={close} refresh={async () => { const latest = await query.refetch(); if (latest.error) throw latest.error; return latest.data! }} />}</Inspector>} />
    {!!value.items.length&&!items.length&&<p role="status">No confirmed competitors match these filters.</p>}
    {!value.items.length && <p className="notice">No confirmed competitors. Waitlisted applicants and unaccepted offers do not appear here.</p>}{!value.can_check_in && <p className="notice">Check-in is unavailable for this event or your current permissions.</p>}
    {notice && <p role="status" className="notice">{notice}</p>}{!!failure && <Failure error={failure} />}

  </section>
}

const arrivalSchema = z.object({ present: z.enum(['yes', 'no']), reason: z.string().trim().min(1, 'Record why this arrival needs correcting.').max(500) })
function ArrivalEditor({ initial, eventID, day, timezone, canEdit, refresh, onDirty, close }: { initial: Entry; eventID: string; day: string; timezone: string; canEdit: boolean; refresh: () => Promise<AttendancePage>; onDirty: (dirty: boolean) => void; close: () => void }) {
  const identity = useIdentity(), cache = useQueryClient(), editor = useRef<HTMLFormElement>(null)
  const [saved, setSaved] = useState(initial), [latest, setLatest] = useState<Entry>(), [reviewed, setReviewed] = useState(false), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState('')
  const retry = useRef<{ signature: string; id: string }>(undefined)
  const form = useForm<z.infer<typeof arrivalSchema>>({ resolver: zodResolver(arrivalSchema), defaultValues: { present: initial.present ? 'yes' : 'no', reason: '' } })
  useEffect(() => { editor.current?.focus(); editor.current?.scrollIntoView({ block: 'start' }) }, [])
  const dirty = form.formState.isDirty, conflict = failure instanceof APIError && failure.status === 409
  useEffect(() => { onDirty(dirty); return () => onDirty(false) }, [dirty, onDirty])
  useBlocker({ shouldBlockFn: () => dirty && !window.confirm('Leave this arrival correction without saving?'), enableBeforeUnload: dirty })
  const history = useQuery({ queryKey: privateKey(identity.person.id, eventID, `arrival-history:${initial.person_id}:${day}`), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/attendance/{person_id}/{day}/history', { params: { path: { event_id: eventID, person_id: initial.person_id, day } } })) })
  async function save(v: z.infer<typeof arrivalSchema>) {
    if (!canEdit || (conflict && (!latest || !reviewed))) return
    setNotice('')
    const revision = conflict && latest ? latest.revision : saved.revision, signature = JSON.stringify([revision, v])
    if (retry.current?.signature !== signature) retry.current = { signature, id: crypto.randomUUID() }
    try {
      const result = unwrap(await api.PUT('/api/v1/events/{event_id}/attendance/{person_id}/{day}', { params: { path: { event_id: eventID, person_id: initial.person_id, day }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${revision}"` } }, body: { request_id: retry.current.id, present: v.present === 'yes', reason: v.reason } }))
      retry.current = undefined; setSaved(result); form.reset({ present: result.present ? 'yes' : 'no', reason: '' }); setFailure(undefined); setLatest(undefined); setReviewed(false); setNotice('Arrival correction saved.'); await refresh(); await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, eventID, `arrival-history:${initial.person_id}:${day}`) })
    } catch (error) { setFailure(error); setLatest(undefined); setReviewed(false); if (error instanceof APIError && error.status === 403) await refresh().catch(setFailure) }
  }
  async function review() {
    setLatest(undefined);setReviewed(false)
    try { const page = await refresh(); const p = page.items.find(p => p.person_id === initial.person_id); if (!p) throw new APIError(403, 'forbidden'); setLatest(p); setReviewed(false) } catch (error) { setFailure(error) }
  }
  return <form ref={editor} tabIndex={-1} className="fleet-planning-section" aria-label="Arrival correction" onSubmit={event => void form.handleSubmit(save)(event)}><h3>Correct {initial.display_name} · {dateLabel(day)}</h3><fieldset disabled={!canEdit || form.formState.isSubmitting}><Label htmlFor="correct-present">Arrival status</Label><Select id="correct-present" {...form.register('present')}><option value="yes">Checked in</option><option value="no">Not checked in</option></Select><Label htmlFor="arrival-reason">Correction reason</Label><Textarea id="arrival-reason" maxLength={500} {...form.register('reason')} />{form.formState.errors.reason && <p role="alert" className="field-error">{form.formState.errors.reason.message}</p>}</fieldset>
    {conflict ? <div role="alert"><p>Arrival status changed. Your correction is preserved.</p><Button type="button" variant="outline" onClick={() => void review()}>Review latest arrival</Button>{latest && <><p>Latest status: {latest.present ? 'Checked in' : 'Not checked in'} · {localDate(latest.updated_at, timezone)}</p><Label className="check"><Input type="checkbox" checked={reviewed} onChange={e => setReviewed(e.target.checked)} />Apply my correction after reviewing the latest status</Label></>}</div> : !!failure && <Failure error={failure} />}
    <div className="actions"><Button type="submit" disabled={!canEdit || form.formState.isSubmitting || (conflict && (!latest || !reviewed))}>Save arrival correction</Button><Button type="button" variant="outline" disabled={form.formState.isSubmitting} onClick={close}>Close correction</Button></div>{notice && <p role="status">{notice}</p>}
    <h4>Arrival history</h4>{history.isPending ? <Loading /> : history.error ? <Failure error={history.error} /> : <ol className="history">{history.data.items.map(h => <li key={h.request_id}>{h.present ? 'Checked in' : 'Not checked in'} · {localDate(h.at, timezone)}{h.reason && <p>{h.reason}</p>}</li>)}</ol>}
  </form>
}

const bufferSchema = z.object({ buffer: z.number().int().min(0).max(10000) })
function CateringEditor({ initial, eventID, onDirty, close, refresh }: { initial: AttendancePage; eventID: string; onDirty: (dirty: boolean) => void; close: () => void; refresh: () => Promise<AttendancePage> }) {
  const identity = useIdentity(), cache = useQueryClient(), [revision, setRevision] = useState(initial.revision), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState('')
  const form = useForm<z.infer<typeof bufferSchema>>({ resolver: zodResolver(bufferSchema), defaultValues: { buffer: initial.counts.catering_buffer } }), dirty = form.formState.isDirty
  useEffect(() => { onDirty(dirty); return () => onDirty(false) }, [dirty, onDirty])
  useBlocker({ shouldBlockFn: () => dirty && !window.confirm('Leave this catering change without saving?'), enableBeforeUnload: dirty })
  async function save(v: z.infer<typeof bufferSchema>) {
    if (!initial.can_configure || (failure instanceof APIError && failure.status === 409)) return
    setFailure(undefined); setNotice('')
    try { const result = unwrap(await api.PUT('/api/v1/events/{event_id}/attendance/catering', { params: { path: { event_id: eventID }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${revision}"` } }, body: v })); setRevision(result.revision); form.reset(v); setNotice('Catering buffer saved.'); await refresh(); await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, eventID, 'event') }) } catch (error) { setFailure(error); if (error instanceof APIError && error.status === 403) await refresh().catch(setFailure) }
  }
  return <form className="fleet-planning-section" aria-label="Catering configuration" onSubmit={form.handleSubmit(save)}><h3>Catering buffer</h3><Label htmlFor="catering-buffer">Extra catering places</Label><Input id="catering-buffer" type="number" min={0} max={10000} {...form.register('buffer', { valueAsNumber: true })} /><p>These places increase catering totals only. They do not count toward the event minimum.</p>{form.formState.errors.buffer && <p role="alert">Enter a whole number from 0 to 10,000.</p>}<div className="actions"><Button type="submit" disabled={form.formState.isSubmitting || !initial.can_configure || (failure instanceof APIError && failure.status === 409)}>Save catering buffer</Button><Button type="button" variant="outline" onClick={close}>Close catering settings</Button></div>{!!failure && <Failure error={failure} />}{failure instanceof APIError && failure.status === 409 && <Button type="button" variant="outline" onClick={async () => { try { const current = await refresh(); setRevision(current.revision); setFailure(undefined); setNotice(`Latest saved buffer: ${current.counts.catering_buffer}. Your proposed buffer is retained.`) } catch (error) { setFailure(error) } }}>Review latest headcounts</Button>}{notice && <p role="status">{notice}</p>}</form>
}

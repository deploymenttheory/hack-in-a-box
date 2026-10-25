import { useCallback, useRef, useState } from 'react'
import { APIError, api, unwrap, type EventRecord } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { useEventReadBoundary } from './event-authority'
import { useTodayRead } from './today-read'
import { usePrivateRequestScope } from './private-request-scope'
import { useCommunicationCommands } from './communication-tests'
import { usePlanningDepartureBarrier } from './planning-operations'
import { CollectionPagination, DetailWorkspace, Inspector, CollectionStateMessage } from './components/patterns'
import { Button } from './components/ui/button'
import { Field, Select } from './components/ui/fields'
import { Failure, Loading } from './components/states'
import { SessionCollection, SessionFacts, SessionDecision, SessionFailure, type BookableSession } from './session-booking-view'
import { matchSessionReceipt, SessionBookingError, type SessionOriginal, type SessionReceipt } from './session-booking-receipt'
import { SessionSetup } from './session-setup'

type PlaceAction = SessionOriginal['action']
type Attendance = components['schemas']['SessionAttendanceEntry']
function useSessionCommands(eventID: string) {
  const identity = useIdentity()
  return useCommunicationCommands(`${identity.person.id}:${identity.draft_scope}:${eventID}:session-bookings:commands`)
}
export function SessionBookings({ event, onDirty, openSetup }: { event: EventRecord; onDirty?: (dirty: boolean) => void; openSetup?: () => void }) {
  const identity = useIdentity(), boundary = useEventReadBoundary(event.id), commands = useSessionCommands(event.id)
  const [own, setOwn] = useState(false), [cursors, setCursors] = useState<(string | undefined)[]>([undefined]), [selected, select] = useState<string>(), [setup, setSetup] = useState<string>(), [failure, setFailure] = useState<unknown>(), [receipt, setReceipt] = useState<SessionReceipt>(), [reading, setReading] = useState(false)
  const recoveryAttempt = useRef(0), opener = useRef<HTMLElement | null>(null), after = cursors.at(-1)
    const load = useCallback(async (signal: AbortSignal) => unwrap(await api.GET('/api/v1/events/{event_id}/sessions', { signal, cache: 'no-store', params: { path: { event_id: event.id }, query: { own, after, limit: 50 } } })), [event.id, own, after])
  const signature = useCallback((page: components['schemas']['SessionBookingPage']) => JSON.stringify(page.items.map(s => [s.id, s.can_manage, s.can_record_attendance, !!s.definition])), [])
  const read = useTodayRead(`${identity.person.id}:${identity.draft_scope}:${event.id}:${boundary?.generation}:${own}:${after ?? ''}:sessions`, load, boundary?.scope, signature)
  const scope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${event.id}:session-recovery`, read.authority)
  usePlanningDepartureBarrier(commands.pending)
  const paused = read.pending || !!read.error || !!boundary?.paused || reading || commands.pending
  async function recover(attempt: typeof commands.attempts[number]) {
    const { requestID, resourceID, subjectID, action } = attempt.command
    if (!requestID || !resourceID || !subjectID || !['reserve', 'cancel', 'present', 'absent'].includes(action) || paused) return
    const ticket = scope.capture(), readAttempt = ++recoveryAttempt.current; setReading(true); setFailure(undefined)
    try {
      ticket.assert()
      const result = unwrap(await api.GET('/api/v1/events/{event_id}/sessions/{session_id}/receipts/{request_id}', { signal: ticket.signal, cache: 'no-store', params: { path: { event_id: event.id, session_id: resourceID, request_id: requestID } } }))
      ticket.assert()
      if (!matchSessionReceipt(result, { requestID, sessionID: resourceID, personID: subjectID, action: action as PlaceAction, revision: attempt.revision })) throw new SessionBookingError('The response does not match the original command. Its outcome remains unknown.')
      commands.complete(attempt.id); setReceipt(result); await read.refresh()
    } catch (error) { if (ticket.current()) { setFailure(error instanceof APIError && error.status === 404 ? new SessionBookingError('No original acknowledgement is available. The original outcome remains unknown; current facts do not prove failure.') : error); if (error instanceof APIError && [401,403].includes(error.status)) { scope.invalidate(); await read.refresh() } } }
    // Clearing latest read bookkeeping carries no private projection or receipt.
    finally { if (readAttempt === recoveryAttempt.current) setReading(false) }
  }
  function back() { if (commands.pending) return; select(undefined); setSetup(undefined); requestAnimationFrame(() => opener.current?.focus()) }
  if (read.denied) return <CollectionStateMessage state={{ kind: 'denied', message: 'Session access is no longer available. Private sessions and rosters have been cleared.' }} />
  return <section className="fleet-planning-private" aria-label="Session bookings"><h2>Sessions and personal agenda</h2><p>Discover optional workshops and office hours. Today and ordinary event preparation remain available independently.</p>
    <div className="fleet-pattern-actions"><Button icon="calendar" variant={!own ? 'default' : 'outline'} disabled={paused || !!selected || !!setup} aria-pressed={!own} onClick={() => { setOwn(false); setCursors([undefined]) }}>Discover sessions</Button><Button icon="user" variant={own ? 'default' : 'outline'} disabled={paused || !!selected || !!setup} aria-pressed={own} onClick={() => { setOwn(true); setCursors([undefined]) }}>Personal agenda</Button><Button icon="refresh" variant="outline" disabled={read.pending || commands.pending} onClick={() => void read.refresh()}>Read current sessions</Button></div>
    {commands.attempts.map(attempt => <aside key={attempt.id} className="fleet-planning-section"><p>{attempt.operation} · {attempt.state === 'pending' ? 'Awaiting original acknowledgement' : 'Original outcome unknown'}. Request {attempt.command.requestID}. Current reads do not settle this outcome.</p>{attempt.state === 'unknown' && ['reserve','cancel','present','absent'].includes(attempt.command.action) && <Button icon="refresh" variant="outline" disabled={paused} onClick={() => void recover(attempt)}>Read original acknowledgement</Button>}{attempt.state === 'unknown' && !['reserve','cancel','present','absent'].includes(attempt.command.action) && <p>This operation has no original acknowledgement lookup. Read the current session, then deliberately select a fresh starting point before reviewing a different change.</p>}</aside>)}
    {receipt && <p role="status">Original acknowledgement: {receipt.state} · Revision {receipt.revision} · Request {receipt.request_id}. Read the personal agenda for current status. This is not notification delivery confirmation.</p>}
    {!!failure && <SessionFailure error={failure} />}
    {!!read.error && <Failure error={read.error} retry={() => void read.refresh()} />}
    {!read.data ? <Loading /> : <fieldset className="fleet-planning-authority" disabled={paused}><DetailWorkspace collection={<><SessionCollection sessions={read.data.items} disabled={paused} open={(s, button) => { opener.current = button; select(s.id); setReceipt(undefined) }} />{!read.data.items.length && <p>{own ? 'No personal session places on this page.' : 'No authorised sessions on this page.'}</p>}<CollectionPagination label="Session pages" source={{ kind: 'cursor', loaded: read.data.items.length, busy: paused, previous: cursors.length > 1 ? () => { setCursors(c => c.slice(0,-1)); select(undefined) } : undefined, next: read.data.next_cursor ? () => { setCursors(c => [...c,read.data?.next_cursor]); select(undefined) } : undefined }} />
      {event.can_edit && <section className="fleet-planning-section"><h3>Set up a bookable session</h3><p>Select an existing agenda item. Its title, audience and time remain owned by event configuration.</p><Field label="Agenda item"><Select value={setup ?? ''} disabled={commands.pending || !!selected} onChange={e => setSetup(e.target.value || undefined)}><option value="">Choose an agenda item</option>{event.configuration.schedule.map(item => <option key={item.id} value={item.id}>{item.title}</option>)}</Select></Field>{!event.configuration.schedule.length && <p>Add an agenda item in event setup first.</p>}{openSetup && <Button icon="calendar" variant="outline" disabled={commands.pending} onClick={openSetup}>Open event schedule setup</Button>}</section>}</>}
      detail={selected ? <SessionDetail key={`${selected}:${boundary?.generation}:${read.generation}`} event={event} sessionID={selected} parent={read.authority} back={back} updated={() => void read.refresh()} onDirty={onDirty} /> : setup ? <SessionSetup key={`${setup}:${boundary?.generation}:${read.generation}`} event={event} sessionID={setup} parent={read.authority} back={back} updated={() => void read.refresh()} onDirty={onDirty} /> : undefined} /></fieldset>}
  </section>
}

function SessionDetail({ event, sessionID, parent, back, updated, onDirty }: { event: EventRecord; sessionID: string; parent: import('./private-request-scope').PrivateRequestScope; back: () => void; updated: () => void; onDirty?: (dirty: boolean) => void }) {
  const identity = useIdentity()
  const load = useCallback(async (signal: AbortSignal) => unwrap(await api.GET('/api/v1/events/{event_id}/sessions/{session_id}', { signal, cache: 'no-store', params: { path: { event_id: event.id, session_id: sessionID } } })), [event.id,sessionID])
  const signature = useCallback((s: BookableSession) => JSON.stringify([s.can_manage,s.can_record_attendance,!!s.definition]), [])
  const read = useTodayRead(`${identity.person.id}:${identity.draft_scope}:${event.id}:${sessionID}:detail`, load, parent, signature)
  if (read.denied) return <CollectionStateMessage state={{ kind: 'denied', message: 'Current session audience or assignment was removed. Private details have been cleared.' }} />
  if (!read.data) return read.error ? <Failure error={read.error} retry={() => void read.refresh()} /> : <Loading />
  return <SessionDetailContent key={read.generation} event={event} sessionID={sessionID} back={back} updated={updated} onDirty={onDirty} read={read} />
}
function SessionDetailContent({ event, sessionID, back, updated, onDirty, read }: { event: EventRecord; sessionID: string; back: () => void; updated: () => void; onDirty?: (dirty: boolean) => void; read: ReturnType<typeof useTodayRead<BookableSession>> }) {
  const identity = useIdentity(), commands = useSessionCommands(event.id), scope = usePrivateRequestScope(`${event.id}:${sessionID}:write`, read.authority), [review, setReview] = useState<{ source: BookableSession; action: PlaceAction; person: string; name?: string; revision: number; requestID: string }>(), [failure, setFailure] = useState<unknown>(), [receipt, setReceipt] = useState<SessionReceipt>(), [editing, setEditing] = useState(false)
  const opener = useRef<HTMLElement | null>(null)
  const blocked = commands.attempts.some(a => a.command.resourceID === sessionID && a.blocks), paused = blocked || read.pending || !!read.error
  function prepare(action: PlaceAction, button: HTMLElement, person?: Attendance) {
    if (!read.data || paused) return
    opener.current = button; setReceipt(undefined); setFailure(undefined)
    setReview({ source: structuredClone(read.data), action, person: person?.person_id ?? identity.person.id, name: person?.display_name, revision: person?.revision ?? read.data.own_booking?.revision ?? 0, requestID: crypto.randomUUID() })
  }
  async function confirm() {
    if (!review || paused || !read.data || review.action === 'reserve' && !read.data.can_reserve || (review.action === 'present' || review.action === 'absent') && !read.data.can_record_attendance || review.source.revision !== read.data.revision || (!review.name && review.revision !== (read.data.own_booking?.revision ?? 0))) return
    const captured = review, ticket = scope.capture(), attendance = captured.action === 'present' || captured.action === 'absent'
    ticket.assert()
    const id = commands.begin({ operation: attendance ? 'Session attendance correction' : 'Session place change', revision: captured.revision, command: { action: captured.action, requestID: captured.requestID, resourceID: sessionID, subjectID: captured.person, expectedVersion: captured.source.revision } })
    setFailure(undefined)
    try {
      ticket.assert(); read.authority.capture().assert()
      const params = { path: { event_id: event.id, session_id: sessionID, person_id: captured.person }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${captured.revision}"` } }
      const result = attendance ? unwrap(await api.PUT('/api/v1/events/{event_id}/sessions/{session_id}/attendance/{person_id}', { params, body: { request_id: captured.requestID, present: captured.action === 'present' } })) : unwrap(await api.POST('/api/v1/events/{event_id}/sessions/{session_id}/booking', { params, body: { request_id: captured.requestID, action: captured.action as 'reserve' | 'cancel', session_revision: captured.source.revision } }))
      if (!matchSessionReceipt(result, { requestID: captured.requestID, sessionID, personID: captured.person, action: captured.action, revision: captured.revision })) throw new Error('Original acknowledgement did not match the reviewed request.')
      commands.complete(id)
      if (ticket.current()) { setReview(undefined); setReceipt(result); await read.refresh(); updated(); requestAnimationFrame(() => opener.current?.focus()) }
    } catch (error) {
      const rejected = error instanceof APIError && [400,401,403,404,409,422].includes(error.status)
      if (rejected) commands.complete(id); else commands.uncertain(id)
      if (ticket.current()) { setReview(undefined); setFailure(error); await read.refresh(); updated() }
    }
  }
  if (read.denied) return <CollectionStateMessage state={{ kind: 'denied', message: 'Current session audience or assignment was removed. Private details have been cleared.' }} />
  if (!read.data) return read.error ? <Failure error={read.error} retry={() => void read.refresh()} /> : <Loading />
  const s = read.data
  if (editing && s.can_manage) return <SessionSetup event={event} sessionID={sessionID} current={s} parent={read.authority} back={() => setEditing(false)} updated={() => { void read.refresh(); updated() }} onDirty={onDirty} />
  return <Inspector identity={sessionID} title={s.title} scope={{ event: event.name, entity: 'Optional session' }} back={{ label: 'Back to sessions', href: '#sessions', onNavigate: back }}><SessionFacts session={s} />{!!read.error && <Failure error={read.error} retry={() => void read.refresh()} />}
    <div className="fleet-pattern-actions"><Button icon="refresh" variant="outline" disabled={read.pending || commands.pending} onClick={() => void read.refresh()}>Read current session</Button>{s.can_manage && <Button icon="calendar" variant="outline" disabled={paused} onClick={() => setEditing(true)}>Configure session</Button>}</div>
    <div hidden={!!review} className="fleet-pattern-actions">{s.can_reserve && !['booked','waitlisted'].includes(s.own_booking?.state ?? '') && <Button icon="check" disabled={paused} onClick={e => prepare('reserve',e.currentTarget)}>{s.available > 0 ? 'Review reservation' : 'Review session waitlist'}</Button>}{['booked','waitlisted'].includes(s.own_booking?.state ?? '') && !s.cancelled && <Button icon="close" variant="outline" disabled={paused} onClick={e => prepare('cancel',e.currentTarget)}>Review place cancellation</Button>}</div>
    {s.can_record_attendance && <SessionRoster key={`${s.id}:${s.revision}:${read.generation}`} eventID={event.id} session={s} parent={read.authority} disabled={paused || !!review} prepare={prepare} />}
    {review && <SessionDecision title={review.name ? `Correct attendance for ${review.name}` : review.action === 'reserve' ? 'Review session reservation' : 'Review session cancellation'} pending={commands.pending} disabled={blocked || read.pending || !!read.error || review.action === 'reserve' && !s.can_reserve || (review.action === 'present' || review.action === 'absent') && !s.can_record_attendance || s.revision !== review.source.revision || (!review.name && review.revision !== (s.own_booking?.revision ?? 0))} confirm={() => void confirm()} correct={() => { setReview(undefined); requestAnimationFrame(() => opener.current?.focus()) }} finalFocus={opener}><h3>{review.source.title}</h3><SessionFacts session={review.source} /><p>{review.name ? `Record ${review.name} as ${review.action}. This corrects session attendance only.` : review.action === 'reserve' ? 'The server allocates an available place or a session waitlist position. Capacity may change before confirmation. Only the original acknowledgement confirms which result was recorded.' : 'Cancel your session place. The server may promote the next eligible session waitlistee.'}</p><p>Captured session revision {review.source.revision} · {review.name ? 'Attendance' : 'Own booking'} revision {review.revision}.</p>{s.revision !== review.source.revision && <p role="alert">Session dates or configuration changed. Cancel this review and review the current session.</p>}</SessionDecision>}
    {receipt && <p role="status">Original acknowledgement: {receipt.state}, revision {receipt.revision}. Request {receipt.request_id}. Current state may be newer.</p>}{!!failure && <SessionFailure error={failure} />}
  </Inspector>
}
function SessionRoster({ eventID, session, parent, disabled, prepare }: { eventID: string; session: BookableSession; parent: import('./private-request-scope').PrivateRequestScope; disabled: boolean; prepare: (action: PlaceAction, opener: HTMLElement, person: Attendance) => void }) {
  const load = useCallback(async (signal: AbortSignal, assert: () => void) => { assert(); return unwrap(await api.GET('/api/v1/events/{event_id}/sessions/{session_id}/attendance', { signal, cache: 'no-store', params: { path: { event_id: eventID, session_id: session.id } } })) }, [eventID,session.id])
  const read = useTodayRead(`${eventID}:${session.id}:attendance`, load, parent)
  if (read.denied) return <p>Facilitator roster access is no longer available.</p>
  return <section className="fleet-planning-section"><h3>Scoped facilitator attendance</h3><p>This session roster grants no unrelated logistics, competition output or assessment access.</p><Button icon="refresh" variant="outline" disabled={read.pending || disabled} onClick={() => void read.refresh()}>Read current attendance</Button>{!!read.error && <Failure error={read.error} />}{!read.data ? <Loading /> : <ul className="fleet-planning-records">{read.data.items.map(person => <li key={person.person_id}><span>{person.display_name} · {person.present ? 'Present' : 'Not recorded present'} · Revision {person.revision}</span><Button icon="check" variant="ghost" disabled={disabled || read.pending || !!read.error} onClick={e => prepare(person.present ? 'absent' : 'present', e.currentTarget, person)}>Review {person.present ? 'absence' : 'presence'} for {person.display_name}</Button></li>)}</ul>}</section>
}

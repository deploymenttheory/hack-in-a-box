import './session-bookings.css'
import type { ReactNode, RefObject } from 'react'
import type { components } from './api/schema.gen'
import { CollectionRecords } from './components/patterns'
import { Button } from './components/ui/button'
import { ConfirmationDialog } from './components/ui/dialog'
import { Failure } from './components/states'
import { SessionBookingError } from './session-booking-receipt'
import { localDate } from './event-time'

export type BookableSession = components['schemas']['BookableSession']
export function sessionState(session: BookableSession) {
  if (session.cancelled) return 'Session cancelled'
  if (session.needs_review) return 'Date or definition needs organiser review'
  if (session.own_booking?.state === 'booked') return 'Booked'
  if (session.own_booking?.state === 'waitlisted') return `Session waitlist · Position ${session.own_booking.waitlist_position ?? 'not available'}`
  if (session.own_booking?.state === 'cancelled') return 'Your place cancelled'
  if (session.own_booking?.state === 'ineligible') return 'Your place is no longer eligible'
  return session.available > 0 ? 'Available' : 'Full · Session waitlist'
}
export function SessionCollection({ sessions, open, disabled }: { sessions: BookableSession[]; open: (session: BookableSession, opener: HTMLButtonElement) => void; disabled?: boolean }) {
  return <CollectionRecords label="Sessions on this page" rows={sessions} rowKey={s => s.id} rowLabel={s => s.title} disabled={disabled} columns={[
    { id: 'time', label: 'Event-local time', essential: true, cell: s => <>{localDate(s.start, s.timezone)}<br />{s.timezone}</> },
    { id: 'state', label: 'Your session status', essential: true, cell: sessionState },
    { id: 'capacity', label: 'Capacity', cell: s => `${s.available} available · ${s.booked}/${s.capacity} booked` },
  ]} actions={s => <Button icon="calendar" variant="ghost" disabled={disabled} onClick={e => open(s, e.currentTarget)}>Open {s.title}</Button>} />
}
export function SessionFacts({ session }: { session: BookableSession }) {
  return <><p role="status">{sessionState(session)}</p><dl className="fleet-session-facts">{[
    ['Starts', localDate(session.start, session.timezone)], ['Ends', localDate(session.end, session.timezone)], ['Timezone', session.timezone],
    ['Room', session.room || 'Not supplied'], ['Booking opens', localDate(session.booking_opens, session.timezone)], ['Booking closes', localDate(session.booking_closes, session.timezone)],
    ['Capacity', `${session.available} available · ${session.booked}/${session.capacity} booked`], ['Calendar version', `${session.calendar_sequence} · Current schedule observation`],
    ...(session.own_booking ? [['Your calendar sequence', `${session.own_booking.calendar_sequence} · Current personal calendar observation`]] : []),
  ].map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}</dl>
    {session.capacity_shortfall > 0 && <p className="notice">Capacity shortfall: {session.capacity_shortfall}. Existing places remain booked. An organiser must review capacity; nobody is silently removed.</p>}
    {session.conflicts.length > 0 && <p className="notice">Schedule conflict with {session.conflicts.length} other booked session(s). Review your personal agenda before reserving. A conflict is a warning, not a new admission rule.</p>}
    {session.meeting_url && <p><a href={session.meeting_url} target="_blank" rel="noreferrer">Open authorised meeting link</a></p>}
    <p className="fleet-scope-label">A session place or session waitlist never confirms event admission, changes team membership or counts towards minimum event attendance. Calendar notification approval, queueing, provider acceptance and delivery are separate from booking.</p></>
}
export function SessionDecision({ title, children, pending, disabled, confirm, correct, finalFocus }: { title: string; children: ReactNode; pending: boolean; disabled: boolean; confirm: () => void; correct: () => void; finalFocus?: RefObject<HTMLElement | null> }) {
  return <ConfirmationDialog open title={title} description="Review the captured details. The server checks current identity, audience, eligibility and revisions when you confirm." confirmLabel="Confirm reviewed change" onConfirm={confirm} onOpenChange={open => { if (!open) correct() }} pending={pending} disabled={disabled} printExcluded finalFocus={() => finalFocus?.current ?? false}>{children}</ConfirmationDialog>
}

export function SessionFailure({ error, retry }: { error: unknown; retry?: () => void }) { return error instanceof SessionBookingError ? <p role="alert">{error.message}</p> : <Failure error={error} retry={retry} /> }

// Reviews capture only displayed session facts. A meeting-link grant and the
// organiser's private definition are never copied into an intent snapshot.
export function sessionReviewFacts(s: BookableSession): BookableSession {
  return structuredClone({ id:s.id,revision:s.revision,calendar_sequence:s.calendar_sequence,title:s.title,timezone:s.timezone,start:s.start,end:s.end,booking_opens:s.booking_opens,booking_closes:s.booking_closes,capacity:s.capacity,booked:s.booked,available:s.available,capacity_shortfall:s.capacity_shortfall,cancelled:s.cancelled,needs_review:s.needs_review,room:s.room,own_booking:s.own_booking,conflicts:s.conflicts,can_reserve:s.can_reserve,can_manage:s.can_manage,can_read_attendance:s.can_read_attendance,can_record_attendance:s.can_record_attendance })
}

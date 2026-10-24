import { useRef, useState } from 'react'
import { api, APIError, unwrap, type EventRecord } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { usePrivateRequestScope } from './private-request-scope'
import { usePlanningDepartureBarrier, usePlanningUnknownCommands } from './planning-operations'
import { readObservations, type ObservationPage } from './mentor-observation-api'
import { MentorTaskReview, MentorUnknown, useMentorTaskNavigation } from './mentor-task'
import { Button } from './components/ui/button'
import { Field, Select, Textarea } from './components/ui/fields'
import { Failure } from './components/states'
import { localDate } from './event-time'
import { MentorHistory } from './mentor-history'

type State = 'on_track' | 'at_risk' | 'blocked'
type Captured = { revision: number; version: number; state: State; reason: string; next_action: string }
type Status = components['schemas']['MentorOperationalStatus']
export function OperationalStatus({ event, teamID, value, refresh }: { event: EventRecord; teamID: string; value: ObservationPage; refresh: () => Promise<unknown> }) {
  const identity = useIdentity(), owner = `${identity.person.id}:${identity.draft_scope}:${event.id}:${teamID}:status`, history = usePlanningUnknownCommands(owner), scope = usePrivateRequestScope(`${owner}:${value.can_change_status}`)
  const [state, setState] = useState<State>('on_track'), [reason, setReason] = useState(''), [action, setAction] = useState(''), [review, setReview] = useState<Captured>(), [unconfirmed, setUnconfirmed] = useState<Captured>(), [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState(''), [conflict, setConflict] = useState(false)
  const root = useRef<HTMLElement>(null), base = useRef<{ revision: number; version: number } | undefined>(undefined), busy = useRef(false)
  const unknown = !!unconfirmed || (!pending && history.unconfirmed), navigation = useMentorTaskNavigation({ root, dirty: !!reason || !!action || !!review, pending, unknown })
  usePlanningDepartureBarrier(pending)
  function capture() { base.current ??= { revision: value.revision, version: value.status?.version ?? 0 } }
  const matches = (status: Status | null | undefined, intent: Captured) => !!status && status.version === intent.version + 1 && status.actor_id === identity.person.id && status.state === intent.state && status.reason === intent.reason && status.next_action === intent.next_action
  async function save() {
    if (!review || busy.current || unknown || !value.can_change_status) return
    const intent = structuredClone(review), ticket = scope.capture(); if (!ticket.current()) return
    busy.current = true; let attempted = false; setPending(true); setFailure(undefined)
    try {
      const current = await readObservations(event.id, teamID); ticket.assert()
      if (!current.can_change_status || current.revision !== intent.revision || (current.status?.version ?? 0) !== intent.version) { setReview(undefined); setConflict(true); await refresh(); return }
      attempted = true
      history.record({ operation: 'Operational check-in', revision: intent.revision, command: { intentID: `${teamID}:status:${intent.version + 1}` } })
      const result = unwrap(await api.POST('/api/v1/events/{event_id}/teams/{team_id}/mentor-observations', { params: { path: { event_id: event.id, team_id: teamID }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${intent.revision}"` } }, body: { action: 'status', version: intent.version, state: intent.state, reason: intent.reason, next_action: intent.next_action } }))
      ticket.assert(); if (!matches(result.status, intent)) throw new Error('Unverified check-in response')
      history.resolve(`${teamID}:status:${intent.version + 1}`); setReview(undefined); setReason(''); setAction(''); base.current = undefined; setConflict(false); setNotice(`Operational check-in version ${intent.version + 1} saved. This changes neither a formal review nor a score.`); navigation.restore(); await refresh()
    } catch (error) { if (ticket.current()) { setFailure(error); if (attempted && (!(error instanceof APIError) || error.status >= 500 || error.status === 408)) { setUnconfirmed(intent); setNotice('Check-in outcome unknown. No write will be replayed.') } else { if (attempted) history.resolve(`${teamID}:status:${intent.version + 1}`); else setNotice('Current check-in source could not be verified. No write was sent.'); setReview(undefined); if (error instanceof APIError && error.status === 409) setConflict(true); await refresh() } } }
    finally { busy.current = false; if (ticket.current()) setPending(false) }
  }
  async function check() {
    if (!unconfirmed || busy.current) { await refresh(); return }
    const ticket = scope.capture(); if (!ticket.current()) return
    busy.current = true; setPending(true)
    try { const page = unwrap(await api.GET('/api/v1/events/{event_id}/teams/{team_id}/mentor-observations/history', { params: { path: { event_id: event.id, team_id: teamID }, query: { status_cursor: unconfirmed.version + 2, limit: 1 } }, signal: ticket.signal })); ticket.assert(); if (matches(page.statuses[0], unconfirmed)) { history.resolve(`${teamID}:status:${unconfirmed.version + 1}`); setUnconfirmed(undefined); setReview(undefined); setReason(''); setAction(''); base.current = undefined; setNotice('Exact original check-in version confirmed from immutable history. No write was replayed.'); await refresh() } else setNotice('No exact original check-in version was confirmed. Current state is not its receipt; writes remain paused.') } catch (error) { if (ticket.current()) setFailure(error) } finally { busy.current = false; if (ticket.current()) setPending(false) }
  }
  return <section ref={root} tabIndex={-1} className="py-6 border-t border-border" aria-label="Operational check-in task"><h3>Operational check-in</h3><p>Mentor/organiser-only context. This changes neither a formal review nor a score.</p>{value.status ? <><p>{value.status.state.replaceAll('_', ' ')} · {localDate(value.status.changed_at, event.configuration.timezone)}</p><p className="whitespace-pre-wrap">{value.status.reason}</p><p className="whitespace-pre-wrap">Next action: {value.status.next_action || 'None recorded'}</p></> : <p>No check-in recorded.</p>}
    {value.can_change_status && !review && !unknown && <form className="grid gap-4 max-w-3xl py-4" onSubmit={e => { e.preventDefault(); capture(); navigation.capture(); setReview({ revision: base.current!.revision, version: base.current!.version, state, reason, next_action: action }) }}><fieldset className="grid gap-4" disabled={pending}>
      <Field id="mentor-status" label="Progress"><Select value={state} onChange={e => { capture(); setState(e.target.value as State) }}><option value="on_track">On track</option><option value="at_risk">At risk</option><option value="blocked">Blocked</option></Select></Field>
      <Field id="status-reason" label="Check-in reason"><Textarea maxLength={2000} value={reason} onChange={e => { capture(); setReason(e.target.value) }} /></Field>
      <Field id="status-action" label="Next action (mentor/organiser-only)"><Textarea maxLength={2000} value={action} onChange={e => { capture(); setAction(e.target.value) }} /></Field>
      <Button type="submit" icon="next" disabled={!reason.trim()}>Review operational check-in</Button></fieldset></form>}
    {review && !unknown && <MentorTaskReview title="Review operational check-in" pending={pending} confirmLabel="Save operational check-in" confirm={() => void save()} correct={() => { setReview(undefined); navigation.restore() }}><p>Team {teamID} · event revision {review.revision} · status version {review.version}</p><p>{review.state.replaceAll('_', ' ')}</p><p>{review.reason}</p><p>Next action: {review.next_action || 'None'}</p></MentorTaskReview>}
    {unknown && <MentorUnknown intent={`${teamID}:status`} pending={pending} read={() => void check()}>The original check-in remains unresolved. Its immutable expected version and actor/content must match; a current GET cannot unlock replay.</MentorUnknown>}
    {conflict && <div role="status"><p>The check-in changed. Review the currently displayed status before deliberately choosing to replace it.</p><Button icon="refresh" variant="outline" disabled={pending} onClick={() => { base.current = { revision: value.revision, version: value.status?.version ?? 0 }; setConflict(false); setFailure(undefined) }}>Use displayed check-in as base</Button></div>}
    {notice && <p role="status">{notice}</p>}{!!failure && <Failure error={failure} />}<MentorHistory event={event} teamID={teamID} />
  </section>
}

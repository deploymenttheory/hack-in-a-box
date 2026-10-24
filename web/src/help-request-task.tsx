import { useRef, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { api, APIError, unwrap, type EventRecord } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { usePrivateRequestScope } from './private-request-scope'
import { usePlanningDepartureBarrier, usePlanningUnknownCommands } from './planning-operations'
import { useMentorTaskNavigation, MentorTaskReview, MentorUnknown } from './mentor-task'
import { Button } from './components/ui/button'
import { Field, Input, Select, Textarea } from './components/ui/fields'
import { Failure } from './components/states'
import { helpKey } from './help-actions'

type Input = components['schemas']['HelpInput']
type Page = components['schemas']['HelpPage']
type Queue = components['schemas']['HelpQueue']
const empty = { category: 'setup', description: '', location: '', meeting_url: '' } as const
type Intent = { requestID: string; commandID: string; input: Input }
export function HelpRequestTask({ event, context, refresh }: { event: EventRecord; context: Queue; refresh: () => Promise<unknown> }) {
  const identity = useIdentity(), cache = useQueryClient()
  const owner = `${identity.person.id}:${identity.draft_scope}:${event.id}:help-create`, history = usePlanningUnknownCommands(owner)
  const scope = usePrivateRequestScope(`${owner}:${context.request_team?.id}:${context.can_request}`)
  const [form, setForm] = useState<Omit<Input, 'team_id'>>({ ...empty }), [review, setReview] = useState<Intent>(), [pending, setPending] = useState(false), [uncertain, setUncertain] = useState(false), [failure, setFailure] = useState<unknown>(), [created, setCreated] = useState<string>(), [placeError, setPlaceError] = useState(false), [originalID, setOriginalID] = useState<string>(), [notice, setNotice] = useState('Unsent form. Nothing is saved yet.')
  const original = useRef<Intent | undefined>(undefined), busy = useRef(false), terminal = useRef(false), locationInput = useRef<HTMLInputElement>(null)
  const unknown = uncertain || history.unconfirmed
  const dirty = !!form.description || !!form.location || !!form.meeting_url || !!review
  const focusRoot = useRef<HTMLElement>(null)
  const navigation = useMentorTaskNavigation({ root: focusRoot, dirty, pending, unknown })
  usePlanningDepartureBarrier(pending)
  const enabled = context.can_request && !!context.request_team
  function change(next: typeof form) { if (unknown || busy.current) return; setForm(next); setPlaceError(false); setCreated(undefined); setFailure(undefined); setNotice('Unsent form. Nothing in this form is saved yet.') }
  function prepare() {
    if (!enabled || busy.current || unknown || !form.description.trim()) return
    if (!form.location.trim() && !form.meeting_url.trim()) { setPlaceError(true); locationInput.current?.focus(); return }
    navigation.capture(); setReview({ requestID: crypto.randomUUID(), commandID: crypto.randomUUID(), input: { ...structuredClone(form), team_id: context.request_team!.id } })
  }
  async function confirmed(value: Page, intent: Intent) {
    if (value.request.id !== intent.requestID || value.request.event_id !== event.id || value.request.requester_id !== identity.person.id || !Object.entries(intent.input).every(([key, v]) => value.request[key as keyof typeof value.request] === v)) throw new Error('Unexpected creation response')
    cache.setQueryData(helpKey(identity.person.id, event.id, value.request.id), value)
    setCreated(value.request.id); terminal.current = false; setUncertain(false); setForm({ ...empty }); setReview(undefined); original.current = undefined; setOriginalID(undefined); history.clear(); setNotice('Help request saved on the server. Its current status appears in the queue.'); await refresh(); navigation.restore()
  }
  async function submit(retry = false) {
    if (busy.current || !enabled || (!retry && (terminal.current || unknown || !review))) return
    if (!navigator.onLine) { setNotice('Offline: no request submitted. Your unsent form remains in this tab.'); return }
    const intent = retry ? original.current : review
    if (!intent) return
    const ticket = scope.capture(); if (!ticket.current()) return
    busy.current = true; original.current = intent; setOriginalID(intent.commandID); setPending(true); setFailure(undefined)
    try {
      const value = unwrap(await api.PUT('/api/v1/events/{event_id}/help/{request_id}', { params: { path: { event_id: event.id, request_id: intent.requestID }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': '"0"', 'Idempotency-Key': intent.commandID } }, body: intent.input }))
      ticket.assert(); await confirmed(value, intent)
    } catch (error) {
      if (!ticket.current()) return
      setFailure(error)
      if (error instanceof APIError && error.status < 500 && error.status !== 408 && !unknown) { original.current = undefined; setOriginalID(undefined); setReview(undefined); setNotice(error.status === 409 ? 'The server rejected this request. Competition-output support requires the scheduled live start and common challenge publication; review current access and event state.' : 'The server rejected this input or access. No new request was confirmed. Your unsent text remains.') }
      else { terminal.current = true; setUncertain(true); if (!history.unconfirmed) history.record({ operation: 'Create help request', revision: 0, command: { intentID: intent.commandID, requestID: intent.requestID } }); setNotice('Request creation outcome is unconfirmed. Keep this exact intent; nothing is replayed automatically.') }
      await refresh()
    } finally { busy.current = false; if (ticket.current()) setPending(false) }
  }
  async function check() {
    if (busy.current || !original.current) { await refresh(); return }
    const intent = original.current, ticket = scope.capture(); if (!ticket.current()) return
    busy.current = true; setPending(true); setFailure(undefined)
    try { const value = unwrap(await api.GET('/api/v1/events/{event_id}/help/{request_id}', { params: { path: { event_id: event.id, request_id: intent.requestID } }, signal: ticket.signal })); ticket.assert(); await confirmed(value, intent) }
    catch (error) { if (ticket.current()) { setFailure(error); setNotice('Creation is still unconfirmed. A hidden or unavailable request is not proof it was never created. No request was replayed.'); await refresh() } }
    finally { busy.current = false; if (ticket.current()) setPending(false) }
  }
  const correct = () => { setReview(undefined); navigation.restore() }
  return <section ref={focusRoot} tabIndex={-1} className="py-6 border-t border-border" aria-label="Request help"><h2>Ask for help{context.request_team && ` · ${context.request_team.name}`}</h2><p>Private context is visible to your current team and eligible helpers/organisers. Unsent text is kept in this task only.</p>{!enabled ? <p>New requests require a current confirmed place, published team membership and open request creation. Existing authorised requests remain readable.</p> : <>
    {!review && !unknown && <form className="grid gap-4 max-w-3xl" onSubmit={e => { e.preventDefault(); prepare() }}><fieldset className="grid gap-4" disabled={pending}>
      <Field id="help-category" label="Support category"><Select value={form.category} onChange={e => change({ ...form, category: e.target.value as Input['category'] })}><option value="setup">Setup</option><option value="logistics">Logistics</option><option value="competition_output">Competition output (live review window)</option></Select></Field>
      <Field id="help-description" label="What do you need help with?"><Textarea rows={5} maxLength={8000} value={form.description} onChange={e => change({ ...form, description: e.target.value })} /></Field>
      <Field id="help-location" label="Location (or an approved meeting route required)" ><Input aria-invalid={placeError || undefined} aria-describedby={placeError ? "help-location-error" : undefined} ref={locationInput} maxLength={300} value={form.location} onChange={e => change({ ...form, location: e.target.value })} /></Field>
      {placeError && <p id="help-location-error" role="alert">Add a location or an approved HTTPS meeting route before submitting.</p>}
      <Field id="help-meeting" label="Approved HTTPS meeting route (or a location required)" description="Meeting hosts must be approved by an organiser. The server validates them; no arrival estimate is promised."><Input type="url" maxLength={2048} value={form.meeting_url} onChange={e => change({ ...form, meeting_url: e.target.value })} /></Field>
      <Button type="submit" icon="next" disabled={!form.description.trim()}>Review help request</Button></fieldset></form>}
    {review && !unknown && <MentorTaskReview title="Review private help request" pending={pending} confirmLabel="Submit help request" confirm={() => void submit()} correct={correct} cancel={correct}><p>Team {context.request_team!.name} · Request {review.requestID}</p><p>{review.input.category.replaceAll('_', ' ')}</p><p className="whitespace-pre-wrap">{review.input.description}</p><p>Location: {review.input.location || 'Not supplied'} · Meeting route: {review.input.meeting_url || 'Not supplied'}</p><p>This requests operational support. It does not complete a review or change a score.</p></MentorTaskReview>}
    {unknown && <MentorUnknown intent={originalID ?? 'retained unresolved creation'} pending={pending} read={() => void check()}>Request creation remains unconfirmed. The original request UUID and full input must match to establish creation. A missing response or GET denial is not absence.</MentorUnknown>}
    {unknown && originalID && <div className="fleet-pattern-actions"><Button variant="outline" icon="refresh" disabled={pending} onClick={() => void check()}>Check whether request was created</Button><Button variant="outline" disabled={pending || !enabled} onClick={() => void submit(true)}>Retry the same request deliberately</Button></div>}
  </>}
    {!unknown && <Button variant="ghost" icon="remove" disabled={pending} onClick={() => navigation.discard(() => { setForm({ ...empty }); setReview(undefined); setFailure(undefined); setNotice('Local form discarded. No request was sent.') })}>Discard local form</Button>}
    <p role="status" aria-label="Help request save state">{notice}</p>{!!failure && <Failure error={failure} />}{created && <Link to="/events/$eventId/help/$requestId" params={{ eventId: event.id, requestId: created }}>Open saved help request</Link>}
  </section>
}

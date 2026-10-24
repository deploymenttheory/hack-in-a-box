import { useEffect, useRef, useState } from 'react'
import { Link, useParams } from '@tanstack/react-router'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, APIError, privateKey, unwrap, type EventRecord } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { localDate } from './event-time'
import { helpKey } from './help-actions'
import { HelpAttentionQueue } from './help-attention-queue'

type Queue = components['schemas']['HelpQueue']
type Input = components['schemas']['HelpInput']
type Help = components['schemas']['HelpPage']
const empty = { category: 'setup', description: '', location: '', meeting_url: '' } as const
function nativeDenial(error: unknown) { return error instanceof APIError && [401, 403, 404, 409].includes(error.status) }
export function HelpWorkspace() {
  const { eventId } = useParams({ strict: false })
  return eventId ? <Workspace key={eventId} eventID={eventId} /> : <Failure error={new Error('Missing event')} />
}
function Workspace({ eventID }: { eventID: string }) {
  const identity = useIdentity()
  const event = useQuery({ queryKey: privateKey(identity.person.id, eventID, 'event'), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}', { params: { path: { event_id: eventID } } })), refetchInterval: 15_000, refetchOnWindowFocus: 'always' })
  const queue = useQuery({ queryKey: privateKey(identity.person.id, eventID, 'help-queue'), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/help', { params: { path: { event_id: eventID } } })), refetchInterval: 15_000, refetchOnWindowFocus: 'always' })
  if (event.isPending || queue.isPending) return <Loading />
  // Native loss of event/read authority unmounts the private form. A transport
  // failure keeps unsent form state while hiding stale server queue metadata.
  if (!event.data || !queue.data || nativeDenial(event.error) || nativeDenial(queue.error)) return <Failure error={event.error ?? queue.error} retry={() => { void event.refetch(); void queue.refetch() }} />
  return <section className="workspace help-workspace"><Link to="/events/$eventId" params={{ eventId: eventID }}>← Back to event</Link><h1>Team help</h1><p>{event.data.name} · {event.data.configuration.timezone}</p><p>Setup and logistics support are operational. Competition-output help opens at {localDate(event.data.configuration.live_start, event.data.configuration.timezone)}, after actual common challenge publication. Help never completes a formal review or changes a score.</p>
    <RequestForm key={queue.data.request_team?.id ?? 'no-team'} event={event.data} context={queue.data} refresh={() => queue.refetch()} />
    {queue.error || event.error ? <Failure error={queue.error ?? event.error} retry={() => { void event.refetch(); void queue.refetch() }} /> : <HelpAttentionQueue key={`${identity.person.id}:${identity.draft_scope}:${eventID}:${queue.data.can_manage}`} eventID={eventID} actorID={identity.person.id} timezone={event.data.configuration.timezone} value={queue.data} />}
  </section>
}
function RequestForm({ event, context, refresh }: { event: EventRecord; context: Queue; refresh: () => Promise<unknown> }) {
  const identity = useIdentity(), cache = useQueryClient(), [form, setForm] = useState<Omit<Input, 'team_id'>>({ ...empty }), [pending, setPending] = useState(false), [unknown, setUnknown] = useState(false), [notice, setNotice] = useState('Unsent form. Leaving this event or account clears it; nothing is saved yet.'), [failure, setFailure] = useState<unknown>(), [created, setCreated] = useState<string>(), [placeError, setPlaceError] = useState(false)
  const command = useRef<{ requestID: string; commandID: string; input: Input } | undefined>(undefined), generation = useRef(0), locationInput = useRef<HTMLInputElement>(null)
  useEffect(() => () => { generation.current++ }, [])
  function change(next: Omit<Input, 'team_id'>) { setForm(next); setPlaceError(false); setCreated(undefined); setFailure(undefined); setNotice('Unsent form. Nothing in this form is saved yet.') }
  async function confirmed(value: Help) {
    cache.setQueryData(helpKey(identity.person.id, event.id, value.request.id), value)
    setCreated(value.request.id); setUnknown(false); setForm({ ...empty }); command.current = undefined; setNotice('Help request saved on the server. Its current status appears in the queue.'); await refresh()
  }
  function discard() { setForm({ ...empty }); setUnknown(false); command.current = undefined; setFailure(undefined); setNotice('Local form discarded. This does not cancel a request that may already exist on the server; check the queue for its current state.') }
  async function submit(retry = false) {
    if (!context.can_request || !context.request_team) return
    if (!retry && !form.location.trim() && !form.meeting_url.trim()) { setPlaceError(true); setNotice('Add a location or an approved HTTPS meeting route. Nothing has been saved.'); locationInput.current?.focus(); return }
    if (!navigator.onLine) { setNotice('Offline: no request submitted. Your unsent form remains in this tab. Reconnect before deliberately submitting.'); return }
    if (!retry) command.current = { requestID: crypto.randomUUID(), commandID: crypto.randomUUID(), input: { ...form, team_id: context.request_team.id } }
    const intent = command.current
    if (!intent) return
    const ticket = generation.current; setPending(true); setFailure(undefined)
    try {
      const value = unwrap(await api.PUT('/api/v1/events/{event_id}/help/{request_id}', { params: { path: { event_id: event.id, request_id: intent.requestID }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': '"0"', 'Idempotency-Key': intent.commandID } }, body: intent.input }))
      if (ticket === generation.current) await confirmed(value)
    } catch (error) {
      if (ticket !== generation.current) return
      setFailure(error)
      if (error instanceof APIError && error.status < 500 && !unknown) { command.current = undefined; setNotice(error.status === 409 ? 'The server rejected this request. Competition-output support requires the scheduled live start and common challenge publication; review current access and event state.' : 'The server rejected this input or access. No new request was confirmed. Your unsent text remains.') }
      else { setUnknown(true); setNotice('Request creation outcome is unconfirmed. Keep this exact intent until you check the server or deliberately retry it; nothing is replayed automatically.') }
      await refresh()
    } finally { if (ticket === generation.current) setPending(false) }
  }
  async function check() {
    const intent = command.current
    if (!intent) return
    const ticket = generation.current; setPending(true); setFailure(undefined)
    try {
      const value = unwrap(await api.GET('/api/v1/events/{event_id}/help/{request_id}', { params: { path: { event_id: event.id, request_id: intent.requestID } } }))
      if (ticket !== generation.current) return
      const request = value.request, matches = request.requester_id === identity.person.id && Object.entries(intent.input).every(([key, v]) => request[key as keyof typeof request] === v)
      if (matches) await confirmed(value)
      else setNotice('Current server state does not confirm this exact input. No request was replayed. Review the queue before deliberately retrying the same intent.')
    } catch (error) { if (ticket === generation.current) { setFailure(error); setNotice('Creation is still unconfirmed. A hidden or unavailable request is not proof it was never created. Check current access before retrying the same intent.'); await refresh() } } finally { if (ticket === generation.current) setPending(false) }
  }
  if (!context.request_team) return <p>New requests require a current confirmed place and published team membership. Helpers use the authorised queue below.</p>
  return <section className="panel" aria-label="Request help"><h2>Ask for help · {context.request_team.name}</h2><p>Private context is visible to your current team and eligible helpers/organisers. Unsent help text stays only in this open form; it is not saved to browser storage.</p>{!context.can_request ? <p>Request creation is currently closed. Existing authorised requests remain readable.</p> : <form onSubmit={e => { e.preventDefault(); void submit() }}><fieldset disabled={pending || unknown}><label htmlFor="help-category">Support category</label><select id="help-category" value={form.category} onChange={e => change({ ...form, category: e.target.value as Input['category'] })}><option value="setup">Setup</option><option value="logistics">Logistics</option><option value="competition_output">Competition output (live review window)</option></select><label htmlFor="help-description">What do you need help with?</label><textarea id="help-description" rows={5} maxLength={8000} value={form.description} onChange={e => change({ ...form, description: e.target.value })} /><label htmlFor="help-location">Location (or an approved meeting route required)</label><input ref={locationInput} id="help-location" aria-invalid={placeError || undefined} aria-describedby="help-place-requirement" maxLength={300} value={form.location} onChange={e => change({ ...form, location: e.target.value })} /><label htmlFor="help-meeting">Approved HTTPS meeting route (or a location required)</label><input id="help-meeting" aria-invalid={placeError || undefined} aria-describedby="help-place-requirement" type="url" maxLength={2048} value={form.meeting_url} onChange={e => change({ ...form, meeting_url: e.target.value })} /><p id="help-place-requirement">Provide a location or an HTTPS meeting route. Meeting hosts must be approved by an organiser; the server validates them. No arrival estimate is promised.</p>{placeError && <p role="alert">Add a location or an approved HTTPS meeting route before submitting.</p>}<Button type="submit" disabled={!form.description.trim()}>Submit help request</Button></fieldset></form>}
    {unknown && <div className="actions"><Button variant="outline" disabled={pending} onClick={() => void check()}>Check whether request was created</Button><Button variant="outline" disabled={pending || !context.can_request} onClick={() => void submit(true)}>Retry the same request deliberately</Button></div>}<Button variant="outline" type="button" disabled={pending} onClick={discard}>Discard local form</Button><p role="status" aria-label="Help request save state">{notice}</p>{!!failure && <Failure error={failure} />}{created && <Link to="/events/$eventId/help/$requestId" params={{ eventId: event.id, requestId: created }}>Open saved help request</Link>}
  </section>
}

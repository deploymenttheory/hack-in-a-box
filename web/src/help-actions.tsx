import { useLayoutEffect, useRef, useState } from 'react'
import { api, APIError, privateKey, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { useQueryClient } from '@tanstack/react-query'
import { Button } from './components/ui/button'
import { Field, Select } from './components/ui/fields'
import { Failure } from './components/states'
import { usePrivateRequestScope } from './private-request-scope'
import { usePlanningDepartureBarrier, usePlanningUnknownCommands } from './planning-operations'
import { MentorTaskReview, MentorUnknown, useMentorTaskNavigation } from './mentor-task'

type Help = components['schemas']['HelpPage']
type Command = components['schemas']['HelpCommand']
type Intent = { id: string; revision: number; body: Command }
export const helpKey = (person: string, event: string, request: string) => [...privateKey(person, event, 'help-request'), request]
export function helpAuthority(value: Help) { return [value.can_claim, value.can_release, value.can_transfer, value.can_resolve, value.can_reopen, value.can_cancel, ...value.transfer_targets.map(p => p.person_id)].join(':') }
export function HelpActions({ value, refresh }: { value: Help; refresh: () => Promise<unknown> }) {
  const identity = useIdentity(), cache = useQueryClient(), owner = `${identity.person.id}:${identity.draft_scope}:${value.request.event_id}:help:${value.request.id}`
  const scope = usePrivateRequestScope(`${owner}:${helpAuthority(value)}`), history = usePlanningUnknownCommands(owner)
  const [target, setTarget] = useState(''), [review, setReview] = useState<Intent>(), [pending, setPending] = useState(false), [notice, setNotice] = useState(''), [failure, setFailure] = useState<unknown>(), [uncertain, setUncertain] = useState(false), [originalID, setOriginalID] = useState<string>()
  const original = useRef<Intent | undefined>(undefined), busy = useRef(false), terminal = useRef(false)
  const currentAuthority = helpAuthority(value), [authority, setAuthority] = useState(currentAuthority)
  if (authority !== currentAuthority) { setAuthority(currentAuthority); setReview(undefined); setTarget(''); setPending(false); setOriginalID(undefined) }
  useLayoutEffect(() => { original.current = undefined }, [currentAuthority])
  const unknown = uncertain || history.unconfirmed
  const focusRoot = useRef<HTMLElement>(null)
  const navigation = useMentorTaskNavigation({ root: focusRoot, dirty: !!target || !!review, pending, unknown })
  usePlanningDepartureBarrier(pending)
  const controls: [boolean, Command['action'], string][] = [[value.can_claim, 'claim', 'Claim request'], [value.can_release, 'release', 'Release to waiting'], [value.can_resolve, 'resolve', 'Resolve request'], [value.can_reopen, 'reopen', 'Reopen request'], [value.can_cancel, 'cancel', 'Cancel request']]
  function prepare(body: Command) { if (busy.current || unknown) return; navigation.capture(); setReview({ id: crypto.randomUUID(), revision: value.request.revision, body: structuredClone(body) }); setFailure(undefined) }
  async function send(retry = false) {
    const command = retry ? original.current : review
    if (busy.current || !command || (!retry && (terminal.current || unknown || command.revision !== value.request.revision))) return
    if (!navigator.onLine) { setNotice('Offline: no action submitted or queued. Reconnect and check the current request.'); return }
    const ticket = scope.capture(); if (!ticket.current()) return
    busy.current = true; original.current = command; setOriginalID(command.id); setPending(true); setFailure(undefined); setNotice('')
    try {
      const updated = unwrap(await api.POST('/api/v1/events/{event_id}/help/{request_id}/actions', { params: { path: { event_id: value.request.event_id, request_id: value.request.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${command.revision}"`, 'Idempotency-Key': command.id } }, body: command.body }))
      ticket.assert()
      if (updated.request.id !== value.request.id || updated.request.event_id !== value.request.event_id) throw new Error('Unexpected help response')
      cache.setQueryData(helpKey(identity.person.id, value.request.event_id, value.request.id), updated)
      setNotice('Request change saved on the server.'); setUncertain(false); terminal.current = false; original.current = undefined; setOriginalID(undefined); history.clear(); setTarget(''); setReview(undefined); navigation.restore()
      await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, value.request.event_id, 'help-queue') })
    } catch (error) {
      if (!ticket.current()) return
      setFailure(error)
      if (error instanceof APIError && error.status < 500 && error.status !== 408 && !unknown) {
        original.current = undefined; setOriginalID(undefined); setReview(undefined)
        setNotice(error.status === 409 ? 'This request changed before your action completed. The latest owner and state are shown; your action was not applied. Review before choosing again.' : 'The server rejected this action. Current access and request state are checked again below.')
      } else { terminal.current = true; setUncertain(true); if (!history.unconfirmed) history.record({ operation: 'Help ownership change', revision: command.revision, command: { intentID: command.id, requestID: value.request.id } }); setNotice('Action outcome is unconfirmed. Checking shows current state, not an original command receipt. No action is replayed automatically.') }
      await refresh()
    } finally { busy.current = false; if (ticket.current()) setPending(false) }
  }
  const correct = () => { setReview(undefined); navigation.restore() }
  return <section ref={focusRoot} tabIndex={-1} className="py-6 border-t border-border" aria-label="Current request actions"><h3>Request actions</h3><p>Current request revision {value.request.revision}. Reopened requests return to Waiting; their history records the new waiting cycle.</p>
    {!review && !unknown && <><div className="fleet-pattern-actions">{controls.filter(([allowed]) => allowed).map(([, action, label]) => <Button icon={action === 'claim' ? 'user' : action === 'resolve' ? 'check' : action === 'cancel' ? 'close' : 'next'} key={action} disabled={pending} onClick={() => prepare({ action, assignee_id: null })}>{label}</Button>)}</div>
    {value.can_transfer && <form className="grid gap-4 max-w-xl py-4" onSubmit={e => { e.preventDefault(); prepare({ action: 'transfer', assignee_id: target }) }}><Field id="help-transfer" label="Transfer to a currently eligible helper" description="Availability, assignments and conflicts are checked again when transferring."><Select value={target} disabled={pending} onChange={e => setTarget(e.target.value)}><option value="">Choose a helper</option>{value.transfer_targets.map(person => <option key={person.person_id} value={person.person_id}>{person.name} · {person.person_id.slice(0, 8)}</option>)}</Select></Field><Button type="submit" icon="next" disabled={pending || !value.transfer_targets.some(p => p.person_id === target)}>Transfer request</Button></form>}</>}
    {review && !unknown && <MentorTaskReview title="Review help ownership change" pending={pending} disabled={review.revision !== value.request.revision} confirm={() => void send()} correct={correct} cancel={correct}><p>Request {value.request.id} · expected revision {review.revision}</p><p>Action: {review.body.action.replaceAll('_', ' ')} · Current owner: {value.assignee_name || 'Unassigned'}</p>{review.body.assignee_id && <p>New helper: {value.transfer_targets.find(p => p.person_id === review.body.assignee_id)?.name ?? review.body.assignee_id}</p>}<p>This changes operational help only. Formal reviews and scores stay separate.</p>{review.revision !== value.request.revision && <p role="alert">This source changed. Correct details and review current ownership before sending.</p>}</MentorTaskReview>}
    {unknown && <MentorUnknown intent={originalID ?? 'retained unresolved help action'} pending={pending} read={() => void refresh()}>The earlier action remains unconfirmed. Current server state cannot prove its original outcome.</MentorUnknown>}
    {unknown && originalID && <div className="fleet-pattern-actions"><Button icon="refresh" variant="outline" disabled={pending} onClick={() => void refresh()}>Check current request</Button><Button variant="outline" disabled={pending} onClick={() => void send(true)}>Retry the same action deliberately</Button><p>A deliberate retry reuses the exact native idempotency key and command.</p></div>}
    {notice && <p role="status" aria-label="Help action state">{notice}</p>}{!!failure && <Failure error={failure} />}
  </section>
}

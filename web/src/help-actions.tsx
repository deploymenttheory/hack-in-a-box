import { useEffect, useRef, useState } from 'react'
import { api, APIError, privateKey, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { useQueryClient } from '@tanstack/react-query'
import { Button } from './components/ui/button'
import { Failure } from './components/states'

type Help = components['schemas']['HelpPage']
type Command = components['schemas']['HelpCommand']
export const helpKey = (person: string, event: string, request: string) => [...privateKey(person, event, 'help-request'), request]
export function HelpActions({ value, refresh }: { value: Help; refresh: () => Promise<unknown> }) {
  const identity = useIdentity(), cache = useQueryClient(), [target, setTarget] = useState(''), [pending, setPending] = useState(false), [notice, setNotice] = useState(''), [failure, setFailure] = useState<unknown>(), [uncertain, setUncertain] = useState(false)
  const intent = useRef<{ id: string; revision: number; body: Command } | undefined>(undefined), generation = useRef(0)
  useEffect(() => () => { generation.current++ }, [])
  async function act(body: Command, retry = false) {
    if (!navigator.onLine) { setNotice('Offline: no action submitted or queued. Reconnect and check the current request before choosing again.'); return }
    const ticket = generation.current
    if (!retry) intent.current = { id: crypto.randomUUID(), revision: value.request.revision, body }
    const command = intent.current
    if (!command) return
    setPending(true); setFailure(undefined); setNotice('')
    try {
      const updated = unwrap(await api.POST('/api/v1/events/{event_id}/help/{request_id}/actions', { params: { path: { event_id: value.request.event_id, request_id: value.request.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${command.revision}"`, 'Idempotency-Key': command.id } }, body: command.body }))
      if (ticket !== generation.current) return
      cache.setQueryData(helpKey(identity.person.id, value.request.event_id, value.request.id), updated)
      setNotice('Request change saved on the server.'); setUncertain(false); intent.current = undefined; setTarget('')
      await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, value.request.event_id, 'help-queue') })
    } catch (error) {
      if (ticket !== generation.current) return
      setFailure(error)
      if (error instanceof APIError && error.status < 500) {
        setUncertain(false); intent.current = undefined
        setNotice(error.status === 409 ? 'This request changed before your action completed. The latest owner and state are shown; your action was not applied. Review before choosing again.' : 'The server rejected this action. Current access and request state are checked again below.')
      } else { setUncertain(true); setNotice('Action outcome is unconfirmed. Check the current request before deciding whether to retry the same action. No action is replayed automatically.') }
      await refresh()
    } finally { if (ticket === generation.current) setPending(false) }
  }
  const controls: [boolean, Command['action'], string][] = [[value.can_claim, 'claim', 'Claim request'], [value.can_release, 'release', 'Release to waiting'], [value.can_resolve, 'resolve', 'Resolve request'], [value.can_reopen, 'reopen', 'Reopen request'], [value.can_cancel, 'cancel', 'Cancel request']]
  return <section aria-label="Current request actions"><h3>Request actions</h3><p>Actions use the current request revision. Reopened requests return to Waiting; their history records the new waiting cycle.</p><div className="actions">{controls.filter(([allowed]) => allowed).map(([, action, label]) => <Button key={action} type="button" disabled={pending || uncertain} onClick={() => void act({ action, assignee_id: null })}>{label}</Button>)}</div>
    {value.can_transfer && <form onSubmit={e => { e.preventDefault(); void act({ action: 'transfer', assignee_id: target }) }}><label htmlFor="help-transfer">Transfer to a currently eligible helper</label><select id="help-transfer" value={target} disabled={pending || uncertain} onChange={e => { setTarget(e.target.value); setFailure(undefined); setNotice('Transfer selection is unsent. Choose Transfer request to save it.') }}><option value="">Choose a helper</option>{value.transfer_targets.map(person => <option key={person.person_id} value={person.person_id}>{person.name} · {person.person_id.slice(0, 8)}</option>)}</select><p>Only eligible helpers for this request are listed. Availability, assignments and conflicts are checked again when transferring.</p><Button type="submit" disabled={pending || uncertain || !value.transfer_targets.some(p => p.person_id === target)}>Transfer request</Button></form>}
    {uncertain && <div><Button variant="outline" disabled={pending} onClick={() => void refresh()}>Check current request</Button><Button variant="outline" disabled={pending} onClick={() => { if (intent.current) void act(intent.current.body, true) }}>Retry the same action deliberately</Button><p>Checking shows current server state, not a receipt for the earlier command. A deliberate retry reuses its idempotency key.</p></div>}{notice && <p role="status" aria-label="Help action state">{notice}</p>}{!!failure && <Failure error={failure} />}
  </section>
}

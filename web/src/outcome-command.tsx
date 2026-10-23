import { useState } from 'react'
import { useBlocker } from '@tanstack/react-router'
import { APIError, getSession } from './api/client'
import { useIdentity } from './session'
import { usePrivateRequestScope, type PrivateRequestScope, type PrivateRequestTicket } from './private-request-scope'
import { intentReceipt, sendIntent, type NativeIntent, type NativeResult } from './outcome-api'
import { Button } from './components/ui/button'
import { Failure } from './components/states'

export const outcomeDenied = (error: unknown) => error instanceof APIError && [401, 403, 404].includes(error.status)
export function useOutcomeCommand(key: string, parent: PrivateRequestScope, preflight: (intent: NativeIntent, ticket: PrivateRequestTicket) => Promise<void>, accepted: (value: NativeResult) => void, onDeny: (error: unknown) => void) {
  const identity = useIdentity(), scope = usePrivateRequestScope(key, parent)
  const [intent, setIntent] = useState<NativeIntent>(), [busy, setBusy] = useState(false), [error, setError] = useState<unknown>(), [notice, setNotice] = useState(''), [retry, setRetry] = useState(false)
  async function write(next: NativeIntent) {
    if (busy || !navigator.onLine) return
    const ticket = scope.capture(); if (!ticket.current()) return
    setBusy(true); setError(undefined); setRetry(false)
    let sent = false
    try {
      ticket.assert(); const session = await getSession({ signal: ticket.signal }); ticket.assert(); if (session.person.id !== identity.person.id || session.draft_scope !== identity.draft_scope) throw new APIError(403, 'forbidden'); await preflight(next, ticket); ticket.assert()
      setIntent(next); sent = true
      const result = await sendIntent(next, identity.csrf_token); ticket.assert()
      if (result.command_id !== next.command.id || result.version !== next.expected + 1) throw new Error('Unrecognised native receipt')
      setIntent(undefined); setNotice('Saved on the server. The exact command receipt is confirmed.'); accepted(result)
    } catch (failure) {
      if (!ticket.current()) return
      setError(failure)
      if (outcomeDenied(failure)) { scope.invalidate(); onDeny(failure); return }
      if (failure instanceof APIError && [400, 409].includes(failure.status)) { setIntent(undefined); setNotice(failure.status === 400 ? 'The server rejected this input. Nothing was saved; your unsent input remains in this tab.' : 'The server changed. Nothing from this command was saved. Review the current version before a new deliberate save.') }
      else if (sent) setNotice('Save outcome is unknown. Keep this tab open and check the exact command receipt before retrying. Nothing will be replayed automatically.')
      else setNotice('Current access or version could not be checked. No write was sent; unsent input remains in this tab.')
    } finally { if (ticket.current()) setBusy(false) }
  }
  async function check() {
    if (!intent || busy) return
    const captured = intent, ticket = scope.capture(); if (!ticket.current()) return
    setBusy(true); setError(undefined); setRetry(false)
    try {
      const session = await getSession({ signal: ticket.signal }); ticket.assert(); if (session.person.id !== identity.person.id || session.draft_scope !== identity.draft_scope) throw new APIError(403, 'forbidden'); const result = await intentReceipt(captured, ticket.signal); ticket.assert()
      if (result.command_id !== captured.command.id || result.version !== captured.expected + 1) throw new Error('Unrecognised native receipt')
      setIntent(undefined); setNotice('The exact command was saved. No request was replayed.'); accepted(result)
    } catch (failure) {
      if (!ticket.current()) return
      if (failure instanceof APIError && failure.status === 404) {
        try { await preflight(captured, ticket); ticket.assert(); setRetry(true); setNotice('No receipt exists and the original version is still current. You may explicitly retry the same command, or keep editing. No write was sent by this check.') }
        catch (currentError) { if (ticket.current()) { setError(currentError); if (outcomeDenied(currentError)) { scope.invalidate(); onDeny(currentError) } else setNotice('No receipt was found, but current authority/version cannot confirm a safe retry. Review current state; no write was sent.') } }
      } else { setError(failure); if (outcomeDenied(failure)) { scope.invalidate(); onDeny(failure) } }
    } finally { if (ticket.current()) setBusy(false) }
  }
  return { busy, intent, error, notice, retry, write, check, retrySame: () => { if (intent && retry) void write(intent) }, clearNotice: () => { setNotice(''); setError(undefined) } }
}
export function CommandState({ value }: { value: ReturnType<typeof useOutcomeCommand> }) {
  return <div><p role="status" aria-label="Outcome save state">{value.notice}</p>{!!value.error && <Failure error={value.error} />}{value.intent && <div className="actions"><Button type="button" variant="outline" disabled={value.busy} onClick={() => void value.check()}>Check exact save receipt</Button>{value.retry && <Button type="button" disabled={value.busy} onClick={value.retrySame}>Retry the same command explicitly</Button>}</div>}</div>
}
export function useOutcomeUnsent(dirty: boolean) { useBlocker({ shouldBlockFn: () => dirty && !window.confirm('Leave with unsent or unconfirmed outcome changes? Private inputs are not recovered after leaving.'), enableBeforeUnload: dirty }) }

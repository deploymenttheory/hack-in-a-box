import { useEffect, useRef, useState } from 'react'
import { useOutcomeAttempts, type OutcomeOwner, type OutcomeAttempt } from './outcome-safety'
import { useBlocker } from '@tanstack/react-router'
import { APIError, getSession } from './api/client'
import { useIdentity } from './session'
import { usePrivateRequestScope, type PrivateRequestScope, type PrivateRequestTicket } from './private-request-scope'
import { readOutcome, readPublication, readReview, intentReceipt, sendIntent, type NativeIntent, type NativeResult } from './outcome-api'
import { Button } from './components/ui/button'
import { Failure } from './components/states'

export const outcomeDenied = (error: unknown) => error instanceof APIError && [401, 403, 404].includes(error.status)
export function useOutcomeCommand(key: string, resource: OutcomeOwner, parent: PrivateRequestScope, preflight: (intent: NativeIntent, ticket: PrivateRequestTicket) => Promise<void>, accepted: (value: NativeResult) => void, onDeny: (error: unknown) => void) {
  const identity = useIdentity(), scope = usePrivateRequestScope(key, parent), tracking = useOutcomeAttempts(resource), lock = useRef(false)
  const [editorID] = useState(() => crypto.randomUUID()), detach = tracking.detach
  useEffect(() => () => { detach(editorID) }, [detach, editorID])
  const [intent, setIntent] = useState<NativeIntent>(), [busy, setBusy] = useState(false), [error, setError] = useState<unknown>(), [notice, setNotice] = useState(''), [retry, setRetry] = useState(false)
  async function write(next: NativeIntent) {
    if (lock.current || !navigator.onLine || (tracking.blocked() && (!intent || intent.command.id !== next.command.id || !retry))) return
    const retryingUnknown = tracking.blocked() && !!intent && intent.command.id === next.command.id && retry
    const ticket = scope.capture(); if (!ticket.current()) return
    lock.current = true; setBusy(true); setError(undefined); setRetry(false)
    let sent = false
    try {
      ticket.assert(); const session = await getSession({ signal: ticket.signal }); ticket.assert(); if (session.person.id !== identity.person.id || session.draft_scope !== identity.draft_scope) throw new APIError(403, 'forbidden'); await preflight(next, ticket); ticket.assert()
      if (!tracking.begin(next, editorID)) return
      setIntent(next); sent = true
      const result = await sendIntent(next, identity.csrf_token)
      if (result.command_id !== next.command.id || result.version !== next.expected + 1) throw new Error('Unrecognised native receipt')
      tracking.complete(next.command.id); ticket.assert()
      setIntent(undefined); setNotice('Saved on the server. The exact command receipt is confirmed.'); accepted(result)
    } catch (failure) {
      if (sent) {
        if (!retryingUnknown && failure instanceof APIError && [400, 401, 403, 404, 409].includes(failure.status)) tracking.complete(next.command.id)
        else tracking.uncertain(next.command.id)
      }
      if (!ticket.current()) return
      setError(failure)
      if (outcomeDenied(failure)) { scope.invalidate(); onDeny(failure); return }
      if (failure instanceof APIError && [400, 409].includes(failure.status)) { setIntent(retryingUnknown ? next : undefined); setNotice(retryingUnknown ? 'The retry was rejected. The original outcome remains unknown; no replacement is permitted until its exact receipt is confirmed.' : failure.status === 400 ? 'The server rejected this input. Nothing was saved; your unsent input remains in this tab.' : 'The server changed. Nothing from this command was saved. Review the current version before a new deliberate save.') }
      else if (sent) setNotice('Save outcome is unknown. Keep this tab open and check the exact command receipt before retrying. Nothing will be replayed automatically.')
      else setNotice('Current access or version could not be checked. No write was sent; unsent input remains in this tab.')
    } finally { lock.current = false; if (ticket.current()) setBusy(false) }
  }
  async function check() {
    if (!intent || lock.current) return
    const captured = intent, ticket = scope.capture(); if (!ticket.current()) return
    lock.current = true; setBusy(true); setError(undefined); setRetry(false)
    try {
      const session = await getSession({ signal: ticket.signal }); ticket.assert(); if (session.person.id !== identity.person.id || session.draft_scope !== identity.draft_scope) throw new APIError(403, 'forbidden'); const result = await intentReceipt(captured, ticket.signal); ticket.assert()
      if (result.command_id !== captured.command.id || result.version !== captured.expected + 1) throw new Error('Unrecognised native receipt')
      tracking.complete(captured.command.id); setIntent(undefined); setNotice('The exact command was saved. No request was replayed.'); accepted(result)
    } catch (failure) {
      if (!ticket.current()) return
      if (failure instanceof APIError && failure.status === 404) {
        try { await preflight(captured, ticket); ticket.assert(); setRetry(true); setNotice('No receipt exists and the original version is still current. You may explicitly retry the same command, or keep editing. No write was sent by this check.') }
        catch (currentError) { if (ticket.current()) { setError(currentError); if (outcomeDenied(currentError)) { scope.invalidate(); onDeny(currentError) } else setNotice('No receipt was found, but current authority/version cannot confirm a safe retry. Review current state; no write was sent.') } }
      } else { setError(failure); if (outcomeDenied(failure)) { scope.invalidate(); onDeny(failure) } }
    } finally { lock.current = false; if (ticket.current()) setBusy(false) }
  }
  async function recover(attempt: OutcomeAttempt) {
    if (lock.current) return
    const ticket = scope.capture(); if (!ticket.current()) return
    lock.current = true; setBusy(true); setError(undefined)
    try {
      const session = await getSession({ signal: ticket.signal }); ticket.assert()
      if (session.person.id !== identity.person.id || session.draft_scope !== identity.draft_scope) throw new APIError(403, 'forbidden')
      const result = attempt.kind === 'outcome' ? await readOutcome(attempt.outcomeID, ticket.signal, attempt.commandID) : attempt.kind === 'publication' ? await readPublication(attempt.outcomeID, ticket.signal, attempt.commandID) : await readReview(attempt.reviewID!, ticket.signal, attempt.commandID)
      ticket.assert()
      if (result.command_id !== attempt.commandID || result.version !== attempt.expected + 1) throw new Error('Unrecognised native receipt')
      tracking.complete(attempt.commandID)
      setNotice('The exact earlier command was saved. No private draft was restored and no request was replayed.')
      return true
      // It may be a different action's original receipt. Refresh the owner via
      // polling; do not feed it into this private editor's accepted callback.
    } catch (failure) {
      if (!ticket.current()) return
      setError(failure)
      if (failure instanceof APIError && failure.status === 404) setNotice('No original receipt is currently available. The earlier outcome remains unknown; no replacement or retry was sent. Private payloads are not retained across navigation.')
      else if (outcomeDenied(failure)) { scope.invalidate(); onDeny(failure) }
    } finally { lock.current = false; if (ticket.current()) setBusy(false) }
  }
  return { busy, intent, error, notice, retry, write, check, attempts: tracking.attempts, blocked: tracking.attempts.length > 0,
    recover, retrySame: () => { if (intent && retry) void write(intent) }, clearNotice: () => { setNotice(''); setError(undefined) } }

}
export function CommandState({ value }: { value: ReturnType<typeof useOutcomeCommand> }) {
  return <div className="fleet-outcome-command"><p role="status" aria-label="Outcome save state">{value.notice}</p>{!!value.error && <Failure error={value.error} />}
    {value.intent ? <div className="fleet-pattern-actions"><Button type="button" icon="refresh" variant="outline" disabled={value.busy} onClick={() => void value.check()}>Check exact save receipt</Button>{value.retry && <Button type="button" icon="save" disabled={value.busy} onClick={value.retrySame}>Retry the same command explicitly</Button>}</div> : null}
  </div>
}

export function OutcomeRecovery({ resource, scope, deny, refreshed }: { resource: OutcomeOwner; scope: PrivateRequestScope; deny: (error: unknown) => void; refreshed?: () => void }) {
  const identity = useIdentity()
  const operation = useOutcomeCommand(`${identity.person.id}:${identity.draft_scope}:original-outcome-recovery:${resource.kind === 'existing' ? resource.id : resource.eventID ?? 'general'}`, resource, scope, async () => { throw new Error('Recovery is read-only') }, () => {}, deny)
  if (!operation.attempts.length && !operation.notice) return null
  return <aside className="fleet-outcome-section" aria-label="Original outcome recovery"><h2>Original command recovery</h2><p>Only safe command identities remain above private editor and permission changes. Current source facts cannot establish the original acknowledgement; replacement writes remain blocked.</p>
    {operation.attempts.map(attempt => <div key={attempt.commandID}><p>{attempt.action.replaceAll('_', ' ')} · Expected version {attempt.expected} · {attempt.state === 'pending' ? 'Awaiting original acknowledgement' : 'Original outcome unknown'}</p>{!attempt.editorID && <Button icon="refresh" type="button" variant="outline" disabled={operation.busy} onClick={async () => { if (await operation.recover(attempt)) refreshed?.() }}>Check earlier exact save receipt</Button>}{attempt.editorID && <p>Use the original open editor's exact receipt check. Its private payload remains confined to that editor.</p>}</div>)}
    <p role="status" aria-label="Earlier command recovery state">{operation.notice}</p>{!!operation.error && <Failure error={operation.error} />}
  </aside>
}

export function useOutcomeUnsent(dirty: boolean) { useBlocker({ shouldBlockFn: () => dirty && !window.confirm('Leave with unsent or unconfirmed outcome changes? Private inputs are not recovered after leaving.'), enableBeforeUnload: dirty }) }

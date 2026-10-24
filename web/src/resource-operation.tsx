import { useRef, useState } from 'react'
import { useBlocker } from '@tanstack/react-router'
import { APIError } from './api/client'
import { usePrivateRequestScope, type PrivateRequestTicket } from './private-request-scope'
import { usePlanningDepartureBarrier, usePlanningUnknownCommands } from './planning-operations'
import { Button } from './components/ui/button'
import { Alert } from './components/ui/surfaces'
import { Failure } from './components/states'

type Intent = { operation: string; revision: number; command: unknown }
export const resourceDenied = (error: unknown): error is APIError => error instanceof APIError && [401, 403, 404].includes(error.status)
// Native resources expose versions/history, not client-command receipts. Reads
// can establish a new reviewed base while the original unknown capsule remains.
export function useResourceOperation(owner: string, deny?: (error: unknown) => void) {
  const scope = usePrivateRequestScope(owner), history = usePlanningUnknownCommands(owner), lock = useRef(false)
  const [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>(), [stale, setStale] = useState(false), [notice, setNotice] = useState(''), [reading, setReading] = useState(false), [currentRead, setCurrentRead] = useState(false)
  const blocked = pending || history.unconfirmed || stale
  usePlanningDepartureBarrier(blocked)
  useBlocker({ shouldBlockFn: () => lock.current || blocked, enableBeforeUnload: blocked || history.attempts.length > 0 })
  async function run<T>(intent: Intent, write: () => Promise<T>, accept: (value: T, ticket: PrivateRequestTicket) => string | Promise<string>, rejected?: (error: APIError, ticket: PrivateRequestTicket) => Promise<void>) {
    if (lock.current || blocked) return false
    const ticket = scope.capture(); if (!ticket.current()) return false
    lock.current = true; setPending(true); setFailure(undefined); setNotice(''); setCurrentRead(false)
    try {
      const value = await write(); ticket.assert()
      const message = await accept(value, ticket); ticket.assert()
      setNotice(message); lock.current = false
      return true
    } catch (error) {
      if (!ticket.current()) return false
      setFailure(error)
      if (resourceDenied(error)) { history.clear(); deny?.(error); lock.current = false }
      else if (!(error instanceof APIError) || error.status >= 500 || error.status === 408) history.record(structuredClone(intent))
      else if (error.code === 'revision_conflict') setStale(true)
      else {
        lock.current = false
        if (rejected) {
          try { await rejected(error, ticket); ticket.assert() }
          catch (readError) { if (ticket.current()) { setFailure(readError); if (resourceDenied(readError)) { history.clear(); deny?.(readError) } } }
        }
      }
      return false
    } finally { if (ticket.current()) setPending(false) }
  }
  async function read<T>(get: (signal: AbortSignal) => Promise<T>, show: (value: T, ticket: PrivateRequestTicket) => void | Promise<void>) {
    if (reading || pending) return
    const ticket = scope.capture(); if (!ticket.current()) return
    setReading(true); setFailure(undefined); setCurrentRead(false)
    try { const value = await get(ticket.signal); ticket.assert(); await show(value, ticket); ticket.assert(); setCurrentRead(true) }
    catch (error) { if (ticket.current()) { setFailure(error); if (resourceDenied(error)) { history.clear(); deny?.(error) } } }
    finally { if (ticket.current()) setReading(false) }
  }
  function useCurrent() { if (!currentRead || pending || reading) return; history.selectNewBase(); setStale(false); setCurrentRead(false); setFailure(undefined); lock.current = false }
  function canLeave() { return !lock.current && !blocked && (!history.attempts.length || window.confirm('Leave with earlier command outcomes still unconfirmed? The original request was not replayed; its in-memory context may be cleared.')) }
  return { capture: () => scope.capture(), run, read, useCurrent, canLeave, blocked, pending, reading, failure, notice, stale, currentRead, unknown: history.unconfirmed, attempts: history.attempts, isBlocked: () => lock.current || blocked }
}
export function ResourceOperationResult({ operation, read, acceptCurrent, hideCurrentChoice = false }: { operation: ReturnType<typeof useResourceOperation>; read: () => void; hideCurrentChoice?: boolean; acceptCurrent?: { label: string; disabled: boolean; onSelect: () => void } }) {
  return <>{operation.pending && <p role="status">Sending the reviewed request…</p>}{operation.notice && <p role="status">{operation.notice}</p>}{!!operation.failure && !operation.unknown && <Failure error={operation.failure} />}{(operation.unknown || operation.stale) && <Alert tone="warning" role="status"><h3>{operation.unknown ? 'Outcome unknown' : 'Current version needed'}</h3><p>{operation.unknown ? 'The original request may have completed. Current state does not prove this exact request. No write is replayed automatically.' : 'The server rejected the stale revision. Read the current state, preserve any unsaved input, then review before another deliberate action.'}</p><Button icon="refresh" variant="outline" pending={operation.reading} onClick={read}>Read current state</Button>{operation.currentRead && <><p>The current server facts are displayed. The original unknown outcome remains unconfirmed.</p>{!hideCurrentChoice && <Button icon="check" variant="outline" disabled={acceptCurrent?.disabled} onClick={acceptCurrent?.onSelect ?? operation.useCurrent}>{acceptCurrent?.label ?? "Use reviewed current state"}</Button>}</>}</Alert>}{operation.attempts.length > 0 && <section className="fleet-resource-section" aria-label="Earlier unconfirmed resource commands"><h3>Earlier unconfirmed requests</h3><ul>{operation.attempts.map((intent, index) => <li key={index}>{intent.operation} · Expected revision {intent.revision} · Original outcome remains unconfirmed</li>)}</ul></section>}</>
}

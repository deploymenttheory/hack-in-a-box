import { useRef, useState } from 'react'
import { useBlocker } from '@tanstack/react-router'
import { usePlanningDepartureBarrier, usePlanningUnknownCommands } from './planning-operations'
import { APIError } from './api/client'
import { usePrivateRequestScope } from './private-request-scope'
import { Button } from './components/ui/button'
import { Alert } from './components/ui/surfaces'
import { Failure } from './components/states'

export type AdmissionIntent = { label: string; resource: string; revision?: number }
type Outcome = { kind: 'idle' } | { kind: 'pending' | 'uncertain' | 'stale'; intent: AdmissionIntent } | { kind: 'rejected'; error: unknown } | { kind: 'confirmed'; message: string }
export function uncertainAdmission(error: unknown) {
  return !(error instanceof APIError) || error.status >= 500 || error.status === 408
}

// Registration responses are resource revisions, not immutable command receipts.
// An unknown write remains fenced; checking current state never replays it.
export function useAdmissionCommand(scopeKey: string) {
  const scope = usePrivateRequestScope(scopeKey), lock = useRef(false)
  const history = usePlanningUnknownCommands(`admission:${scopeKey}`)
  const remembered = history.attempts.at(-1)?.command as AdmissionIntent | undefined
  const [outcome, setOutcome] = useState<Outcome>({ kind: 'idle' })
  const [failure, setFailure] = useState<unknown>(), [reading, setReading] = useState(false)
  const visibleOutcome: Outcome = outcome.kind === 'idle' && history.unconfirmed && remembered ? { kind: 'uncertain', intent: remembered } : outcome
  usePlanningDepartureBarrier(outcome.kind === 'pending')
  useBlocker({ shouldBlockFn: () => lock.current && outcome.kind === 'pending', enableBeforeUnload: outcome.kind === 'pending' || history.unconfirmed })
  async function run<T>(intent: AdmissionIntent, write: () => Promise<T>, confirmed: (value: T) => string | Promise<string>) {
    if (lock.current || history.unconfirmed) return false
    const ticket = scope.capture()
    if (!ticket.current()) return false
    lock.current = true; setFailure(undefined); setOutcome({ kind: 'pending', intent })
    try {
      const result = await write()
      if (!ticket.current()) return false
      const message = await confirmed(result)
      if (!ticket.current()) return false
      setOutcome({ kind: 'confirmed', message }); lock.current = false
      return true
    } catch (error) {
      if (!ticket.current()) return false
      setFailure(error)
      if (uncertainAdmission(error)) { history.record({ operation: intent.label, revision: intent.revision ?? 0, command: intent }); setOutcome({ kind: 'uncertain', intent }) }
      else if (error instanceof APIError && error.status === 409 && error.code === 'revision_conflict') setOutcome({ kind: 'stale', intent })
      else { if (error instanceof APIError && [401, 403, 404].includes(error.status)) history.clear(); setOutcome({ kind: 'rejected', error }); lock.current = false }
      return false
    }
  }
  async function readCurrent(read: () => Promise<unknown>) {
    if (reading) return
    const ticket = scope.capture()
    if (!ticket.current()) return
    setReading(true); setFailure(undefined)
    try {
      await read()
      if (ticket.current() && outcome.kind === 'stale') { setOutcome({ kind: 'idle' }); lock.current = false }
    } catch (error) { if (ticket.current()) { if (error instanceof APIError && [401, 403, 404].includes(error.status)) history.clear(); setFailure(error) } }
    finally { if (ticket.current()) setReading(false) }
  }
  return { outcome: visibleOutcome, failure, reading, run, readCurrent, isBlocked: () => lock.current || history.unconfirmed, pending: outcome.kind === 'pending', blocked: history.unconfirmed || outcome.kind === 'pending' || outcome.kind === 'uncertain' || outcome.kind === 'stale' }
}
export function AdmissionFeedback({ command, read }: { command: ReturnType<typeof useAdmissionCommand>; read: () => Promise<unknown> }) {
  const value = command.outcome
  if (value.kind === 'idle') return null
  if (value.kind === 'confirmed') return <p role="status" className="fleet-admission-receipt">{value.message}</p>
  if (value.kind === 'rejected') return <Failure error={value.error} />
  if (value.kind === 'pending') return <p role="status">Sending {value.intent.label.toLowerCase()}…</p>
  return <Alert tone="warning" role="status"><h3>{value.kind === 'uncertain' ? 'Outcome unknown' : 'Current version needed'}</h3><p>{value.intent.label} · Reference {value.intent.resource}{value.intent.revision !== undefined && ' · Expected revision ' + value.intent.revision}</p><p>{value.kind === 'uncertain' ? 'The original request may have completed. No request will be sent again automatically. Read the current server state; a latest resource version does not prove the outcome of this exact request.' : 'The server rejected the stale request. Read the current version, then review your retained input before another deliberate decision.'}</p><Button variant="outline" icon="refresh" pending={command.reading} onClick={() => void command.readCurrent(read)}>Check current status</Button>{!!command.failure && <p>The current check did not establish an exact command outcome. Keep this request context; no write was replayed.</p>}</Alert>
}

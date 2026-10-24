import { useBlocker } from '@tanstack/react-router'
import { createContext, useCallback, useContext, useLayoutEffect, useState, type ReactNode } from 'react'

// Event-lifetime, memory-only evidence of an uncertain attempt. A subsequent
// GET or successful different command never proves the original result.
type Attempt = { operation: string; revision: number; command: unknown; acknowledged?: true }
type History = Record<string, { requiresReview: boolean; attempts: Attempt[] }>
type Redact = (owns: (owner: string) => boolean, redact: (attempt: Attempt) => Attempt) => void
const Operations = createContext<{ history: History; update: (owner: string, change: (value: History[string]) => History[string]) => void; redact: Redact } | null>(null)
export function PlanningOperations({ children }: { children: ReactNode }) {
  const [history, setHistory] = useState<History>({})
  const update = useCallback((owner: string, change: (value: History[string]) => History[string]) => setHistory(previous => ({ ...previous, [owner]: change(previous[owner] ?? { requiresReview: false, attempts: [] }) })), [])
  const redact = useCallback<Redact>((owns, redactAttempt) => setHistory(previous => Object.fromEntries(Object.entries(previous).map(([owner, value]) => [owner, owns(owner) ? { ...value, attempts: value.attempts.map(redactAttempt) } : value]))), [])
  const attempts = Object.values(history).flatMap(value => value.attempts)
  useBlocker({ shouldBlockFn: () => attempts.length > 0 && (!planningDepartureAllowed() || !window.confirm('Leave this event session with earlier command outcomes still unconfirmed? Their in-memory history will be cleared.')), enableBeforeUnload: attempts.length > 0 })
  return <Operations.Provider value={{ history, update, redact }}>{attempts.length > 0 && <aside className="fleet-planning-section" aria-label="Unconfirmed command history"><h2>Unconfirmed earlier commands</h2><p>Retained in memory for this event session. Current reads and later commands do not establish these original outcomes.</p><ul>{attempts.map((attempt, index) => <li key={index}>{attempt.operation} · Expected revision {attempt.revision} · Original outcome remains unconfirmed</li>)}</ul></aside>}{children}</Operations.Provider>
}
// Redaction retains the original unknown outcome/revision and departure policy.
// An owner must match its exact resource namespace; unrelated histories survive.
export function usePlanningUnknownRedaction(owns: (owner: string) => boolean) {
  const shared = useContext(Operations)
  return (redact: (attempt: Attempt) => Attempt) => shared?.redact(owns, redact)
}
export function usePlanningUnknownCommands(owner: string) {
  const shared = useContext(Operations)
  const [local, setLocal] = useState<History[string]>({ requiresReview: false, attempts: [] })
  const value = shared?.history[owner] ?? local
  function update(change: (value: History[string]) => History[string]) { if (shared) shared.update(owner, change); else setLocal(change) }
  return { unconfirmed: value.requiresReview, attempts: value.attempts,
    record: (attempt: Attempt) => update(previous => ({ requiresReview: true, attempts: [...previous.attempts, structuredClone(attempt)] })),
    redactCommands: (redact: (command: unknown) => unknown) => update(previous => ({ ...previous, attempts: previous.attempts.map(attempt => ({ ...attempt, command: redact(attempt.command) })) })),
    resolve: (intentID: string) => update(previous => { const attempts = previous.attempts.filter(attempt => !(attempt.command && typeof attempt.command === 'object' && 'intentID' in attempt.command && attempt.command.intentID === intentID)); return { requiresReview: attempts.some(attempt => !attempt.acknowledged), attempts } }),
    clear: () => update(() => ({ requiresReview: false, attempts: [] })),
    // A deliberate new base permits a different command; the old capsule stays.
    selectNewBase: () => update(previous => ({ ...previous, requiresReview: false, attempts: previous.attempts.map(attempt => ({ ...attempt, acknowledged: true })) })) }
}
export function planningDepartureAllowed() { return window.dispatchEvent(new Event('planning-departure', { cancelable: true })) }
export function usePlanningDepartureBarrier(blocked: boolean) {
  useLayoutEffect(() => {
    if (!blocked) return
    const prevent = (event: Event) => event.preventDefault()
    window.addEventListener('planning-departure', prevent)
    return () => window.removeEventListener('planning-departure', prevent)
  }, [blocked])
}

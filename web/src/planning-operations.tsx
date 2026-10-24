import { useBlocker } from '@tanstack/react-router'
import { createContext, useCallback, useContext, useLayoutEffect, useState, type ReactNode } from 'react'

// Event-lifetime, memory-only evidence of an uncertain attempt. A subsequent
// GET or successful different command never proves the original result.
type Attempt = { operation: string; revision: number; command: unknown }
type History = Record<string, { requiresReview: boolean; attempts: Attempt[] }>
const Operations = createContext<{ history: History; update: (owner: string, change: (value: History[string]) => History[string]) => void } | null>(null)
export function PlanningOperations({ children }: { children: ReactNode }) {
  const [history, setHistory] = useState<History>({})
  const update = useCallback((owner: string, change: (value: History[string]) => History[string]) => setHistory(previous => ({ ...previous, [owner]: change(previous[owner] ?? { requiresReview: false, attempts: [] }) })), [])
  const attempts = Object.values(history).flatMap(value => value.attempts)
  useBlocker({ shouldBlockFn: () => attempts.length > 0 && (!planningDepartureAllowed() || !window.confirm('Leave this event session with earlier command outcomes still unconfirmed? Their in-memory history will be cleared.')), enableBeforeUnload: attempts.length > 0 })
  return <Operations.Provider value={{ history, update }}>{attempts.length > 0 && <aside className="fleet-planning-section" aria-label="Unconfirmed command history"><h2>Unconfirmed earlier commands</h2><p>Retained in memory for this event session. Current reads and later commands do not establish these original outcomes.</p><ul>{attempts.map((attempt, index) => <li key={index}>{attempt.operation} · Expected revision {attempt.revision} · Original outcome remains unconfirmed</li>)}</ul></aside>}{children}</Operations.Provider>
}
export function usePlanningUnknownCommands(owner: string) {
  const shared = useContext(Operations)
  const [local, setLocal] = useState<History[string]>({ requiresReview: false, attempts: [] })
  const value = shared?.history[owner] ?? local
  function update(change: (value: History[string]) => History[string]) { if (shared) shared.update(owner, change); else setLocal(change) }
  return { unconfirmed: value.requiresReview, attempts: value.attempts,
    record: (attempt: Attempt) => update(previous => ({ requiresReview: true, attempts: [...previous.attempts, structuredClone(attempt)] })),
    redactCommands: (redact: (command: unknown) => unknown) => update(previous => ({ ...previous, attempts: previous.attempts.map(attempt => ({ ...attempt, command: redact(attempt.command) })) })),
    clear: () => update(() => ({ requiresReview: false, attempts: [] })),
    // A deliberate new base permits a different command; the old capsule stays.
    selectNewBase: () => update(previous => ({ ...previous, requiresReview: false })) }
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

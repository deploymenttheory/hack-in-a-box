import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'

type State = components['schemas']['SMTPTestDeliveryStatus']['state']
export type QueuedEmailTest = { testID: string; campaignID: string; campaignVersion: number; providerRevision: string; state?: State }
type Metadata = { intentID?: string; action: string; revision?: number; requestID?: string }
type Attempt = { id: string; operation: string; revision: number; command: Metadata; state: 'pending' | 'unknown'; blocks: boolean }
type Memory = { tests: Record<string, QueuedEmailTest[]>; commands: Record<string, Attempt[]> }
type Store = { memory: Memory; change: (update: (value: Memory) => Memory) => boolean }
const Context = createContext<Store | null>(null)
const empty = (): Memory => ({ tests: {}, commands: {} })
function unresolved(test: QueuedEmailTest) { return !test.state || ['pending', 'retryable', 'ambiguous'].includes(test.state) }

// Authenticated actor/session lifetime, above sibling routes and presentation
// keys. Only explicitly whitelisted metadata belongs here; never private drafts,
// rendered output, recipients, reasons, connection labels or credentials.
export function CommunicationSafety({ children }: { children: ReactNode }) {
  const [memory, setMemory] = useState<Memory>(empty), current = useRef(memory), active = useRef(true)
  const change = useCallback((update: (value: Memory) => Memory) => { if (!active.current) return false; current.current = update(current.current); setMemory(current.current); return true }, [])
  useEffect(() => {
    active.current = true
    const clear = () => { active.current = false; current.current = empty(); setMemory(current.current) }
    const events = ['session-ending', 'session-expired', 'mentor-draft-invalidated']
    for (const name of events) window.addEventListener(name, clear)
    return () => { active.current = false; for (const name of events) window.removeEventListener(name, clear) }
  }, [])
  return <Context.Provider value={{ memory, change }}>{children}</Context.Provider>
}
function useStore() { const store = useContext(Context); if (!store) throw new Error('Communications require authenticated safety tracking'); return store }
export function useCommunicationTests(eventID: string) {
  const identity = useIdentity(), store = useStore(), owner = `${identity.person.id}:${identity.draft_scope}:${eventID}`
  const items = store.memory.tests[owner] ?? [], change = store.change
  const observed = useCallback((testID: string, state: State) => { change(value => ({ ...value, tests: { ...value.tests, [owner]: (value.tests[owner] ?? []).map(test => test.testID === testID && test.state !== state ? { ...test, state } : test) } })) }, [change, owner])
  return { items, blocked: () => items.some(unresolved), observed,
    queued: (test: Omit<QueuedEmailTest, 'state'>) => store.change(value => {
      const previous = value.tests[owner] ?? []
      return previous.some(item => item.testID === test.testID) ? value : { ...value, tests: { ...value.tests, [owner]: [...previous.filter(unresolved), ...previous.filter(item => !unresolved(item)).slice(-7), test] } }
    }) }
}
export function useCommunicationCommands(owner: string) {
  const store = useStore(), attempts = store.memory.commands[owner] ?? []
  function update(change: (value: Attempt[]) => Attempt[]) { return store.change(value => ({ ...value, commands: { ...value.commands, [owner]: change(value.commands[owner] ?? []) } })) }
  return { attempts, unconfirmed: attempts.some(attempt => attempt.blocks || attempt.state === 'pending'), pending: attempts.some(attempt => attempt.state === 'pending'),
    begin: (attempt: { operation: string; revision: number; command: Metadata }, existing?: string) => {
      const id = existing ?? crypto.randomUUID()
      // Whitelist again at the storage boundary: caller objects cannot smuggle
      // private text or secret-bearing command payloads into session memory.
      const { intentID, action, revision, requestID } = attempt.command
      update(previous => existing ? previous.map(item => item.id === existing ? { ...item, state: 'pending', blocks: true } : item) : [...previous, { id, operation: attempt.operation, revision: attempt.revision, command: { intentID, action, revision, requestID }, state: 'pending', blocks: true }])
      return id
    },
    complete: (id: string) => update(previous => previous.filter(attempt => attempt.id !== id)),
    uncertain: (id: string) => update(previous => previous.map(attempt => attempt.id === id ? { ...attempt, state: 'unknown', blocks: true } : attempt)),
    selectNewBase: () => update(previous => previous.some(attempt => attempt.state === 'pending') ? previous : previous.map(attempt => ({ ...attempt, blocks: false }))) }
}
export function CommunicationCommandHistory({ attempts }: { attempts: Attempt[] }) {
  return attempts.length > 0 && <aside className="fleet-planning-section" aria-label="Unconfirmed command history"><h2>Unconfirmed earlier commands</h2><p>Safe metadata is retained for this signed-in session, across routes and editing versions. Current facts do not establish these original acknowledgements.</p><ul>{attempts.map(attempt => <li key={attempt.id}>{attempt.operation} · Expected revision {attempt.revision} · {attempt.state === 'pending' ? 'Awaiting original acknowledgement' : 'Original outcome remains unconfirmed'}</li>)}</ul></aside>
}

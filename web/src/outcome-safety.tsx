import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { useIdentity } from './session'
import type { NativeIntent } from './outcome-api'

export type OutcomeOwner = { kind: 'existing'; id: string; eventID?: string | null } | { kind: 'proposal'; eventID?: string }
export type OutcomeAttempt = { kind: NativeIntent['kind']; outcomeID: string; reviewID?: string; commandID: string; expected: number; action: string; state: 'pending' | 'unknown'; editorID?: string }
type Memory = Record<string, OutcomeAttempt[]>
type Store = { memory: Memory; change: (update: (value: Memory) => Memory) => boolean; current: () => Memory }
const Context = createContext<Store | null>(null)
const uuid = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/i

// Strict metadata-only boundary, above private route/editor lifetimes. Never
// retain a command body, reason, definition, evidence, links or rendered text.
export function outcomeMetadata(intent: NativeIntent): OutcomeAttempt {
  if (!['outcome', 'publication', 'review'].includes(intent.kind)) throw new Error('Invalid outcome command kind')
  const { kind, outcomeID, expected } = intent, commandID = intent.command.id, action = intent.command.action
  const actions = kind === 'outcome' ? ['propose', 'revise', 'adopt', 'moderate', 'assign', 'progress', 'evidence'] : kind === 'publication' ? ['save', 'publish', 'withdraw'] : ['save', 'approve', 'cancel']
  const reviewID = kind === 'review' ? intent.reviewID : undefined
  if (!uuid.test(outcomeID) || !uuid.test(commandID) || (reviewID !== undefined && !uuid.test(reviewID)) || !Number.isSafeInteger(expected) || expected < 0 || !actions.includes(action)) throw new Error('Invalid outcome command metadata')
  return { kind, outcomeID, expected, commandID, action, ...(reviewID ? { reviewID } : {}), state: 'pending' }
}
export function OutcomeSafety({ children }: { children: ReactNode }) {
  const [memory, setMemory] = useState<Memory>({}), latest = useRef(memory), active = useRef(true)
  const change = useCallback((update: (value: Memory) => Memory) => {
    if (!active.current) return false
    latest.current = update(latest.current); setMemory(latest.current); return true
  }, [])
  useEffect(() => {
    active.current = true
    const clear = () => { active.current = false; latest.current = {}; setMemory({}) }
    const events = ['session-ending', 'session-expired', 'mentor-draft-invalidated']
    for (const event of events) window.addEventListener(event, clear)
    return () => { active.current = false; for (const event of events) window.removeEventListener(event, clear) }
  }, [])
  return <Context.Provider value={{ memory, change, current: () => latest.current }}>{children}</Context.Provider>
}
export function useOutcomeAttempts(resource: OutcomeOwner) {
  const store = useContext(Context), identity = useIdentity()
  if (!store) throw new Error('Outcome commands require authenticated metadata tracking')
  // Existing resource IDs remain the same across event adoption, route changes,
  // source revisions and capability downgrades. Proposal scopes retain event ID.
  if (!['existing', 'proposal'].includes(resource.kind)) throw new Error('Invalid outcome owner kind')
  const resourceID = resource.kind === 'existing' ? (uuid.test(resource.id) ? resource.id : 'invalid-resource') : resource.eventID ? (uuid.test(resource.eventID) ? resource.eventID : 'invalid-event') : null
  const owner = JSON.stringify([identity.person.id, identity.draft_scope, resource.kind, resourceID])
  const attempts = store.memory[owner] ?? [], change = store.change
  const detach = useCallback((editorID: string) => change(memory => ({ ...memory, [owner]: (memory[owner] ?? []).map(item => item.editorID === editorID ? { ...item, editorID: undefined } : item) })), [change, owner])
  return { attempts, detach,
    begin: (intent: NativeIntent, editorID?: string) => {
      if (editorID !== undefined && !uuid.test(editorID)) throw new Error('Invalid outcome editor identity')
      const metadata = outcomeMetadata(intent)
      let allowed = false
      store.change(memory => {
        const prior = memory[owner] ?? []
        if (prior.some(item => item.commandID !== metadata.commandID)) return memory
        allowed = true
        return { ...memory, [owner]: [{ ...metadata, ...(editorID ? { editorID } : {}) }] }
      })
      return allowed
    },
    blocked: () => (store.current()[owner] ?? []).length > 0,
    complete: (id: string) => store.change(memory => ({ ...memory, [owner]: (memory[owner] ?? []).filter(item => item.commandID !== id) })),
    uncertain: (id: string) => store.change(memory => ({ ...memory, [owner]: (memory[owner] ?? []).map(item => item.commandID === id ? { ...item, state: 'unknown' } : item) })),
  }
}

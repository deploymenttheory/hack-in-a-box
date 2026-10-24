import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import type { components } from './api/schema.gen'

type State = components['schemas']['SMTPTestDeliveryStatus']['state']
// Known queue acknowledgements have their own identity. This is not uncertain
// command history: only a GET for this exact native TEST can settle its state.
export type QueuedEmailTest = { testID: string; campaignID: string; campaignVersion: number; providerRevision: string; state?: State }
type Tests = {
  items: QueuedEmailTest[]
  blocked: () => boolean
  queued: (test: Omit<QueuedEmailTest, 'state'>) => void
  observed: (testID: string, state: State) => void
}
const Context = createContext<Tests | null>(null)
export function CommunicationTests({ children }: { children: ReactNode }) {
  const inherited = useContext(Context)
  return inherited ? children : <TestLifetime>{children}</TestLifetime>
}
function unresolved(test: QueuedEmailTest) { return !test.state || ['pending', 'retryable', 'ambiguous'].includes(test.state) }
function TestLifetime({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<QueuedEmailTest[]>([]), current = useRef(items)
  const update = useCallback((change: (items: QueuedEmailTest[]) => QueuedEmailTest[]) => { current.current = change(current.current); setItems(current.current) }, [])
  const observed = useCallback((testID: string, state: State) => update(previous => previous.map(test => test.testID === testID && test.state !== state ? { ...test, state } : test)), [update])
  useEffect(() => {
    const clear = () => update(() => [])
    const events = ['session-ending', 'session-expired', 'mentor-draft-invalidated']
    for (const name of events) window.addEventListener(name, clear)
    return () => { for (const name of events) window.removeEventListener(name, clear) }
  }, [update])
  return <Context.Provider value={{ items, blocked: () => current.current.some(unresolved), queued: test => update(previous => previous.some(value => value.testID === test.testID) ? previous : [...previous.filter(unresolved), ...previous.filter(value => !unresolved(value)).slice(-7), test]), observed }}>{children}</Context.Provider>
}
export function useCommunicationTests() {
  const value = useContext(Context)
  if (!value) throw new Error('Communication tests require their event lifetime')
  return value
}

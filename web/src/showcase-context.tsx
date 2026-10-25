import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react'

const ShowcaseContext = createContext<{ term: string; setTerm: (term: string) => void; clear: () => void } | null>(null)

// Lives inside the identity/session-keyed private cache. Search text may describe
// restricted work, so it must never enter document URLs or persistent storage.
export function ShowcaseMemory({ children }: { children: ReactNode }) {
 const [term, setTerm] = useState('')
 const clear = useCallback(() => setTerm(''), [])
 useEffect(() => {
  const events = ['session-ending', 'session-expired', 'mentor-draft-invalidated']
  for (const event of events) window.addEventListener(event, clear)
  return () => { for (const event of events) window.removeEventListener(event, clear) }
 }, [clear])
 return <ShowcaseContext.Provider value={{ term, setTerm, clear }}>{children}</ShowcaseContext.Provider>
}
export function useShowcaseMemory() {
 const memory = useContext(ShowcaseContext)
 if (!memory) throw new Error('Showcase requires the current private session')
 return memory
}

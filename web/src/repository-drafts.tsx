import { createContext, useCallback, useContext, useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { useBlocker } from '@tanstack/react-router'
const Context = createContext<{ dirty: boolean; record: (id: string, dirty: boolean) => void; isDirty: () => boolean } | null>(null)
export function RepositoryDrafts({ children, onDirty }: { children: ReactNode; onDirty?: (dirty: boolean) => void }) {
  const [owners, setOwners] = useState<Record<string, boolean>>({}), current = useRef(owners), dirty = Object.values(owners).some(Boolean)
  const isDirty = useCallback(() => Object.values(current.current).some(Boolean), [])
  const record = useCallback((id: string, value: boolean) => { if (current.current[id] === value) return; current.current = { ...current.current, [id]: value }; setOwners(current.current) }, [])
  useEffect(() => { onDirty?.(dirty); return () => onDirty?.(false) }, [dirty, onDirty])
  useBlocker({ shouldBlockFn: () => isDirty() && !window.confirm('Leave repository work with unsent or unconfirmed changes? Private drafts will be discarded; original unconfirmed command metadata remains for this signed-in session.'), enableBeforeUnload: isDirty })
  return <Context.Provider value={{ dirty, record, isDirty }}>{children}</Context.Provider>
}
export function useRepositoryDraft(dirty: boolean) {
  const id = useId(), record = useContext(Context)?.record
  useEffect(() => { record?.(id, dirty); return () => record?.(id, false) }, [id, dirty, record])
  return () => record?.(id, false)
}
export function useRepositoryDeparture() {
  const owner = useContext(Context)
  return () => !owner?.isDirty() || window.confirm('Discard private repository drafts and leave this review? Original unconfirmed command metadata stays blocked for this signed-in session.')
}

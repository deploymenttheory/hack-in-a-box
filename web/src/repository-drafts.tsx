import { createContext, useCallback, useContext, useEffect, useId, useState, type ReactNode } from 'react'
import { useBlocker } from '@tanstack/react-router'
const Context = createContext<{ dirty: boolean; record: (id: string, dirty: boolean) => void } | null>(null)
export function RepositoryDrafts({ children, onDirty }: { children: ReactNode; onDirty?: (dirty: boolean) => void }) {
  const [owners, setOwners] = useState<Record<string, boolean>>({}), dirty = Object.values(owners).some(Boolean)
  const record = useCallback((id: string, value: boolean) => setOwners(previous => previous[id] === value ? previous : { ...previous, [id]: value }), [])
  useEffect(() => { onDirty?.(dirty); return () => onDirty?.(false) }, [dirty, onDirty])
  useBlocker({ shouldBlockFn: () => dirty && !window.confirm('Leave repository work with unsent or unconfirmed changes? Private drafts will be discarded; original unconfirmed command metadata remains for this signed-in session.'), enableBeforeUnload: dirty })
  return <Context.Provider value={{ dirty, record }}>{children}</Context.Provider>
}
export function useRepositoryDraft(dirty: boolean) {
  const id = useId(), record = useContext(Context)?.record
  useEffect(() => { record?.(id, dirty); return () => record?.(id, false) }, [id, dirty, record])
}
export function useRepositoryDeparture() {
  const dirty = useContext(Context)?.dirty
  return () => !dirty || window.confirm('Discard private repository drafts and leave this review? Original unconfirmed command metadata stays blocked for this signed-in session.')
}

import { useCallback, useEffect, useMemo, useState } from 'react'
import { usePrivateRequestScope } from './private-request-scope'
import { nativeDenied } from './survey-context'

type Snapshot<T> = { key: string; data?: T; error?: unknown; pending: boolean; denied: boolean }
// Raw answers and restricted prose belong to this mounted owner, never QueryClient
// or browser storage. Each read replaces the previous read lifetime; denial is
// sticky until a deliberate retry and cannot be undone by an older continuation.
export function useSurveyPrivateRead<T>(key: string, load: (signal: AbortSignal) => Promise<T>, enabled = true) {
  const scope = usePrivateRequestScope(key), lifetime = useMemo(() => ({ scope, denied: false }), [scope])
  const [snapshot, setSnapshot] = useState<Snapshot<T>>({ key, pending: enabled, denied: false })
  const read = useCallback(async (retry = false) => {
    if (!enabled || lifetime.denied && !retry) throw new DOMException('Survey read unavailable', 'AbortError')
    lifetime.denied = false; scope.renew()
    const ticket = scope.capture()
    setSnapshot(previous => ({ ...(previous.key === key ? previous : { key }), pending: true, denied: false, error: undefined }))
    try {
      const data = await load(ticket.signal); ticket.assert()
      setSnapshot({ key, data, pending: false, denied: false }); return data
    } catch (error) {
      if (ticket.current()) {
        const denied = nativeDenied(error); lifetime.denied = denied
        setSnapshot(previous => ({ key, data: denied ? undefined : previous.key === key ? previous.data : undefined, error, pending: false, denied }))
        if (denied) scope.invalidate()
      }
      throw error
    }
  }, [key, enabled, load, scope, lifetime])
  useEffect(() => {
    if (!enabled) return
    const refresh = () => { if (!lifetime.denied) void read().catch(() => {}) }
    refresh(); const timer = window.setInterval(refresh, 15_000)
    window.addEventListener('focus', refresh)
    return () => { window.clearInterval(timer); window.removeEventListener('focus', refresh) }
  }, [read, enabled, lifetime])
  const current = snapshot.key === key ? snapshot : { key, pending: enabled, denied: false }
  const revoke = useCallback((error: unknown) => { lifetime.denied = true; scope.invalidate(); setSnapshot({ key, error, pending: false, denied: true }) }, [key, scope, lifetime])
  const accept = useCallback((data: T) => { if (!lifetime.denied && scope.capture().current()) setSnapshot({ key, data, pending: false, denied: false }) }, [key, scope, lifetime])
  return { ...current, read, revoke, accept }
}

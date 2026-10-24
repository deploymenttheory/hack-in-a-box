import { createContext, useContext, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { usePrivateRequestScope, type PrivateRequestScope } from './private-request-scope'
import { nativeDenied } from './survey-context'

// Resource-local authority chains fence descendants synchronously when a parent
// denies access, before React disposes its private child state. No payload lives
// in this context; it carries only the mounted owner's cancellation lifetime.
export const SurveyAuthority = createContext<PrivateRequestScope | undefined>(undefined)
export function useSurveyRequestScope(key: string) {
  return usePrivateRequestScope(key, useContext(SurveyAuthority))
}

type Snapshot<T> = { key: string; data?: T; error?: unknown; pending: boolean; denied: boolean }
// Raw answers and restricted prose belong to this mounted owner, never QueryClient
// or browser storage. Each read replaces the previous read lifetime; denial is
// sticky until a deliberate retry and cannot be undone by an older continuation.
export function useSurveyPrivateRead<T>(key: string, load: (signal: AbortSignal) => Promise<T>, enabled = true) {
  const authority = useSurveyRequestScope(`${key}:authority`), scope = usePrivateRequestScope(`${key}:read`, authority), lifetime = useMemo(() => ({ scope, denied: false, ended: false }), [scope])
  const [snapshot, setSnapshot] = useState<Snapshot<T>>({ key, pending: enabled, denied: false })
  const read = useCallback(async (retry = false) => {
    if (!enabled || lifetime.ended || lifetime.denied && !retry) throw new DOMException('Survey read unavailable', 'AbortError')
    if (retry && lifetime.denied) authority.renew()
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
        if (denied) { authority.invalidate(); scope.invalidate() }
      }
      throw error
    }
  }, [key, enabled, load, scope, authority, lifetime])
  useEffect(() => {
    if (!enabled) return
    const refresh = () => { if (!lifetime.denied) void read().catch(() => {}) }
    refresh(); const timer = window.setInterval(refresh, 15_000)
    window.addEventListener('focus', refresh)
    return () => { window.clearInterval(timer); window.removeEventListener('focus', refresh) }
  }, [read, enabled, lifetime])
  useEffect(() => {
    const end = () => { lifetime.ended = true; lifetime.denied = true; authority.invalidate(); scope.invalidate(); setSnapshot({ key, pending: false, denied: true, error: new Error('This private survey session has ended. Sign in again to check access.') }) }
    const events = ['session-ending', 'session-expired', 'mentor-draft-invalidated']
    for (const event of events) window.addEventListener(event, end)
    return () => { for (const event of events) window.removeEventListener(event, end) }
  }, [key, authority, scope, lifetime])
  const current = snapshot.key === key ? snapshot : { key, pending: enabled, denied: false }
  const revoke = useCallback((error: unknown) => { lifetime.denied = true; authority.invalidate(); scope.invalidate(); setSnapshot({ key, error, pending: false, denied: true }) }, [key, scope, authority, lifetime])
  const accept = useCallback((data: T) => { if (!lifetime.denied && scope.capture().current()) setSnapshot({ key, data, pending: false, denied: false }) }, [key, scope, lifetime])
  return { ...current, read, revoke, accept, authority, capture: () => scope.capture() }
}

// Cursor pages remain private to the same owner. Append uses the current read
// ticket, so a poll/denial/refresh fences an older held page before acceptance.
export function useSurveyPrivatePages<T, C>(key: string, load: (signal: AbortSignal, cursor: C | undefined) => Promise<T>, next: (page: T) => C | undefined, enabled = true) {
  const pagination = useRef({ key, count: 1, busy: false })
  const [nextPending, setNextPending] = useState(false), [pageError, setPageError] = useState<{ key: string; error: unknown }>()
  const readPages = useCallback(async (signal: AbortSignal) => {
    if (pagination.current.key !== key) pagination.current = { key, count: 1, busy: false }
    const pages: T[] = []; let cursor: C | undefined
    for (let index = 0; index < pagination.current.count; index++) { const page = await load(signal, cursor); pages.push(page); cursor = next(page); if (cursor === undefined) break }
    return pages
  }, [load, next, key])
  const owner = useSurveyPrivateRead(key, readPages, enabled)
  async function fetchNextPage() {
    if (pagination.current.busy || owner.pending || !owner.data?.length) return
    const cursor = next(owner.data.at(-1)!); if (cursor === undefined) return
    const paging = pagination.current; paging.busy = true; setNextPending(true); setPageError(undefined)
    const ticket = owner.capture()
    try {
      const current = owner.data, page = await load(ticket.signal, cursor); ticket.assert()
      owner.accept([...current, page]); paging.count = current.length + 1
    } catch (error) { if (ticket.current()) { if (nativeDenied(error)) owner.revoke(error); else setPageError({ key, error }) } }
    finally { paging.busy = false; setNextPending(false) }
  }
  return { ...owner, data: owner.data && { pages: owner.data }, error: owner.error ?? (pageError?.key === key ? pageError.error : undefined), isPending: !owner.data && owner.pending, isFetchingNextPage: nextPending, hasNextPage: !!owner.data?.length && next(owner.data.at(-1)!) !== undefined, fetchNextPage, refetch: async () => { setPageError(undefined); try { return { data: { pages: await owner.read(true) }, error: undefined } } catch (error) { return { data: undefined, error } } } }
}

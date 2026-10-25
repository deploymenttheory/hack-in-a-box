import { useCallback, useEffect, useMemo, useState } from 'react'
import { APIError } from './api/client'
import { usePrivateRequestScope, type PrivateRequestScope } from './private-request-scope'

const ordinaryDenials = [401, 403, 404]

// A mounted read lifetime only: no task store, query cache, private payload
// persistence, inferred receipt or write replay. Definite denial stays latched.
export function useTodayRead<T>(key: string, load: (signal: AbortSignal, assertCurrent: () => void) => Promise<T>, parent?: PrivateRequestScope, identityOf?: (value: T) => string, deniedStatuses: readonly number[] = ordinaryDenials) {
  const authority = usePrivateRequestScope(`${key}:authority`, parent)
  const scope = usePrivateRequestScope(`${key}:read`, authority)
  const life = useMemo(() => ({ scope, denied: false, ended: false, reading: false, attempt: 0, signature: undefined as string | undefined }), [scope])
  const [generation, setGeneration] = useState(0)
  const [snapshot, setSnapshot] = useState<{ key: string; data?: T; error?: unknown; pending: boolean; denied: boolean }>({ key, pending: true, denied: false })
  const read = useCallback(async (explicit = false) => {
    if (life.ended || life.denied && !explicit || life.reading && !explicit) return
    if (life.denied) authority.renew()
    life.reading = true; const attempt = ++life.attempt
    scope.renew(); const ticket = scope.capture()
    setSnapshot(previous => ({ ...(previous.key === key ? previous : { key }), pending: true, denied: false, error: undefined }))
    try {
      ticket.assert(); const data = await load(ticket.signal, ticket.assert); ticket.assert()
      const signature = identityOf?.(data)
      // Fence old private child reads synchronously on an actual native role or
      // authority boundary, before React disposes/replaces those child owners.
      if (life.signature !== undefined && life.signature !== signature) { authority.renew(); setGeneration(value => value + 1) }
      life.signature = signature; life.denied = false
      setSnapshot({ key, data, pending: false, denied: false })
    } catch (error) {
      if (ticket.current()) {
        const denied = error instanceof APIError && deniedStatuses.includes(error.status)
        life.denied = denied
        if (denied) { authority.invalidate(); scope.invalidate() }
        setSnapshot(previous => ({ key, data: denied ? undefined : previous.key === key ? previous.data : undefined, error, pending: false, denied }))
      }
    } finally { if (attempt === life.attempt) life.reading = false }
  }, [key, load, scope, authority, life, identityOf, deniedStatuses])
  useEffect(() => {
    const refresh = () => { void read() }; refresh()
    const timer = setInterval(refresh, 15_000); window.addEventListener('focus', refresh)
    return () => { clearInterval(timer); window.removeEventListener('focus', refresh) }
  }, [read])
  useEffect(() => {
    const end = () => {
      life.ended = true; life.denied = true; authority.invalidate(); scope.invalidate()
      setSnapshot({ key, pending: false, denied: true, error: new Error('This event session has ended. Sign in again to check Today.') })
    }
    const events = ['session-ending', 'session-expired', 'mentor-draft-invalidated']
    events.forEach(event => window.addEventListener(event, end))
    return () => events.forEach(event => window.removeEventListener(event, end))
  }, [scope, authority, life, key])
  return { ...(snapshot.key === key ? snapshot : { key, pending: true, denied: false }), refresh: () => read(true), authority, generation }
}

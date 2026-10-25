import { createContext, useCallback, useContext, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { APIError, api, privateKey, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { usePrivateRequestScope, type PrivateRequestScope } from './private-request-scope'

// This is the existing event GET's private lifetime, not a permission store.
// The backend still decides every capability and every owning-domain command.
const fingerprint = (event: EventRecord) => JSON.stringify([event.id, event.stage, ...[event.can_edit, event.can_read_staff, event.can_manage_owner, event.can_transition, event.can_present, event.can_read_review_coverage, event.can_read_attendance, event.can_read_logistics].map(Boolean)])
type Boundary = { eventID: string; scope: PrivateRequestScope; generation: number; paused: boolean; observeToday: (current: EventRecord) => boolean }
const Context = createContext<Boundary | null>(null)
export const EventReadScopeProvider = Context.Provider
export function useEventReadBoundary(eventID: string) { const value = useContext(Context); return value?.eventID === eventID ? value : undefined }

export function useEventAuthority(eventID: string) {
  const identity = useIdentity(), cache = useQueryClient(), owner = `${identity.person.id}:${identity.draft_scope}:${eventID}`
  const key = useMemo(() => privateKey(identity.person.id, eventID, 'event'), [identity.person.id, eventID])
  const scope = usePrivateRequestScope(`${owner}:event-authority`), reads = usePrivateRequestScope(`${owner}:event-authority-read`)
  const latest = useRef<{ signature?: string; revision?: number; denied: boolean; checking: boolean }>({ denied: false, checking: false })
  const pendingRenewal = useRef(false)
  const [verified, setVerified] = useState(false), [denial, setDenial] = useState<unknown>(), [checking, setChecking] = useState(false), [generation, setGeneration] = useState(0)
  const query = useQuery<EventRecord | null>({ queryKey: key, enabled: !denial, retry: false, refetchOnMount: 'always', refetchOnWindowFocus: 'always', queryFn: async ({ signal }) => {
    reads.renew(); const ticket = reads.capture()
    const current = unwrap(await api.GET('/api/v1/events/{event_id}', { signal, cache: 'no-store', params: { path: { event_id: eventID } } }))
    ticket.assert()
    if (current.id !== eventID) throw new Error('The current event identity changed')
    return current
  } })
  // The portal header also observes this canonical query. Watch its accepted
  // native GET result synchronously, regardless of which observer began it.
  useLayoutEffect(() => cache.getQueryCache().subscribe(event => {
    if (event.type !== 'updated' || event.query.queryHash !== cache.getQueryCache().find({ queryKey: key, exact: true })?.queryHash) return
    if (event.action.type === 'error') {
      const error = event.action.error
      if (!(error instanceof APIError) || ![401, 403, 404].includes(error.status)) return
      scope.invalidate(); pendingRenewal.current = false; latest.current.denied = true; latest.current.checking = false
      setDenial(error); setVerified(false); setChecking(false)
      cache.setQueryData<EventRecord | null>(key, null)
      return
    }
    if (event.action.type !== 'success') return
    const current = event.query.state.data as EventRecord | null | undefined
    if (!current) return
    if (current.id !== eventID) { scope.invalidate(); pendingRenewal.current = false; latest.current.denied = true; setVerified(false); setDenial(new Error('The current event identity changed')); cache.setQueryData<EventRecord | null>(key, null); return }
    const signature = fingerprint(current)
    if (event.action.manual) {
      if (latest.current.denied) { cache.setQueryData<EventRecord | null>(key, null); return }
      if (latest.current.signature && latest.current.signature !== signature) {
        scope.invalidate(); latest.current.checking = true; setChecking(true); setVerified(false)
        void cache.refetchQueries({ queryKey: key, exact: true, type: 'active' })
      }
      return
    }
    // Equal entity revisions can still accompany changed actor capabilities.
    // Fence before React observes the new canonical event projection.
    if (latest.current.signature !== undefined && latest.current.signature !== signature || !scope.capture().current()) { scope.invalidate(); pendingRenewal.current = true; setGeneration(value => value + 1) }
    latest.current = { signature, revision: current.revision, denied: false, checking: false }
    setVerified(true); setDenial(undefined); setChecking(false)
  }), [cache, key, eventID, scope])
  useLayoutEffect(() => { if (pendingRenewal.current && !latest.current.denied) { pendingRenewal.current = false; scope.renew() } }, [generation, scope])
  const observeToday = useCallback((current: EventRecord) => {
    const previous = latest.current
    if (previous.denied || !previous.signature) return false
    const changed = fingerprint(current) !== previous.signature
    if (changed && !previous.checking) {
      previous.checking = true; scope.invalidate(); setChecking(true)
      void cache.refetchQueries({ queryKey: key, exact: true, type: 'active' })
    } else if (!changed && current.revision !== previous.revision && !previous.checking) {
      // Metadata revision drift is a read signal, never a cache overwrite or an
      // authority change that would discard unchanged-authority unsent work.
      void cache.refetchQueries({ queryKey: key, exact: true, type: 'active' })
    }
    return !changed && current.revision >= (previous.revision ?? current.revision)
  }, [cache, key, scope])
  const boundary = useMemo(() => ({ eventID, scope, generation, paused: checking || !!denial || !!query.error || !verified, observeToday }), [eventID, scope, generation, checking, denial, query.error, verified, observeToday])
  return { query, scope, boundary, generation, verified, denial, checking, observeToday, refresh: () => query.refetch() }
}

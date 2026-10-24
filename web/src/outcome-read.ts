import { useCallback, useEffect, useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { privateKey } from './api/client'
import { useIdentity } from './session'
import { usePrivateRequestScope, type PrivateRequestScope } from './private-request-scope'
import { outcomeDenied } from './outcome-command'

// Cached private data is never a mount's authority. A route must obtain its own
// current GET, even when its parent has temporarily lost and regained access.
export function useOutcomeRead<T>(id: string, category: string, read: (signal: AbortSignal) => Promise<T>) {
  const identity = useIdentity(), cache = useQueryClient()
  const [mount] = useState(() => crypto.randomUUID()), [generation, setGeneration] = useState(0)
  const [blocked, setBlocked] = useState<unknown>()
  const scope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${category}:${id}`)
  const prefix = useMemo(() => privateKey(identity.person.id, id, category), [identity.person.id, id, category])
  const deny = useCallback((error: unknown) => {
    scope.invalidate(); setBlocked(error)
    void cache.cancelQueries({ queryKey: prefix }); cache.removeQueries({ queryKey: prefix })
  }, [scope, cache, prefix])
  useEffect(() => () => { void cache.cancelQueries({ queryKey: prefix }); cache.removeQueries({ queryKey: prefix }) }, [cache, prefix])
  const current = useQuery({ queryKey: [...prefix, identity.draft_scope, mount, generation], gcTime: 0, enabled: !blocked,
    queryFn: async ({ signal }) => {
      await Promise.resolve()
      const ticket = scope.capture(); ticket.assert()
      try { const result = await read(signal); ticket.assert(); return result }
      catch (error) { if (ticket.current() && outcomeDenied(error)) deny(error); throw error }
    }, retry: false, refetchInterval: 15_000, refetchOnWindowFocus: 'always' })
  const checkAccess = () => { scope.renew(); setBlocked(undefined); setGeneration(value => value + 1) }
  return { current, scope, prefix, deny, blocked, checkAccess }
}

// Each nested private projection also owns its mount. Losing a capability and
// regaining only the parent must not resurrect its old options or draft view.
export function useOutcomeProjection(id: string, projection: string, parent: PrivateRequestScope) {
  const identity = useIdentity(), [mount] = useState(() => crypto.randomUUID())
  const key = useMemo(() => [...privateKey(identity.person.id, id, 'outcome'), identity.draft_scope, projection, mount], [identity.person.id, identity.draft_scope, id, projection, mount])
  const scope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${id}:${projection}:${mount}`, parent), cache = useQueryClient()
  useEffect(() => () => { void cache.cancelQueries({ queryKey: key }); cache.removeQueries({ queryKey: key }) }, [cache, key])
  return { key, scope }
}

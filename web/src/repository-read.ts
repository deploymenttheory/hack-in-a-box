import { useCallback, useEffect, useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { APIError, privateKey } from './api/client'
import { useIdentity } from './session'
import { usePrivateRequestScope } from './private-request-scope'
export function repositoryDenied(error: unknown) { return error instanceof APIError && [401, 403, 404].includes(error.status) }
// Every opening obtains its own current authorisation. Cached projections are
// never permission to reopen a private repository or access review.
export function useRepositoryRead<T>(eventID: string, resource: string, read: (signal: AbortSignal) => Promise<T>) {
  const identity = useIdentity(), cache = useQueryClient()
  const [mount] = useState(() => crypto.randomUUID()), [generation, setGeneration] = useState(0), [denied, setDenied] = useState<unknown>()
  const scope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${eventID}:repositories:${resource}:${mount}`)
  const key = useMemo(() => [...privateKey(identity.person.id, eventID, 'repository-admin'), identity.draft_scope, resource, mount], [identity.person.id, identity.draft_scope, eventID, resource, mount])
  const deny = useCallback((error: unknown) => { scope.invalidate(); setDenied(error); void cache.cancelQueries({ queryKey: key }); cache.removeQueries({ queryKey: key }) }, [cache, key, scope])
  useEffect(() => () => { void cache.cancelQueries({ queryKey: key }); cache.removeQueries({ queryKey: key }) }, [cache, key])
  const query = useQuery({ queryKey: [...key, generation], gcTime: 0, enabled: !denied, retry: false, refetchInterval: 15_000, refetchOnWindowFocus: 'always', queryFn: async ({ signal }) => {
    await Promise.resolve(); const ticket = scope.capture(); ticket.assert()
    try { const value = await read(signal); ticket.assert(); return value } catch (error) { if (ticket.current() && repositoryDenied(error)) deny(error); throw error }
  } })
  return { query, scope, denied, deny, check: () => { scope.renew(); setDenied(undefined); setGeneration(value => value + 1) } }
}

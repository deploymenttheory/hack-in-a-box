import { useLayoutEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { APIError, privateKey } from './api/client'
import { useIdentity } from './session'
import { usePrivateRequestScope } from './private-request-scope'
import { Alert } from './components/ui/surfaces'
import { Button } from './components/ui/button'
import type { CollectionState } from './components/patterns/model'

// These APIs return current pages, not durable per-command receipts. A lost
// response stays uncertain even when a subsequent read shows the desired state.
export function useCoveragePage<T extends { revision: number }>(eventID: string, resource: string, load: (signal: AbortSignal) => Promise<T>, write: (input: unknown, revision: number) => Promise<T>) {
  const identity = useIdentity(), cache = useQueryClient()
  const scopeKey = `${identity.person.id}:${identity.draft_scope}:${eventID}:${resource}`
  const scope = usePrivateRequestScope(scopeKey)
  const [command, setCommand] = useState<{ kind: 'idle' | 'pending' | 'saved' | 'rejected' | 'stale' | 'unknown' | 'denied'; message?: string; intent?: string; currentRevision?: number }>({ kind: 'idle' })
  const lock = useRef(false), terminal = useRef(false)
  const key = useMemo(() => privateKey(identity.person.id, eventID, resource), [identity.person.id, eventID, resource])
  const query = useQuery({ queryKey: key, retry: false, refetchOnMount: 'always', refetchOnWindowFocus: 'always', refetchInterval: 30_000, queryFn: async ({ signal }) => { const ticket = scope.capture(); const result = await load(signal); ticket.assert(); return result } })
  const denied = query.error instanceof APIError && [401, 403, 404].includes(query.error.status)
  useLayoutEffect(() => { if (denied) scope.invalidate() }, [denied, scope])
  async function read() {
    const ticket = scope.capture()
    if (!ticket.current()) return
    const result = await query.refetch()
    if (!ticket.current()) return
    const data = result.data
    if (data && !result.error) setCommand(previous => previous.kind === 'unknown' ? { ...previous, currentRevision: data.revision, message: 'Current roster loaded. It cannot prove whether this original change completed. No write will be replayed.' } : previous)
  }
  async function act(input: unknown, revision: number, allowed: boolean) {
    if (lock.current || terminal.current || !allowed || denied || query.error || command.kind === 'unknown' || command.kind === 'denied') return false
    const ticket = scope.capture()
    if (!ticket.current()) return false
    if (query.data?.revision !== revision) { setCommand({ kind: 'stale', message: 'The roster changed. Review the current changes again before publishing.' }); return false }
    lock.current = true
    const intent = crypto.randomUUID()
    setCommand({ kind: 'pending', intent, message: 'Sending the reviewed change…' })
    try {
      const updated = await write(input, revision)
      ticket.assert()
      if (updated.revision !== revision + 1) throw new Error('Unverified response')
      cache.setQueryData(key, updated)
      setCommand({ kind: 'saved', message: `Server response received at roster revision ${updated.revision}. Review the current roster before another change.` })
      void cache.invalidateQueries({ queryKey: ['private', identity.person.id, eventID] })
      return true
    } catch (error) {
      if (!ticket.current()) return false
      if (error instanceof APIError && error.status >= 400 && error.status < 500 && error.status !== 408) {
        const revoked = [401, 403, 404].includes(error.status)
        const message = error.code === 'teams_unready' ? 'Publication rejected: resolve teams below three, select a draft-member captain and respect hard capacity. Existing published assignments were not changed. Freeze also requires every confirmed competitor to be assigned.' : error.message
        setCommand({ kind: revoked ? 'denied' : error.status === 409 ? 'stale' : 'rejected', message })
        if (revoked) { terminal.current = true; scope.invalidate(); cache.removeQueries({ queryKey: key }) }
        else void query.refetch()
      } else { terminal.current = true; setCommand({ kind: 'unknown', intent, message: 'The response was interrupted. This change may have completed. Read the current roster; this API has no exact command-status receipt. Do not resend the original change.' }) }
      return false
    } finally { lock.current = false }
  }
  const state: CollectionState = denied || command.kind === 'denied' ? { kind: 'denied', message: 'Current access to this roster is unavailable. Private details have been removed.' } : query.isPending || !query.isFetchedAfterMount ? { kind: 'loading' } : query.error ? { kind: 'failed', message: 'The current roster could not be verified. Changes are paused.', read: { label: 'Read current roster', onSelect: () => void read() } } : { kind: 'ready' }
  return { query, scope, scopeKey, command, state, act, read, blocked: command.kind === 'pending' || command.kind === 'unknown' || state.kind !== 'ready' }
}
export function CoverageCommand({ command, read, children }: { command: { kind: string; message?: string; intent?: string; currentRevision?: number }; read: () => Promise<void>; children?: ReactNode }) {
  if (command.kind === 'idle') return null
  return <Alert role={command.kind === 'denied' || command.kind === 'rejected' || command.kind === 'stale' ? 'alert' : 'status'} tone={command.kind === 'unknown' || command.kind === 'stale' ? 'warning' : command.kind === 'saved' ? 'success' : 'neutral'}>
    {command.kind === 'unknown' && <h3>Outcome unknown</h3>}<p>{command.message}</p>{command.intent && <p className="fleet-reference">Local intent {command.intent} · diagnostic reference, not a server receipt</p>}{command.currentRevision !== undefined && <p>Current roster revision {command.currentRevision}. Original outcome remains unconfirmed.</p>}{command.kind === 'unknown' && <Button icon="refresh" variant="outline" onClick={() => void read()}>Read current roster</Button>}{children}
  </Alert>
}

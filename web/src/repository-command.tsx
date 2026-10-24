import { useRef, useState } from 'react'
import { APIError } from './api/client'
import { useIdentity } from './session'
import { useCommunicationCommands, CommunicationCommandHistory } from './communication-tests'
import type { PrivateRequestScope } from './private-request-scope'
import { Failure } from './components/states'

// Only safe IDs and native revisions outlive the private editor. In particular,
// configuration secrets, people, reasons and exact bodies never enter this store.
export function useRepositoryCommand(eventID: string, resource: string, scope: PrivateRequestScope, deny?: (error: unknown) => void) {
  const identity = useIdentity(), history = useCommunicationCommands(`${identity.person.id}:${identity.draft_scope}:${eventID}:repositories:${resource}`)
  const lock = useRef(false), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState('')
  async function send<T>(action: string, revision: number, request: () => Promise<T>, accepted: (result: T) => void, exact?: { planID: string; version?: number; originalID?: string; intentFingerprint?: string }) {
    if (lock.current || history.pending || (history.unconfirmed && !exact?.originalID)) return
    const ticket = scope.capture(); if (!ticket.current()) return
    lock.current = true; setFailure(undefined); setNotice('')
    const id = history.begin({ operation: `Repository ${action}`, revision, command: { action, resourceID: exact?.planID, expectedVersion: exact?.version, intentFingerprint: exact?.intentFingerprint } }, exact?.originalID)
    try {
      const result = await request()
      // The original response may settle its own safe metadata after its private
      // editor closes; it must never update a replacement owner's private state.
      history.complete(id)
      if (ticket.current()) accepted(result)
    } catch (error) {
      const definite = error instanceof APIError && error.status < 500 && ![408, 429].includes(error.status)
      if (definite && !exact?.originalID) history.complete(id); else history.uncertain(id)
      if (ticket.current() && error instanceof APIError && [401, 403, 404].includes(error.status)) deny?.(error)
      if (ticket.current()) { setFailure(error); setNotice(definite && !exact?.originalID ? 'The request was rejected. Review the current facts before another deliberate change.' : 'The original command outcome is unknown. Current status reads do not acknowledge it; replacement stays blocked.') }
    } finally { lock.current = false }
    return id
  }
  return { send, history, failure, notice, blocked: history.pending || history.unconfirmed, pending: history.pending }
}
export function RepositoryCommandStatus({ command }: { command: ReturnType<typeof useRepositoryCommand> }) {
  return <>{command.failure ? <Failure error={command.failure} /> : null}{command.notice && <p role="status">{command.notice}</p>}<CommunicationCommandHistory attempts={command.history.attempts} /></>
}

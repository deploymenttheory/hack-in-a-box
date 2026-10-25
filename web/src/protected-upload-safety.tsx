import { useLayoutEffect, type ReactNode } from 'react'
import { useBlocker } from '@tanstack/react-router'
import { CommunicationCommandHistory, useCommunicationCommandNamespace } from './communication-tests'
import { usePlanningDepartureBarrier } from './planning-operations'
import { useIdentity } from './session'

const confirmDeparture = () => window.confirm('Leave with a protected upload outcome still unconfirmed? Its original file may have been stored. No upload is replayed automatically.')

// This actor/session owner survives event, form and capability changes. It keeps
// only original-attempt metadata; file bytes and attachment effects stay private.
export function ProtectedUploadSafety({ children }: { children: ReactNode }) {
 const identity = useIdentity(), attempts = useCommunicationCommandNamespace(`${identity.person.id}:${identity.draft_scope}:protected-uploads`)
 const pending = attempts.some(attempt => attempt.state === 'pending'), unknown = attempts.some(attempt => attempt.blocks && attempt.state === 'unknown')
 usePlanningDepartureBarrier(pending)
 useBlocker({ shouldBlockFn: () => pending || unknown && !confirmDeparture(), enableBeforeUnload: attempts.length > 0 })
 useLayoutEffect(() => {
  if (!unknown || pending) return
  const guard = (event: Event) => { if (!confirmDeparture()) event.preventDefault() }
  window.addEventListener('planning-departure', guard)
  return () => window.removeEventListener('planning-departure', guard)
 }, [unknown, pending])
 return <><div className="fleet-today-private"><CommunicationCommandHistory attempts={attempts} />{attempts.length > 0 && <p>Upload history records a stored file request, not acceptance of an attachment or formal submission. Current reads cannot confirm the original upload response.</p>}</div>{children}</>
}

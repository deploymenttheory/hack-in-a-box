import { useLayoutEffect, type ReactNode } from 'react'
import { useBlocker } from '@tanstack/react-router'
import { CommunicationCommandHistory, useCommunicationCommandNamespace } from './communication-tests'
import { useIdentity } from './session'
import { usePlanningDepartureBarrier } from './planning-operations'

const confirmDeparture = () => window.confirm('Leave this workspace with an earlier preparation, evidence, reading, judging setup or session outcome still unconfirmed? The original outcome remains unconfirmed; no request is replayed.')

// This safe observer sits above capability-keyed planning/private task owners.
// It owns no task payload, replay, current-state receipt or authority decision.
export function TodayActionSafety({ eventID, children }: { eventID: string; children: ReactNode }) {
  const identity = useIdentity(), prefix = `${identity.person.id}:${identity.draft_scope}:${eventID}`
  const preparation = useCommunicationCommandNamespace(`${prefix}:preparation`), acknowledgements = useCommunicationCommandNamespace(`${prefix}:resource-acknowledgement`), sessions = useCommunicationCommandNamespace(`${prefix}:session-bookings`), code = useCommunicationCommandNamespace(`${prefix}:submission-freeze`), evidence = useCommunicationCommandNamespace(`${prefix}:team-deliverables`), judgingSetup = useCommunicationCommandNamespace(`${prefix}:judging-setup`), attempts = [...preparation, ...acknowledgements, ...sessions, ...code, ...evidence, ...judgingSetup]
  const pending = attempts.some(attempt => attempt.state === 'pending'), unknown = attempts.some(attempt => attempt.blocks && attempt.state === 'unknown')
  usePlanningDepartureBarrier(pending)
  useBlocker({ shouldBlockFn: () => pending || unknown && !confirmDeparture(), enableBeforeUnload: attempts.length > 0 })
  useLayoutEffect(() => {
    if (!unknown || pending) return
    const guard = (event: Event) => { if (!confirmDeparture()) event.preventDefault() }
    window.addEventListener('planning-departure', guard)
    return () => window.removeEventListener('planning-departure', guard)
  }, [unknown, pending])
  return <><div className="fleet-today-private"><CommunicationCommandHistory attempts={attempts} />{attempts.length > 0 && <p className="fleet-scope-label">Task or released-resource references: {[...new Set(attempts.map(attempt => attempt.command.resourceID).filter(Boolean))].join(', ') || 'No original resource reference available'}. Current reads cannot establish these original outcomes.</p>}</div>{children}</>
}

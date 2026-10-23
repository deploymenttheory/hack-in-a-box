import { useLayoutEffect, useMemo } from 'react'
import { mentorSessionGeneration } from './mentor-drafts'

// A private UI lifetime fence, not an authorisation decision. Read cancellation
// is best effort: continuations must check their ticket before further effects.
export class PrivateRequestScope {
  private epoch = 0
  private active = true
  private reads = new AbortController()
  constructor(readonly key: string, private parent?: PrivateRequestScope) {}
  invalidate = () => { this.active = false; this.epoch++; this.reads.abort() }
  renew = () => { this.invalidate(); this.active = true; this.reads = new AbortController() }
  capture() {
    const epoch = this.epoch, session = mentorSessionGeneration(), parent = this.parent?.capture()
    const current = (): boolean => this.active && epoch === this.epoch && session === mentorSessionGeneration() && (!parent || parent.current())
    return { current, signal: this.reads.signal, assert: () => { if (!current()) throw new DOMException('Private request scope ended', 'AbortError') } }
  }
}
export type PrivateRequestTicket = ReturnType<PrivateRequestScope['capture']>
export function usePrivateRequestScope(key: string, parent?: PrivateRequestScope) {
  const scope = useMemo(() => new PrivateRequestScope(key, parent), [key, parent])
  useLayoutEffect(() => {
    scope.renew()
    const events = ['session-ending', 'session-expired', 'mentor-draft-invalidated']
    for (const event of events) window.addEventListener(event, scope.invalidate)
    return () => { scope.invalidate(); for (const event of events) window.removeEventListener(event, scope.invalidate) }
  }, [scope])
  return scope
}

import { useEffect, useRef, type ReactNode, type RefObject } from 'react'
import { useBlocker } from '@tanstack/react-router'
import { Button } from './components/ui/button'
import { Alert } from './components/ui/surfaces'
import { useCoverageFocus, type CoverageFocusTarget } from './coverage-navigation'

// Local drafts and unresolved commands have different departure semantics. A
// pending request cannot be abandoned; leaving an uncertain task is deliberate.
export function useMentorTaskNavigation({ dirty, pending, unknown, root }: { dirty: boolean; pending: boolean; unknown: boolean; root: RefObject<HTMLElement | null> }) {
  const focus = useCoverageFocus(root), opener = useRef<CoverageFocusTarget | undefined>(undefined)
  useBlocker({ shouldBlockFn: () => pending || ((dirty || unknown) && !window.confirm(unknown ? 'Leave this task with an original command outcome still unconfirmed? No command will be replayed.' : 'Discard the private unsent changes and leave this task?')), enableBeforeUnload: dirty || pending || unknown })
  return { capture: () => { opener.current = focus.capture() }, restore: () => focus.restore(opener.current, true), discard: (leave: () => void) => { if (pending || ((dirty || unknown) && !window.confirm(unknown ? 'Leave this original outcome unconfirmed? No request will be replayed.' : 'Discard private unsent changes?'))) return false; leave(); focus.restore(opener.current, true); return true } }
}
export function MentorTaskReview({ title, children, confirm, correct, cancel, pending, disabled, confirmLabel = 'Confirm reviewed change' }: { title: string; children: ReactNode; confirm: () => void; correct: () => void; cancel?: () => void; pending: boolean; disabled?: boolean; confirmLabel?: string }) {
  const heading=useRef<HTMLHeadingElement>(null)
  useEffect(()=>{heading.current?.focus()},[])
  return <section className="fleet-review-flow" aria-label={title}><p className="fleet-scope-label">Review exact source and effect</p><h3 ref={heading} tabIndex={-1}>{title}</h3><div className="fleet-review-content">{children}</div><div className="fleet-pattern-actions"><Button icon="back" variant="outline" disabled={pending} onClick={correct}>Correct details</Button>{cancel && <Button variant="ghost" disabled={pending} onClick={cancel}>Cancel change</Button>}<Button icon="check" pending={pending} disabled={disabled} onClick={confirm}>{confirmLabel}</Button></div></section>
}
export function MentorUnknown({ intent, children, read, pending }: { intent: string; children: ReactNode; read: () => void; pending: boolean }) {
  return <Alert tone="warning" role="status"><h3>Outcome unknown</h3><p>{children}</p><p className="fleet-reference">Local intent {intent} · diagnostic reference, not a server receipt</p><Button icon="refresh" variant="outline" pending={pending} onClick={read}>Check original outcome</Button></Alert>
}

import { useEffect, useRef, useState } from 'react'
import { Button } from './components/ui/button'
import { MentorDraftStore, type CurrentDraftTarget, type DraftResult, type DraftReview, type DraftTarget } from './mentor-drafts'

// The actual mentor form owns text entry and all server writes. This primitive
// only offers deliberate, currently-authorised local recovery for that owner.
type Props = { store: MentorDraftStore; target: DraftTarget; readCurrent: () => Promise<CurrentDraftTarget>; onRecover: (review: DraftReview) => void; onInvalidate: () => void }
export function MentorDraftRecovery(props: Props) { return <Recovery key={JSON.stringify(props.target)} {...props} /> }
function Recovery({ store, target, readCurrent, onRecover, onInvalidate }: Props) {
  const [review, setReview] = useState<DraftReview | DraftResult>(), [pending, setPending] = useState(false)
  const operation = useRef(0)
  const lifetime = useRef<AbortController | null>(null)
  useEffect(() => {
    const controller = new AbortController()
    lifetime.current = controller
    const clear = () => { controller.abort(); setReview({ status: 'invalidated' }); setPending(false); onInvalidate() }
    window.addEventListener('mentor-draft-invalidated', clear); window.addEventListener('session-ending', clear); window.addEventListener('session-expired', clear); window.addEventListener('mentor-draft-scope', clear)
    return () => { controller.abort(); window.removeEventListener('mentor-draft-invalidated', clear); window.removeEventListener('session-ending', clear); window.removeEventListener('session-expired', clear); window.removeEventListener('mentor-draft-scope', clear) }
  }, [onInvalidate])
  async function check(recover: boolean) {
    const signal = lifetime.current?.signal
    if (!signal || signal.aborted) return
    const ticket = ++operation.current
    setPending(true); setReview(undefined)
    try {
      const next = await store.review(target, readCurrent)
      if (signal.aborted || ticket !== operation.current) return
      setReview(next)
      if (recover && next.status === 'ready') onRecover(next)
    } finally { if (!signal.aborted && ticket === operation.current) setPending(false) }
  }
  const messages: Record<DraftResult['status'], string> = { missing: 'No local recovery copy is available.', expired: 'The local recovery copy expired after 30 minutes.', unavailable: 'Recovery is unavailable. Reconnect and check again; no server save has been confirmed.', blocked: 'Current event access or the live review window does not allow recovery. The private local copy was removed.', invalidated: 'The session or draft changed. Check again after signing in.' }
  return <section className="panel mentor-recovery" aria-label="Recover unsent mentor text"><h3>Local text recovery</h3><p>A local copy is not a saved server note. Restoring it never submits an observation or score.</p><div className="actions"><Button type="button" variant="outline" disabled={pending} onClick={() => void check(false)}>{pending ? 'Checking current access…' : 'Check for local text'}</Button><Button type="button" variant="outline" disabled={pending} onClick={() => { store.discard(target); setReview({ status: 'missing' }) }}>Discard local copy</Button></div>{review && (review.status === 'ready' ? <div><p role="status">Unsent text is available until {new Date(review.expiresAt).toLocaleTimeString()}. {review.changed ? 'The server changed. Review its current version before choosing how to save.' : 'Current event access has been checked.'}</p><Button type="button" disabled={pending} onClick={() => void check(true)}>Restore text to the editor</Button></div> : <p role="status">{messages[review.status]}</p>)}</section>
}

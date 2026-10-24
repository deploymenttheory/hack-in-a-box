import { useEffect, useRef, type ReactNode } from 'react'
import type { components } from './api/schema.gen'
import { Button } from './components/ui/button'
type S = components['schemas']

export function RepositoryActionReview({ title, children, pending, disabled, confirmLabel, confirm, correct }: { title: string; children: ReactNode; pending: boolean; disabled?: boolean; confirmLabel: string; confirm: () => void; correct: () => void }) {
  const heading = useRef<HTMLHeadingElement>(null)
  useEffect(() => { heading.current?.focus() }, [])
  // Native autofocus is not supported on headings. Callback-ref focuses only
  // this review opening; it neither queues a request nor reads private data.
  return <section className="fleet-review-flow" aria-label={title}><h4 tabIndex={-1} ref={heading}>{title}</h4>{children}<div className="fleet-pattern-actions"><Button icon="back" variant="outline" disabled={pending || disabled} onClick={correct}>Correct details</Button><Button icon="check" pending={pending} disabled={disabled} onClick={confirm}>{confirmLabel}</Button></div></section>
}

export function RepositoryPlan({ plan }: { plan: S['RepositoryAccessPlan'] }) {
  return <section aria-label="Exact repository access plan"><p>Plan {plan.id} · Version {plan.version} · Numeric repository {plan.repository_id}</p><p>{plan.approved_at ? 'Approved plan; delivery is tracked separately.' : 'Not approved. No external changes are authorised by viewing.'}</p><ul>{plan.actions.map(action => <li key={action.id}>{action.kind === 'invite' ? 'Invite' : 'Remove managed access for'} person {action.member.person_id} · Verified GitHub ID {action.member.github_id} · {action.member.role} · {action.member.permission === 'push' ? 'Write' : 'Read'} access · Action {action.id}{action.member.coverage_start ? ` · Coverage ${action.member.coverage_start} to ${action.member.coverage_end ?? 'open'}` : ''}</li>)}</ul>{!plan.actions.length && <p>No external changes in this plan.</p>}<ul>{plan.flags.map((flag, index) => <li key={index}>{flag.code.replaceAll('_', ' ')}</li>)}</ul></section>
}

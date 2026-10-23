import type { ReactNode } from 'react'
import { Button } from '../ui/button'
import { Alert } from '../ui/surfaces'
import { LiveMessage } from '../ui/feedback'
import { PatternActionControl } from './actions'
import type { CommandView, ScopeLabel } from './model'

// The owner supplies a verified exact receipt and domain-specific wording. An
// HTTP response, latest resource version or locally completed task is not proof.
export function CommandState({ value }: { value: CommandView }) {
  if (value.kind === 'idle') return null
  if (value.kind === 'denied') return <Alert role="alert" tone="destructive"><p>{value.message}</p></Alert>
  if (value.kind === 'pending') return <LiveMessage><p>{value.message}</p><p className="fleet-reference">Command {value.intentID}</p></LiveMessage>
  if (value.kind === 'receipt') return <section className="fleet-command-result" aria-label="Exact command receipt"><LiveMessage><p>{value.message}</p></LiveMessage><dl><div><dt>Command</dt><dd>{value.intentID}</dd></div><div><dt>Receipt</dt><dd>{value.receiptID}</dd></div></dl>{value.details}{value.next && <PatternActionControl action={value.next} primary />}</section>
  if (value.kind === 'uncertain') return <Alert tone="warning" role="status"><h3>Outcome unknown</h3><p>{value.message}</p><p className="fleet-reference">Command {value.intentID}</p>{value.readStatus && <Button icon="refresh" variant="outline" pending={value.checking} onClick={value.readStatus}>Check exact command status</Button>}</Alert>
  return <Alert tone={value.kind === 'not-sent' ? 'neutral' : 'warning'} role="status"><p>{value.message}</p>{value.correct && <Button variant="outline" onClick={value.correct}>Review and correct</Button>}</Alert>
}

export function ReviewFlow({ title, scope, stage, editor, summary, command, onReview, onCorrect, onConfirm, onCancel, confirmLabel, disabled = false }: { title: string; scope: ScopeLabel; stage: 'edit' | 'review' | 'result'; editor: ReactNode; summary: ReactNode; command: CommandView; onReview: () => void; onCorrect: () => void; onConfirm: () => void; onCancel: () => void; confirmLabel: string; disabled?: boolean }) {
  const pending = command.kind === 'pending', uncertain = command.kind === 'uncertain', denied = command.kind === 'denied'
  if (denied) return <section><CommandState value={command} /></section>
  return <section className="fleet-review-flow" aria-label={title}><p className="fleet-scope-label">{scope.event && <>{scope.event} · </>}{scope.entity}</p><h3>{title}</h3>{!denied && <ol className="fleet-review-steps" aria-label="Review steps">{(['edit', 'review', 'result'] as const).map((step, index) => <li key={step} aria-current={stage === step ? 'step' : undefined}>{index + 1}. {step === 'edit' ? 'Prepare' : step === 'review' ? 'Review' : 'Result'}</li>)}</ol>}
    {!denied && <div className="fleet-review-content">{stage === 'edit' ? editor : stage === 'review' ? summary : null}</div>}
    <CommandState value={command} />
    {!denied && !uncertain && stage !== 'result' && <div className="fleet-pattern-actions fleet-review-actions"><Button variant="outline" onClick={onCancel} disabled={pending}>Cancel</Button>{stage === 'edit' ? <Button icon="next" onClick={onReview} disabled={disabled || pending}>Review change</Button> : <><Button variant="outline" onClick={onCorrect} disabled={pending}>Correct details</Button><Button icon="check" onClick={onConfirm} pending={pending} disabled={disabled}>{confirmLabel}</Button></>}</div>}
  </section>
}

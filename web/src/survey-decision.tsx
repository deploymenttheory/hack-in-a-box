import { useLayoutEffect, useRef, type ComponentProps, type ReactNode, type Ref } from 'react'
import type { components } from './api/schema.gen'
import { Button } from './components/ui/button'
import type { IconName } from './components/ui/icon'
import { ConfirmationDialog } from './components/ui/dialog'
import { localDate } from './event-time'

export type SurveyDecisionAction = { label: string; icon?: IconName; disabled?: boolean; variant?: 'default' | 'outline' | 'ghost'; ref?: Ref<HTMLButtonElement> } & ({ submit: true; onSelect?: never } | { submit?: false; onSelect: (button: HTMLButtonElement) => void })
export type SurveyDecisionControlsProps = { status: { label: string; message: ReactNode }; actions: readonly SurveyDecisionAction[]; review?: ComponentProps<typeof ConfirmationDialog>; children?: ReactNode }
// Controlled production presentation only: owners retain private payloads,
// native authority, original command identity and exact receipt matching.
export function SurveyDecisionControls({ status, actions, review, children }: SurveyDecisionControlsProps) {
  const root = useRef<HTMLDivElement>(null), wasOpen = useRef(false), lastFocused = useRef<HTMLElement | undefined>(undefined)
  useLayoutEffect(() => {
    const closed = wasOpen.current && !review?.open; wasOpen.current = !!review?.open
    if (!closed) return
    const frame = requestAnimationFrame(() => {
      const finalFocus = review?.finalFocus
      // Callback owners can deliberately return an invalid field after a native
      // rejection. The dialog owns that destination; do not race it with fallback.
      if (typeof finalFocus === 'function') return
      const target = finalFocus && typeof finalFocus === 'object' && 'current' in finalFocus ? finalFocus.current : undefined
      if (target instanceof HTMLElement && target.isConnected && !target.matches(':disabled, [aria-disabled="true"]')) return
      root.current?.querySelector<HTMLElement>('button:not(:disabled), a[href]')?.focus()
    })
    return () => cancelAnimationFrame(frame)
  }, [review?.open, review?.finalFocus])
  useLayoutEffect(() => {
    if (lastFocused.current && !lastFocused.current.isConnected && document.activeElement === document.body) root.current?.querySelector<HTMLElement>('button:not(:disabled), a[href]')?.focus()
  })
  return <div onFocus={event => { if (event.target instanceof HTMLElement) lastFocused.current = event.target }} ref={root} className="fleet-survey-decision"><p role="status" aria-label={status.label}>{status.message}</p>{children}<div className="fleet-pattern-actions">{actions.map(action => <Button key={action.label} ref={action.ref} type={action.submit ? 'submit' : 'button'} icon={action.icon} variant={action.variant ?? 'default'} disabled={action.disabled} onClick={action.submit ? undefined : event => action.onSelect(event.currentTarget)}>{action.label}</Button>)}</div>{review && <ConfirmationDialog {...review} />}</div>
}
export function SurveyAnswerSummary({ items }: { items: readonly { id: string; label: string; value: string }[] }) {
  return <dl className="fleet-survey-answer-review">{items.map(item => <div key={item.id}><dt>{item.label}</dt><dd>{item.value}</dd></div>)}</dl>
}
export function SurveyDefinitionReview({ definition, timezone }: { definition: components['schemas']['SurveyDefinition']; timezone: string }) {
  return <section aria-label="Exact survey definition review"><dl className="fleet-survey-facts"><div><dt>Questionnaire</dt><dd>{definition.form.name}</dd></div><div><dt>Audience</dt><dd>{definition.form.audience}</dd></div><div><dt>Opens</dt><dd>{localDate(definition.opens_at, timezone)}</dd></div><div><dt>Closes</dt><dd>{localDate(definition.closes_at, timezone)}</dd></div><div><dt>Minimum report cohort</dt><dd>{definition.minimum_cohort}</dd></div></dl><ol className="fleet-survey-questions">{definition.form.fields.map(field => <li key={field.id}><h4>{field.label}</h4><p>{field.type} · {field.required ? 'Required' : 'Optional'} · {field.visible_when ? `Conditional on ${field.visible_when.field_id} = ${field.visible_when.equals}` : 'Always visible'}</p>{field.help && <p>{field.help}</p>}{field.choices.length > 0 && <p>Choices: {field.choices.join(' · ')}</p>}</li>)}</ol>{definition.measures.length ? <ul>{definition.measures.map(measure => <li key={measure.question_id}>Question {measure.question_id} · {measure.construct} · Exact objective {measure.measure.objective.id} version {measure.measure.objective.version} · measure {measure.measure.measure_id}</li>)}</ul> : <p>No exact success measure is linked. This does not mean zero achievement.</p>}</section>
}

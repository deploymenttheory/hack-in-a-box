import { useRef, useState } from 'react'
import { RubricPlanningFields, validateRubricPlanningForm, revealRubricInvalidField } from '../rubric-planning-fields'
import { RubricPlanningReview } from '../rubric-planning-review'
import { Button } from '../components/ui/button'
import { ConfirmationDialog } from '../components/ui/dialog'
import { rubricGuideDefinition } from './rubric-fixture'
import { Snippet } from './snippet'

export function RubricPlanningExample() {
  const [value, setValue] = useState(rubricGuideDefinition), [captured, setCaptured] = useState<ReturnType<typeof rubricGuideDefinition>>(), [notice, setNotice] = useState('')
  const form = useRef<HTMLFormElement>(null), opener = useRef<HTMLButtonElement>(null)
  return <section className="guide-section" aria-label="Rubric authoring example"><h2>Rubric authoring and captured decision review</h2><p>These are the production fields and review, using local synthetic values. This example creates no application requests, saves, approvals or publication.</p>
    <form ref={form} onSubmit={event => { event.preventDefault(); if (validateRubricPlanningForm(form.current)) setCaptured(structuredClone(value)) }} onInvalidCapture={event => revealRubricInvalidField(event.target as HTMLElement)}><RubricPlanningFields value={value} change={setValue} /><div className="fleet-pattern-actions"><Button ref={opener} type="submit" icon="next">Review guide rubric</Button><Button type="button" icon="refresh" variant="outline" onClick={() => { setValue(rubricGuideDefinition()); setNotice('') }}>Reset rubric example</Button></div></form>
    <ConfirmationDialog open={!!captured} title="Review synthetic rubric" description="This local example cannot save or approve a native rubric." confirmLabel="Finish local rubric review" finalFocus={opener} onOpenChange={open => { if (!open) setCaptured(undefined) }} onConfirm={() => { setCaptured(undefined); setNotice('Local review finished. No rubric was saved, approved or published.') }} printExcluded>{captured && <RubricPlanningReview definition={captured} revision={0} version={0} reason="Synthetic local example only" action="save" />}</ConfirmationDialog>
    {notice && <p role="status">{notice}</p>}<p>Keep stable category and award identities. Name each numeric bound and anchor, reveal an invalid section before focusing it, and restore the actual review opener on correction. Current reads pause decisions while local editing retains focus. A current API read never acknowledges an unknown save or approval.</p>
    <Snippet>{'<RubricPlanningFields value={localDefinition} change={setLocalDefinition}\n  disabled={commandPending || originalCommandUnknown} />\n<RubricPlanningReview definition={capturedDefinition}\n  revision={capturedRevision} version={capturedVersion}\n  reason={capturedReason} action="approve" />\n// The native owner separately checks current authority and exact version.'}</Snippet>
  </section>
}

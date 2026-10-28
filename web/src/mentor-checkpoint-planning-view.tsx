import type { FormEvent, RefObject } from 'react'
import { ActionLink, Button } from './components/ui/button'
import { Field, Input, Select, Textarea } from './components/ui/fields'
import { ConfirmationDialog } from './components/ui/dialog'
import { CollectionRecords } from './components/patterns'
import { localDate } from './event-time'
import { rubricReferenceKey } from './rubric-planning-model'
import { mentorAssessmentRuns, type CheckpointInput, type CheckpointIntent, type MentorAssessmentPurpose, type MentorCheckpointPlanning } from './mentor-checkpoint-planning-model'

export type MentorPlannerInput = { task: 'run' | 'checkpoint'; purpose: MentorAssessmentPurpose; rubric: string; targetID: string; checkpoint: CheckpointInput; reason: string }
export type MentorPlannerReview = { kind: 'run'; source: MentorCheckpointPlanning; input: MentorPlannerInput } | { kind: 'checkpoint'; source: MentorCheckpointPlanning; intent: CheckpointIntent }
export function emptyMentorPlannerInput(source: MentorCheckpointPlanning): MentorPlannerInput {
  return { task: mentorAssessmentRuns(source).length ? 'checkpoint' : 'run', purpose: 'checkpoint', rubric: '', targetID: '', checkpoint: { runID: '', name: '', opens: '', closes: '', dst: '', reason: '' }, reason: '' }
}
export const mentorPurposeLabel = (purpose: string) => purpose === 'mentor_final' ? 'Frozen-output qualification' : 'Live observations'
// Editing makes the affected validation result stale; unrelated errors remain
// visible. Local-time choices also invalidate their dependent window messages.
export function editedMentorPlannerErrors(before: MentorPlannerInput, next: MentorPlannerInput, errors: Record<string, string>) {
  if (before.task !== next.task || before.targetID !== next.targetID) return {}
  const result = { ...errors }
  if (next.task === 'run') {
    if (before.rubric !== next.rubric) delete result.rubric
    if (before.purpose !== next.purpose) delete result.purpose
    if (before.reason !== next.reason) delete result.reason
  } else {
    if (before.checkpoint.runID !== next.checkpoint.runID) delete result.run
    if (before.checkpoint.name !== next.checkpoint.name) delete result.name
    if (before.checkpoint.reason !== next.checkpoint.reason) delete result.reason
    if (before.checkpoint.opens !== next.checkpoint.opens || before.checkpoint.closes !== next.checkpoint.closes || before.checkpoint.dst !== next.checkpoint.dst) { delete result.opens; delete result.closes }
  }
  return result
}
export function MentorPlannerFields({ source, input, change, errors, disabled, review, form }: {
  source: MentorCheckpointPlanning; input: MentorPlannerInput; change: (input: MentorPlannerInput) => void; errors: Record<string, string>; disabled: boolean; review: (button: HTMLElement) => void; form: RefObject<HTMLFormElement | null>
}) {
  const runs = mentorAssessmentRuns(source), checkpoint = input.checkpoint, edit = (next: Partial<CheckpointInput>) => change({ ...input, checkpoint: { ...checkpoint, ...next } })
  function submit(event: FormEvent<HTMLFormElement>) { event.preventDefault(); const button = (event.nativeEvent as SubmitEvent).submitter; if (button instanceof HTMLElement) review(button) }
  function target(id: string) {
    const current = source.checkpoints.items.find(value => value.definition.id === id)
    change({ ...input, targetID: id, checkpoint: current ? { runID: current.definition.run_id, name: current.definition.name, opens: current.definition.opens_wall_time, closes: current.definition.closes_wall_time, dst: current.definition.dst_choice, reason: '' } : emptyMentorPlannerInput(source).checkpoint })
  }
  return <form ref={form} noValidate onSubmit={submit} className="grid gap-5 py-5">
    <Field label="Planning task"><Select disabled={disabled} value={input.task} onChange={event => change({ ...emptyMentorPlannerInput(source), task: event.target.value as MentorPlannerInput['task'] })}><option value="run">Create mentor assessment run</option><option value="checkpoint">Configure checkpoint window</option></Select></Field>
    {input.task === 'run' ? <>
      <h3 className="text-base font-semibold">Create a separate mentor assessment</h3><p>Live observations are provisional. Final mentor qualification reviews fresh frozen output and never carries live scores forward.</p>
      <Field label="Assessment purpose" error={errors.purpose}><Select value={input.purpose} disabled={disabled} onChange={event => change({ ...input, purpose: event.target.value as MentorAssessmentPurpose })}><option value="checkpoint">Live observations</option>{source.approved?.definition.mode === 'mentor_qualification' && <option value="mentor_final">Frozen-output qualification</option>}</Select></Field>
      <Field label="Approved mentor rubric" error={errors.rubric}><Select value={input.rubric} disabled={disabled} onChange={event => change({ ...input, rubric: event.target.value })}><option value="">Choose approved criteria</option>{source.approved && <option value={rubricReferenceKey(source.approved.reference)}>{source.approved.definition.name} · Version {source.approved.reference.version}</option>}</Select></Field>
      {!source.approved && <p>Approve the criteria in Judging criteria before planning an assessment.</p>}
      <Field label="Assessment planning reason" error={errors.reason}><Textarea value={input.reason} disabled={disabled} maxLength={2000} onChange={event => change({ ...input, reason: event.target.value })} /></Field>
    </> : <>
      <h3 className="text-base font-semibold">Configure a named checkpoint</h3><p>Times use {source.event.configuration.timezone}. Live observations stay inside the live-to-cutoff window. Frozen-output qualification opens at or after cutoff.</p>
      <Field label="Saved checkpoint" error={errors.target}><Select value={input.targetID} disabled={disabled} onChange={event => target(event.target.value)}><option value="">New checkpoint</option>{source.checkpoints.items.map(value => <option key={value.definition.id} value={value.definition.id}>{value.definition.name} · Version {value.version}</option>)}</Select></Field>
      <Field label="Mentor assessment run" error={errors.run}><Select value={checkpoint.runID} disabled={disabled} onChange={event => edit({ runID: event.target.value })}><option value="">Choose an assessment run</option>{runs.map((run, index) => <option key={run.id} value={run.id}>{mentorPurposeLabel(run.purpose)} · Rubric version {run.reference.version} · Run {index + 1}</option>)}</Select></Field>
      {!runs.length && <p>Create an assessment run first, then configure its checkpoint window.</p>}
      {!checkpoint.runID ? <p className="text-sm text-muted-foreground">Choose an assessment run to name its checkpoint and review the window.</p> : <>
      <Field label="Checkpoint name" error={errors.name}><Input value={checkpoint.name} disabled={disabled} maxLength={160} onChange={event => edit({ name: event.target.value })} /></Field>
      <div className="grid gap-5 md:grid-cols-2"><Field label="Checkpoint opens" description={source.event.configuration.timezone} error={errors.opens}><Input type="datetime-local" step="1" value={checkpoint.opens} disabled={disabled} onChange={event => edit({ opens: event.target.value })} /></Field><Field label="Checkpoint closes" description={source.event.configuration.timezone} error={errors.closes}><Input type="datetime-local" step="1" value={checkpoint.closes} disabled={disabled} onChange={event => edit({ closes: event.target.value })} /></Field></div>
      <Field label="Repeated local clock times"><Select value={checkpoint.dst} disabled={disabled} onChange={event => edit({ dst: event.target.value as CheckpointInput['dst'] })}><option value="">Ask when ambiguous</option><option value="earlier">Earlier occurrence</option><option value="later">Later occurrence</option></Select></Field>
      <Field label="Checkpoint configuration reason" error={errors.reason}><Textarea value={checkpoint.reason} disabled={disabled} maxLength={2000} onChange={event => edit({ reason: event.target.value })} /></Field>
      </>}
    </>}
    <div className="fleet-pattern-actions"><Button icon="next" type="submit" disabled={disabled}>{input.task === 'run' ? 'Review mentor assessment' : 'Review checkpoint window'}</Button></div>
  </form>
}
export function MentorPlannerCurrent({ source }: { source: MentorCheckpointPlanning }) {
  return <section aria-label="Current mentor checkpoints" className="space-y-3 py-5 border-t border-border"><h3 className="text-base font-semibold">Published checkpoint windows</h3>{source.checkpoints.items.length ? <CollectionRecords label="Mentor checkpoint windows" rows={source.checkpoints.items} rowKey={value => value.definition.id} rowLabel={value => value.definition.name} columns={[
    { id: 'purpose', label: 'Assessment', cell: value => mentorPurposeLabel(value.purpose), essential: true },
    { id: 'window', label: 'Resolved window', cell: value => `${localDate(value.opens_at, value.timezone)} — ${localDate(value.closes_at, value.timezone)} · ${value.timezone}`, essential: true },
    { id: 'source', label: 'Source version', cell: value => `Checkpoint ${value.version} · Rubric ${value.rubric.version}` },
  ]} /> : <p>No mentor checkpoint windows are configured.</p>}<div className="fleet-pattern-actions"><ActionLink icon="list" href={`/events/${source.event.id}?section=mentors`}>Review published mentor coverage</ActionLink><ActionLink icon="next" href={`/events/${source.event.id}/review`}>Open support and review coverage</ActionLink></div><p className="text-sm text-muted-foreground">Assigned mentors open their own independent scorecards from Mentor workspace. Configuring a checkpoint neither submits a card nor signs off or publishes a result.</p></section>
}
export function MentorPlannerDecision({ value, pending, disabled, confirm, cancel, returnFocus, error }: {
  value: MentorPlannerReview; pending: boolean; disabled: boolean; confirm: () => void; cancel: () => void; returnFocus: () => HTMLElement | null; error?: string
}) {
  return <ConfirmationDialog open title={value.kind === 'run' ? 'Review mentor assessment run' : 'Review mentor checkpoint window'} description="Confirm this captured source and effect. Current organiser authority and versions are checked again before writing." confirmLabel={value.kind === 'run' ? 'Create reviewed mentor assessment' : 'Save reviewed checkpoint window'} pending={pending} disabled={disabled} onConfirm={confirm} onOpenChange={open => { if (!open) cancel() }} finalFocus={returnFocus} printExcluded>
    <p>{value.source.event.name}</p>{value.kind === 'run' ? <><p>{mentorPurposeLabel(value.input.purpose)} · {value.source.approved?.definition.name} · Approved version {value.source.approved?.reference.version} · Rubric revision {value.source.configuration.revision}</p><p>{value.input.reason.trim()}</p></> : <><p>{value.intent.command.definition.name} · {mentorPurposeLabel(value.intent.run.purpose)} · Rubric version {value.intent.run.reference.version}</p><p>Checkpoint version {value.intent.command.version} → {value.intent.command.version + 1} · Event revision {value.source.checkpoints.revision}</p><p>{localDate(value.intent.window.opens, value.intent.timezone)} — {localDate(value.intent.window.closes, value.intent.timezone)} · {value.intent.timezone}</p><p>Local times: {value.intent.command.definition.opens_wall_time} — {value.intent.command.definition.closes_wall_time} · Repeated clock choice: {value.intent.command.definition.dst_choice || 'No ambiguous occurrence selected'}</p><p>{value.intent.command.reason}</p></>}{error && <p role="alert">{error}</p>}
  </ConfirmationDialog>
}

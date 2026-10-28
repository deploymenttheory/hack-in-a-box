import { useId, type FormEvent, type RefObject } from 'react'
import { Button } from './components/ui/button'
import { Checkbox, Field, FieldError, Label, Select, Textarea } from './components/ui/fields'
import { ConfirmationDialog } from './components/ui/dialog'
import { CollectionRecords } from './components/patterns'
import { currentJudges, publishedJudgingTeams, type JudgingSetup } from './judging-setup-model'
import { rubricReferenceKey } from './rubric-planning-model'

export type JudgingSetupInput = { rubric: string; teamID: string; judgeIDs: string[]; reason: string; acknowledgeGap: boolean }
export const emptyJudgingSetupInput = (): JudgingSetupInput => ({ rubric: '', teamID: '', judgeIDs: [], reason: '', acknowledgeGap: false })
export type JudgingSetupReview = { kind: 'create' | 'assign'; source: JudgingSetup; input: JudgingSetupInput }

function JudgingChoice({ label, checked, change, disabled, error }: { label: string; checked: boolean; change: (checked: boolean) => void; disabled?: boolean; error?: string }) {
  const id = useId(), errorID = `${id}-error`
  return <div><Label htmlFor={id} className="flex items-start gap-3"><Checkbox id={id} className="mt-0.5 shrink-0" checked={checked} disabled={disabled} aria-invalid={!!error} aria-describedby={error ? errorID : undefined} onChange={event => change(event.target.checked)} /><span>{label}</span></Label>{error && <FieldError id={errorID}>{error}</FieldError>}</div>
}
export function JudgingSetupFields({ source, input, change, errors, disabled, review, form }: {
  source: JudgingSetup; input: JudgingSetupInput; change: (input: JudgingSetupInput) => void; errors: Record<string, string>; disabled: boolean
  review: (button: HTMLElement) => void; form: RefObject<HTMLFormElement | null>
}) {
  const judges = currentJudges(source), teams = publishedJudgingTeams(source), required = source.round?.required_judges ?? source.approved?.definition.required_judge_count ?? 0
  const missing = Math.max(0, required - input.judgeIDs.length), retained = input.judgeIDs.filter(id => !judges.some(judge => judge.person_id === id))
  function submit(event: FormEvent<HTMLFormElement>) { event.preventDefault(); const button = (event.nativeEvent as SubmitEvent).submitter; if (button instanceof HTMLElement) review(button) }
  return <form ref={form} noValidate className="grid gap-5 py-5" onSubmit={submit}>
    {!source.run ? <>
      <h3 className="text-base font-semibold">Create the final judging round</h3>
      <p>Select the approved criteria for independent final judging. This plans a round; it does not open scoring or publish results.</p>
      <Field label="Approved rubric version" error={errors.rubric}><Select value={input.rubric} disabled={disabled} onChange={event => change({ ...input, rubric: event.target.value })}>
        <option value="">Choose an approved version</option>
        {source.approved && <option value={rubricReferenceKey(source.approved.reference)}>{source.approved.definition.name} · Version {source.approved.reference.version}</option>}
      </Select></Field>
      {!source.approved && <p>No approved rubric is available. Approve the criteria in Judging criteria, then read current setup.</p>}
    </> : <>
      <h3 className="text-base font-semibold">Assign judges to a team</h3>
      <p>Final round · Rubric version {source.run.reference.version} · {required} required judges per team. Assignments do not grant early access to competition evidence.</p>
      <Field label="Team to assign" error={errors.team}><Select value={input.teamID} disabled={disabled} onChange={event => {
        const teamID = event.target.value, assignment = source.round?.assignments.find(value => value.team_id === teamID)
        change({ ...input, teamID, judgeIDs: assignment?.judge_ids ?? [], reason: '', acknowledgeGap: false })
      }}><option value="">Choose a frozen team</option>{teams.map(team => <option key={team.id} value={team.id}>{team.name}</option>)}</Select></Field>
      {!teams.length && <p>No frozen published teams are available yet.</p>}
      {!input.teamID ? <p className="text-sm text-muted-foreground">Choose a team to review its current judges and coverage.</p> : <>
        <fieldset disabled={disabled} className="grid gap-3 border-0 p-0 m-0">
          <legend className="mb-3 text-sm font-semibold">Current judges</legend>
          {judges.map((judge, index) => <JudgingChoice key={judge.person_id} label={judge.display_name} checked={input.judgeIDs.includes(judge.person_id)} error={index === 0 && !retained.length ? errors.judges : undefined} change={checked => change({ ...input, judgeIDs: checked ? [...input.judgeIDs, judge.person_id] : input.judgeIDs.filter(id => id !== judge.person_id), acknowledgeGap: false })} />)}
          {retained.map((id, index) => <JudgingChoice key={id} label={`${source.staff.items.find(person => person.person_id === id)?.display_name ?? 'Former judge'} (inactive; uncheck to remove)`} checked error={index === 0 ? errors.judges : undefined} change={() => change({ ...input, judgeIDs: input.judgeIDs.filter(value => value !== id), acknowledgeGap: false })} />)}
          {!judges.length && <p>No current judge roles are available. Review staff assignments first.</p>}
        </fieldset>
        <p role="status" aria-label="Selected judge coverage">{input.judgeIDs.length} selected · {required} required{missing ? ` · ${missing} unfilled` : ''}. Current roles and conflicts are checked at confirmation.</p>
        {missing > 0 && <JudgingChoice label={`I understand this leaves ${missing} required judge ${missing === 1 ? 'place' : 'places'} unfilled`} checked={input.acknowledgeGap} disabled={disabled} error={errors.gap} change={acknowledgeGap => change({ ...input, acknowledgeGap })} />}
        <p className="text-sm text-muted-foreground">Changing an existing assignment replaces that team’s judge cohort and can invalidate pending result approvals. Other teams retain their assignments.</p>
      </>}
    </>}
    {(!source.run || !!input.teamID) && <Field label={source.run ? 'Assignment reason' : 'Round creation reason'} error={errors.reason}><Textarea required maxLength={source.run ? 4000 : 2000} value={input.reason} disabled={disabled} onChange={event => change({ ...input, reason: event.target.value })} /></Field>}
    <div className="fleet-pattern-actions"><Button type="submit" icon={source.run ? 'user' : 'add'} disabled={disabled || (!source.run && !source.approved)}>{source.run ? 'Review judge assignment' : 'Review final round creation'}</Button></div>
  </form>
}
export function JudgingSetupCurrent({ source }: { source: JudgingSetup }) {
  if (!source.run || !source.round) return null
  const rows = publishedJudgingTeams(source).map(team => ({ team, assignment: source.round!.assignments.find(value => value.team_id === team.id) }))
  return <section aria-label="Current final judge assignments" className="space-y-3 py-5 border-t border-border"><h3 className="text-base font-semibold">Current team coverage</h3><CollectionRecords label="Final judging coverage" rows={rows} rowKey={row => row.team.id} rowLabel={row => row.team.name} columns={[
    { id: 'judges', label: 'Assigned judges', essential: true, cell: row => row.assignment?.judge_ids.map(id => source.staff.items.find(person => person.person_id === id)?.display_name ?? 'Former judge').join(', ') || 'Not assigned' },
    { id: 'missing', label: 'Required places unfilled', essential: true, cell: row => row.assignment?.missing ?? source.round!.required_judges },
  ]} /><p className="text-sm text-muted-foreground">Judges may rehearse fictional practice before live start. Real scoring remains gated by the actual cutoff, eligible frozen evidence, reviewed code and current assignments.</p></section>
}
export function JudgingSetupDecision({ value, pending, disabled, confirm, cancel, returnFocus, error }: {
  value: JudgingSetupReview; pending: boolean; disabled: boolean; confirm: () => void; cancel: () => void; returnFocus: () => HTMLElement | null; error?: string
}) {
  const { source, input } = value, team = source.teams.teams.find(team => team.id === input.teamID)
  return <ConfirmationDialog open title={value.kind === 'create' ? 'Create final judging round' : 'Review final judge assignment'} description="Confirm these captured criteria and named assignments. Current authority and source are checked again before any change." confirmLabel={value.kind === 'create' ? 'Create reviewed final round' : 'Save reviewed judge assignment'} pending={pending} disabled={disabled} onConfirm={confirm} onOpenChange={open => { if (!open) cancel() }} finalFocus={returnFocus} printExcluded>
    <p>{source.event.name} · {value.kind === 'create' ? `Rubric revision ${source.configuration.revision}` : `Round revision ${source.round?.revision}`}</p>
    {value.kind === 'create' ? <p>{source.approved?.definition.name} · Approved version {source.approved?.reference.version}. Practice and final scores stay separate.</p> : <><p>Team: {team?.name} · Rubric version {source.run?.reference.version}</p><ul>{input.judgeIDs.map(id => <li key={id}>{source.staff.items.find(person => person.person_id === id)?.display_name ?? 'Former judge'}</li>)}</ul><p>{input.judgeIDs.length} selected · {source.round?.required_judges} required{input.acknowledgeGap && ' · Unfilled places explicitly acknowledged'}. This replaces only the selected team’s assignments.</p></>}
    <p>{input.reason.trim()}</p>{error && <p role="alert">{error}</p>}
  </ConfirmationDialog>
}

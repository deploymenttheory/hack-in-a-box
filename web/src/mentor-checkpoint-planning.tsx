import { useCallback, useLayoutEffect, useRef, useState } from 'react'
import { useBlocker } from '@tanstack/react-router'
import { api, APIError, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { useEventReadBoundary } from './event-authority'
import { usePrivateRequestScope, type PrivateRequestScope } from './private-request-scope'
import { useTodayRead } from './today-read'
import { useCommunicationCommands } from './communication-tests'
import { usePlanningDepartureBarrier } from './planning-operations'
import { createdAssessmentRun } from './assessment-run-model'
import { readChallengeRubric } from './challenge-rubric-model'
import { ActionLink, Button } from './components/ui/button'
import { ConfirmationDialog } from './components/ui/dialog'
import { focusFirstInvalid } from './components/ui/fields'
import { Failure, Loading } from './components/states'
import { checkpointProblems, checkpointWindowProblems, CheckpointPlanningResponseIssue, CheckpointTimeIssue, loadMentorCheckpointPlanning, matchesCheckpointResponse, mentorAssessmentRuns, mentorPlanningIdentity, mentorPlanningVersion, mentorRunProblems, resolveCheckpointWindow, type MentorCheckpointPlanning } from './mentor-checkpoint-planning-model'
import { editedMentorPlannerErrors, emptyMentorPlannerInput, MentorPlannerCurrent, MentorPlannerDecision, MentorPlannerFields, type MentorPlannerReview } from './mentor-checkpoint-planning-view'

type Commands = ReturnType<typeof useCommunicationCommands>
const denied = (error: unknown) => error instanceof APIError && [401, 403, 404].includes(error.status)
export function MentorCheckpointPlanner({ event, onDirty }: { event: EventRecord; onDirty: (dirty: boolean) => void }) {
  const identity = useIdentity(), boundary = useEventReadBoundary(event.id)
  const owner = `${identity.person.id}:${identity.draft_scope}:${event.id}:mentor-checkpoints:commands`, commands = useCommunicationCommands(owner)
  const load = useCallback((signal: AbortSignal, assert: () => void) => loadMentorCheckpointPlanning(event.id, signal, assert), [event.id])
  const read = useTodayRead(owner, load, boundary?.scope, mentorPlanningIdentity), [notice, setNotice] = useState('')
  return <section aria-label="Organiser mentor checkpoint planning" className="space-y-3 py-6 border-y border-border fleet-print-excluded">
    <div className="flex flex-wrap items-center justify-between gap-3"><h2 className="text-lg font-semibold">Mentor assessment planning</h2><ActionLink icon="book" variant="ghost" href={`/events/${event.id}?section=judging%20criteria`}>Review judging criteria</ActionLink></div><p>Create a separate mentor assessment, then configure its checkpoint window. Published coverage determines which mentors can submit independent cards.</p>
    {read.denied ? <><p>Current organiser authority is unavailable. Private planning inputs have been cleared.</p><Button icon="refresh" onClick={() => void read.refresh()}>Check mentor planning access</Button></> : <>{read.error && <Failure error={read.error} retry={() => void read.refresh()} />}{!read.data ? !read.error && <Loading /> : <><p role="status" aria-label="Last acknowledged mentor planning change">{notice}</p><PlannerEditor key={read.generation} source={read.data} parent={read.authority} commands={commands} paused={read.pending || !!read.error || !!boundary?.paused} refresh={read.refresh} observe={read.observeCurrent} acknowledged={setNotice} onDirty={onDirty} /></>}</>}
  </section>
}
function PlannerEditor({ source, parent, commands, paused, refresh, observe, acknowledged, onDirty }: {
  source: MentorCheckpointPlanning; parent: PrivateRequestScope; commands: Commands; paused: boolean; refresh: () => Promise<void>; observe: (source: MentorCheckpointPlanning) => void; acknowledged: (message: string) => void; onDirty: (dirty: boolean) => void
}) {
  const identity = useIdentity(), boundary = useEventReadBoundary(source.event.id), scope = usePrivateRequestScope(`${source.event.id}:mentor-planner-editor`, parent)
  const [input, setInput] = useState(() => emptyMentorPlannerInput(source)), [errors, setErrors] = useState<Record<string, string>>({}), [review, setReview] = useState<MentorPlannerReview>()
  const [checking, setChecking] = useState(false), [failure, setFailure] = useState<unknown>(), [preflightFailure, setPreflightFailure] = useState(false), [needsCurrent, setNeedsCurrent] = useState(false), [denial, setDenial] = useState(false), [fresh, setFresh] = useState<MentorCheckpointPlanning>()
  const form = useRef<HTMLFormElement>(null), heading = useRef<HTMLHeadingElement>(null), opener = useRef<HTMLElement | null>(null), lock = useRef(false), focusValidation = useRef(false)
  const busy = checking || commands.pending, closed = ['completed', 'archived', 'cancelled'].includes(source.event.stage), blocked = paused || busy || needsCurrent || commands.unconfirmed || closed
  const saved = source.checkpoints.items.find(value => value.definition.id === input.targetID)
  const baseline = saved ? { runID: saved.definition.run_id, name: saved.definition.name, opens: saved.definition.opens_wall_time, closes: saved.definition.closes_wall_time, dst: saved.definition.dst_choice, reason: '' } : emptyMentorPlannerInput(source).checkpoint
  const dirty = !!review || !!fresh || (input.task === 'run' ? !!input.reason || !!input.rubric || input.purpose !== 'checkpoint' : JSON.stringify(input.checkpoint) !== JSON.stringify(baseline))
  usePlanningDepartureBarrier(busy)
  useBlocker({ shouldBlockFn: () => commands.pending || commands.unconfirmed ? false : checking || dirty && !window.confirm('Leave these unsaved mentor planning inputs?'), enableBeforeUnload: busy || dirty || commands.unconfirmed })
  useLayoutEffect(() => { onDirty(dirty); return () => onDirty(false) }, [dirty, onDirty])
  useLayoutEffect(() => { if (focusValidation.current) { focusValidation.current = false; if (form.current) focusFirstInvalid(form.current) } }, [errors])
  function showIssues(issues: Record<string, string>) { focusValidation.current = Object.keys(issues).length > 0; setErrors(issues) }
  function clearDenied() { scope.invalidate(); setInput(emptyMentorPlannerInput(source)); setReview(undefined); setFresh(undefined); setDenial(true); void refresh() }
  async function current() {
    const ticket = scope.capture()
    try {
      const latest = await loadMentorCheckpointPlanning(source.event.id, ticket.signal, ticket.assert); ticket.assert()
      if (boundary && !boundary.observeToday(latest.event)) throw new APIError(409, 'revision_conflict')
      if (mentorPlanningIdentity(latest) !== mentorPlanningIdentity(source)) { observe(latest); throw new APIError(409, 'planning_source_changed') }
      return latest
    } catch (error) { if (ticket.current() && denied(error)) clearDenied(); throw error }
  }
  async function begin(button: HTMLElement) {
    if (blocked || lock.current) return
    const invalid = input.task === 'run' ? mentorRunProblems(source, input.purpose, input.rubric, input.reason) : checkpointProblems(source, input.checkpoint)
    if (input.task === 'checkpoint' && input.targetID && !saved) invalid.target = 'Read the current saved checkpoint before editing.'
    showIssues(invalid); if (Object.keys(invalid).length) return
    lock.current = true; setChecking(true); setFailure(undefined); setPreflightFailure(false); opener.current = button
    const ticket = scope.capture()
    try {
      const latest = await current(); ticket.assert()
      if (mentorPlanningVersion(latest) !== mentorPlanningVersion(source)) { setNeedsCurrent(true); throw new APIError(409, 'planning_source_changed') }
      if (input.task === 'run') setReview({ kind: 'run', source: structuredClone(source), input: structuredClone(input) })
      else {
        const run = mentorAssessmentRuns(source).find(value => value.id === input.checkpoint.runID)!
        const rubric = await readChallengeRubric(source.event.id, run.reference, ticket.signal, ticket.assert); ticket.assert()
        if (!rubric.selected) throw new APIError(409, 'rubric_changed')
        const window = await resolveCheckpointWindow(source, input.checkpoint, identity.csrf_token, ticket.signal, ticket.assert); ticket.assert()
        const issues = checkpointWindowProblems(source.event, run.purpose as 'checkpoint' | 'mentor_final', window)
        showIssues(issues); if (Object.keys(issues).length) return
        const wall = (value: string) => value.length === 16 ? value + ':00' : value
        setReview({ kind: 'checkpoint', source: structuredClone(source), intent: { run: structuredClone(run), criteria: rubric.selected.definition, timezone: source.event.configuration.timezone, window, command: { version: saved?.version ?? 0, reason: input.checkpoint.reason.trim(), definition: { id: saved?.definition.id ?? crypto.randomUUID(), run_id: run.id, name: input.checkpoint.name.trim(), opens_wall_time: wall(input.checkpoint.opens), closes_wall_time: wall(input.checkpoint.closes), dst_choice: input.checkpoint.dst } } } })
      }
    } catch (error) { if (ticket.current()) { if (denied(error)) clearDenied(); else if (error instanceof CheckpointTimeIssue) showIssues({ [error.field]: error.message }); else { setFailure(error); setPreflightFailure(true) } } }
    finally { lock.current = false; if (ticket.current()) setChecking(false) }
  }
  async function confirm() {
    if (!review || blocked || lock.current) return
    lock.current = true; setChecking(true); setFailure(undefined); setPreflightFailure(false)
    const captured = review, ticket = scope.capture(); let attemptID: string | undefined
    try {
      const latest = await current(); ticket.assert()
      if (mentorPlanningVersion(latest) !== mentorPlanningVersion(captured.source)) { setNeedsCurrent(true); throw new APIError(409, 'planning_source_changed') }
      const revision = captured.kind === 'run' ? captured.source.configuration.revision : captured.source.checkpoints.revision
      attemptID = commands.begin({ operation: captured.kind === 'run' ? 'Create mentor assessment' : 'Configure mentor checkpoint', revision, command: { action: captured.kind === 'run' ? 'create_run' : 'configure_checkpoint', intentID: crypto.randomUUID(), resourceID: captured.kind === 'run' ? captured.source.approved!.reference.id : captured.intent.command.definition.id, subjectID: captured.kind === 'checkpoint' ? captured.intent.run.id : undefined, expectedVersion: captured.kind === 'checkpoint' ? captured.intent.command.version + 1 : undefined } })
      if (captured.kind === 'run') {
        const result = unwrap(await api.POST('/api/v1/events/{event_id}/rubric/configuration', { params: { path: { event_id: source.event.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${revision}"` } }, body: { action: 'create_run', purpose: captured.input.purpose, version: captured.source.approved!.reference.version, reason: captured.input.reason.trim() } }))
        if (!createdAssessmentRun(captured.source.configuration, result, captured.source.approved!.reference, captured.input.purpose)) throw new CheckpointPlanningResponseIssue('The original response did not acknowledge the exact reviewed mentor assessment.')
      } else {
        const result = unwrap(await api.POST('/api/v1/events/{event_id}/scorecard-checkpoints', { params: { path: { event_id: source.event.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${revision}"` } }, body: captured.intent.command }))
        if (!matchesCheckpointResponse(result, captured.intent)) throw new CheckpointPlanningResponseIssue('The original response did not acknowledge the exact reviewed checkpoint and window.')
      }
      commands.complete(attemptID); ticket.assert(); setInput({ ...emptyMentorPlannerInput(source), task: 'checkpoint' }); setReview(undefined)
      acknowledged(captured.kind === 'run' ? 'Mentor assessment created from the reviewed criteria. Next: configure its named checkpoint window.' : 'The reviewed checkpoint window was acknowledged. Next: review published mentor coverage and independent scorecards.')
      await refresh()
    } catch (error) {
      if (attemptID) { if (error instanceof APIError && error.status >= 400 && error.status < 500 && error.status !== 408) commands.complete(attemptID); else commands.uncertain(attemptID) }
      if (ticket.current()) { if (denied(error)) clearDenied(); else { setFailure(error); setPreflightFailure(!attemptID); if (attemptID) setReview(undefined); if (error instanceof APIError && error.status === 409) setNeedsCurrent(true) } }
    } finally { lock.current = false; if (ticket.current()) setChecking(false) }
  }
  async function readCurrent() {
    if (busy || lock.current) return
    lock.current = true; setChecking(true); const ticket = scope.capture()
    try { await refresh(); if (ticket.current()) { setNeedsCurrent(false); setFailure(undefined); setReview(undefined) } }
    finally { lock.current = false; if (ticket.current()) setChecking(false) }
  }
  async function reviewFresh() {
    if (busy || lock.current) return
    lock.current = true; setChecking(true); const ticket = scope.capture(); setFailure(undefined)
    try { const latest = await current(); ticket.assert(); setFresh(latest) }
    catch (error) { if (ticket.current()) setFailure(error) }
    finally { lock.current = false; if (ticket.current()) setChecking(false) }
  }
  async function acceptFresh() {
    if (!fresh || busy || lock.current) return
    lock.current = true; setChecking(true); const ticket = scope.capture()
    try { const latest = await current(); ticket.assert(); if (mentorPlanningVersion(latest) !== mentorPlanningVersion(fresh)) throw new Error('The planner changed again. Cancel and read its current state.'); commands.selectNewBase(); setInput(emptyMentorPlannerInput(latest)); setFresh(undefined); setNeedsCurrent(false); await refresh() }
    catch (error) { if (ticket.current()) setFailure(error) }
    finally { lock.current = false; if (ticket.current()) setChecking(false) }
  }
  if (denial) return <p>Current organiser authority ended. Private planning inputs have been cleared.</p>
  return <><h3 ref={heading} tabIndex={-1} className="text-base font-semibold">Assessment and checkpoint windows</h3><Button icon="refresh" variant="outline" disabled={busy} onClick={() => void readCurrent()}>Read current mentor planning</Button>{closed && <p>The event is closed. Mentor planning is read-only.</p>}{needsCurrent && <p role="status">The captured source changed. Read current mentor planning and review again.</p>}
    {!closed && <div hidden={!!review || !!fresh}><MentorPlannerFields source={source} input={input} change={next => { setErrors(editedMentorPlannerErrors(input, next, errors)); setInput(next) }} errors={errors} disabled={blocked} review={button => void begin(button)} form={form} /></div>}
    {!!failure && !review && (failure instanceof CheckpointPlanningResponseIssue ? <p role="alert">{failure.message}</p> : <Failure error={failure} />)}{preflightFailure && !review && <p role="status">Current planning could not be confirmed. No configuration write was sent.</p>}
    {commands.unconfirmed && <><p role="status">The original planning outcome is unknown. A current read cannot acknowledge it, and no configuration is replayed automatically.</p><Button icon="next" variant="outline" disabled={busy} onClick={event => { opener.current = event.currentTarget; void reviewFresh() }}>Review current planner as a new starting point</Button></>}
    <MentorPlannerCurrent source={source} />
    {review && <MentorPlannerDecision value={review} pending={busy} disabled={paused || commands.unconfirmed || needsCurrent} confirm={() => void confirm()} cancel={() => setReview(undefined)} returnFocus={() => opener.current?.isConnected && !opener.current.matches(':disabled') ? opener.current : heading.current} error={preflightFailure ? 'Current source could not be confirmed. Nothing was sent. Correct or read current planning before another review.' : undefined} />}
    {fresh && <ConfirmationDialog open title="Review current mentor planning base" description="Use this current source for a separate deliberate review. Earlier unknown outcomes remain unknown; nothing is acknowledged or replayed." confirmLabel="Use reviewed mentor planning base" pending={busy} disabled={paused} onConfirm={() => void acceptFresh()} onOpenChange={open => { if (!open) setFresh(undefined) }} finalFocus={() => opener.current?.isConnected && !opener.current.matches(':disabled') ? opener.current : heading.current} printExcluded><p>{fresh.event.name} · Event revision {fresh.event.revision} · Rubric revision {fresh.configuration.revision}</p><MentorPlannerCurrent source={fresh} />{!!failure && <Failure error={failure} />}</ConfirmationDialog>}
  </>
}

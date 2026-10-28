import { useCallback, useLayoutEffect, useRef, useState } from 'react'
import { useBlocker } from '@tanstack/react-router'
import { api, APIError, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { useEventReadBoundary } from './event-authority'
import { usePrivateRequestScope, type PrivateRequestScope } from './private-request-scope'
import { useTodayRead } from './today-read'
import { useCommunicationCommands } from './communication-tests'
import { usePlanningDepartureBarrier } from './planning-operations'
import { ActionLink, Button } from './components/ui/button'
import { ConfirmationDialog } from './components/ui/dialog'
import { focusFirstInvalid } from './components/ui/fields'
import { Failure, Loading } from './components/states'
import { assignmentFingerprint, assignmentProblems, roundCreationProblems, JudgingSetupReceiptIssue, createdFinalRound, judgingSetupIdentity, judgingSetupVersion, loadJudgingSetup, matchesAssignmentReceipt, type AssignmentIntent, type JudgeAssignmentCommand, type JudgingSetup } from './judging-setup-model'
import { emptyJudgingSetupInput, JudgingSetupCurrent, JudgingSetupDecision, JudgingSetupFields, type JudgingSetupReview } from './judging-setup-view'

type Commands = ReturnType<typeof useCommunicationCommands>
const denied = (error: unknown) => error instanceof APIError && [401, 403, 404].includes(error.status)

export function OrganiserJudgingSetup({ event, parent }: { event: EventRecord; parent: PrivateRequestScope }) {
  const identity = useIdentity(), boundary = useEventReadBoundary(event.id)
  const owner = `${identity.person.id}:${identity.draft_scope}:${event.id}:judging-setup`
  const commands = useCommunicationCommands(owner)
  const load = useCallback((signal: AbortSignal, assert: () => void) => loadJudgingSetup(event.id, signal, assert), [event.id])
  const read = useTodayRead(owner, load, parent, judgingSetupIdentity)
  const [notice, setNotice] = useState('')
  return <section aria-label="Organiser final judging setup" className="py-6 border-y border-border fleet-print-excluded">
    <h2>Final judging setup</h2><ActionLink icon="book" href={`/events/${event.id}?section=judging%20criteria`}>Review judging criteria</ActionLink><p>Create the independent final round, then review the judges for each frozen team. Practice, competition scores and result publication remain separate tasks.</p>
    {read.denied ? <><p>Current organiser access is unavailable. Private setup inputs and reviews have been cleared.</p><Button icon="refresh" onClick={() => void read.refresh()}>Check current setup access</Button></> : <>{read.error && <Failure error={read.error} retry={() => void read.refresh()} />}{!read.data ? !read.error && <Loading /> : <><p role="status" aria-label="Last acknowledged setup change">{notice}</p><SetupEditor key={read.generation} source={read.data} parent={read.authority} commands={commands} paused={read.pending || !!read.error || !!boundary?.paused} refresh={read.refresh} observe={read.observeCurrent} acknowledged={setNotice} /></>}</>}
  </section>
}
function SetupEditor({ source, parent, commands, paused, refresh, observe, acknowledged }: {
  source: JudgingSetup; parent: PrivateRequestScope; commands: Commands; paused: boolean; refresh: () => Promise<void>; observe: (source: JudgingSetup) => void; acknowledged: (message: string) => void
}) {
  const identity = useIdentity(), boundary = useEventReadBoundary(source.event.id)
  const scope = usePrivateRequestScope(`${source.event.id}:judging-setup-editor`, parent)
  const [input, setInput] = useState(emptyJudgingSetupInput), [errors, setErrors] = useState<Record<string, string>>({}), [review, setReview] = useState<JudgingSetupReview>()
  const [checking, setChecking] = useState(false), [failure, setFailure] = useState<unknown>(), [failureStage, setFailureStage] = useState<'before' | 'sent'>(), [needsCurrent, setNeedsCurrent] = useState(false)
  const [denial, setDenial] = useState(false), [fresh, setFresh] = useState<JudgingSetup>()
  const lock = useRef(false), form = useRef<HTMLFormElement>(null), heading = useRef<HTMLHeadingElement>(null), opener = useRef<HTMLElement | null>(null)
  const busy = checking || commands.pending, closed = ['completed', 'archived', 'cancelled'].includes(source.event.stage)
  const blocked = paused || busy || commands.unconfirmed || needsCurrent || closed
  const savedJudges = source.round?.assignments.find(assignment => assignment.team_id === input.teamID)?.judge_ids ?? []
  const cohortChanged = JSON.stringify([...input.judgeIDs].sort()) !== JSON.stringify([...savedJudges].sort())
  const dirty = !!input.reason || !!input.rubric || cohortChanged || input.acknowledgeGap || !!review || !!fresh
  usePlanningDepartureBarrier(busy)
  useBlocker({ shouldBlockFn: () => commands.pending || commands.unconfirmed ? false : checking || dirty && !window.confirm('Leave these unsaved judging setup inputs?'), enableBeforeUnload: busy || dirty || commands.unconfirmed })
  useLayoutEffect(() => { if (form.current && Object.keys(errors).length) focusFirstInvalid(form.current) }, [errors])
  function clearDenied() { scope.invalidate(); setInput(emptyJudgingSetupInput()); setReview(undefined); setFresh(undefined); setDenial(true); void refresh() }
  async function current() {
    const ticket = scope.capture()
    try {
      const value = await loadJudgingSetup(source.event.id, ticket.signal, ticket.assert); ticket.assert()
      if (boundary && !boundary.observeToday(value.event)) throw new APIError(409, 'revision_conflict')
      if (judgingSetupIdentity(value) !== judgingSetupIdentity(source)) { observe(value); throw new APIError(409, 'revision_conflict') }
      return value
    } catch (error) { if (ticket.current() && denied(error)) clearDenied(); throw error }
  }
  function begin(button: HTMLElement) {
    if (blocked || lock.current) return
    const invalid = source.run ? assignmentProblems(source, input.teamID, input.judgeIDs, input.reason, input.acknowledgeGap) : roundCreationProblems(source, input.rubric, input.reason)
    setErrors(invalid); if (Object.keys(invalid).length) return
    opener.current = button; setFailure(undefined); setFailureStage(undefined)
    setReview({ kind: source.run ? 'assign' : 'create', source: structuredClone(source), input: structuredClone(input) })
  }
  async function confirm() {
    if (!review || blocked || lock.current) return
    lock.current = true; setChecking(true); setFailure(undefined); setFailureStage(undefined)
    const captured = review, ticket = scope.capture(); let attemptID: string | undefined
    try {
      const latest = await current(); ticket.assert()
      if (judgingSetupVersion(latest) !== judgingSetupVersion(captured.source)) { setNeedsCurrent(true); throw new APIError(409, 'revision_conflict') }
      const revision = captured.kind === 'create' ? captured.source.configuration.revision : captured.source.round!.revision
      const body: JudgeAssignmentCommand | undefined = captured.kind === 'assign' ? { request_id: crypto.randomUUID(), action: 'assign', team_id: captured.input.teamID, judge_ids: [...captured.input.judgeIDs], reason: captured.input.reason.trim() } : undefined
      const fingerprint = body ? await assignmentFingerprint(captured.source.run!.id, revision, body) : undefined
      ticket.assert()
      attemptID = commands.begin({ operation: captured.kind === 'create' ? 'Create final judging round' : 'Assign final judges', revision, command: { action: captured.kind === 'create' ? 'create_run' : 'assign', intentID: crypto.randomUUID(), requestID: body?.request_id, resourceID: captured.source.run?.id ?? captured.source.configuration.reference.id, subjectID: body?.team_id, intentFingerprint: fingerprint } })
      if (captured.kind === 'create') {
        const result = unwrap(await api.POST('/api/v1/events/{event_id}/rubric/configuration', { params: { path: { event_id: source.event.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${revision}"` } }, body: { action: 'create_run', purpose: 'judge_final', version: captured.source.approved!.reference.version, reason: captured.input.reason.trim() } }))
        if (!createdFinalRound(captured.source, result, captured.source.approved!.reference)) throw new JudgingSetupReceiptIssue('The original response did not acknowledge the exact reviewed final round.')
      } else {
        const receipt = unwrap(await api.POST('/api/v1/events/{event_id}/judging/{run_id}', { params: { path: { event_id: source.event.id, run_id: captured.source.run!.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${revision}"` } }, body: body! }))
        const intent: AssignmentIntent = { requestID: body!.request_id, runID: captured.source.run!.id, teamID: body!.team_id, revision, fingerprint: fingerprint! }
        if (!matchesAssignmentReceipt(receipt, intent, identity.person.id)) throw new JudgingSetupReceiptIssue('The original response did not match the reviewed assignment request.')
      }
      commands.complete(attemptID); ticket.assert()
      setInput(emptyJudgingSetupInput()); setReview(undefined)
      acknowledged(captured.kind === 'create' ? 'Final round created from the reviewed rubric. Next: assign judges to each frozen team.' : 'The reviewed judge assignment was acknowledged. Review current coverage, then choose another team if needed.')
      await refresh()
    } catch (error) {
      if (attemptID) {
        if (error instanceof APIError && error.status >= 400 && error.status < 500 && error.status !== 408) commands.complete(attemptID)
        else commands.uncertain(attemptID)
      }
      if (ticket.current()) {
        if (denied(error)) clearDenied()
        else { setFailure(error); setFailureStage(attemptID ? 'sent' : 'before'); if (attemptID) setReview(undefined); if (error instanceof APIError && error.status === 409) setNeedsCurrent(true) }
      }
    } finally { lock.current = false; if (ticket.current()) setChecking(false) }
  }
  async function readCurrent() {
    if (busy || lock.current) return
    lock.current = true; setChecking(true); const ticket = scope.capture()
    try { await refresh(); if (ticket.current()) { setNeedsCurrent(false); setFailure(undefined); setReview(undefined) } }
    finally { lock.current = false; if (ticket.current()) setChecking(false) }
  }
  async function recover(attempt: Commands['attempts'][number]) {
    const metadata = attempt.command
    if (busy || lock.current || metadata.action !== 'assign' || !metadata.requestID || !metadata.resourceID || !metadata.subjectID || !metadata.intentFingerprint) return
    lock.current = true; setChecking(true); const ticket = scope.capture(); setFailure(undefined)
    try {
      const result = unwrap(await api.GET('/api/v1/events/{event_id}/judging-receipts/{request_id}', { signal: ticket.signal, cache: 'no-store', params: { path: { event_id: source.event.id, request_id: metadata.requestID } } }))
      ticket.assert()
      if (!result.receipt || !matchesAssignmentReceipt(result.receipt, { requestID: metadata.requestID, runID: metadata.resourceID, teamID: metadata.subjectID, revision: attempt.revision, fingerprint: metadata.intentFingerprint }, identity.person.id)) throw new JudgingSetupReceiptIssue('No matching original assignment acknowledgement is available. Its outcome remains unknown.')
      commands.complete(attempt.id); acknowledged('The exact original assignment acknowledgement was recovered. No write was replayed.'); await refresh()
    } catch (error) { if (ticket.current()) { if (denied(error)) clearDenied(); else setFailure(error) } }
    finally { lock.current = false; if (ticket.current()) setChecking(false) }
  }
  async function reviewFresh() {
    if (busy || lock.current) return
    lock.current = true; setChecking(true); const ticket = scope.capture(); setFailure(undefined)
    try { const value = await current(); ticket.assert(); setFresh(value) }
    catch (error) { if (ticket.current()) setFailure(error) }
    finally { lock.current = false; if (ticket.current()) setChecking(false) }
  }
  async function acceptFresh() {
    if (!fresh || busy || lock.current) return
    lock.current = true; setChecking(true); const ticket = scope.capture()
    try {
      const latest = await current(); ticket.assert()
      if (judgingSetupVersion(latest) !== judgingSetupVersion(fresh)) throw new Error('The setup changed again. Cancel and review its current state.')
      commands.selectNewBase(); setInput(emptyJudgingSetupInput()); setFresh(undefined); await refresh()
    } catch (error) { if (ticket.current()) setFailure(error) }
    finally { lock.current = false; if (ticket.current()) setChecking(false) }
  }
  if (denial) return <p>Current organiser authority ended. Private setup inputs have been cleared.</p>
  return <><h3 ref={heading} tabIndex={-1}>Plan and review final judging</h3><Button icon="refresh" variant="outline" disabled={busy} onClick={() => void readCurrent()}>Read current judging setup</Button>{closed && <p>The competition is closed. Final judging setup is read-only.</p>}
    <JudgingSetupCurrent source={source} />
    {needsCurrent && <p role="status">The starting point changed. Read current judging setup, then review your retained inputs again.</p>}
    {!closed && <div hidden={!!review || !!fresh}><JudgingSetupFields source={source} input={input} change={setInput} errors={errors} disabled={blocked} review={begin} form={form} /></div>}
    {!!failure && !review && (failure instanceof JudgingSetupReceiptIssue ? <p role="alert">{failure.message}</p> : <Failure error={failure} />)}
    {commands.unconfirmed && <><p role="status">The original change outcome is unknown. A current read does not confirm it, and no write is replayed automatically.</p>{commands.attempts.filter(attempt => attempt.state === 'unknown' && attempt.command.action === 'assign').map(attempt => <Button key={attempt.id} icon="refresh" variant="outline" disabled={busy} onClick={() => void recover(attempt)}>Read original assignment acknowledgement</Button>)}<Button icon="next" variant="outline" disabled={busy} onClick={event => { opener.current = event.currentTarget; void reviewFresh() }}>Review current setup as a new starting point</Button></>}
    {review && <JudgingSetupDecision value={review} pending={busy} disabled={paused || commands.unconfirmed || needsCurrent} confirm={() => void confirm()} cancel={() => setReview(undefined)} returnFocus={() => opener.current?.isConnected && !opener.current.matches(':disabled') ? opener.current : heading.current} error={failureStage === 'before' ? 'Current setup could not be confirmed. Nothing was sent. Read current facts or retry this review.' : undefined} />}
    {fresh && <ConfirmationDialog open title="Review current judging starting point" description="Discard unsent fields and use this current setup for another deliberate review. Earlier unknown requests remain unknown; nothing is replayed." confirmLabel="Use reviewed current setup" pending={busy} disabled={paused} onConfirm={() => void acceptFresh()} onOpenChange={open => { if (!open) setFresh(undefined) }} finalFocus={() => opener.current?.isConnected && !opener.current.matches(':disabled') ? opener.current : heading.current} printExcluded><p>{fresh.event.name} · Rubric revision {fresh.configuration.revision} · {fresh.run ? `Final round rubric version ${fresh.run.reference.version}` : 'No current final round'}</p><JudgingSetupCurrent source={fresh} />{!!failure && <Failure error={failure} />}</ConfirmationDialog>}
  </>
}

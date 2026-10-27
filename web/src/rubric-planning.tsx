import { useCallback, useEffect, useRef, useState } from 'react'
import { useBlocker } from '@tanstack/react-router'
import { api, APIError, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { useEventReadBoundary } from './event-authority'
import { usePrivateRequestScope } from './private-request-scope'
import { useTodayRead } from './today-read'
import { useAdmissionCommand, AdmissionFeedback } from './registration-command'
import { CollectionHeading } from './components/patterns'
import { Button } from './components/ui/button'
import { Field, Textarea } from './components/ui/fields'
import { ConfirmationDialog } from './components/ui/dialog'
import { Failure, Loading } from './components/states'
import { RubricPlanningFields, validateRubricPlanningForm, revealRubricInvalidField, type RubricConfiguration, type RubricDefinition } from './rubric-planning-fields'
import { RubricPlanningReview } from './rubric-planning-review'
import { rubricDefinitionKey, rubricReferenceKey } from './rubric-planning-model'
import { usePlanningDepartureBarrier } from './planning-operations'

export function RubricPlanning({ event, onDirty }: { event: EventRecord; onDirty: (dirty: boolean) => void }) {
  const identity = useIdentity(), boundary = useEventReadBoundary(event.id)
  return <RubricLifetime key={`${identity.person.id}:${identity.draft_scope}:${event.id}:${boundary?.generation ?? 0}`} event={event} onDirty={onDirty} />
}
function RubricLifetime({ event, onDirty }: { event: EventRecord; onDirty: (dirty: boolean) => void }) {
  const identity = useIdentity(), boundary = useEventReadBoundary(event.id)
  const owner = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${event.id}:rubric`, boundary?.scope)
  const [denial, setDenial] = useState<unknown>()
  const load = useCallback(async (signal: AbortSignal) => unwrap(await api.GET('/api/v1/events/{event_id}/rubric/configuration', { signal, cache: 'no-store', params: { path: { event_id: event.id } } })), [event.id])
  const read = useTodayRead(`${identity.person.id}:${identity.draft_scope}:${event.id}:rubric`, load, owner)
  if (denial || read.denied) return <Failure error={denial ?? read.error} />
  if (!read.data) return read.error ? <Failure error={read.error} retry={() => void read.refresh()} /> : <Loading />
  return <section className="fleet-print-excluded"><CollectionHeading title="Judging criteria" scope={{ event: event.name, entity: 'Private rubric planning' }} description="Edit the common criteria, review a private draft, then approve its exact version before configuring challenge publication." />
    {!!read.error && <Failure error={read.error} retry={() => void read.refresh()} />}
    <RubricEditor event={event} initial={read.data} observed={read.data} paused={!event.can_edit || !!boundary?.paused || read.pending || !!read.error} parent={read.authority} onDirty={onDirty} deny={error => { owner.invalidate(); setDenial(error) }} />
  </section>
}
type Decision = { action: 'save' | 'approve'; configuration: RubricConfiguration; definition: RubricDefinition; reason: string }
function RubricEditor({ event, initial, observed, paused, parent, onDirty, deny }: { event: EventRecord; initial: RubricConfiguration; observed: RubricConfiguration; paused: boolean; parent: ReturnType<typeof usePrivateRequestScope>; onDirty: (dirty: boolean) => void; deny: (error: unknown) => void }) {
  const identity = useIdentity(), boundary = useEventReadBoundary(event.id)
  const scope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${event.id}:rubric-editor`, parent)
  const command = useAdmissionCommand(`${identity.person.id}:${identity.draft_scope}:${event.id}:rubric-command`)
  const [saved, setSaved] = useState(initial), [definition, setDefinition] = useState(() => structuredClone(initial.definition))
  const [reason, setReason] = useState(''), [review, setReview] = useState<Decision>(), [notice, setNotice] = useState('')
  const [checking, setChecking] = useState(false), [preflightError, setPreflightError] = useState<unknown>(), [needsCurrent, setNeedsCurrent] = useState(false)
  const preflightLock = useRef(false)
  const form = useRef<HTMLFormElement>(null), saveButton = useRef<HTMLButtonElement>(null), approveButton = useRef<HTMLButtonElement>(null)
  const reasonField = useRef<HTMLTextAreaElement>(null), versionStatus = useRef<HTMLParagraphElement>(null), returnFocus = useRef<HTMLElement | null>(null)
  const dirty = rubricDefinitionKey(definition) !== rubricDefinitionKey(saved.definition), fieldsDisabled = command.blocked || checking, blocked = paused || fieldsDisabled
  const drifted = observed.revision > saved.revision
  const saveReady = !drifted && !needsCurrent && (dirty || saved.reference.version === 0)
  const approvalReady = !drifted && !needsCurrent && !dirty && saved.reference.version > 0 && !saved.approved
  useEffect(() => { onDirty(dirty || !!reason || !!review || command.pending || checking); return () => onDirty(false) }, [dirty, reason, review, command.pending, checking, onDirty])
  useEffect(() => { if (command.failure instanceof APIError && [401, 403, 404].includes(command.failure.status)) deny(command.failure) }, [command.failure, deny])
  usePlanningDepartureBarrier(checking)
  useBlocker({ shouldBlockFn: () => command.pending || checking || (dirty || !!reason || !!review) && !window.confirm('Leave with unsaved rubric planning?'), enableBeforeUnload: dirty || !!reason || command.pending || checking })

  async function current() {
    const ticket = scope.capture(); ticket.assert()
    try {
    const [configuration, authority] = await Promise.all([
      api.GET('/api/v1/events/{event_id}/rubric/configuration', { signal: ticket.signal, cache: 'no-store', params: { path: { event_id: event.id } } }).then(unwrap),
      api.GET('/api/v1/events/{event_id}', { signal: ticket.signal, cache: 'no-store', params: { path: { event_id: event.id } } }).then(unwrap),
    ])
    ticket.assert()
    if (authority.id !== event.id || !authority.can_edit) { const error = new APIError(403, 'forbidden'); deny(error); throw error }
    if (boundary && !boundary.observeToday(authority)) throw new APIError(409, 'revision_conflict')
    return configuration
    } catch (error) { if (error instanceof APIError && [401, 403, 404].includes(error.status)) deny(error); throw error }
  }
  async function readCurrent() {
    const ticket = scope.capture(), value = await current(); ticket.assert()
    setSaved(value); setReview(undefined); setNeedsCurrent(false); setPreflightError(undefined)
    setNotice('Current rubric read. Your local definition is retained for another review. This read does not acknowledge an unknown command.')
  }
  function begin(action: Decision['action']) {
    if (blocked || drifted || needsCurrent || !validateRubricPlanningForm(form.current)) return
    returnFocus.current = action === 'save' ? saveButton.current : approveButton.current
    setReview({ action, configuration: structuredClone(saved), definition: structuredClone(action === 'save' ? definition : saved.definition), reason: reason.trim() })
  }
  async function confirm() {
    if (!review || blocked || preflightLock.current) return
    const captured = review, ticket = scope.capture()
    preflightLock.current = true; setChecking(true); setPreflightError(undefined)
    try {
      const latest = await current(); ticket.assert()
      if (latest.revision !== captured.configuration.revision || rubricReferenceKey(latest.reference) !== rubricReferenceKey(captured.configuration.reference) || rubricDefinitionKey(latest.definition) !== rubricDefinitionKey(captured.configuration.definition)) { setNeedsCurrent(true); setReview(undefined); return }
    } catch (error) {
      if (ticket.current()) { setPreflightError(error); if (error instanceof APIError && error.status === 409) { setNeedsCurrent(true); setReview(undefined) } }
      return
    } finally { preflightLock.current = false; if (ticket.current()) setChecking(false) }
    ticket.assert()
    await command.run({ label: captured.action === 'save' ? 'Save rubric draft' : 'Approve rubric version', resource: `${event.id}/rubric`, revision: captured.configuration.revision }, async () => {
      ticket.assert()
      const result = unwrap(await api.POST('/api/v1/events/{event_id}/rubric/configuration', { signal: ticket.signal, params: { path: { event_id: event.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${captured.configuration.revision}"` } }, body: captured.action === 'save' ? { action: 'save', definition: captured.definition, reason: captured.reason } : { action: 'approve', version: captured.configuration.reference.version, reason: captured.reason } }))
      ticket.assert()
      if (result.revision !== captured.configuration.revision + 1 || captured.action === 'save' && (result.reference.version !== captured.configuration.reference.version + 1 || rubricDefinitionKey(result.definition) !== rubricDefinitionKey(captured.definition)) || captured.action === 'approve' && (!result.approved || rubricReferenceKey(result.reference) !== rubricReferenceKey(captured.configuration.reference))) throw new Error('The rubric response did not match the reviewed decision')
      return result
    }, result => { ticket.assert(); returnFocus.current = captured.action === 'save' ? reasonField.current : versionStatus.current; setSaved(result); setDefinition(structuredClone(result.definition)); setReason(''); setReview(undefined); return `${captured.action === 'save' ? 'Private draft saved' : 'Exact rubric version approved'} · Version ${result.reference.version} · Revision ${result.revision}. Criteria have not been released.` })
    if (ticket.current()) setReview(undefined)
  }
  return <><p ref={versionStatus} tabIndex={-1}>Saved version {saved.reference.version || 'not created'} · Revision {saved.revision} · {saved.approved ? 'Approved' : 'Not approved'}{saved.published_version !== null && ` · Published version ${saved.published_version}`}</p>
    {saved.runs.some(run => !!run.started_at) && <p role="status">Assessment has started. Existing runs retain their exact version. New criteria cannot silently reweight submitted scores; run restart and publication safeguards remain enforced by the server.</p>}
    {(drifted || needsCurrent) && <div role="status"><h3>Current version needed</h3><p>A newer rubric revision is available. Read and review it before another decision.</p><Button icon="refresh" variant="outline" disabled={command.pending} onClick={() => void command.readCurrent(readCurrent)}>Read current rubric version</Button></div>}
    <form ref={form} onSubmit={event => { event.preventDefault(); begin('save') }} onInvalidCapture={event => revealRubricInvalidField(event.target as HTMLElement)}>
      <RubricPlanningFields value={definition} change={setDefinition} disabled={fieldsDisabled} />
      <div className="grid gap-4 border-t border-border py-5"><Field label="Rubric decision reason" error={reason && !reason.trim() ? 'Enter a decision reason.' : undefined} description="Recorded with the reviewed save or approval."><Textarea ref={reasonField} value={reason} required maxLength={2000} disabled={fieldsDisabled} onChange={event => setReason(event.target.value)} /></Field><div className="fleet-pattern-actions"><Button ref={saveButton} focusableWhenDisabled={paused && !fieldsDisabled && saveReady} type="submit" icon="next" disabled={blocked || !saveReady}>Review draft save</Button><Button ref={approveButton} focusableWhenDisabled={paused && !fieldsDisabled && approvalReady} type="button" icon="shield" variant="outline" disabled={blocked || !approvalReady} onClick={() => begin('approve')}>Review version approval</Button></div></div>
    </form>
    <AdmissionFeedback command={command} read={readCurrent} />{notice && <p role="status">{notice}</p>}
    <ConfirmationDialog open={!!review} title={review?.action === 'approve' ? 'Approve exact rubric version' : 'Review private rubric draft'} description="Review the captured criteria and reason. This decision does not publish the challenge or start scoring." confirmLabel={review?.action === 'approve' ? 'Approve reviewed version' : 'Save reviewed draft'} pending={command.pending || checking} disabled={paused || command.blocked || needsCurrent} finalFocus={() => returnFocus.current?.isConnected && !returnFocus.current.matches(':disabled') ? returnFocus.current : versionStatus.current} onOpenChange={open => { if (!open) setReview(undefined) }} onConfirm={() => void confirm()} printExcluded>{!!preflightError && <Failure error={preflightError} />}{review && <RubricPlanningReview definition={review.definition} revision={review.configuration.revision} version={review.configuration.reference.version} reason={review.reason} action={review.action} />}</ConfirmationDialog>
  </>
}

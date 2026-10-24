import { useCallback, useEffect, useRef, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { api, APIError, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { SurveyDecisionControls } from './survey-decision'
import { Field, Textarea, Checkbox, Label } from './components/ui/fields'
import { Failure, Loading } from './components/states'
import { localDate } from './event-time'
import { CollectionHeading, CollectionFrame, CollectionRecords, CollectionToolbar, CollectionPagination, type CollectionState } from './components/patterns'
import { useSurveyPrivatePages, useSurveyPrivateRead } from './survey-private-read'
import { useResourceReturnFocus } from './resource-focus'
import { usePrivateRequestScope } from './private-request-scope'
import { usePlanningDepartureBarrier } from './planning-operations'
import { CommunicationCommandHistory, useCommunicationCommands } from './communication-tests'
import { SurveyResponseExport } from './survey-export'
import { SurveyReportView } from './survey-results'
import { answerText, nativeDenied, unknownOutcome, useSurveyLeaveWarning, type SurveyConfiguration } from './survey-context'

type ReportReview = components['schemas']['SurveyReviewedReport']
type ReportCommand = components['schemas']['SurveyReportCommand']
type ReportIntent = { command: ReportCommand; revision: number; intentID: string; metadataID?: string }
export function SurveyReportReview({ eventId, configuration, timezone, writable, onDirty, deny }: { eventId: string; configuration: SurveyConfiguration; timezone: string; writable: boolean; onDirty: (dirty: boolean) => void; deny: (error: unknown) => void }) {
  const identity = useIdentity(), id = configuration.id, key = `${identity.person.id}:${identity.draft_scope}:${eventId}:${id}:report-preview`
  const load = useCallback(async (signal: AbortSignal) => {
    try { return unwrap(await api.GET('/api/v1/events/{event_id}/surveys/{survey_id}/report/review', { signal, cache: 'no-store', params: { path: { event_id: eventId, survey_id: id } } })) }
    catch (error) { if (!(error instanceof APIError) || error.status !== 404) throw error
      // A missing first review is not proof of access. Recheck native survey
      // configuration authority before treating that 404 as an empty review.
      unwrap(await api.GET('/api/v1/events/{event_id}/surveys/{survey_id}/configuration', { signal, cache: 'no-store', params: { path: { event_id: eventId, survey_id: id } } })); return null
    }
  }, [eventId, id])
  const query = useSurveyPrivateRead(key, load)
  useEffect(() => { if (query.denied) deny(query.error) }, [query.denied, query.error, deny])
  const latest = query.data ?? undefined
  return <section className="fleet-survey-section"><CollectionHeading title="Frozen aggregates and explicitly reviewed text" scope={{ entity: 'Restricted report review' }} description="Wait until close. The first deliberate text review freezes one suppressed cohort for this question version. Prose corrections retain that aggregate; raw answers never become published prose automatically." />
    {!!query.error && query.data !== undefined && <p role="alert">Current review authority could not be refreshed. Local text and captured intent remain in this page; writes are paused.</p>}
    {query.data === undefined ? query.error ? <Failure error={query.error} retry={() => { void query.read(true).catch(() => {}) }} /> : <Loading /> : <>
      {latest && <details className="fleet-survey-section"><summary>Inspect the exact retained frozen report and reviewed prose</summary><p>Organiser preview. Wider release exists only after explicit publication.</p><SurveyReportView report={{ ...latest.report, reviewed_text_version: latest.published_at ? latest.version : 0, published_at: latest.published_at }} timezone={timezone} /></details>}
      <ReportEditor key={`${id}:${writable}`} eventId={eventId} configuration={configuration} initial={latest} writable={writable} networkBlocked={!!query.error || query.pending} receive={value => query.accept(value)} refresh={async () => (await query.read(true)) ?? undefined} onDirty={onDirty} deny={deny} />
    </>}
  </section>
}
function ReportEditor({ eventId, configuration, initial, writable, networkBlocked, receive, refresh, onDirty, deny }: { eventId: string; configuration: SurveyConfiguration; initial?: ReportReview; writable: boolean; networkBlocked: boolean; receive: (value: ReportReview) => void; refresh: () => Promise<ReportReview | undefined>; onDirty: (value: boolean) => void; deny: (error: unknown) => void }) {
  const identity = useIdentity(), owner = `${identity.person.id}:${identity.draft_scope}:${eventId}:${configuration.id}:report-command`, scope = usePrivateRequestScope(owner), history = useCommunicationCommands(owner), lock = useRef(false), opener = useRef<HTMLButtonElement>(null)
  const [saved, setSaved] = useState(initial), [summary, setSummary] = useState(initial?.report.reviewed_summary ?? ''), [reviewed, setReviewed] = useState(false), [pending, setPending] = useState(false), [reading, setReading] = useState(false), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState(''), [review, setReview] = useState<ReportIntent>(), [uncertain, setUncertain] = useState<ReportIntent>(), [conflict, setConflict] = useState(false), [currentRead, setCurrentRead] = useState(false)
  const dirty = summary !== (saved?.report.reviewed_summary ?? ''), changedElsewhere = (initial?.version ?? 0) !== (saved?.version ?? 0) || initial?.review_id !== saved?.review_id
  useEffect(() => { onDirty(dirty || !!review || !!uncertain || pending || history.unconfirmed); return () => onDirty(false) }, [dirty, review, uncertain, pending, history.unconfirmed, onDirty])
  useSurveyLeaveWarning(dirty || !!review || !!uncertain, pending || history.pending)
  usePlanningDepartureBarrier(pending || history.pending)
  const blocked = !writable || networkBlocked || pending || reading || history.pending || history.unconfirmed || !!uncertain || conflict || changedElsewhere
  function adopt(value?: ReportReview) { setSaved(value); setSummary(value?.report.reviewed_summary ?? ''); setReviewed(false); setUncertain(undefined); setReview(undefined); setConflict(false); setCurrentRead(false) }
  function prepare(action: ReportCommand['action'], element: HTMLButtonElement) {
    if (blocked || action === 'review_text' && !reviewed) return
    opener.current = element
    setReview({ command: { action, review_id: action === 'review_text' ? crypto.randomUUID() : saved!.review_id, summary: action === 'review_text' ? summary.trim() : '', reviewed_text: action === 'review_text' }, revision: saved?.version ?? 0, intentID: crypto.randomUUID() })
  }
  async function act(intent: ReportIntent, retry = false) {
    if (lock.current || networkBlocked || !writable || history.pending || !retry && (history.unconfirmed || conflict || changedElsewhere)) return
    lock.current = true; setPending(true); setFailure(undefined); setNotice('')
    const ticket = scope.capture(); let metadataID: string | undefined, issued = false
    try {
      ticket.assert()
      if (retry) { if (intent.command.action !== 'review_text') return; await refresh(); ticket.assert() }
      metadataID = history.begin({ operation: `Survey report ${intent.command.action}`, revision: intent.revision, command: { action: intent.command.action, intentID: intent.intentID, requestID: intent.command.review_id, revision: intent.revision } }, intent.metadataID)
      issued = true
      const value = unwrap(await api.PUT('/api/v1/events/{event_id}/surveys/{survey_id}/report/review', { params: { path: { event_id: eventId, survey_id: configuration.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${intent.revision}"` } }, body: intent.command }))
      if (value.review_id !== intent.command.review_id || value.version !== intent.revision + (intent.command.action === 'review_text' ? 1 : 0) || value.report.survey_id !== configuration.id || value.report.survey_version !== configuration.version || intent.command.action === 'review_text' && (value.reviewer_id !== identity.person.id || value.report.reviewed_summary !== intent.command.summary) || intent.command.action === 'approve' && !value.approved_at || intent.command.action === 'publish' && !value.published_at) throw new Error('Unexpected native report response')
      history.complete(metadataID); if (!ticket.current()) return
      adopt(value); receive(value); setNotice(intent.command.action === 'review_text' ? `Reviewed summary version ${value.version} retained against the frozen aggregate. It is not yet published.` : intent.command.action === 'approve' ? 'This exact reviewed summary is approved, not yet published.' : 'This reviewed summary is published. Its frozen aggregate is unchanged.')
      // A retained retry can return an older review. Read latest separately;
      // never replace that exact response with a claim about the current head.
      await refresh().catch(() => {})
    } catch (error) {
      if (issued && metadataID) { if (retry || unknownOutcome(error)) history.uncertain(metadataID); else history.complete(metadataID) }
      if (!ticket.current()) return
      setReview(undefined); if (nativeDenied(error)) { deny(error); return }; setFailure(error)
      if (issued && (retry || unknownOutcome(error))) setUncertain({ ...intent, metadataID }); else if (issued && error instanceof APIError && error.status === 409) setConflict(true)
    } finally { lock.current = false; if (ticket.current()) setPending(false) }
  }
  async function readCurrent() {
    if (reading || pending || history.pending) return
    const ticket = scope.capture(); setReading(true); setFailure(undefined)
    try { await refresh(); ticket.assert(); setCurrentRead(true); setNotice('Current report facts read. They do not confirm the original approve, publish or text operation.') }
    catch (error) { if (ticket.current()) { if (nativeDenied(error)) deny(error); else setFailure(error) } }
    finally { if (ticket.current()) setReading(false) }
  }
  function loadCurrent() {
    if ((uncertain || history.unconfirmed || conflict) && !currentRead) return
    if ((dirty || uncertain || review) && !window.confirm('Discard local prose and its private retry payload, then use the reviewed current report? Earlier safe command metadata remains unconfirmed.')) return
    adopt(initial); history.selectNewBase(); setFailure(undefined); setNotice('Loaded current report facts as a new base. Earlier command outcomes remain unconfirmed.')
  }
  return <section className="fleet-survey-report-editor"><h3>Review the text for wider release</h3><p>Suppression is frozen by the server. Do not include identifiable responses, small-cell clues or unsupported adoption/ROI claims in wider prose.</p><CommunicationCommandHistory attempts={history.attempts} /><fieldset className="fleet-survey-fields" disabled={blocked || !!review}><legend>Explicit redaction review</legend><Field label="Redacted summary for wider release"><Textarea value={summary} maxLength={12000} onChange={event => { setSummary(event.target.value); setReviewed(false) }} /></Field><Label className="check"><Checkbox checked={reviewed} onChange={event => setReviewed(event.target.checked)} />I reviewed and redacted this wording for wider release.</Label></fieldset>
    <SurveyDecisionControls status={{ label: 'Survey report status', message: notice || (uncertain || history.unconfirmed ? 'The operation outcome is unknown. No write is replayed automatically.' : dirty ? 'Unsent reviewed prose.' : 'Current retained review.') }} actions={[
      { label: 'Review frozen cohort and summary', icon: 'check', disabled: blocked || !reviewed || !!saved?.report.suppressed && !!summary.trim(), onSelect: button => prepare('review_text', button) },
      { label: 'Review exact summary approval', icon: 'check', variant: 'outline', disabled: blocked || dirty || !saved || !!saved.approved_at, onSelect: button => prepare('approve', button) },
      { label: 'Review report publication', icon: 'send', variant: 'outline', disabled: blocked || dirty || !saved?.approved_at || !!saved.published_at, onSelect: button => prepare('publish', button) },
      ...(uncertain?.command.action === 'review_text' ? [{ label: 'Retry the same report text command', icon: 'refresh' as const, variant: 'outline' as const, disabled: !writable || pending || history.pending || networkBlocked, onSelect: () => { void act(uncertain, true) } }] : []),
      ...(uncertain || history.unconfirmed || conflict || changedElsewhere ? [{ label: 'Read current report facts', icon: 'refresh' as const, variant: 'outline' as const, disabled: pending || history.pending || reading, onSelect: () => { void readCurrent() } }] : []),
      ...(dirty || uncertain || history.unconfirmed || conflict || changedElsewhere ? [{ label: 'Load current report review', variant: 'outline' as const, disabled: pending || history.pending || reading || (history.unconfirmed || !!uncertain || conflict) && !currentRead, onSelect: loadCurrent }] : []),
    ]} review={{ open: !!review, onOpenChange: open => { if (!open && !pending) setReview(undefined) }, title: 'Review exact report action', description: review?.command.action === 'review_text' ? 'This explicit text review freezes one native suppressed cohort on the first review. The exact reviewed prose below is retained privately until separately approved and published.' : review?.command.action === 'approve' ? 'Approve only this retained reviewed-text identity and version. This does not publish it.' : 'Publish only this exact approved reviewed-text identity and its already frozen suppressed aggregate.', confirmLabel: review?.command.action === 'review_text' ? 'Freeze cohort and save reviewed summary' : review?.command.action === 'approve' ? 'Approve exact reviewed summary' : 'Publish reviewed report', pending, disabled: !writable || networkBlocked || history.pending || history.unconfirmed, finalFocus: opener, onConfirm: () => { if (review) void act(review) }, children: <><p>Expected review version {review?.revision} · Review identity {review?.command.review_id}</p><p className="fleet-survey-reviewed-prose">{review?.command.action === 'review_text' ? review.command.summary || 'No wider prose.' : saved?.report.reviewed_summary || 'No wider prose.'}</p>{saved?.report.suppressed && <p>The frozen cohort is suppressed; no wider prose or hidden counts can be added.</p>}</> }}>
      {saved?.published_at && <Link to="/events/$eventId/surveys/$surveyId/results" params={{ eventId, surveyId: configuration.id }}>Open published report</Link>}{!!failure && <Failure error={failure} />}{changedElsewhere && <p role="alert">A different report review is current. Read and deliberately load it before another command.</p>}{currentRead && <p>Current facts are displayed. The earlier original outcome remains unconfirmed; adopting them permits only a separately reviewed new action.</p>}
    </SurveyDecisionControls>
  </section>
}
export function SurveyPrivateResponses({ eventId, configuration, timezone, deny }: { eventId: string; configuration: SurveyConfiguration; timezone: string; deny: (error: unknown) => void }) {
  const identity = useIdentity(), [selected, setSelected] = useState<string>(), root = useRef<HTMLElement>(null), detailHeading = useRef<HTMLHeadingElement>(null), focus = useResourceReturnFocus(root)
  const key = `${identity.person.id}:${identity.draft_scope}:${eventId}:${configuration.id}:authored-responses`
  const load = useCallback(async (signal: AbortSignal, after?: string) => unwrap(await api.GET('/api/v1/events/{event_id}/surveys/{survey_id}/responses', { signal, cache: 'no-store', params: { path: { event_id: eventId, survey_id: configuration.id }, query: { after, limit: 20 } } })), [eventId, configuration.id])
  const next = useCallback((page: components['schemas']['SurveyResponsePage']) => page.next_person ?? undefined, [])
  const query = useSurveyPrivatePages(key, load, next)
  useEffect(() => { if (query.denied) deny(query.error) }, [query.denied, query.error, deny])
  useEffect(() => { detailHeading.current?.focus() }, [selected])
  const items = query.data?.pages.flatMap(page => page.items) ?? [], current = items.find(item => item.person_id === selected)
  const state: CollectionState = query.denied ? { kind: 'denied', message: 'Current private response access is unavailable.' } : query.error ? { kind: query.data ? 'paused' : 'failed', message: 'Current response authority could not be checked.', read: { label: 'Check private response access', onSelect: () => { void query.refetch() } } } : query.isPending ? { kind: 'loading' } : items.length ? { kind: 'ready' } : { kind: 'empty', message: 'No retained responses. This is not a claim of zero learning or adoption.' }
  return <section ref={root} className="fleet-survey-section survey-private"><CollectionHeading title="Restricted authored responses" scope={{ entity: 'Current organisers only' }} description="Identifiable responses stay separate from the wider report. Review and redact authored text deliberately; no answer is published automatically." />
    <SurveyResponseExport eventId={eventId} surveyId={configuration.id} authority={query.authority} disabled={!!query.error || query.pending} deny={deny} />
    {current && !query.denied ? <section className="fleet-survey-inspector" aria-label="Restricted response detail"><Button icon="back" variant="ghost" onClick={() => { setSelected(undefined); focus.restore() }}>Back to responses</Button><h3 tabIndex={-1} ref={detailHeading}>{current.person_name}</h3><p>Response revision {current.response.revision} · Question version {current.response.form_version} · {localDate(current.response.submitted_at, timezone)}</p><ResponseAnswers configuration={configuration} answers={current.response.answers} /><ResponseHistory key={`${key}:${current.person_id}`} eventId={eventId} configuration={configuration} personId={current.person_id} timezone={timezone} deny={deny} /></section> : <CollectionFrame scopeKey={key} state={state} heading={null} toolbar={<CollectionToolbar source="cursor" summary="Only current native-authorised response heads are shown. Open one respondent to inspect their answers and retained revisions." />}>
      <CollectionRecords label="Restricted response directory" rows={items} rowKey={item => item.person_id} rowLabel={item => item.person_name} columns={[{ id: 'revision', label: 'Response revision', cell: item => item.response.revision }, { id: 'form', label: 'Questions', cell: item => `Version ${item.response.form_version}` }, { id: 'submitted', label: 'Submitted', cell: item => localDate(item.response.submitted_at, timezone) }]} actions={item => <Button data-record-focus={item.person_id} icon="list" variant="outline" onClick={event => { focus.capture(event.currentTarget); setSelected(item.person_id) }}>Review response from {item.person_name}</Button>} />
      <CollectionPagination label="Restricted response pages" source={{ kind: 'cursor', loaded: items.length, busy: query.isFetchingNextPage, next: query.hasNextPage ? () => void query.fetchNextPage() : undefined }} />
    </CollectionFrame>}
  </section>
}
function ResponseHistory({ eventId, configuration, personId, timezone, deny }: { eventId: string; configuration: SurveyConfiguration; personId: string; timezone: string; deny: (error: unknown) => void }) {
  const identity = useIdentity(), key = `${identity.person.id}:${identity.draft_scope}:${eventId}:${configuration.id}:${personId}:response-history`
  const load = useCallback(async (signal: AbortSignal, before_revision?: number) => unwrap(await api.GET('/api/v1/events/{event_id}/surveys/{survey_id}/responses/{person_id}/history', { signal, cache: 'no-store', params: { path: { event_id: eventId, survey_id: configuration.id, person_id: personId }, query: { before_revision, limit: 20 } } })), [eventId, configuration.id, personId])
  const next = useCallback((page: components['schemas']['SurveyResponse'][]) => page.length === 20 ? page.at(-1)?.revision : undefined, [])
  const query = useSurveyPrivatePages(key, load, next)
  useEffect(() => { if (query.denied) deny(query.error) }, [query.denied, query.error, deny])
  const revisions = query.data?.pages.flatMap(page => page) ?? []
  return <section className="fleet-survey-section" aria-label="Retained response history"><h3>Retained response history</h3>{query.error ? <Failure error={query.error} retry={() => void query.refetch()} /> : query.isPending ? <Loading /> : <><ol className="fleet-survey-history">{revisions.map(response => <ResponseRevision key={response.revision} response={response} configuration={configuration} timezone={timezone} />)}</ol>{!revisions.length && <p>No retained response revisions are available.</p>}<CollectionPagination label="Retained response revisions" source={{ kind: 'cursor', loaded: revisions.length, busy: query.isFetchingNextPage, next: query.hasNextPage ? () => void query.fetchNextPage() : undefined }} /></>}</section>
}
function ResponseRevision({ response, configuration, timezone }: { response: components['schemas']['SurveyResponse']; configuration: SurveyConfiguration; timezone: string }) {
  const [open, setOpen] = useState(false)
  return <li><details open={open} onToggle={event => setOpen(event.currentTarget.open)}><summary>Revision {response.revision} · {localDate(response.submitted_at, timezone)} · Question version {response.form_version}</summary>{open && <ResponseAnswers configuration={configuration} answers={response.answers} />}</details></li>
}
function ResponseAnswers({ configuration, answers }: { configuration: SurveyConfiguration; answers: Record<string, unknown> }) {
  return <dl className="fleet-survey-answer-review">{configuration.definition.form.fields.map(field => <div key={field.id}><dt>{field.label}</dt><dd>{answerText(answers[field.id])}</dd></div>)}</dl>
}

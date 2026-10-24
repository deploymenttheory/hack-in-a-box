import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useParams } from '@tanstack/react-router'
import { api, APIError, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Failure, Loading } from './components/states'
import { localDate } from './event-time'
import { SurveyDecisionControls, SurveyAnswerSummary } from './survey-decision'
import { CollectionHeading } from './components/patterns'
import { FormQuestions, visibleFields, type Answers } from './form-questions'
import { usePrivateRequestScope } from './private-request-scope'
import { CommunicationCommandHistory, useCommunicationCommands } from './communication-tests'
import { useSurveyPrivateRead } from './survey-private-read'
import { answerText, same, useSurveyLeaveWarning, unknownOutcome, nativeDenied, type ParticipantSurvey } from './survey-context'

export function SurveyResponsePage() {
  const { eventId, surveyId } = useParams({ strict: false }) as { eventId: string; surveyId: string }, identity = useIdentity()
  return <OwnSurvey key={`${identity.person.id}:${identity.draft_scope}:${eventId}:${surveyId}`} eventId={eventId} surveyId={surveyId} />
}
function OwnSurvey({ eventId, surveyId }: { eventId: string; surveyId: string }) {
  const identity = useIdentity(), owner = `${identity.person.id}:${identity.draft_scope}:${eventId}:${surveyId}:response`
  const history = useCommunicationCommands(owner)
  const load = useCallback(async (signal: AbortSignal) => unwrap(await api.GET('/api/v1/events/{event_id}/surveys/{survey_id}', { signal, cache: 'no-store', params: { path: { event_id: eventId, survey_id: surveyId } } })), [eventId, surveyId])
  const survey = useSurveyPrivateRead(owner, load), page = survey.denied ? undefined : survey.data
  async function refresh() { return survey.read(true) }
  async function currentDenial(error: unknown) {
    // A closed questionnaire may still permit the person's retained history.
    // A denied/currently unreadable owner must discard every private child.
    try { const current = await survey.read(); if (current.can_respond) survey.revoke(error) }
    catch { survey.revoke(error) }
  }
  return <section className="fleet-survey-workspace survey-private"><nav className="fleet-pattern-actions"><Link to="/inbox">Your inbox</Link><Link to="/events/$eventId/surveys" params={{ eventId }}>Your surveys and reports</Link></nav>
    <CollectionHeading title="Post-event survey" scope={{ entity: 'Your private response' }} description="Review the fixed questions and the exact response you choose to send." secondary={[{ label: 'Refresh survey', icon: 'refresh', onSelect: () => { void refresh().catch(() => {}) } }]} />
    <CommunicationCommandHistory attempts={history.attempts} />
    {survey.denied ? <Failure error={survey.error} retry={() => { void refresh().catch(() => {}) }} /> : !page ? survey.error ? <Failure error={survey.error} retry={() => { void refresh().catch(() => {}) }} /> : <Loading /> : <>
      {!!survey.error && <p role="alert">Current authority could not be refreshed. Local answers and the exact retry command remain in this page; writes are paused until a successful refresh.</p>}
      <header className="fleet-survey-section"><p className="fleet-scope-label">Fixed question version {page.version}</p><h2>{page.form.name}</h2><p role="note">{page.response_visibility}</p><p>Opens {localDate(page.opens_at, page.timezone)} · Closes {localDate(page.closes_at, page.timezone)} · {page.timezone}</p><p>{page.can_respond ? 'This survey is open. You can update your response until close. Questions marked required must be answered; other questions are optional.' : 'Response editing is currently closed or unavailable.'}</p></header>
      {page.response ? <section className="fleet-survey-section" aria-label="Your saved response"><h3>Your saved response: revision {page.response.revision}, submitted {localDate(page.response.submitted_at, page.timezone)}.</h3><AnswerReview page={page} answers={page.response.answers} /></section> : <p>You have not submitted a response.</p>}
      {page.can_respond ? <ResponseForm key={`${page.version}:respond`} owner={owner} page={page} refresh={refresh} networkBlocked={!!survey.error || survey.pending} eventId={eventId} surveyId={surveyId} deny={error => { void currentDenial(error) }} /> : <ol className="fleet-survey-questions">{page.form.fields.map(field => <li key={field.id}><h3>{field.label}</h3>{field.help && <p>{field.help}</p>}</li>)}</ol>}
      {page.measures.map(measure => <p key={measure.question_id}>Self-reported {measure.construct === 'self_reported_adoption' ? 'adoption' : 'learning'} · objective {measure.measure.objective.id} version {measure.measure.objective.version}. This does not independently evidence adoption.</p>)}
    </>}
  </section>
}
type ResponseCommand = components['schemas']['SurveyResponseCommand']
type Response = components['schemas']['SurveyResponse']
type Intent = { command: ResponseCommand; revision: number; metadataID?: string }
function ResponseForm({ page, owner, eventId, surveyId, refresh, networkBlocked, deny }: { page: ParticipantSurvey; owner: string; eventId: string; surveyId: string; refresh: () => Promise<ParticipantSurvey>; networkBlocked: boolean; deny: (error: unknown) => void }) {
  const identity = useIdentity(), scope = usePrivateRequestScope(`${owner}:editor`), history = useCommunicationCommands(owner), lock = useRef(false), opener = useRef<HTMLButtonElement>(null)
  const [saved, setSaved] = useState(page.response), [answers, setAnswers] = useState<Answers>(() => ({ ...page.response?.answers })), [pending, setPending] = useState(false), [uncertain, setUncertain] = useState<Intent>(), [review, setReview] = useState<Intent>(), [failure, setFailure] = useState<unknown>(), [receipt, setReceipt] = useState<{ commandID: string; response: Response }>(), [conflict, setConflict] = useState(false), [currentRead, setCurrentRead] = useState(false)
  const dirty = !same(answers, saved?.answers ?? {}), changedElsewhere = (page.response?.revision ?? 0) !== (saved?.revision ?? 0)
  useSurveyLeaveWarning(dirty || !!review || !!uncertain, pending || history.pending)
  function currentAnswers() { const ids = new Set(visibleFields(page.form, answers).map(field => field.id)); return Object.fromEntries(Object.entries(answers).filter(([id, value]) => ids.has(id) && value !== undefined)) }
  async function send(intent: Intent, retry = false) {
    if (lock.current || networkBlocked || history.pending || !retry && (history.unconfirmed || conflict || changedElsewhere)) return
    lock.current = true; setPending(true); setFailure(undefined); setReceipt(undefined)
    const ticket = scope.capture(); let metadataID: string | undefined, issued = false
    try {
      ticket.assert()
      if (retry) { const current = await refresh(); ticket.assert(); if (!current.can_respond || current.version !== intent.command.form_version) return }
      metadataID = history.begin({ operation: 'Survey response', revision: intent.revision, command: { action: 'respond', requestID: intent.command.command_id, revision: intent.revision } }, intent.metadataID)
      issued = true
      const response = unwrap(await api.PUT('/api/v1/events/{event_id}/surveys/{survey_id}/response', { params: { path: { event_id: eventId, survey_id: surveyId }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${intent.revision}"` } }, body: intent.command }))
      if (response.revision !== intent.revision + 1 || response.form_version !== intent.command.form_version || !same(response.answers, intent.command.answers) || !response.submitted_at) throw new Error('Unexpected response command acknowledgement')
      // Only the response to this original native command (including its exact
      // captured retry) can settle its safe metadata, even after private disposal.
      history.complete(metadataID)
      if (!ticket.current()) return
      setSaved(response); setAnswers({ ...response.answers }); setUncertain(undefined); setReview(undefined); setConflict(false); setCurrentRead(false); setReceipt({ commandID: intent.command.command_id, response })
      await refresh().catch(() => {})
    } catch (error) {
      if (issued && metadataID) { if (retry || unknownOutcome(error)) history.uncertain(metadataID); else history.complete(metadataID) }
      if (!ticket.current()) return
      setReview(undefined)
      if (nativeDenied(error)) { deny(error); return }
      setFailure(error)
      if (issued && (retry || unknownOutcome(error))) setUncertain({ ...intent, metadataID })
      else if (issued && error instanceof APIError && error.status === 409) { setConflict(true); await refresh().catch(() => {}) }
    } finally { lock.current = false; if (ticket.current()) setPending(false) }
  }
  async function checkCurrent() { try { await refresh(); if (scope.capture().current()) setCurrentRead(true) } catch (error) { if (scope.capture().current()) setFailure(error) } }
  function loadCurrent() {
    if ((dirty || uncertain) && !window.confirm('Discard local answers and the retained private retry payload, then use the reviewed current response? Earlier unknown command metadata remains unconfirmed.')) return
    if (!currentRead && (uncertain || history.unconfirmed || conflict)) return
    setSaved(page.response); setAnswers({ ...page.response?.answers }); setUncertain(undefined); setReview(undefined); setConflict(false); setFailure(undefined); setReceipt(undefined); setCurrentRead(false); history.selectNewBase()
  }
  const blocked = networkBlocked || pending || history.pending || history.unconfirmed || !!uncertain || conflict || changedElsewhere
  useEffect(() => { if (failure instanceof APIError && failure.status === 422) opener.current?.form?.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus() }, [failure])
  return <form className="fleet-survey-form" onSubmit={event => { event.preventDefault(); if (!blocked && dirty) setReview({ command: { command_id: crypto.randomUUID(), form_version: page.version, answers: structuredClone(currentAnswers()) }, revision: saved?.revision ?? 0 }) }}>
    <fieldset className="fleet-survey-fields" disabled={blocked || !!review}><legend>{saved ? 'Edit your response' : 'Your response'}</legend><FormQuestions definition={page.form} answers={answers} change={setAnswers} showOptional issues={failure instanceof APIError ? failure.blockers : []} /></fieldset>
    <SurveyDecisionControls status={{ label: 'Survey response status', message: receipt ? `Response command confirmed at revision ${receipt.response.revision}. Current saved response is read separately; the original command returned this exact retained revision.` : uncertain || history.unconfirmed ? 'The outcome is unknown. No new command will be sent automatically. You may explicitly retry the captured same command while the survey remains open, or review current state.' : dirty ? 'Unsent changes, kept only in this page.' : 'No unsent changes.' }} actions={[
      uncertain ? { label: 'Retry the same survey response', icon: 'refresh', disabled: pending || history.pending || networkBlocked, onSelect: () => { void send(uncertain, true) } } : { label: 'Review response', icon: 'check', ref: opener, submit: true, disabled: blocked || !dirty },
      { label: 'Check saved response', icon: 'refresh', variant: 'outline', disabled: pending || history.pending, onSelect: () => { void checkCurrent() } },
      ...(dirty || uncertain || conflict || changedElsewhere || history.unconfirmed ? [{ label: 'Load current saved response', variant: 'outline' as const, disabled: pending || history.pending || networkBlocked || (history.unconfirmed || !!uncertain || conflict) && !currentRead, onSelect: loadCurrent }] : []),
    ]} review={{ open: !!review, onOpenChange: open => { if (!open && !pending) setReview(undefined) }, title: 'Review your private response', description: 'Only you and current organisers can read these answers. This is not an anonymous response. Confirming sends exactly the visible answers below for this fixed question version.', confirmLabel: 'Save response', pending, disabled: networkBlocked || history.pending || history.unconfirmed, finalFocus: opener, onConfirm: () => { if (review) void send(review) }, children: <><p>Question version {review?.command.form_version} · Expected response revision {review?.revision}</p>{review && <AnswerReview page={page} answers={review.command.answers} />}</> }}>
      {receipt && <dl className="fleet-survey-facts"><div><dt>Confirmed response command</dt><dd>{receipt.commandID}</dd></div><div><dt>Exact returned response revision</dt><dd>{receipt.response.revision}</dd></div></dl>}{!!failure && <Failure error={failure} />}{(conflict || changedElsewhere) && <p role="alert">The saved response changed. Review the current saved response above before replacing your local draft.</p>}{currentRead && (uncertain || history.unconfirmed) && <p>The displayed saved response is current state. It does not confirm the original command. Selecting it as a new base keeps the earlier outcome unconfirmed.</p>}
    </SurveyDecisionControls>
  </form>
}
function AnswerReview({ page, answers }: { page: ParticipantSurvey; answers: Answers }) {
  return <SurveyAnswerSummary items={visibleFields(page.form, answers).map(field => ({ id: field.id, label: field.label, value: answerText(answers[field.id]) }))} />
}

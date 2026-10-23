import { useEffect, useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from '@tanstack/react-router'
import { api, APIError, privateKey, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Failure, Loading } from './components/states'
import { localDate } from './event-time'
import { Button } from './components/ui/button'
import { FormQuestions, visibleFields, type Answers } from './form-questions'
import { answerText, same, useActivePage, useSurveyLeaveWarning, unknownOutcome, nativeDenied, type ParticipantSurvey } from './survey-context'

export function SurveyResponsePage() {
  const { eventId, surveyId } = useParams({ strict: false }) as { eventId: string; surveyId: string }, identity = useIdentity()
  return <OwnSurvey key={`${identity.person.id}:${identity.draft_scope}:${eventId}:${surveyId}`} eventId={eventId} surveyId={surveyId} />
}
function OwnSurvey({ eventId, surveyId }: { eventId: string; surveyId: string }) {
  const identity = useIdentity(), cache = useQueryClient(), [denied, setDenied] = useState<unknown>()
  const key = useMemo(() => [...privateKey(identity.person.id, eventId, 'own-survey'), identity.draft_scope, surveyId], [identity.person.id, identity.draft_scope, eventId, surveyId])
  const survey = useQuery({ queryKey: key, gcTime: 0, staleTime: 0, retry: false, refetchOnWindowFocus: 'always', refetchInterval: 15_000,
    queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/surveys/{survey_id}', { params: { path: { event_id: eventId, survey_id: surveyId } } })) })
  useEffect(() => () => { cache.removeQueries({ queryKey: key }) }, [cache, key])
  useEffect(() => { if (nativeDenied(survey.error) || denied) cache.removeQueries({ queryKey: key }) }, [survey.error, denied, cache, key])
  async function refresh() { const value = await survey.refetch(); if (value.error) { if (nativeDenied(value.error)) setDenied(value.error); throw value.error }; return value.data! }
 async function currentDenial(error: unknown) { const latest = await survey.refetch(); if (latest.error || latest.data?.can_respond !== false) setDenied(error) }
  const page = !nativeDenied(survey.error) && !denied ? survey.data : undefined
  const networkBlocked = !!survey.error
  return <section className="workspace survey-workspace"><nav className="actions"><Link to="/inbox">← Your inbox</Link><Link to="/events/$eventId/surveys" params={{ eventId }}>Your surveys and reports</Link></nav><h1>Post-event survey</h1><Button variant="outline" onClick={() => { setDenied(undefined); void refresh().catch(() => {}) }}>Refresh survey</Button>
    {!!survey.error && !nativeDenied(survey.error) && page && <p role="alert">Current authority could not be refreshed. Local answers and the exact retry command remain in this page; writes are paused until a successful refresh.</p>}
    {denied || nativeDenied(survey.error) || !page && survey.error ? <Failure error={denied || survey.error} retry={() => { setDenied(undefined); void refresh().catch(() => {}) }} /> : survey.isPending || !page ? <Loading /> : page && <article className="panel survey-private">
      <p className="eyebrow">Fixed question version {page.version}</p><h2>{page.form.name}</h2><p role="note">{page.response_visibility}</p>
      <p>Opens {localDate(page.opens_at, page.timezone)} · Closes {localDate(page.closes_at, page.timezone)} · {page.timezone}</p>
      <p>{page.can_respond ? 'This survey is open. You can update your response until close. Questions marked required must be answered; other questions are optional.' : 'Response editing is currently closed or unavailable.'}</p>
      {page.response ? <section aria-label="Your saved response"><h3>Your saved response: revision {page.response.revision}, submitted {localDate(page.response.submitted_at, page.timezone)}.</h3><dl>{visibleFields(page.form, page.response.answers).map(field => <div key={field.id}><dt>{field.label}</dt><dd className="help-problem">{answerText(page.response?.answers[field.id])}</dd></div>)}</dl></section> : <p>You have not submitted a response.</p>}
      {page.can_respond ? <ResponseForm page={page} eventId={eventId} surveyId={surveyId} refresh={refresh} networkBlocked={networkBlocked} deny={error => { void currentDenial(error) }} /> : <ol>{page.form.fields.map(field => <li key={field.id}><h3>{field.label}</h3>{field.help && <p>{field.help}</p>}</li>)}</ol>}
      {page.measures.map(m => <p key={m.question_id}>Self-reported {m.construct === 'self_reported_adoption' ? 'adoption' : 'learning'} · objective {m.measure.objective.id} version {m.measure.objective.version}. This does not independently evidence adoption.</p>)}
    </article>}
  </section>
}
type ResponseCommand = components['schemas']['SurveyResponseCommand']
function ResponseForm({ page, eventId, surveyId, refresh, networkBlocked, deny }: { page: ParticipantSurvey; eventId: string; surveyId: string; refresh: () => Promise<ParticipantSurvey>; networkBlocked: boolean; deny: (error: unknown) => void }) {
  const identity = useIdentity(), active = useActivePage()
  const [saved, setSaved] = useState(page.response), [answers, setAnswers] = useState<Answers>(() => ({ ...page.response?.answers })), [pending, setPending] = useState(false), [uncertain, setUncertain] = useState<{ command: ResponseCommand; revision: number }>(), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState(''), [conflict, setConflict] = useState(false)
  const dirty = !same(answers, saved?.answers ?? {})
  const changedElsewhere = (page.response?.revision ?? 0) !== (saved?.revision ?? 0)
  useSurveyLeaveWarning(dirty || !!uncertain)
  function currentAnswers() { const ids = new Set(visibleFields(page.form, answers).map(f => f.id)); return Object.fromEntries(Object.entries(answers).filter(([id, value]) => ids.has(id) && value !== undefined)) }
  async function send(retained?: { command: ResponseCommand; revision: number }) {
    const intent = retained ?? { command: { command_id: crypto.randomUUID(), form_version: page.version, answers: currentAnswers() }, revision: saved?.revision ?? 0 }
    setPending(true); setFailure(undefined); setNotice('')
    try {
      if (retained) { const current = await refresh(); if (!active.current || !current.can_respond) return }
      const response = unwrap(await api.PUT('/api/v1/events/{event_id}/surveys/{survey_id}/response', { params: { path: { event_id: eventId, survey_id: surveyId }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${intent.revision}"` } }, body: intent.command }))
      if (!active.current) return
      setSaved(response); setAnswers({ ...response.answers }); setUncertain(undefined); setConflict(false); setNotice(`Response command confirmed at revision ${response.revision}. The current saved response is shown above; you can edit until close.`); await refresh().catch(() => {})
    } catch (error) {
      if (!active.current) return
      if (nativeDenied(error)) { deny(error); return }
      setFailure(error)
      if (unknownOutcome(error)) setUncertain(intent)
      else { setUncertain(undefined); if (error instanceof APIError && error.status === 409) setConflict(true); await refresh().catch(() => {}) }
    } finally { if (active.current) setPending(false) }
  }
  function loadCurrent() {
    if ((dirty || uncertain) && !window.confirm('Discard local answers and the retained uncertain command, then load the current saved response?')) return
    setSaved(page.response); setAnswers({ ...page.response?.answers }); setUncertain(undefined); setConflict(false); setFailure(undefined); setNotice('Loaded the current server response.')
  }
  const blocked = networkBlocked || pending || !!uncertain || conflict || changedElsewhere
  return <form className="survey-form" onSubmit={e => { e.preventDefault(); void send() }}>
    <fieldset disabled={blocked}><legend>{saved ? 'Edit your response' : 'Your response'}</legend><FormQuestions definition={page.form} answers={answers} change={setAnswers} showOptional issues={failure instanceof APIError ? failure.blockers : []} /></fieldset>
    <p role="status" aria-label="Survey response status">{notice || (uncertain ? 'The outcome is unknown. No new command will be sent automatically. You may explicitly retry the same command while the survey remains open.' : dirty ? 'Unsent changes, kept only in this page.' : 'No unsent changes.')}</p>
    {!!failure && <Failure error={failure} />}
    {(conflict || changedElsewhere) && <p role="alert">The saved response changed. Review the current saved response above before replacing your local draft.</p>}
    <div className="actions">{uncertain ? <Button type="button" disabled={pending || networkBlocked} onClick={() => void send(uncertain)}>Retry the same survey response</Button> : <Button type="submit" disabled={blocked || !dirty}>Save response</Button>}<Button type="button" variant="outline" disabled={pending} onClick={() => void refresh().catch(() => {})}>Check saved response</Button>{(dirty || uncertain || conflict || changedElsewhere) && <Button type="button" variant="outline" disabled={pending} onClick={loadCurrent}>Load current saved response</Button>}</div>
  </form>
}

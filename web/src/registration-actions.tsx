import { useCallback, useEffect, useRef, useState } from 'react'
import { useBlocker } from '@tanstack/react-router'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, APIError, privateKey, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { ConfirmationDialog } from './components/ui/dialog'
import { Failure, Loading } from './components/states'
import { FormQuestions, visibleFields, type Answers } from './form-questions'
import { usePrivateRequestScope } from './private-request-scope'
import { AdmissionFeedback, useAdmissionCommand } from './registration-command'

type Registration = components['schemas']['Registration']
type AnswerState = { locked: boolean; dirty: boolean }
function sameAnswers(left: Answers, right: Answers) { const sorted = (value: Answers) => JSON.stringify(Object.keys(value).sort().map(key => [key, value[key]])); return sorted(left) === sorted(right) }
type ResponseAction = 'accept' | 'decline' | 'withdraw'
const actionLabels: Record<ResponseAction, { title: string; confirm: string }> = {
  accept: { title: 'Review offer acceptance', confirm: 'Confirm acceptance' },
  decline: { title: 'Review offer decline', confirm: 'Confirm decline' },
  withdraw: { title: 'Review application withdrawal', confirm: 'Confirm withdrawal' },
}
export function RegistrationActions({ registration: r, frozen }: { registration: Registration; frozen: boolean }) {
  const identity = useIdentity(), cache = useQueryClient(), opener = useRef<HTMLButtonElement>(null)
  const [editing, setEditing] = useState(false), [answerState, setAnswerState] = useState<AnswerState>({ locked: false, dirty: false }), [review, setReview] = useState<{ action: ResponseAction; registration: Registration }>()
  const answerStateRef = useRef<AnswerState>({ locked: false, dirty: false })
  const changeAnswerState = useCallback((state: AnswerState) => { answerStateRef.current = state; setAnswerState(state) }, [])
  const command = useAdmissionCommand(identity.person.id + ':' + identity.draft_scope + ':' + r.event_id + ':' + r.id)
  const readCurrent = async () => { await cache.fetchQuery({ queryKey: privateKey(identity.person.id, r.event_id, 'registration-page'), staleTime: 0, retry: false, queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/registration/events/{event_id}', { signal, params: { path: { event_id: r.event_id } } })) }); setReview(undefined) }
  function allowAnswerDeparture() {
    if (answerStateRef.current.locked) return false
    return !answerStateRef.current.dirty || window.confirm('Discard your unsaved corrected answers and leave the answer review?')
  }
  function prepare(action: ResponseAction, trigger: HTMLButtonElement) {
    if (command.blocked || !allowAnswerDeparture()) return
    if (editing) setEditing(false)
    opener.current = trigger; setReview({ action, registration: structuredClone(r) })
  }
  async function confirm() {
    if (!review || !allowAnswerDeparture()) return
    if (editing) setEditing(false)
    const captured = review
    await command.run({ label: actionLabels[captured.action].title, resource: captured.registration.id, revision: captured.registration.revision }, async () => {
      const result = unwrap(await api.POST('/api/v1/events/{event_id}/registrations/{registration_id}/respond', {
        params: { path: { event_id: captured.registration.event_id, registration_id: captured.registration.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': '"' + captured.registration.revision + '"' } }, body: { action: captured.action },
      }))
      if (result.id !== captured.registration.id || result.event_id !== captured.registration.event_id || result.person_id !== identity.person.id || result.revision <= captured.registration.revision) throw new Error('Unverified registration response')
      return result
    }, async result => {
      setReview(undefined)
      await cache.invalidateQueries({ queryKey: ['private', identity.person.id] })
      return result.offer?.state === 'expired' ? 'This offer has expired. Your latest status is shown above.' : 'The server returned application revision ' + result.revision + '. Your latest status is shown above.'
    })
  }
  const active = !['withdrawn', 'declined'].includes(r.state)
  const withdrawal = frozen ? 'Withdraw? Your team will be smaller. Reassignment and backfill are unavailable after team freeze.' : 'Withdraw? If your team falls below three, the organiser must reassign the remaining members before freeze. This does not cancel the event.'
  const description = review?.action === 'withdraw' ? withdrawal : review?.action === 'decline' ? 'Declining releases the reserved offer. The current server deadline and revision decide whether this response is accepted.' : 'Accept this named competitor offer. A pending offer reserves a place; only the server response can confirm your admission.'
  return <>{active && <><div className="fleet-pattern-actions">{r.state === 'offered' && <><Button icon="check" disabled={command.blocked || answerState.locked} onClick={e => prepare('accept', e.currentTarget)}>Accept place</Button><Button icon="close" disabled={command.blocked || answerState.locked} variant="outline" onClick={e => prepare('decline', e.currentTarget)}>Decline offer</Button></>}<Button icon="list" variant="outline" disabled={command.blocked || answerState.locked} onClick={() => { if (!editing || allowAnswerDeparture()) setEditing(!editing) }}>{editing ? 'Close answers' : 'Review or correct answers'}</Button><Button icon="remove" variant="outline" disabled={command.blocked || answerState.locked} onClick={e => prepare('withdraw', e.currentTarget)}>Withdraw application</Button></div><p className="fleet-reference">{frozen ? 'Teams are frozen. Withdrawal leaves your team smaller; members cannot move to another team. The remaining team may continue or withdraw.' : 'Before team freeze, a team falling below three needs organiser-approved reassignment. Withdrawal does not automatically cancel the hack.'}</p>{editing && <ResponseEditor eventID={r.event_id} registrationID={r.id} onState={changeAnswerState} />}</>}
    <AdmissionFeedback command={command} read={readCurrent} />
    <ConfirmationDialog open={!!review} onOpenChange={open => { if (!open) setReview(undefined) }} title={review ? actionLabels[review.action].title : 'Review application decision'} description={description} confirmLabel={review ? actionLabels[review.action].confirm : 'Confirm decision'} pending={command.pending} disabled={answerState.locked || (command.blocked && !command.pending)} finalFocus={opener} onConfirm={() => void confirm()}><dl className="fleet-admission-facts"><div><dt>Application reference</dt><dd>{review?.registration.id}</dd></div><div><dt>Audience</dt><dd>{review?.registration.audience}</dd></div><div><dt>Reviewed revision</dt><dd>{review?.registration.revision}</dd></div></dl>{review && <AdmissionFeedback command={command} read={readCurrent} />}</ConfirmationDialog>
  </>
}
function ResponseEditor({ eventID, registrationID, onState }: { eventID: string; registrationID: string; onState: (state: AnswerState) => void }) {
  const identity = useIdentity(), [denied, setDenied] = useState<APIError>()
  const query = useQuery({ queryKey: privateKey(identity.person.id, eventID, 'response:' + registrationID), queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/events/{event_id}/registrations/{registration_id}/response', { params: { path: { event_id: eventID, registration_id: registrationID } }, signal })) })
  if (denied) return <Failure error={denied} />
  if (query.isPending) return <Loading />
  if (!query.data || (query.error instanceof APIError && [401, 403, 404].includes(query.error.status))) return <Failure error={query.error} retry={() => void query.refetch()} />
  return <AnswerCorrection key={registrationID} eventID={eventID} registrationID={registrationID} response={query.data} paused={!!query.error} onState={onState} onDenied={setDenied} read={async () => { const result = await query.refetch(); if (result.error) throw result.error }} />
}
function AnswerCorrection({ eventID, registrationID, response, read, paused, onState, onDenied }: { paused: boolean; onState: (state: AnswerState) => void; onDenied: (error: APIError) => void; eventID: string; registrationID: string; response: components['schemas']['RegistrationResponse']; read: () => Promise<unknown> }) {
  const identity = useIdentity(), cache = useQueryClient(), [answers, setAnswers] = useState<Answers>(response.answers), [review, setReview] = useState(false)
  const command = useAdmissionCommand(identity.person.id + ':' + identity.draft_scope + ':' + eventID + ':' + registrationID + ':answers')
  const ownership = usePrivateRequestScope(identity.person.id + ':' + identity.draft_scope + ':' + eventID + ':' + registrationID + ':answer-ownership')
  const dirty = !sameAnswers(answers, response.answers) || review
  useBlocker({ shouldBlockFn: () => command.isBlocked() || (dirty && !window.confirm('Discard your unsaved corrected answers and leave the answer review?')), enableBeforeUnload: dirty || command.blocked })
  useEffect(() => { onState({ locked: command.blocked, dirty }); return () => onState({ locked: false, dirty: false }) }, [command.blocked, dirty, onState])
  useEffect(() => { if (command.failure instanceof APIError && [401, 403, 404].includes(command.failure.status)) onDenied(command.failure) }, [command.failure, onDenied])
  async function save() {
    if (!review || command.blocked || paused) return
    const ticket = ownership.capture(); if (!ticket.current()) return
    onState({ locked: true, dirty })
    const expected = response.revision, captured = Object.fromEntries(visibleFields(response.definition, answers).filter(field => field.id in answers).map(field => [field.id, structuredClone(answers[field.id])]))
    const accepted = await command.run({ label: 'Save corrected answers', resource: registrationID, revision: expected }, async () => {
      const result = unwrap(await api.PUT('/api/v1/events/{event_id}/registrations/{registration_id}/response', { params: { path: { event_id: eventID, registration_id: registrationID }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': '"' + expected + '"' } }, body: captured }))
      if (result.revision <= expected || result.definition.id !== response.definition.id) throw new Error('Unverified corrected response')
      return result
    }, async result => { setAnswers(result.answers); setReview(false); await cache.invalidateQueries({ queryKey: ['private', identity.person.id, eventID] }); return 'Corrected answers saved at server revision ' + result.revision + '.' })
    if (ticket.current()) onState({ locked: command.isBlocked(), dirty: accepted ? false : dirty })
  }
  return <form className="fleet-admission-form" onSubmit={e => { e.preventDefault(); void save() }}><h3>Your submitted answers</h3><p>Corrections keep the form version you originally answered. Current response revision {response.revision}.</p><fieldset className="fleet-admission-fields" disabled={command.blocked || paused || review}><legend>Correct your answers</legend><FormQuestions definition={response.definition} answers={answers} change={setAnswers} issues={command.failure instanceof APIError ? command.failure.blockers : []} /></fieldset>{review ? <section className="fleet-admission-review"><h4>Review corrected answers</h4><p>Only the visible answers for the original published form version are submitted. Review your changes before saving.</p><dl className="fleet-admission-summary">{visibleFields(response.definition, answers).map(field => <div key={field.id}><dt>{field.label}</dt><dd>{Array.isArray(answers[field.id]) ? (answers[field.id] as unknown[]).map(String).join(", ") : String(answers[field.id] ?? "Not answered")}</dd></div>)}</dl><div className="fleet-pattern-actions"><Button type="button" variant="outline" disabled={command.blocked} onClick={() => setReview(false)}>Correct details</Button><Button type="submit" icon="save" pending={command.pending} disabled={command.blocked && !command.pending}>Save corrected answers</Button></div></section> : <Button type="button" icon="next" disabled={command.blocked || paused} onClick={() => setReview(true)}>Review corrected answers</Button>}<AdmissionFeedback command={command} read={async () => { await read(); setReview(false) }} /></form>
}

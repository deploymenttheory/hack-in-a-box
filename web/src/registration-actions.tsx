import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, APIError, privateKey, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { FormQuestions, type Answers } from './form-questions'

export function RegistrationActions({ registration: r, frozen }: { registration: components['schemas']['Registration']; frozen: boolean }) {
  const identity = useIdentity(), cache = useQueryClient(), [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>(), [editing, setEditing] = useState(false), [notice, setNotice] = useState('')
  async function act(action: 'accept' | 'decline' | 'withdraw') {
    if (pending) return
    if (action === 'withdraw' && !window.confirm(frozen ? 'Withdraw? Your team will be smaller. Reassignment and backfill are unavailable after team freeze.' : 'Withdraw? If your team falls below three, the organiser must reassign the remaining members before freeze. This does not cancel the event.')) return
    setPending(true); setFailure(undefined)
    try { const result = unwrap(await api.POST('/api/v1/events/{event_id}/registrations/{registration_id}/respond', { params: { path: { event_id: r.event_id, registration_id: r.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${r.revision}"` } }, body: { action } })); setNotice(result.offer?.state === 'expired' ? 'This offer has expired. Your latest status is shown above.' : 'Your latest status is shown above.'); await cache.invalidateQueries({ queryKey: ['private', identity.person.id] }) } catch (error) { setFailure(error) } finally { setPending(false) }
  }
  const active = !['withdrawn', 'declined'].includes(r.state)
  return <>{active && <><div className="actions">{r.state === 'offered' && <><Button disabled={pending} onClick={() => void act('accept')}>Accept place</Button><Button disabled={pending} variant="outline" onClick={() => void act('decline')}>Decline offer</Button></>}<Button variant="outline" disabled={pending} onClick={() => setEditing(!editing)}>{editing ? 'Close answers' : 'Review or correct answers'}</Button><Button variant="outline" disabled={pending} onClick={() => void act('withdraw')}>Withdraw application</Button></div><p className="muted">{frozen ? 'Teams are frozen. Withdrawal leaves your team smaller; members cannot move to another team. The remaining team may continue or withdraw.' : 'Before team freeze, a team falling below three needs organiser-approved reassignment. Withdrawal does not automatically cancel the hack.'}</p>{editing && <ResponseEditor eventID={r.event_id} registrationID={r.id} />}</>}{notice && <p role="status">{notice}</p>}{!!failure && <Failure error={failure} />}</>
}
function ResponseEditor({ eventID, registrationID }: { eventID: string; registrationID: string }) {
  const identity = useIdentity()
  const query = useQuery({ queryKey: privateKey(identity.person.id, eventID, `response:${registrationID}`), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/registrations/{registration_id}/response', { params: { path: { event_id: eventID, registration_id: registrationID } } })) })
  if (query.isPending) return <Loading />
  if (query.error) return <Failure error={query.error} />
  return <AnswerCorrection key={query.data.revision} eventID={eventID} registrationID={registrationID} response={query.data} />
}
function AnswerCorrection({ eventID, registrationID, response }: { eventID: string; registrationID: string; response: components['schemas']['RegistrationResponse'] }) {
  const identity = useIdentity(), cache = useQueryClient(), [answers, setAnswers] = useState<Answers>(response.answers), [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>()
  async function save() {
    setPending(true); setFailure(undefined)
    try { unwrap(await api.PUT('/api/v1/events/{event_id}/registrations/{registration_id}/response', { params: { path: { event_id: eventID, registration_id: registrationID }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${response.revision}"` } }, body: answers })); await cache.invalidateQueries({ queryKey: ['private', identity.person.id, eventID] }) } catch (error) { setFailure(error) } finally { setPending(false) }
  }
  return <form className="panel section-heading" onSubmit={e => { e.preventDefault(); void save() }}><h3>Your submitted answers</h3><p>Corrections keep the form version you originally answered.</p><FormQuestions definition={response.definition} answers={answers} change={setAnswers} issues={failure instanceof APIError ? failure.blockers : []} /><Button type="submit" disabled={pending}>Save corrected answers</Button>{!!failure && <Failure error={failure} />}</form>
}

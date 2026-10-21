import { useState } from 'react'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, privateKey, unwrap, type EventRecord } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { eventTime } from './registration'

export function ApplicantReview({ event, openSetup }: { event: EventRecord; openSetup: () => void }) {
  const identity = useIdentity(), [selected, setSelected] = useState<string>(), [filter, setFilter] = useState('all')
  const query = useInfiniteQuery({ queryKey: privateKey(identity.person.id, event.id, 'applicants'), initialPageParam: 0, queryFn: async ({ pageParam }) => unwrap(await api.GET('/api/v1/events/{event_id}/registrations/review', { params: { path: { event_id: event.id }, query: { after: pageParam } } })), getNextPageParam: page => page.next_cursor ?? undefined })
  return <section><h2>Applicant review</h2><p>{event.confirmed_attendees} confirmed competitors · capacity {event.configuration.capacity} · minimum {event.configuration.minimum_attendance}</p>{event.can_edit && <><Button variant="outline" onClick={openSetup}>Change capacity in event setup</Button><OfferPolicy event={event} /></>}<label htmlFor="applicant-filter">Applications shown</label><select id="applicant-filter" value={filter} onChange={e => setFilter(e.target.value)}><option value="all">All applicants</option><option value="attendee">Competitors and waitlist</option><option value="facilitator">Facilitator applications</option></select>{query.isPending ? <Loading /> : query.error ? <Failure error={query.error} /> : <><div className="section-heading">{query.data.pages.flatMap(p => p.items).filter(a => filter === 'all' || a.registration.kind === filter).map(a => <article className="panel section-heading" key={a.registration.id}><h3>{a.display_name}</h3><p>{a.registration.audience} · {a.registration.state}{a.registration.waitlist_position != null && ` · Waitlist ${a.registration.waitlist_position}`}</p>{a.registration.offer && <p>Offer {a.registration.offer.state}: {eventTime(a.registration.offer.expires_at, event.configuration.timezone)}</p>}<Button variant="outline" onClick={() => setSelected(a.registration.id)}>Review {a.display_name}</Button></article>)}</div>{query.hasNextPage && <Button disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>Load more applicants</Button>}</>}{selected && <ApplicantDetails key={selected} event={event} id={selected} />}</section>
}
function OfferPolicy({ event }: { event: EventRecord }) {
  const identity = useIdentity()
  const query = useQuery({ queryKey: privateKey(identity.person.id, event.id, 'offer-policy'), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/registrations/offer-policy', { params: { path: { event_id: event.id } } })) })
  if (query.isPending) return <Loading />
  if (query.error) return <Failure error={query.error} />
  return <PolicyEditor key={query.data.revision} event={event} value={query.data} />
}
function PolicyEditor({ event, value }: { event: EventRecord; value: components['schemas']['OfferSettings'] }) {
  const identity = useIdentity(), cache = useQueryClient(), [automated, setAutomated] = useState(value.automated), [days, setDays] = useState(value.working_days), [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>()
  async function save() {
    setPending(true); setFailure(undefined)
    try { unwrap(await api.PUT('/api/v1/events/{event_id}/registrations/offer-policy', { params: { path: { event_id: event.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${value.revision}"` } }, body: { automated, working_days: days } })); await cache.invalidateQueries({ queryKey: ['private', identity.person.id, event.id] }) } catch (error) { setFailure(error) } finally { setPending(false) }
  }
  return <form className="panel section-heading" onSubmit={e => { e.preventDefault(); void save() }}><h3>Waitlist automation</h3><p>Approved automation offers available places in FIFO order, including during preparation, until final admission. Existing offers still expire when automation is disabled.</p><label htmlFor="offer-days">Working days to respond</label><input id="offer-days" type="number" min={1} max={30} value={days} onChange={e => setDays(Number(e.target.value))} /><label className="check"><input type="checkbox" checked={automated} onChange={e => setAutomated(e.target.checked)} />Approve automatic FIFO offers</label><Button type="submit" disabled={pending}>Save offer policy</Button>{!!failure && <Failure error={failure} />}</form>
}
function ApplicantDetails({ event, id }: { event: EventRecord; id: string }) {
  const identity = useIdentity(), cache = useQueryClient(), [reason, setReason] = useState(''), [ack, setAck] = useState(false), [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState('')
  const query = useQuery({ queryKey: privateKey(identity.person.id, event.id, `applicant:${id}`), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/registrations/{registration_id}/review', { params: { path: { event_id: event.id, registration_id: id } } })) })
  if (query.isPending) return <Loading />
  if (query.error) return <Failure error={query.error} />
  const { applicant, response, history, fallback_available } = query.data, r = applicant.registration
  async function decide(action: 'selected' | 'backup' | 'declined' | 'invite' | 'promote') {
    setPending(true); setFailure(undefined); setNotice('')
    const params = { path: { event_id: event.id, registration_id: id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${r.revision}"` } }
    try {
      if (action === 'promote') unwrap(await api.POST('/api/v1/events/{event_id}/registrations/{registration_id}/promote', { params, body: { reason } }))
      else unwrap(await api.POST('/api/v1/events/{event_id}/registrations/{registration_id}/decision', { params, body: { action, reason, acknowledge_competition_restriction: ack } }))
      await cache.invalidateQueries({ queryKey: ['private', identity.person.id, event.id] }); setNotice(action === 'invite' ? 'Invitation queued. The person must apply separately; no place has been allocated.' : 'Decision recorded.'); setReason(''); setAck(false)
    } catch (error) { setFailure(error) } finally { setPending(false) }
  }
  return <section className="panel section-heading"><h3>Reviewing {applicant.display_name}</h3><p>{r.audience} · {r.state}</p>{response ? <dl>{response.definition.fields.filter(f => f.id in response.answers).map(f => <div key={f.id}><dt>{f.label}</dt><dd>{Array.isArray(response.answers[f.id]) ? (response.answers[f.id] as unknown[]).map(String).join(', ') : String(response.answers[f.id])}</dd></div>)}</dl> : <p>Answers have been removed under the event retention policy.</p>}<p>{r.fallback_to_attendee ? 'Prefers an invitation to apply separately as a competitor if not selected.' : 'No fallback invitation requested.'}</p>{event.can_edit && <><label htmlFor="review-reason">Decision or exception reason</label><textarea id="review-reason" maxLength={2000} value={reason} onChange={e => setReason(e.target.value)} /><p>Selection as organiser permanently prevents competition in this event. Organiser, mentor and judge selection cannot silently withdraw an active competitor application.</p>{r.audience === 'organiser' && <label className="check"><input type="checkbox" checked={ack} onChange={e => setAck(e.target.checked)} />I acknowledge the permanent competition restriction.</label>}<div className="actions">{r.kind === 'facilitator' && r.state !== 'withdrawn' && <><Button disabled={pending || !reason.trim() || (r.audience === 'organiser' && !ack)} onClick={() => void decide('selected')}>Select facilitator</Button><Button variant="outline" disabled={pending || !reason.trim()} onClick={() => void decide('backup')}>Keep as backup</Button><Button variant="outline" disabled={pending || !reason.trim()} onClick={() => void decide('declined')}>Decline application</Button>{fallback_available && <Button variant="outline" disabled={pending || !reason.trim()} onClick={() => void decide('invite')}>Invite separate attendee application</Button>}</>}{r.kind === 'attendee' && r.state === 'waitlisted' && <Button disabled={pending || !reason.trim()} onClick={() => void decide('promote')}>Promote with recorded exception</Button>}</div></>}<h4>Decision history</h4><ul>{history.map((h, i) => <li key={i}>{eventTime(h.at, event.configuration.timezone)} · {h.action} · {h.from || 'new'} → {h.to}{h.reason && ` · ${h.reason}`}</li>)}</ul>{notice && <p role="status">{notice}</p>}{!!failure && <Failure error={failure} />}</section>
}

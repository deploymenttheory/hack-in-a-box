import { localDate } from './event-time'
import { useCallback, useRef, useState } from 'react'
import { useBlocker, useParams } from '@tanstack/react-router'
import { api, unwrap } from './api/client'
import { useIdentity } from './session'
import { EventBrand } from './branding'
import { useEventAuthority } from './event-authority'
import { useTodayRead } from './today-read'
import { type PrivateRequestScope } from './private-request-scope'
import { Button } from './components/ui/button'
import { ResultsLink as ActionLink } from './results-link'
import { Field, Input, Select, Textarea, focusFirstInvalid } from './components/ui/fields'
import { Failure, Loading } from './components/states'
import { AnnouncedResults, AwardSummary, ResultReview } from './results-view'
import { assertResultReady, matchesResultReceipt, resultActionLabel, resultActionReady, resultBinding, resultFingerprint, type ResultAward, type ResultCommand, type ResultPage } from './results-model'
import { decisionNamespace, OriginalResultHistory, ResultDecisionState, useResultDecision } from './results-decision'
const privateRead = async (eventID: string, signal: AbortSignal) => unwrap(await api.GET('/api/v1/events/{event_id}/results', { signal, cache: 'no-store', params: { path: { event_id: eventID } } }))

export function AnnouncedResultsPage() {
 const { eventId } = useParams({ strict: false })
 return <EventBrand eventID={eventId!}><AnnouncedEvent eventID={eventId!} /></EventBrand>
}
function AnnouncedEvent({ eventID }: { eventID: string }) {
 const authority = useEventAuthority(eventID)
 if (authority.denial) return <Failure error={authority.denial} retry={() => void authority.refresh()} />
 if (!authority.verified || !authority.query.data) return <Loading />
 return <AnnouncedReader key={authority.generation} eventID={eventID} name={authority.query.data.name} timezone={authority.query.data.configuration.timezone} parent={authority.scope} paused={authority.boundary.paused} />
}
function AnnouncedReader({ eventID, name, timezone, parent, paused }: { eventID: string; name: string; timezone: string; parent: PrivateRequestScope; paused: boolean }) {
 const load = useCallback(async (signal: AbortSignal) => unwrap(await api.GET('/api/v1/events/{event_id}/results/announcements', { signal, cache: 'no-store', params: { path: { event_id: eventID } } })), [eventID])
 const read = useTodayRead(`${eventID}:announcements`, load, parent)
 return <section className="fleet-results"><ActionLink href={`/events/${eventID}`} icon="back" variant="ghost">Back to event</ActionLink><header><p className="fleet-results-eyebrow">{name}</p><h1>Event results</h1><p>Celebrate the outcomes announced by your organiser.</p></header>{!!read.error && <Failure error={read.error} retry={() => void read.refresh()} />}{!read.denied && read.data ? <AnnouncedResults items={read.data.items} timezone={timezone} /> : !read.error && <Loading />}<Button icon="refresh" variant="outline" disabled={paused || read.pending} onClick={() => void read.refresh()}>Check announcements</Button><ActionLink href={`/events/${eventID}/team-feedback`} icon="next">Read your team’s released feedback</ActionLink></section>
}
export function ResultsPreparationPage() {
 const { eventId } = useParams({ strict: false }), identity = useIdentity()
 const owner = decisionNamespace(identity.person.id, identity.draft_scope, eventId!)
 if (import.meta.env.MODE !== 'admin') return <p>Open the administration portal to prepare results.</p>
 return <><OriginalResultHistory owner={owner} read={async (requestID, signal) => unwrap(await api.GET('/api/v1/events/{event_id}/results/requests/{request_id}', { signal, cache: 'no-store', params: { path: { event_id: eventId!, request_id: requestID } } }))} matches={(result, intent) => !!result.receipt && matchesResultReceipt(result.receipt, intent)} /><EventBrand eventID={eventId!}><ResultsEvent eventID={eventId!} /></EventBrand></>
}
function ResultsEvent({ eventID }: { eventID: string }) {
 const authority = useEventAuthority(eventID)
 if (authority.denial || authority.query.error && !authority.query.data) return <Failure error={authority.denial ?? authority.query.error} retry={() => void authority.refresh()} />
 if (!authority.verified || !authority.query.data) return <Loading />
 return <ResultsReader key={authority.generation} eventID={eventID} name={authority.query.data.name} timezone={authority.query.data.configuration.timezone} parent={authority.scope} paused={authority.boundary.paused} />
}
function ResultsReader({ eventID, name, timezone, parent, paused }: { eventID: string; name: string; timezone: string; parent: PrivateRequestScope; paused: boolean }) {
 const load = useCallback(async (signal: AbortSignal) => {
  const [page, roster] = await Promise.all([privateRead(eventID, signal), api.GET('/api/v1/events/{event_id}/teams', { signal, cache: 'no-store', params: { path: { event_id: eventID } } }).then(unwrap)])
  return { page, names: Object.fromEntries(roster.teams.map(t => [t.id, t.name])) }
 }, [eventID])
 const signature = useCallback((value: Awaited<ReturnType<typeof load>>) => JSON.stringify([value.page.preview.binding, value.page.preview.finalists_binding, value.names]), [])
 const read = useTodayRead(`${eventID}:private-results`, load, parent, signature)
 if (read.denied) return <section className="fleet-results"><h1>Results preparation unavailable</h1><p>Current organiser access is required. Private results and held reviews have been cleared.</p><Button icon="refresh" onClick={() => void read.refresh()}>Check current access</Button></section>
 if (!read.data) return read.error ? <Failure error={read.error} retry={() => void read.refresh()} /> : <Loading />
 return <section className="fleet-results fleet-results-private">{!!read.error && <Failure error={read.error} retry={() => void read.refresh()} />}<ResultEditor key={read.generation} eventID={eventID} name={name} timezone={timezone} page={read.data.page} names={read.data.names} parent={read.authority} paused={paused || read.pending || !!read.error} refresh={() => read.refresh()} /></section>
}
function ResultEditor({ eventID, name, timezone, page, names, parent, paused, refresh }: { eventID: string; name: string; timezone: string; page: ResultPage; names: Record<string, string>; parent: PrivateRequestScope; paused: boolean; refresh: () => Promise<unknown> }) {
 const identity = useIdentity(), [reason, setReason] = useState(''), [awards, setAwards] = useState<ResultAward[]>(() => page.preview.criteria?.prizes.map(prize => ({ prize_id: prize.id, name: prize.name, category_id: prize.category_id, team_id: '', amount: '', currency: '' })) ?? []), [errors, setErrors] = useState<Record<string, string>>({})
 const teamName = (id: string) => names[id] ?? `Team ${id}`, form = useRef<HTMLFormElement>(null)
 const owner = decisionNamespace(identity.person.id, identity.draft_scope, eventID)
 const decision = useResultDecision({ owner, parent, paused, page, version: page => page.round.version, revision: page => page.revision, read: signal => privateRead(eventID, signal), fingerprint: resultFingerprint, ready: assertResultReady, send: async (body: ResultCommand, revision) => unwrap(await api.POST('/api/v1/events/{event_id}/results', { params: { path: { event_id: eventID }, header: { 'If-Match': `"${revision}"`, 'X-CSRF-Token': identity.csrf_token } }, body })), matches: (result, intent) => !!result.receipt && matchesResultReceipt(result.receipt, intent), refresh, acknowledged: current => { setReason(''); setAwards((current ?? page).preview.criteria?.prizes.map(prize => ({ prize_id: prize.id, name: prize.name, category_id: prize.category_id, team_id: '', amount: '', currency: '' })) ?? []) } })
 const dirty = !!reason || awards.some(a => !!a.team_id || !!a.amount || !!a.currency)
 useBlocker({ shouldBlockFn: () => decision.pending || !decision.unknown && dirty && !window.confirm('Leave with unsaved award preparation?'), enableBeforeUnload: dirty || decision.pending })
 async function review(action: ResultCommand['action'], source: HTMLElement) {
  const errors: Record<string, string> = {}
  if (!reason.trim()) errors.reason = 'Record the reason for this exact decision.'
  if (new TextEncoder().encode(reason).length > 2000) errors.reason = 'Use at most 2,000 bytes for the reason.'
  if (action === 'confirm') awards.forEach((a, i) => { if (!a.team_id) errors[`team-${i}`] = 'Select the team receiving this award.'; if ((a.amount || a.currency) && (!/^[0-9]+(?:\.[0-9]+)?$/.test(a.amount) || !/^[A-Z]{3}$/.test(a.currency))) errors[`money-${i}`] = 'Use a positive amount and three uppercase currency letters, or leave both empty.' })
  setErrors(errors)
  if (Object.keys(errors).length) { requestAnimationFrame(() => { if (form.current) focusFirstInvalid(form.current) }); return }
  const body: ResultCommand = { request_id: crypto.randomUUID(), version: page.round.version, action, source_binding: resultBinding(page, action), reason, ...(action === 'confirm' ? { awards } : {}) }
  const summary = action === 'confirm' ? <AwardSummary awards={awards} teamName={teamName} /> : action === 'publish_awards' ? <AwardSummary awards={page.round.confirmation?.awards ?? []} teamName={teamName} /> : action === 'publish_finalists' ? <><h3>Finalists</h3><ul>{page.preview.selection?.finalist_ids.map(id => <li key={id}>{teamName(id)}</li>)}</ul><h3>Reserves</h3><ul>{page.preview.selection?.reserve_ids.map(id => <li key={id}>{teamName(id)}</li>)}</ul></> : <p>Capture the current signed source for private preparation.</p>
  await decision.prepare(body, summary, source)
 }
 return <><ActionLink href={`/events/${eventID}`} icon="back" variant="ghost">Back to event</ActionLink><header><p className="fleet-results-eyebrow">{name} · Organiser only · Result version {page.round.version}</p><h1>Prepare and announce results</h1><p>Review the signed sources, confirm awards privately, then choose each announcement separately.</p></header><div className="fleet-pattern-actions"><Button icon="refresh" variant="outline" disabled={decision.pending} onClick={() => void refresh()}>Read current results</Button><ActionLink icon="book" href={`/events/${eventID}/results`}>Read announced results</ActionLink><ActionLink icon="next" href={`/events/${eventID}/team-feedback`}>Prepare team feedback separately</ActionLink></div><section><h2>Current source</h2><ActionLink icon="next" href={`/events/${eventID}/results/ballots`}>Open your assigned final tie ballots</ActionLink><p>{page.current ? 'Prepared source matches current assessment.' : 'Preparation needs a fresh source review.'}</p>{page.preview.blockers.length > 0 && <ul>{page.preview.blockers.map((blocker, i) => <li key={i}>{blocker.replaceAll('_', ' ')}</li>)}</ul>}<p>Mentor scores do not carry into the fresh judged final.</p><div className="fleet-results-table"><table><caption>Private final tally · exact category averages summed across required judges</caption><thead><tr><th scope="col">Team</th><th scope="col">Private rank</th><th scope="col">Total</th><th scope="col">Required judges</th></tr></thead><tbody>{page.preview.tallies.map(t => <tr key={t.team_id}><th scope="row">{teamName(t.team_id)}</th><td>{t.rank}</td><td>{t.total.display} <small>({t.total.numerator}/{t.total.denominator})</small></td><td>{t.required_judges}</td></tr>)}</tbody></table></div>{page.tie_states.map(t => <p key={t.tie.id}>{t.tie.kind === 'final' ? 'Final tie' : 'Category tie'} · {t.votes_received}/{t.required_judge_ids.length} required votes · {t.winner_id ? `Recorded winner: ${teamName(t.winner_id)}` : t.blocker.replaceAll('_', ' ') || 'Decision required'}</p>)}</section><form ref={form} className="fleet-results-form" hidden={!!decision.review} onSubmit={event => event.preventDefault()}><fieldset disabled={decision.blocked}><legend>1. Prepare and confirm privately</legend><p>Review every award and choose its recipient. Confirmation checks the current judging decisions.</p>{awards.map((award, i) => <fieldset key={award.prize_id}><legend>{award.name}</legend><Field label="Award recipient" id={`award-team-${i}`} error={errors[`team-${i}`]}><Select value={award.team_id} onChange={event => setAwards(values => values.map((a, at) => at === i ? { ...a, team_id: event.target.value } : a))}><option value="">Select a team</option>{Object.entries(names).map(([id, name]) => <option key={id} value={id}>{name}</option>)}</Select></Field><Field label="Award amount (optional)" id={`award-amount-${i}`} error={errors[`money-${i}`]}><Input inputMode="decimal" maxLength={40} value={award.amount} onChange={event => setAwards(values => values.map((a, at) => at === i ? { ...a, amount: event.target.value } : a))} /></Field><Field label="Currency (with an amount)" id={`award-currency-${i}`} error={errors[`money-${i}`]}><Input maxLength={3} value={award.currency} onChange={event => setAwards(values => values.map((a, at) => at === i ? { ...a, currency: event.target.value } : a))} /></Field></fieldset>)}<Field id="result-reason" label="Decision reason" error={errors.reason}><Textarea value={reason} onChange={event => setReason(event.target.value)} maxLength={2000} /></Field><div className="fleet-pattern-actions">{(['prepare', 'confirm'] as const).map(action => <Button key={action} icon={action === 'prepare' ? 'list' : 'check'} variant={action === 'prepare' ? 'outline' : 'default'} disabled={!resultActionReady(page, action)} onClick={event => void review(action, event.currentTarget)}>{resultActionLabel[action]}</Button>)}</div></fieldset>{page.round.confirmation && <section><h2>Privately confirmed awards</h2><AwardSummary awards={page.round.confirmation.awards} teamName={teamName} /><p>Confirmation by {page.round.confirmation.actor_id} · {localDate(page.round.confirmation.at, timezone)} · {timezone}</p></section>}<fieldset disabled={decision.blocked}><legend>2. Choose each announcement</legend><p>Finalists, awards and personal feedback have independent release controls.</p><div className="fleet-pattern-actions">{(['publish_finalists', 'publish_awards'] as const).map(action => <Button key={action} icon="send" variant="outline" disabled={!resultActionReady(page, action)} onClick={event => void review(action, event.currentTarget)}>{resultActionLabel[action]}</Button>)}</div></fieldset></form><ResultDecisionState {...decision} />{decision.review && <ResultReview action={decision.review.body.action} version={decision.review.intent.version} reason={decision.review.body.reason} pending={decision.pending} disabled={decision.blocked && !decision.pending} confirm={() => void decision.confirm()} cancel={decision.cancel} returnFocus={decision.returnFocus}>{decision.review.summary}{decision.stale && <p role="status">The starting point changed. Cancel this review, read current facts and review a fresh decision.</p>}{decision.failureStage === 'before_write' && <p role="alert">Current access, source or starting point could not be confirmed. Nothing was sent. Cancel this review, read current facts and review again.</p>}{!!decision.failure && <Failure error={decision.failure} />}</ResultReview>}<AnnouncedResults items={page.announcements} timezone={timezone} /></>
}

import { useCallback, useRef, useState } from 'react'
import { useBlocker, useParams } from '@tanstack/react-router'
import { api, APIError, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import { nilUUID } from './results-model'
import { advisoryFingerprint, matchesAdvisoryReceipt, type AdvisoryCommand as Command } from './mentor-advisory-model'
import { decisionNamespace, OriginalResultHistory, ResultDecisionState, useResultDecision } from './results-decision'
import { useIdentity } from './session'
import { useEventAuthority } from './event-authority'
import { useTodayRead } from './today-read'
import type { PrivateRequestScope } from './private-request-scope'
import { EventBrand } from './branding'
import { ResultsLink } from './results-link'
import { Button } from './components/ui/button'
import { Field, Select, focusFirstInvalid } from './components/ui/fields'
import { Failure, Loading } from './components/states'
import { MentorAdvisoryFields, MentorAdvisoryReading, MentorAdvisoryReview } from './mentor-advisory-view'
type Page = components['schemas']['ResultAdvisoryPage']
const advisoryDenials = [401, 403, 404, 409]
const advisoryContextEnded = (error: unknown) => error instanceof APIError && advisoryDenials.includes(error.status)
const key = (t: { tie_id: string; team_id: string }) => `${t.tie_id}:${t.team_id}`
async function load(eventID: string, signal: AbortSignal, actorID: string) {
 const page = unwrap(await api.GET('/api/v1/events/{event_id}/results/mentor-advisories', { signal, cache: 'no-store', params: { path: { event_id: eventID } } }))
 if (page.run_id === nilUUID || page.targets.some(t => t.own && (t.own.author_id !== actorID || t.own.tie_id !== t.tie_id || t.own.team_id !== t.team_id || t.own.run_id !== page.run_id || t.own.source_binding !== page.source_binding || t.own.submission_revision !== t.submission_revision || t.own.version !== t.version))) throw new Error('The current advisory projection did not match your own assigned frozen team.')
 return page
}
export function MentorAdvisoryPage() {
 const { eventId } = useParams({ strict: false }), identity = useIdentity(), owner = decisionNamespace(identity.person.id, identity.draft_scope, eventId!, 'mentor-advisories')
 return <><OriginalResultHistory owner={owner} read={async (requestID, signal) => unwrap(await api.GET('/api/v1/events/{event_id}/results/mentor-advisories/requests/{request_id}', { signal, cache: 'no-store', params: { path: { event_id: eventId!, request_id: requestID } } }))} matches={(result, intent) => !!result.receipt && matchesAdvisoryReceipt(result.receipt, intent)} /><EventBrand eventID={eventId!}><AdvisoryEvent eventID={eventId!} /></EventBrand></>
}
function AdvisoryEvent({ eventID }: { eventID: string }) {
 const event = useEventAuthority(eventID)
 if (event.denial || event.query.error && !event.verified) return <Failure error={event.denial ?? event.query.error} retry={() => void event.refresh()} />
 if (!event.verified || !event.query.data) return <Loading />
 return <AdvisoryReader key={event.generation} eventID={eventID} eventName={event.query.data.name} parent={event.scope} paused={event.boundary.paused} />
}
function AdvisoryReader({ eventID, eventName, parent, paused }: { eventID: string; eventName: string; parent: PrivateRequestScope; paused: boolean }) {
 const identity = useIdentity(), readCurrent = useCallback((signal: AbortSignal) => load(eventID, signal, identity.person.id), [eventID, identity.person.id]), signature = useCallback((p: Page) => JSON.stringify([p.run_id, p.source_binding, p.targets.map(t => [t.tie_id, t.category_id, t.category_name, t.team_id, t.team_name, t.submission_revision])]), [])
 const read = useTodayRead(`${eventID}:mentor-advisories`, readCurrent, parent, signature, advisoryDenials)
 if (read.denied) return <section><h1>Mentor advisory unavailable</h1><p>Current published mentor assignment and a prepared frozen category-tie context are required. Private views and held reviews have been cleared.</p><Button icon="refresh" onClick={() => void read.refresh()}>Check current advisory access</Button></section>
 if (!read.data) return read.error ? <Failure error={read.error} retry={() => void read.refresh()} /> : <Loading />
 return <section className="fleet-results fleet-results-private"><ResultsLink icon="back" href={`/events/${eventID}/mentor`}>Back to mentor workspace</ResultsLink><header><p className="fleet-results-eyebrow">{eventName} · Your current assigned category ties</p><h1>Share mentor advisory views</h1><p>Offer observations for your own assigned frozen builds. Judges all vote and need a strict majority. Your views remain advisory evidence; they never add a vote or carry mentor scores or ranks into the final result.</p></header>{!!read.error && <Failure error={read.error} retry={() => void read.refresh()} />}<AdvisoryEditor key={read.generation} eventID={eventID} page={read.data} parent={read.authority} paused={paused || read.pending || !!read.error} refresh={() => read.refresh()} observeCurrent={read.observeCurrent} /><Button icon="refresh" variant="outline" disabled={read.pending} onClick={() => void read.refresh()}>Read current advisory assignments</Button></section>
}
function AdvisoryEditor({ eventID, page, parent, paused, refresh, observeCurrent }: { eventID: string; page: Page; parent: PrivateRequestScope; paused: boolean; refresh: () => Promise<unknown>; observeCurrent: (page: Page) => void }) {
 const identity = useIdentity(), [selected, setSelected] = useState(''), [views, setViews] = useState(''), [reason, setReason] = useState(''), [errors, setErrors] = useState<Record<string,string>>({}), form = useRef<HTMLFormElement>(null)
 const target = page.targets.find(t => key(t) === selected), dirty = !!views || !!reason
 const ready = (current: Page, body: Command) => {
  const target = current.targets.find(t => t.tie_id === body.tie_id && t.team_id === body.team_id)
  if (!target || current.run_id !== page.run_id || current.source_binding !== body.source_binding || target.submission_revision !== body.submission_revision || target.version !== body.version) throw new Error('Current coverage, frozen context or your contribution changed. Nothing was sent. Read current assignments and review again.')
 }
 const owner = decisionNamespace(identity.person.id, identity.draft_scope, eventID, 'mentor-advisories')
 const decision = useResultDecision({ owner, parent, paused, observeCurrent, page, revision: p => p.revision, version: p => p.targets.find(t => key(t) === selected)?.version ?? 0, read: signal => load(eventID, signal, identity.person.id), fingerprint: advisoryFingerprint, ready, metadata: body => ({ resourceID: page.run_id, subjectID: body.team_id, intentID: body.tie_id }), send: async (body: Command, revision) => unwrap(await api.POST('/api/v1/events/{event_id}/results/mentor-advisories', { params: { path: { event_id: eventID }, header: { 'If-Match': `"${revision}"`, 'X-CSRF-Token': identity.csrf_token } }, body })), matches: (result, intent) => !!result.receipt && matchesAdvisoryReceipt(result.receipt, intent), contextDenied: advisoryContextEnded, refresh, acknowledged: () => { setViews(''); setReason(''); setErrors({}) } })
 useBlocker({ shouldBlockFn: () => decision.pending || !decision.unknown && dirty && !window.confirm('Leave with unsent advisory views?'), enableBeforeUnload: dirty || decision.pending })
 async function review(source: HTMLElement) {
  if (!target) return
  const errors: Record<string,string> = {}, bytes = (v: string) => new TextEncoder().encode(v).length
  if (!views.trim() || bytes(views) > 4000) errors.views = 'Record your views from 1 to 4,000 bytes.'
  if (!reason.trim() || bytes(reason) > 2000) errors.reason = 'Record a reason from 1 to 2,000 bytes.'
  setErrors(errors); if (Object.keys(errors).length) { requestAnimationFrame(() => form.current && focusFirstInvalid(form.current)); return }
  const body: Command = { request_id: crypto.randomUUID(), action: 'advise', version: target.version, source_binding: page.source_binding, tie_id: target.tie_id, team_id: target.team_id, submission_revision: target.submission_revision, views, reason }
  await decision.prepare(body, null, source)
 }
 return <><div hidden={!!decision.review}><Field label="Your assigned team and category"><Select disabled={decision.blocked} value={selected} onChange={event => { if (dirty && !window.confirm('Discard these unsent views and choose another assigned category?')) return; setSelected(event.target.value); setViews(''); setReason(''); setErrors({}) }}><option value="">Choose an assigned category tie</option>{page.targets.map(t => <option key={key(t)} value={key(t)}>{t.team_name} · {t.category_name}</option>)}</Select></Field>{!page.targets.length && <p>No current all-team category ties involve your assigned teams. No advisory is required.</p>}</div>{target && <><section aria-label="Your contribution context"><h2>{target.team_name} · {target.category_name}</h2><p>Frozen submission {target.submission_revision} · Your contribution version {target.version}</p><ResultsLink icon="book" href={`/events/${eventID}/teams/${target.team_id}/build`}>Read assigned frozen build</ResultsLink><MentorAdvisoryReading items={target.own ? [target.own] : []} teamName={() => target.team_name} /></section><form ref={form} hidden={!!decision.review} className="fleet-results-form" onSubmit={event => { event.preventDefault(); const source = (event.nativeEvent as SubmitEvent).submitter; if (source instanceof HTMLElement) void review(source) }}><fieldset disabled={decision.blocked}><legend>{target.own ? 'Correct your own advisory contribution' : 'Add your own advisory contribution'}</legend><MentorAdvisoryFields views={views} reason={reason} changeViews={setViews} changeReason={setReason} errors={errors} /><Button icon="check" type="submit">Review advisory views</Button></fieldset></form></>}<ResultDecisionState {...decision} />{decision.review && <MentorAdvisoryReview team={page.targets.find(t => key(t) === key(decision.review!.body))!.team_name} category={page.targets.find(t => key(t) === key(decision.review!.body))!.category_name} views={decision.review.body.views} reason={decision.review.body.reason} previousVersion={decision.review.body.version} pending={decision.pending} disabled={decision.blocked && !decision.pending} confirm={() => void decision.confirm()} cancel={decision.cancel} returnFocus={decision.returnFocus}>{decision.stale && <p role="status">Current facts changed. Cancel this review and read current assignments.</p>}{!!decision.failure && <Failure error={decision.failure} />}</MentorAdvisoryReview>}</>
}

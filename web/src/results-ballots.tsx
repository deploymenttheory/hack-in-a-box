import { MentorAdvisoryReading } from './mentor-advisory-view'
import { useCallback, useRef, useState } from 'react'
import { useBlocker, useParams } from '@tanstack/react-router'
import { api, unwrap } from './api/client'
import { useIdentity } from './session'
import { EventBrand } from './branding'
import { useEventAuthority } from './event-authority'
import { useTodayRead } from './today-read'
import type { PrivateRequestScope } from './private-request-scope'
import { ballotFingerprint, matchesBallotReceipt, type BallotCommand } from './result-ballot-model'
import { decisionNamespace, OriginalResultHistory, ResultDecisionState, useResultDecision } from './results-decision'
import { Button } from './components/ui/button'
import { ResultsLink as ActionLink } from './results-link'
import { BallotReview, BallotTasks } from './results-view'
import { Field, Select, Textarea, focusFirstInvalid } from './components/ui/fields'
import { Failure, Loading } from './components/states'
import './results.css'
const loadBallots = async (eventID: string, signal: AbortSignal, actorID: string) => {
 const page = unwrap(await api.GET('/api/v1/events/{event_id}/results/ballots', { signal, cache: 'no-store', params: { path: { event_id: eventID } } }))
 if (!page.judge_run_id || page.judge_run_id === '00000000-0000-0000-0000-000000000000' || page.ties.some(t => t.own_vote && (t.own_vote.judge_id !== actorID || t.own_vote.tie_id !== t.state.tie.id || !t.state.tie.team_ids.includes(t.own_vote.team_id) || t.own_vote.evidence.some(e => !t.state.tie.team_ids.includes(e.team_id))))) throw new Error('The current ballot projection did not match your own assigned vote.')
 const candidates = [...new Set(page.ties.flatMap(t => t.state.tie.team_ids))]
 const briefs = await Promise.all(candidates.map(async teamID => {
  const value = unwrap(await api.GET('/api/v1/events/{event_id}/judging/{run_id}/teams/{team_id}', { signal, cache: 'no-store', params: { path: { event_id: eventID, run_id: page.judge_run_id, team_id: teamID } } }))
  if (value.run_id !== page.judge_run_id || value.team_id !== teamID) throw new Error('The current assigned candidate brief did not match the native ballot source.')
  return { id: teamID, name: value.team_name, categories: value.criteria.categories.map(c => ({ id: c.id, label: c.label })), reference: value.reference, revision: value.submission_revision, repository: value.repository, evidence: value.submission.evidence.map(e => ({ id: e.deliverable_id, label: e.label })) }
 }))
 const categories = briefs[0]?.categories ?? []
 if (briefs.some(b => !b.name.trim() || JSON.stringify(b.categories) !== JSON.stringify(categories) || JSON.stringify(b.reference) !== JSON.stringify(briefs[0].reference)) || page.ties.some(t => t.state.tie.kind === 'category' && !categories.some(c => c.id === t.state.tie.category_id))) throw new Error('The published categories did not match the current assigned ballot source.')
 return { page, briefs, categories }
}
type BallotState = Awaited<ReturnType<typeof loadBallots>>
export function ResultsBallotsPage() {
 const { eventId } = useParams({ strict: false }), identity = useIdentity(), owner = decisionNamespace(identity.person.id, identity.draft_scope, eventId!, 'result-ballots')
 return <><OriginalResultHistory owner={owner} read={async (requestID, signal) => unwrap(await api.GET('/api/v1/events/{event_id}/results/requests/{request_id}', { signal, cache: 'no-store', params: { path: { event_id: eventId!, request_id: requestID } } }))} matches={(result, intent) => !!result.receipt && matchesBallotReceipt(result.receipt, intent)} /><EventBrand eventID={eventId!}><BallotEvent eventID={eventId!} /></EventBrand></>
}
function BallotEvent({ eventID }: { eventID: string }) {
 const event = useEventAuthority(eventID)
 if (event.denial || event.query.error && !event.verified) return <Failure error={event.denial ?? event.query.error} retry={() => void event.refresh()} />
 if (!event.verified || !event.query.data) return <Loading />
 return <BallotReader key={event.generation} eventID={eventID} eventName={event.query.data.name} parent={event.scope} paused={event.boundary.paused} />
}
function BallotReader({ eventID, eventName, parent, paused }: { eventID: string; eventName: string; parent: PrivateRequestScope; paused: boolean }) {
 const identity = useIdentity(), load = useCallback((signal: AbortSignal) => loadBallots(eventID, signal, identity.person.id), [eventID, identity.person.id])
 const signature = useCallback((v: BallotState) => JSON.stringify([v.page.source_binding, v.page.judge_run_id, v.page.ties.map(t => t.state.tie), v.briefs]), [])
 const read = useTodayRead(`${eventID}:judge-ballots`, load, parent, signature)
 if (read.denied) return <section><h1>Tie ballots unavailable</h1><p>Current judge assignment is required. Private candidate evidence and held reviews have been cleared.</p><Button icon="refresh" onClick={() => void read.refresh()}>Check current ballot access</Button></section>
 if (!read.data) return read.error ? <Failure error={read.error} retry={() => void read.refresh()} /> : <Loading />
 return <section className="fleet-results fleet-results-private"><ActionLink icon="back" href={`/events/${eventID}/judging`}>Back to judge desk</ActionLink><header><p className="fleet-results-eyebrow">{eventName} · Your assigned judge ballots</p><h1>Review assigned ties</h1><p>Every required judge must vote. A strict majority is required; a split requires an explicit reviewed replacement vote.</p></header>{!!read.error && <Failure error={read.error} retry={() => void read.refresh()} />}<BallotEditor key={read.generation} eventID={eventID} state={read.data} parent={read.authority} paused={paused || read.pending || !!read.error} refresh={() => read.refresh()} /><Button icon="refresh" variant="outline" disabled={read.pending} onClick={() => void read.refresh()}>Read current tie ballots</Button></section>
}
function BallotEditor({ eventID, state, parent, paused, refresh }: { eventID: string; state: BallotState; parent: PrivateRequestScope; paused: boolean; refresh: () => Promise<unknown> }) {
 const identity = useIdentity(), [selected, setSelected] = useState(''), [winner, setWinner] = useState(''), [reason, setReason] = useState(''), [notes, setNotes] = useState<Record<string, string>>({}), [artifacts, setArtifacts] = useState<Record<string, string>>({}), [errors, setErrors] = useState<Record<string, string>>({}), form = useRef<HTMLFormElement>(null), selectedHeading = useRef<HTMLHeadingElement>(null)
 const ties = state.page.ties, tie = ties.find(t => t.state.tie.id === selected)
 const names = (id: string) => state.briefs.find(b => b.id === id)?.name ?? `Team ${id}`
 const label = (item: typeof ties[number]) => item.state.tie.kind === 'final' ? 'Overall final' + (ties.filter(t => t.state.tie.kind === 'final').length > 1 ? ' · ' + item.state.tie.team_ids.map(names).join(' / ') : '') : state.categories.find(c => c.id === item.state.tie.category_id)!.label + ' category'
 const choose = (id: string) => {
  if (decision.blocked || decision.review || dirty && !window.confirm('Discard this unsent ballot and choose another tie?')) return
  setSelected(id); setWinner(''); setReason(''); setNotes({}); setArtifacts({}); setErrors({}); requestAnimationFrame(() => selectedHeading.current?.focus())
 }
 const ready = (current: BallotState, body: BallotCommand) => {
  const target = current.page.ties.find(t => t.state.tie.id === body.tie_id)
  if (!target || current.page.judge_run_id !== state.page.judge_run_id || current.page.source_binding !== body.source_binding || body.version !== current.page.version || !target.state.tie.team_ids.includes(body.team_id) || JSON.stringify(current.briefs) !== JSON.stringify(state.briefs)) throw new Error('The assigned candidates or frozen evidence changed. Nothing was sent. Read current ballots and review again.')
 }
 const owner = decisionNamespace(identity.person.id, identity.draft_scope, eventID, 'result-ballots')
 const decision = useResultDecision({ owner, parent, paused, page: state, revision: value => value.page.revision, version: value => value.page.version, read: signal => loadBallots(eventID, signal, identity.person.id), fingerprint: ballotFingerprint, ready, metadata: body => ({ resourceID: state.page.judge_run_id, subjectID: body.team_id, intentID: JSON.stringify({ tie: body.tie_id, teams: tie!.state.tie.team_ids }) }), send: async (body: BallotCommand, revision) => unwrap(await api.POST('/api/v1/events/{event_id}/results/ballots', { params: { path: { event_id: eventID }, header: { 'If-Match': `"${revision}"`, 'X-CSRF-Token': identity.csrf_token } }, body })), matches: (result, intent) => !!result.receipt && matchesBallotReceipt(result.receipt, intent), refresh, acknowledged: () => { setWinner(''); setReason(''); setNotes({}); setArtifacts({}) } })
 const dirty = !!winner || !!reason || Object.values(notes).some(Boolean) || Object.values(artifacts).some(Boolean)
 useBlocker({ shouldBlockFn: () => decision.pending || !decision.unknown && dirty && !window.confirm('Leave with unsent tie evidence or reason?'), enableBeforeUnload: dirty || decision.pending })
 async function review(source: HTMLElement) {
  if (!tie) return
  const errors: Record<string, string> = {}, bytes = (s: string) => new TextEncoder().encode(s).length
  if (!tie.state.tie.team_ids.includes(winner)) errors.winner = 'Select a current assigned candidate.'
  if (!reason.trim() || bytes(reason) > 2000) errors.reason = 'Record a reason from 1 to 2,000 bytes.'
  const evidence = tie.state.tie.team_ids.flatMap(id => {
   const note = notes[id] ?? '', deliverable = artifacts[id] ?? ''
   if (bytes(note) > 4000) errors[id] = 'Use at most 4,000 bytes for this observed evidence.'
   if (!note.trim() && !deliverable) return []
   return [{ team_id: id, deliverable_id: deliverable || null, note }]
  })
  if (!evidence.length) errors.evidence = 'Reference a current frozen deliverable or record your observed evidence.'
  setErrors(errors); if (Object.keys(errors).length) { requestAnimationFrame(() => form.current && focusFirstInvalid(form.current)); return }
  const body: BallotCommand = { request_id: crypto.randomUUID(), version: state.page.version, action: 'vote', source_binding: state.page.source_binding, tie_id: tie.state.tie.id, team_id: winner, evidence, reason }
  await decision.prepare(body, <ul>{evidence.map(e => <li key={e.team_id}>{names(e.team_id)} · {e.deliverable_id ? state.briefs.find(b => b.id === e.team_id)?.evidence.find(a => a.id === e.deliverable_id)?.label : 'Own observed evidence'} · {e.note}</li>)}</ul>, source)
 }
 return <><div hidden={!!decision.review}><BallotTasks items={ties.map(item => ({ id: item.state.tie.id, label: label(item), progress: `${item.state.votes_received}/${item.state.required_judge_ids.length} required ballots`, outcome: item.state.winner_id ? `Recorded winner: ${names(item.state.winner_id)}` : item.state.blocker.replaceAll('_', ' '), ownVote: !!item.own_vote }))} selected={selected} disabled={decision.blocked} choose={choose} /></div>{tie ? <><section aria-label="Selected tie"><h2 ref={selectedHeading} tabIndex={-1}>{label(tie)}</h2><p>{tie.state.votes_received}/{tie.state.required_judge_ids.length} required ballots · {tie.state.winner_id ? `Recorded winner: ${names(tie.state.winner_id)}` : tie.state.blocker.replaceAll('_', ' ')}</p>{tie.own_vote && <details><summary>Your currently recorded vote</summary><p>{names(tie.own_vote.team_id)} · {tie.own_vote.reason}</p><ul>{tie.own_vote.evidence.map((e, i) => <li key={i}>{names(e.team_id)} · {e.note}</li>)}</ul></details>}</section>{tie.state.tie.kind === 'category' && <MentorAdvisoryReading items={tie.advisories ?? []} teamName={names} />}<form ref={form} hidden={!!decision.review} className="fleet-results-form" onSubmit={event => { event.preventDefault(); const button = (event.nativeEvent as SubmitEvent).submitter; if (button instanceof HTMLElement) void review(button) }}><fieldset disabled={decision.blocked}><legend>{tie.own_vote ? 'Explicit replacement ballot' : 'Your tie ballot'}</legend>{tie.state.tie.team_ids.map(id => { const brief = state.briefs.find(b => b.id === id)!; return <section key={id}><h3>{brief.name}</h3><p>Frozen final revision {brief.revision} · Code evidence {brief.repository.status === 'not_applicable' ? 'File/URL-only final' : brief.repository.status.replaceAll('_', ' ')}</p><ActionLink icon="book" href={`/events/${eventID}/teams/${id}/build?judge_run=${state.page.judge_run_id}`}>Read {brief.name} frozen build</ActionLink><Field label={`Frozen evidence for ${brief.name}`}><Select value={artifacts[id] ?? ''} onChange={event => setArtifacts(v => ({ ...v, [id]: event.target.value }))}><option value="">Use my own observation</option>{brief.evidence.map(e => <option key={e.id} value={e.id}>{e.label}</option>)}</Select></Field><Field label={`Your observed evidence for ${brief.name}`} error={errors[id] ?? errors.evidence}><Textarea maxLength={4000} value={notes[id] ?? ''} onChange={event => setNotes(v => ({ ...v, [id]: event.target.value }))} /></Field></section> })}<Field label="Your selected winner" error={errors.winner}><Select value={winner} onChange={event => setWinner(event.target.value)}><option value="">Choose a candidate</option>{tie.state.tie.team_ids.map(id => <option key={id} value={id}>{names(id)}</option>)}</Select></Field><Field label="Reason for your tie vote" error={errors.reason}><Textarea maxLength={2000} value={reason} onChange={event => setReason(event.target.value)} /></Field><Button icon="check" type="submit">Review tie vote</Button></fieldset></form>{ties.indexOf(tie) + 1 < ties.length && <Button icon="next" variant="outline" disabled={decision.blocked || !!decision.review} onClick={() => choose(ties[ties.indexOf(tie) + 1].state.tie.id)}>Review next assigned tie</Button>}</> : <p>{ties.length ? 'Choose an assigned tie to review its current candidates and frozen evidence.' : 'No current assigned ties require a ballot.'}</p>}<ResultDecisionState {...decision} />{decision.review && <BallotReview tieLabel={label(ties.find(item => item.state.tie.id === decision.review!.body.tie_id)!)} winner={names(decision.review.body.team_id)} reason={decision.review.body.reason} pending={decision.pending} disabled={decision.blocked && !decision.pending} confirm={() => void decision.confirm()} cancel={decision.cancel} returnFocus={decision.returnFocus}>{decision.review.summary}{decision.stale && <p role="status">The starting point changed. Cancel this review, read current facts and review a fresh decision.</p>}{decision.failureStage === 'before_write' && <p role="alert">Current access, source or starting point could not be confirmed. Nothing was sent. Cancel this review, read current facts and review again.</p>}{!!decision.failure && <Failure error={decision.failure} />}</BallotReview>}</>
}

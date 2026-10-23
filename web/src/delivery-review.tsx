import { useEffect, useState } from 'react'
import { Link, useBlocker } from '@tanstack/react-router'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, APIError, unwrap } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { localDate } from './event-time'
import { actionName, denied, stateName, uncertain, useCurrentDelivery, useDelivery, useDeliveryDenial, type Action, type Decision, type Detail } from './delivery-context'
import { DeliverySummary } from './delivery-summary'

export function DeliveryReview({ id }: { id: string }) {
  const { event, key } = useDelivery(), cache = useQueryClient(), [beforeAttempt, setBeforeAttempt] = useState<number>(), [beforeReview, setBeforeReview] = useState<string>()
  const [leaveRisk, setLeaveRisk] = useState(false)
  function changeEvidence(change: () => void) { if (!leaveRisk || window.confirm('Leave this private unsent recovery reason or uncertain decision to change history pages? It will be cleared.')) change() }
  const query = useQuery({ queryKey: [...key, 'detail', id, beforeAttempt, beforeReview], gcTime: 0, retry: false, refetchInterval: 10_000, refetchOnWindowFocus: 'always',
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/events/{event_id}/communications/deliveries/{delivery_id}', { signal, cache: 'no-store', params: { path: { event_id: event.id, delivery_id: id }, query: { before_attempt: beforeAttempt, before_review: beforeReview } } })) })
  useDeliveryDenial(query.error)
  if (denied(query.error)) return null
  if (!query.data) return query.error ? <Failure error={query.error} retry={() => void query.refetch()} /> : <Loading />
  const detail = query.data
  return <><Link to="/events/$eventId/communications/deliveries" params={{ eventId: event.id }}>← All delivery selections</Link><section className="panel"><h2>{detail.item.recipient_name || (detail.item.channel === 'teams' ? 'Shared event channel' : 'Retained recipient')}</h2><DeliverySummary item={detail.item} /><p>Recipient and message identity are retained from the original intent. Recovery never changes its recipient, body, calendar or campaign version.</p>{detail.item.state === 'unavailable' && <p>No email attempt was made. A later verified address does not silently deliver this old selection; use the reviewed occurrence workflow.</p>}</section>
    {query.error && <Failure error={query.error} retry={() => void query.refetch()} />}
    <RecoveryForm key={`${id}:${beforeAttempt ?? 0}:${beforeReview ?? 'latest'}`} detail={detail} readFresh={!query.error} onLeaveRisk={setLeaveRisk} receive={fresh => { cache.setQueryData([...key, 'detail', id, beforeAttempt, beforeReview], fresh) }} reload={async () => { const result = await query.refetch(); if (result.error) throw result.error; return result.data! }} />
    <section className="panel"><h2>Immutable provider attempt history</h2>{!detail.attempts.length && <p>No provider attempt has been prepared. In-app persistence and unavailable-recipient evidence do not imply an external send.</p>}<ol>{detail.attempts.map(attempt => <li key={attempt.id}><h3>Attempt {attempt.number}: {stateName(attempt.state)}</h3><p>Prepared {localDate(attempt.started_at, event.configuration.timezone)}</p><p>{attempt.reason.replaceAll('_', ' ')}{attempt.http_status && ` · HTTP ${attempt.http_status}`}</p><p>{attempt.recorded_at ? `Receipt recorded ${localDate(attempt.recorded_at, event.configuration.timezone)}` : 'No durable receipt recorded; acceptance remains uncertain.'}</p>{attempt.retry_at && <p>Provider backoff until {localDate(attempt.retry_at, event.configuration.timezone)}</p>}</li>)}</ol>{detail.next_attempt_number && <Button variant="outline" onClick={() => changeEvidence(() => setBeforeAttempt(detail.next_attempt_number!))}>Older provider attempts</Button>}{beforeAttempt && <Button variant="outline" onClick={() => changeEvidence(() => setBeforeAttempt(undefined))}>Latest provider attempts</Button>}</section>
    <section className="panel"><h2>Recorded operator decisions</h2><p>These decisions are separate from provider evidence. Resolving without resend never manufactures a successful receipt.</p><ol>{detail.reviews.map(review => <li key={review.id}><h3>{actionName[review.action]}</h3><p>{localDate(review.created_at, event.configuration.timezone)} · Operator {review.actor_id}</p><p className="delivery-reason">{review.reason}</p></li>)}</ol>{!detail.reviews.length && <p>No operator recovery decision recorded.</p>}{detail.next_review_id && <Button variant="outline" onClick={() => changeEvidence(() => setBeforeReview(detail.next_review_id!))}>Older operator decisions</Button>}{beforeReview && <Button variant="outline" onClick={() => changeEvidence(() => setBeforeReview(undefined))}>Latest operator decisions</Button>}</section>
  </>
}

function RecoveryForm({ detail, readFresh, reload, receive, onLeaveRisk }: { detail: Detail; readFresh: boolean; reload: () => Promise<Detail>; receive: (fresh: Detail) => void; onLeaveRisk: (risk: boolean) => void }) {
  const identity = useIdentity(), { event, key, deny } = useDelivery(), cache = useQueryClient(), active = useCurrentDelivery()
  const [action, setAction] = useState<Action | ''>(''), [reason, setReason] = useState(''), [acknowledged, setAcknowledged] = useState(false), [busy, setBusy] = useState(false), [notice, setNotice] = useState(''), [failure, setFailure] = useState<unknown>(), [pending, setPending] = useState<{ input: Decision; revision: number }>()
  const leaveRisk = !!reason || !!pending || busy
  useEffect(() => { onLeaveRisk(leaveRisk); return () => onLeaveRisk(false) }, [leaveRisk, onLeaveRisk])
  useBlocker({ shouldBlockFn: () => (!!reason || !!pending) && !window.confirm('Leave this private recovery reason or uncertain decision? It is retained only in this page and will be cleared.'), enableBeforeUnload: !!reason || !!pending })
  const valid = !!reason.trim() && new TextEncoder().encode(reason.trim()).length <= 2000 && !reason.includes('\0')
  const allowed = !!action && detail.actions.includes(action)
  async function send(retry = false) {
    if (busy || (!retry && (!readFresh || !allowed || !valid || (action === 'resend_acknowledged' && !acknowledged)))) return
    const retained = retry ? pending : undefined
    if (retry && !retained) return
    setBusy(true); setFailure(undefined); setNotice('')
    let attempted: { input: Decision; revision: number } | undefined
    try {
      if (!retained) {
        const current = await reload()
        if (!active.current) return
        if (current.item.revision !== detail.item.revision || !current.actions.includes(action as Action)) { setNotice('Current delivery policy or revision changed. Review the latest evidence and choose an allowed action before another decision.'); return }
      }
      attempted = retained ?? { input: { request_id: crypto.randomUUID(), action: action as Action, reason: reason.trim() }, revision: detail.item.revision }
      const fresh = unwrap(await api.POST('/api/v1/events/{event_id}/communications/deliveries/{delivery_id}', { params: { path: { event_id: event.id, delivery_id: detail.item.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${attempted.revision}"` } }, body: attempted.input }))
      if (!active.current) return
      receive(fresh); setPending(undefined); setReason(''); setAcknowledged(false); setAction('')
      setNotice(attempted.input.action === 'resend_acknowledged' ? 'The explicit resend decision was recorded. At most one additional attempt is authorised; current delivery policy is checked again before any effect. This is not a delivery receipt.' : attempted.input.action === 'resolve_no_resend' ? 'The no-resend decision was recorded. The original uncertain outcome remains uncertain; no additional send was requested.' : attempted.input.action === 'cancel_resend' ? 'Only the queued resend was cancelled. Earlier provider outcomes remain retained and were not recalled.' : attempted.input.action === 'cancel' ? 'Pending work was stopped. No earlier provider effect was recalled.' : 'The reviewed retry was queued. No provider acceptance or delivery is confirmed by this action.')
      void cache.invalidateQueries({ queryKey: [...key, 'detail'] })
    } catch (error) {
      if (!active.current) return
      setFailure(error); deny(error)
      if (attempted && uncertain(error)) { setPending(attempted); setNotice('Outcome unknown. No action will be repeated automatically. An explicit retry uses this exact decision and cannot grant an extra resend.') }
      else if (error instanceof APIError && error.status === 409) setNotice('No new decision was confirmed. Refresh current evidence; your reason remains unsent in this page.')
      else setNotice('No new recovery decision was confirmed. Your reason remains only in this page.')
    } finally { if (active.current) setBusy(false) }
  }
  async function refreshEvidence() {
    setFailure(undefined)
    try {
      const fresh = await reload()
      if (!active.current) return
      const retained = pending && fresh.reviews.find(review => review.id === pending.input.request_id && review.actor_id === identity.person.id && review.action === pending.input.action && review.reason === pending.input.reason && review.expected_revision === pending.revision)
      if (retained) { setPending(undefined); setReason(''); setAction(''); setAcknowledged(false); setNotice('The server confirms the exact recorded decision. No action was replayed; provider outcomes remain separate evidence.') }
      else setNotice(pending ? 'The current readable history does not confirm this exact decision. Its outcome remains unknown; no action was replayed.' : 'Current delivery evidence refreshed. Your reason remains unsent until explicitly recorded.')
    } catch (error) { if (active.current) { setFailure(error); deny(error) } }
  }
  return <section className="panel delivery-private"><h2>Explicit recovery decision</h2>{detail.paused && <p className="notice">Automation is paused. Retry and resend require current permission; cancellation and no-resend resolution are separate controls.</p>}{!detail.actions.length && !pending && <p>No recovery action is currently authorised for this evidence. Provider acceptance, local inbox persistence and unavailable recipients do not imply a safe generic retry.</p>}
    {action === 'retry' && <p>Retry schedules the existing delivery after a definite rejection. Its original approval must remain valid; this does not authorise another send after acceptance or an uncertain outcome.</p>}
    <p role="status">{pending ? 'Outcome unknown — no automatic replay' : reason ? 'Unsent private decision reason' : 'No unsent recovery decision'}</p><form onSubmit={e => { e.preventDefault(); void send() }}><fieldset disabled={busy || !!pending || !readFresh || detail.archived}><legend>Reviewed action and reason</legend><label htmlFor="delivery-action">Recovery action</label><select id="delivery-action" value={action} onChange={e => { setAction(e.target.value as Action | ''); setAcknowledged(false) }}><option value="">Choose an allowed current action</option>{detail.actions.map(value => <option key={value} value={value}>{actionName[value]}</option>)}</select><label htmlFor="delivery-reason">Private operator reason</label><textarea id="delivery-reason" rows={4} maxLength={2000} value={reason} onChange={e => setReason(e.target.value)} /><p>{new TextEncoder().encode(reason.trim()).length} / 2,000 UTF-8 bytes. Do not include addresses, credentials or message contents.</p>{action === 'resend_acknowledged' && <label className="check"><input type="checkbox" checked={acknowledged} onChange={e => setAcknowledged(e.target.checked)} />I explicitly authorise one additional attempt to the original recipient or shared channel and accept the risk of a duplicate, including when the original provider accepted it.</label>}<Button type="submit" disabled={!allowed || !valid || (action === 'resend_acknowledged' && !acknowledged)}>Record explicit decision</Button></fieldset></form>{pending && <Button disabled={busy || detail.archived} onClick={() => void send(true)}>Retry the same recovery decision</Button>}<Button variant="outline" disabled={busy} onClick={() => void refreshEvidence()}>Refresh current delivery evidence</Button>{notice && <p role="status">{notice}</p>}{!!failure && <Failure error={failure} />}</section>
}

import { repositoryIntentFingerprint, repositoryReason } from './repository-fingerprint'
import { useRepositoryDraft } from './repository-drafts'
import { RepositoryActionReview, RepositoryPlan } from './repository-review'
import { useRef, useState } from 'react'
import { api, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { RepositoryProjectionMismatch, useRepositoryRead } from './repository-read'
import { RepositoryCommandStatus, useRepositoryCommand } from './repository-command'
import { Button } from './components/ui/button'
import { Field, Textarea } from './components/ui/fields'
import { StatusBadge } from './components/ui/feedback'
import { Failure, Loading } from './components/states'
class OriginalReceiptIssue extends Error {}
type S = components['schemas']
type RepositoryLabels = { eventName: string; teamName?: string; repositoryPath: string }
type Reviewed = { labels: RepositoryLabels; body: S['RepositoryAccessCommand']; revision: number; plan: S['RepositoryAccessPlan'] | null; repositoryID: number; intentFingerprint?: string }
export function RepositoryAccess({ eventID, teamID, repositoryID, labels, writable, unused = false }: { eventID: string; teamID: string; repositoryID: number; labels: RepositoryLabels; writable: boolean; unused?: boolean }) {
  const owner = useRepositoryRead(eventID, `access:${teamID}`, async signal => { const value = unwrap(await api.GET('/api/v1/events/{event_id}/repositories/{team_id}/access', { signal, cache: 'no-store', params: { path: { event_id: eventID, team_id: teamID } } })); if (value.team_id !== teamID || (value.plan && (value.plan.team_id !== teamID || value.plan.event_id !== eventID || value.plan.repository_id !== repositoryID))) throw new RepositoryProjectionMismatch('The current repository access projection no longer matches this team and repository.'); return value })
  return <section className="fleet-repository-section"><h3>Repository access</h3>{owner.denied ? <><Failure error={owner.denied} /><Button icon="refresh" onClick={owner.check}>Check current access review</Button></> : owner.query.isPending ? <Loading /> : owner.query.error && !owner.query.data ? <Failure error={owner.query.error} retry={() => void owner.query.refetch()} /> : <>{owner.query.error ? <Failure error={owner.query.error} retry={() => void owner.query.refetch()} /> : null}<AccessEditor key={`${writable}:${owner.query.data!.can_manage}`} {...{ eventID, teamID, repositoryID, labels, writable, unused }} owner={owner} value={owner.query.data!} /></>}</section>
}
function AccessEditor({ eventID, teamID, repositoryID, labels, writable, unused, owner, value }: { eventID: string; teamID: string; repositoryID: number; labels: RepositoryLabels; writable: boolean; unused: boolean; owner: ReturnType<typeof useRepositoryRead<S['RepositoryAccessView']>>; value: S['RepositoryAccessView'] }) {
  const identity = useIdentity(), operation = useRepositoryCommand(eventID, `access:${teamID}`, owner.scope, owner.deny)
  const preparing = useRef(false)
  const [checking, setChecking] = useState<string>(), [preparationPending, setPreparationPending] = useState(false), [preparationFailure, setPreparationFailure] = useState<unknown>()
  const [reason, setReason] = useState(''), [review, setReview] = useState<Reviewed>(), [retained, setRetained] = useState<Reviewed>(), [receipt, setReceipt] = useState<S['RepositoryAccessPlan']>()
  const opener = useRef<HTMLElement | null>(null), readControl = useRef<HTMLButtonElement>(null), allowed = writable && value.can_manage
  useRepositoryDraft(!!reason || !!review || operation.blocked)
  const stale = !!review && (review.revision !== value.revision || review.repositoryID !== repositoryID || (review.body.action === 'approve' && (value.plan?.id !== review.body.plan_id || value.plan?.version !== review.body.plan_version || value.plan?.fingerprint !== review.plan?.fingerprint)))
  async function prepare(action: S['RepositoryAccessCommand']['action'], control: HTMLElement) {
    if (preparing.current || operation.blocked || !allowed || owner.paused) return
    preparing.current = true; setPreparationPending(true); setPreparationFailure(undefined)
    const ticket = owner.scope.capture(); opener.current = control
    const body: S['RepositoryAccessCommand'] = { action, reason: repositoryReason(reason), ...(action === 'review' ? { plan_id: crypto.randomUUID() } : action === 'approve' && value.plan ? { plan_id: value.plan.id, plan_version: value.plan.version } : {}) }
    const captured: Reviewed = structuredClone({ labels, body, revision: value.revision, repositoryID, plan: value.plan })
    try {
      if (action !== 'refresh') captured.intentFingerprint = await repositoryIntentFingerprint({ action, actorID: identity.person.id, eventID, teamID, repositoryID, planID: body.plan_id!, version: body.plan_version, reason: body.reason })
      if (ticket.current()) setReview(captured)
    } catch (error) { if (ticket.current()) setPreparationFailure(error) }
    finally { preparing.current = false; if (ticket.current()) setPreparationPending(false) }
  }

  function correct() { setReview(undefined); requestAnimationFrame(() => { if (opener.current?.isConnected && !(opener.current instanceof HTMLButtonElement && opener.current.disabled)) opener.current.focus(); else readControl.current?.focus() }) }
  async function send(intent: Reviewed, originalID?: string) {
    if (!allowed || owner.paused || (!originalID && stale)) return
    setRetained(intent)
    await operation.send(intent.body.action, intent.revision, async () => {
      const result = unwrap(await api.POST('/api/v1/events/{event_id}/repositories/{team_id}/access', { cache: 'no-store', params: { path: { event_id: eventID, team_id: teamID }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${intent.revision}"` } }, body: intent.body }))
      if (result.current.team_id !== teamID || (intent.body.action !== 'refresh' && (!result.receipt || result.receipt.id !== intent.body.plan_id || result.receipt.team_id !== teamID || result.receipt.event_id !== eventID || result.receipt.repository_id !== intent.repositoryID || (intent.body.action === 'review' ? result.receipt.reason !== intent.body.reason : result.receipt.version !== intent.body.plan_version || result.receipt.approval_reason !== intent.body.reason || !result.receipt.approved_at || result.receipt.approved_by !== identity.person.id)))) throw new Error('The original access receipt could not be verified.')
      if (intent.body.action !== 'refresh' && result.receipt) {
        const fingerprint = await repositoryIntentFingerprint({ action: intent.body.action, actorID: identity.person.id, eventID, teamID, repositoryID: result.receipt.repository_id, planID: result.receipt.id, version: result.receipt.version, reason: intent.body.action === 'review' ? result.receipt.reason : result.receipt.approval_reason })
        if (fingerprint !== intent.intentFingerprint) throw new Error('The returned original command identity does not match the captured review.')
      }
      return result
    }, result => { setReceipt(intent.body.action === 'refresh' ? undefined : result.receipt ?? undefined); setRetained(undefined); setReview(undefined); setReason(''); void owner.query.refetch() }, intent.body.plan_id ? { planID: intent.body.plan_id, version: intent.body.plan_version, originalID, intentFingerprint: intent.intentFingerprint } : undefined)
  }
  async function readOriginal(attempt: (typeof operation.history.attempts)[number]) {
    if (checking || !attempt.command.resourceID || !attempt.command.intentFingerprint || !['review', 'approve'].includes(attempt.command.action)) return
    const ticket = owner.scope.capture(); setChecking(attempt.id); setPreparationFailure(undefined)
    try {
      const original = unwrap(await api.GET('/api/v1/events/{event_id}/repositories/{team_id}/access/receipts/{plan_id}', { signal: ticket.signal, cache: 'no-store', params: { path: { event_id: eventID, team_id: teamID, plan_id: attempt.command.resourceID }, query: { action: attempt.command.action as 'review' | 'approve' } } }))
      ticket.assert()
      const plan = original.plan
      if (!plan || !original.command_fingerprint) { setPreparationFailure(new OriginalReceiptIssue('No matching original receipt is available. The original outcome remains unknown; absence does not permit replacement.')); return }
      if (original.action !== attempt.command.action || original.actor_id !== identity.person.id || plan.id !== attempt.command.resourceID || plan.event_id !== eventID || plan.team_id !== teamID || plan.repository_id !== repositoryID || original.command_fingerprint !== attempt.command.intentFingerprint || (original.action === 'approve' && (plan.version !== attempt.command.expectedVersion || plan.approved_by !== identity.person.id || !plan.approved_at))) throw new OriginalReceiptIssue('The original receipt identity did not match. Replacement remains blocked.')
      const computed = await repositoryIntentFingerprint({ action: original.action, actorID: original.actor_id, eventID, teamID, repositoryID: plan.repository_id, planID: plan.id, version: plan.version, reason: original.action === 'review' ? plan.reason : plan.approval_reason })
      ticket.assert()
      if (computed !== attempt.command.intentFingerprint) throw new OriginalReceiptIssue('The original receipt body did not match. Replacement remains blocked.')
      operation.history.complete(attempt.id); setReceipt(plan); setReview(undefined); setRetained(undefined); setReason(''); void owner.query.refetch()
    } catch (error) { if (ticket.current()) { setPreparationFailure(error); if (error instanceof Error && 'status' in error && [401, 403, 404].includes(Number(error.status))) owner.deny(error) } }
    finally { if (ticket.current()) setChecking(undefined) }
  }
  const unresolved = operation.history.attempts.find(attempt => attempt.blocks && attempt.state === 'unknown')
  return <><p>{unused ? 'This repository is unused and unscored. Historical access and original receipts may be read; no new external access is authorised.' : value.grants_open ? 'Current published membership may receive reviewed access.' : 'New access grants are currently closed. No action here changes team allocation or the freeze cutoff.'}</p>
    <ul>{value.flags.map((flag, index) => <li key={index}>{flag.code.replaceAll('_', ' ')}{flag.person_id ? ` · Person ${flag.person_id}` : ''}{flag.github_id ? ` · GitHub ID ${flag.github_id}` : ''}</li>)}</ul>
    <Button ref={readControl} icon="refresh" variant="outline" onClick={() => void owner.query.refetch()}>Read current access facts</Button><h4>Recorded invitations and access</h4>{!value.grants.length && <p>No recorded grants.</p>}<ul>{value.grants.map(grant => <li key={grant.id}>Person {grant.member.person_id} · GitHub ID {grant.member.github_id} · {grant.member.permission === 'push' ? 'Write' : 'Read'} · <StatusBadge>{grant.state}</StatusBadge>{grant.effect_uncertain && <strong> · Provider effect unknown; do not replace</strong>} · {grant.diagnostic || 'No diagnostic reported'}{grant.invitation_id ? ` · Invitation ${grant.invitation_id}` : ''}</li>)}</ul>
    <h4>Current planned effects</h4>{value.plan ? <RepositoryPlan plan={value.plan} /> : <p>No retained access plan. Review current provider access and published membership before approval.</p>}
    <ul>{value.execution.map(effect => <li key={effect.action_id}>Action {effect.action_id} · {effect.state} · {effect.attempted ? 'Provider attempt recorded' : 'Not attempted'} · {effect.diagnostic || 'No diagnostic reported'}</li>)}</ul>
    {allowed && <><div hidden={!!review}><Field label="Reason for repository access change"><Textarea value={reason} onChange={e => setReason(e.target.value)} maxLength={2000} disabled={operation.blocked || owner.paused} /></Field><div className="fleet-pattern-actions"><Button icon="refresh" variant="outline" disabled={owner.paused || preparationPending || operation.blocked || !repositoryReason(reason)} onClick={e => prepare('refresh', e.currentTarget)}>Review provider observation refresh</Button><Button icon="list" variant={value.plan && !value.plan.approved_at && !value.flags.some(flag => flag.code === 'plan_stale') ? 'outline' : 'default'} disabled={owner.paused || preparationPending || operation.blocked || !repositoryReason(reason)} onClick={e => prepare('review', e.currentTarget)}>Review current membership reconciliation</Button><Button icon="check" disabled={owner.paused || preparationPending || operation.blocked || !repositoryReason(reason) || !value.plan || !!value.plan.approved_at || value.flags.some(flag => flag.code === 'plan_stale')} onClick={e => prepare('approve', e.currentTarget)}>Review exact access approval</Button></div></div>
    {review && <RepositoryActionReview correctDisabled={operation.history.unconfirmed} title="Review repository access change" pending={operation.pending} disabled={owner.paused || operation.history.unconfirmed || stale} correct={correct} confirmLabel={review.body.action === 'approve' ? 'Approve exact access plan' : review.body.action === 'review' ? 'Create exact reconciliation plan' : 'Refresh provider observation'} confirm={() => void send(review)}><p><strong>{review.labels.eventName}{review.labels.teamName && <> · {review.labels.teamName}</>} · {review.labels.repositoryPath}</strong></p><p>Event {eventID} · Team {teamID} · Numeric repository {review.repositoryID} · Event revision {review.revision}</p><p>{review.body.action === 'approve' ? 'Approve only these retained people and actions. Queueing an invitation does not confirm acceptance. Ambiguous effects never permit blind replacement.' : review.body.action === 'review' ? 'Read provider access and current published membership, then retain a new review plan. This does not invite or remove anyone.' : 'Read provider access and invitation facts. This does not invite or remove anyone and does not acknowledge an earlier command.'}</p>{review.body.action === 'approve' && review.plan && <RepositoryPlan plan={review.plan} />}<p>Reason: {review.body.reason}</p>{stale && <p role="status">Current facts changed. Correct details and review the current base before submitting.</p>}</RepositoryActionReview>}</>}
    {receipt && <section aria-label="Exact repository access receipt"><h4>Original access command acknowledged</h4><p>Plan {receipt.id} · Version {receipt.version} · Repository {receipt.repository_id}</p><p>{receipt.approved_at ? 'The exact plan approval was recorded. Provider delivery and invitation acceptance remain separate.' : 'The exact reconciliation plan was retained. No external access approval is implied.'}</p></section>}
    {preparationFailure instanceof OriginalReceiptIssue ? <p role="alert">{preparationFailure.message}</p> : preparationFailure ? <Failure error={preparationFailure} /> : null}<RepositoryCommandStatus command={operation} />
    {operation.history.attempts.filter(attempt => attempt.state === 'unknown' && attempt.command.intentFingerprint && attempt.command.resourceID).map(attempt => <Button key={attempt.id} icon="refresh" variant="outline" pending={checking === attempt.id} disabled={!!checking && checking !== attempt.id} onClick={() => void readOriginal(attempt)}>Read original {attempt.command.action} receipt</Button>)}
    {unresolved && retained && allowed && retained.body.action !== 'refresh' && <Button icon="refresh" variant="outline" disabled={owner.paused || operation.pending || value.revision !== retained.revision} onClick={() => void send(retained, unresolved.id)}>Retry the same original access command</Button>}
    {unresolved && <p>The original command remains unconfirmed. Only its exact retained plan/reason/revision can be retried while this authorised editor remains open; a current-state GET cannot prove the receipt. The separate original-receipt lookup can recover only the immutable matching actor and command identity. If authority or revision changes, replacement stays blocked for this signed-in session.</p>}
  </>
}

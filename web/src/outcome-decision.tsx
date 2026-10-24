import { useEffect, useRef, useState } from 'react'
import type { NativeIntent, NativeResult, OutcomeDefinition, Publication, Review } from './outcome-api'
import { Button } from './components/ui/button'
import { StatusBadge } from './components/ui/feedback'

// The reviewed private payload remains local to its authorised editor. Return
// to the actual initiating control when correcting; no guessed mobile target.
export function useOutcomeReview() {
  const [reviewed, review] = useState<NativeIntent>(), opener = useRef<HTMLElement | null>(null)
  function setReviewed(value?: NativeIntent, initiatingControl?: HTMLElement | null) {
    if (value) opener.current = initiatingControl ?? (document.activeElement instanceof HTMLElement ? document.activeElement : null)
    review(value ? structuredClone(value) : undefined)
  }
  const correctReview = () => { review(undefined); requestAnimationFrame(() => { if (opener.current?.isConnected) opener.current.focus() }) }
  return { reviewed, setReviewed, correctReview }
}

function DefinitionReview({ value }: { value: OutcomeDefinition }) {
  return <dl className="fleet-outcome-facts"><div><dt>Problem or opportunity</dt><dd>{value.problem}</dd></div><div><dt>Who benefits?</dt><dd>{value.audience}</dd></div><div><dt>Proposed value</dt><dd>{value.proposed_value}</dd></div><div><dt>Proposed solution</dt><dd>{value.solution || 'Not recorded'}</dd></div><div><dt>External backlog reference</dt><dd>{value.backlog_url || 'Not recorded'}</dd></div><div><dt>Source kind</dt><dd>{value.kind === 'pitch' ? 'Future-theme pitch' : 'Team outcome'}</dd></div><div><dt>Theme sensitivity</dt><dd>{value.theme_sensitive ? 'Actual release embargo applies' : 'Not marked theme-sensitive'}</dd></div></dl>
}
function PublicationReview({ fields }: { fields: Publication['fields'] }) {
  return <><dl className="fleet-outcome-facts"><div><dt>Reviewed problem</dt><dd>{fields.problem}</dd></div><div><dt>Reviewed solution</dt><dd>{fields.solution}</dd></div><div><dt>Approved audience</dt><dd>{fields.audience}</dd></div><div><dt>Share source event name</dt><dd>{fields.share_event_name ? 'Yes' : 'No'}</dd></div><div><dt>Share source team name</dt><dd>{fields.share_team_name ? 'Yes' : 'No'}</dd></div></dl><h5>Reviewed links</h5>{fields.links.length ? <ul>{fields.links.map(link => <li key={link.id}>{link.label} · {link.url}</li>)}</ul> : <p>No external links selected.</p>}<h5>Exact media references</h5>{fields.media.length ? <ul>{fields.media.map(media => <li key={media.id}>Event {media.event_id} · Media {media.id} · Version {media.version}</li>)}</ul> : <p>No media references selected.</p>}<p>Every reader must still pass current media audience and consent checks.</p></>
}
function ReviewPlan({ schedule, message, owner }: { schedule: Review['schedule']; message: Review['message']; owner?: string | null }) {
  return <><dl className="fleet-outcome-facts"><div><dt>Native owner selector</dt><dd>{owner || 'Current eligible source owner, checked by the server'}</dd></div><div><dt>Nominal checkpoint</dt><dd>{schedule.days} days</dd></div><div><dt>Actual local date and time</dt><dd>{schedule.wall_time} ({schedule.timezone})</dd></div><div><dt>Daylight-saving overlap</dt><dd>{schedule.dst_choice || 'Require an unambiguous time'}</dd></div><div><dt>Private channels</dt><dd>{schedule.channels.join(', ')}</dd></div><div><dt>Reviewed subject</dt><dd>{message.subject}</dd></div><div><dt>Reviewed plain-text message</dt><dd className="outcome-prose">{message.body}</dd></div></dl><p>Approval binds the native current owner, exact template, provider, source and privacy revisions. Rescheduling never settles an unknown original delivery.</p></>
}
export function OutcomeIntentSummary({ intent, original }: { intent: NativeIntent; original?: NativeResult }) {
  if (intent.kind === 'outcome') {
    const command = intent.command, evidence = command.evidence
    return <>
      {command.definition && <DefinitionReview value={command.definition} />}
      {command.provenance && <dl className="fleet-outcome-facts"><div><dt>Source event</dt><dd>{command.provenance.event_id || 'Independent organisation idea'}</dd></div><div><dt>Source team</dt><dd>{command.provenance.team_id || 'No team association'}</dd></div></dl>}
      {command.status && <p>Reasoned status: <StatusBadge>{command.status.replaceAll('_', ' ')}</StatusBadge></p>}
      {command.action === 'assign' && <dl className="fleet-outcome-facts"><div><dt>Follow-through owner</dt><dd>{command.owner_id || 'Clear the owner assignment'}</dd></div><div><dt>Review due date</dt><dd>{command.review_at ? new Date(command.review_at).toLocaleString() : 'No review due date'}</dd></div></dl>}
      {command.measures && <><h5>Exact objective references</h5>{command.measures.length ? <ul>{command.measures.map(ref => <li key={`${ref.objective.id}:${ref.objective.version}:${ref.measure_id}`}>Objective {ref.objective.id} · Version {ref.objective.version} · Measure {ref.measure_id}</li>)}</ul> : <p>No objective measure references selected.</p>}</>}
      {evidence && <><dl className="fleet-outcome-facts"><div><dt>Evidence basis</dt><dd>{evidence.basis.replaceAll('_', ' ')}</dd></div><div><dt>Claim or observation</dt><dd>{evidence.claim}</dd></div><div><dt>Exact value</dt><dd>{evidence.value ?? 'Unknown'}</dd></div><div><dt>Denominator</dt><dd>{evidence.denominator ?? 'Unknown'}</dd></div><div><dt>Evidence source</dt><dd>{evidence.source || 'Not recorded'}</dd></div><div><dt>Observation date</dt><dd>{evidence.observed_at ? new Date(evidence.observed_at).toLocaleString() : 'Unknown'}</dd></div></dl><h5>Retained evidence measure references</h5>{evidence.measures.length ? <ul>{evidence.measures.map(ref => <li key={`${ref.objective.id}:${ref.objective.version}:${ref.measure_id}`}>Objective {ref.objective.id} · Version {ref.objective.version} · Measure {ref.measure_id}</li>)}</ul> : <p>No measure reference selected.</p>}</>}
      <p>A proposed benefit, self-report or deliberate status does not establish demonstrated impact, ROI, an award or a published project.</p>
    </>
  }
  if (intent.kind === 'publication') {
    const retained = original && 'fields' in original ? original.fields : undefined
    return <><p>{intent.command.action === 'withdraw' ? 'Withdraw the separately published projection. Private source records remain restricted.' : intent.command.action === 'save' ? 'Save these separate positive fields as a draft. This does not publish them.' : 'Release only these deliberately reviewed positive fields for the selected audience.'}</p>{(intent.command.fields ?? retained) && <PublicationReview fields={(intent.command.fields ?? retained)!} />}<p>Source version {intent.command.source_version}; publication version {intent.expected}. Corrections require another explicit publication.</p></>
  }
  const retained = original && 'schedule' in original ? original : undefined
  const schedule = intent.command.schedule ?? retained?.schedule, message = intent.command.message ?? retained?.message
  return <><p>{intent.command.action === 'save' ? 'Save this review draft. This grants no permission to send.' : intent.command.action === 'approve' ? 'Approve exactly this current plan for the native owner.' : 'Cancel the current plan. This does not retract an already accepted or uncertain external delivery.'}</p>{schedule && message && <ReviewPlan schedule={schedule} message={message} owner={retained?.owner_id} />}<p>Source version {intent.command.source_version}; plan version {intent.expected}.</p></>
}
export function OutcomeDecision({ intent, original, title, confirmLabel, pending, blocked, stale = false, confirm, correct }: { intent: NativeIntent; original?: NativeResult; title: string; confirmLabel: string; pending: boolean; blocked: boolean; stale?: boolean; confirm: () => void; correct: () => void }) {
  const heading = useRef<HTMLHeadingElement>(null)
  useEffect(() => { heading.current?.focus() }, [intent.command.id])
  return <section className="fleet-review-flow" aria-label={`Review ${title.toLowerCase()}`}><ol className="fleet-review-steps" aria-label="Change steps"><li>1. Prepare</li><li aria-current="step">2. Review</li><li>3. Result</li></ol><h4 tabIndex={-1} ref={heading}>Review {title.toLowerCase()}</h4><OutcomeIntentSummary intent={intent} original={original} /><dl className="fleet-outcome-facts"><div><dt>Reason for this decision</dt><dd>{intent.command.reason}</dd></div><div><dt>Original expected version</dt><dd>{intent.expected}</dd></div></dl>{stale && <p role="status">The current source or version changed. Correct this review and deliberately choose the current base before another save.</p>}<div className="fleet-pattern-actions fleet-review-actions"><Button icon="back" type="button" variant="outline" disabled={pending || blocked} onClick={correct}>Correct details</Button><Button icon="check" type="button" pending={pending} disabled={blocked || stale || !navigator.onLine} onClick={confirm}>{confirmLabel}</Button></div></section>
}

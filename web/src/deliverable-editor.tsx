import { useEffect, useState } from 'react'
import { useBlocker } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { api, privateKey, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { useCurrentTime } from './use-current-time'
import { Button } from './components/ui/button'
import { Input, Textarea, Select, Label, Checkbox } from './components/ui/fields'
import { ConfirmationDialog } from './components/ui/dialog'
import { localDate } from './event-time'
import { DeliverableUpload } from './deliverable-upload'
import { EvidenceList } from './deliverable-evidence'
import { resourceDenied, ResourceOperationResult, useResourceOperation } from './resource-operation'
import { evidenceProblems, missingRequirements, sameEvidence, words, type Draft, type Evidence, type SubmissionPage, type Team } from './deliverable-model'

type Latest = { page: SubmissionPage; draft?: Draft }
type Review = { kind: 'draft' | 'formal'; evidence: Evidence[]; reason: string; revision: number }
export function DeliverableEditor({ event, team, page, initialDraft, organiser, onDirty, onDeny, paused = false }: { event: EventRecord; team: Team; page: SubmissionPage; initialDraft?: Draft; organiser: boolean; onDirty: (dirty: boolean) => void; onDeny?: (error: unknown) => void; paused?: boolean }) {
  const identity = useIdentity(), cache = useQueryClient(), now = useCurrentTime()
  const initialEvidence = initialDraft?.revision ? initialDraft.evidence : page.final?.evidence ?? []
  const [items, setItems] = useState<Evidence[]>(initialEvidence), [saved, setSaved] = useState(initialEvidence), [draftRevision, setDraftRevision] = useState(initialDraft?.revision ?? 0), [formalRevision, setFormalRevision] = useState(page.revision)
  const [reason, setReason] = useState(''), [uploading, setUploading] = useState(false), [latest, setLatest] = useState<Latest>(), [reviewed, setReviewed] = useState(false), [review, setReview] = useState<Review>(), [selected, setSelected] = useState(page.definitions[0]?.id ?? '')
  const operation = useResourceOperation(`${identity.person.id}:${identity.draft_scope}:${event.id}:${team.id}:evidence`, onDeny)
  const dirty = !sameEvidence(items, saved) || !!reason || !!review, busy = operation.pending || uploading
  const locked = paused || operation.blocked || (!page.can_draft && !page.can_submit), problems = evidenceProblems(items), missing = missingRequirements(page.definitions, items)
  const lateChange = page.definitions.some(definition => now >= new Date(definition.due_at).getTime() && JSON.stringify(items.find(item => item.deliverable_id === definition.id)) !== JSON.stringify(page.final?.evidence.find(item => item.deliverable_id === definition.id)))
  const needsReason = organiser && (page.outcome !== 'open' || !!page.frozen_at || now >= new Date(page.cutoff).getTime() || lateChange), staleFormal = page.revision !== formalRevision
  useEffect(() => { onDirty(dirty || uploading || operation.blocked); return () => onDirty(false) }, [dirty, uploading, operation.blocked, onDirty])
  useBlocker({ shouldBlockFn: () => operation.isBlocked() || uploading || (dirty && !window.confirm('Leave these unsaved evidence changes?')), enableBeforeUnload: dirty || uploading || operation.blocked })
  function change(item: Evidence) { setItems(old => old.filter(value => value.deliverable_id !== item.deliverable_id).concat(item)) }
  function prepare(kind: Review['kind']) {
    if (locked || busy || problems.length || (kind === 'formal' && (!page.can_submit || missing.length || staleFormal || (needsReason && !reason.trim()))) || (kind === 'draft' && !page.can_draft)) return
    setReview({ kind, evidence: structuredClone(items), reason: reason.trim(), revision: kind === 'draft' ? draftRevision : formalRevision })
  }
  async function confirm() {
    if (!review || paused || uploading) return
    const intent = review, ticket = operation.capture()
    await operation.run({ operation: intent.kind === 'draft' ? 'Save private evidence draft' : 'Submit formal evidence', revision: intent.revision, command: { team_id: team.id, evidence: intent.evidence, ...(intent.kind === 'formal' ? { reason: intent.reason } : {}) } }, async () => {
      if (intent.kind === 'draft') {
        const result = unwrap(await api.PUT('/api/v1/events/{event_id}/teams/{team_id}/submission/draft', { params: { path: { event_id: event.id, team_id: team.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${intent.revision}"` } }, body: { evidence: intent.evidence } }))
        if (result.revision <= intent.revision || !sameEvidence(result.evidence, intent.evidence)) throw new Error('Unexpected private draft response')
        return { kind: 'draft' as const, value: result }
      }
      const result = unwrap(await api.PUT('/api/v1/events/{event_id}/teams/{team_id}/submission', { params: { path: { event_id: event.id, team_id: team.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${intent.revision}"` } }, body: { evidence: intent.evidence, reason: intent.reason } }))
      if (result.team_id !== team.id || !result.final || result.final.revision <= intent.revision || !sameEvidence(result.final.evidence, intent.evidence)) throw new Error('Unexpected formal submission response')
      return { kind: 'formal' as const, value: result }
    }, async (result, current) => {
      setReview(undefined); setSaved(intent.evidence)
      if (result.kind === 'draft') { setDraftRevision(result.value.revision); cache.setQueryData(privateKey(identity.person.id, event.id, `submission-draft-${team.id}`), result.value); return 'Private draft saved. No formal submission was accepted or reviewed.' }
      setFormalRevision(result.value.revision); setReason(''); cache.setQueryData(privateKey(identity.person.id, event.id, `submission-${team.id}`), result.value); await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, event.id, `submission-history-${team.id}`) }); current.assert(); return `Formal version ${result.value.final!.revision} submitted. Completion review is pending.`
    })
    if (ticket.current()) setReview(undefined)
  }
  function reviewLatest() { void operation.read(async signal => {
    const fresh = unwrap(await api.GET('/api/v1/events/{event_id}/teams/{team_id}/submission', { signal, params: { path: { event_id: event.id, team_id: team.id } } }))
    const draft = fresh.can_read_draft ? unwrap(await api.GET('/api/v1/events/{event_id}/teams/{team_id}/submission/draft', { signal, params: { path: { event_id: event.id, team_id: team.id } } })) : undefined
    return { page: fresh, draft }
  }, fresh => { setLatest(fresh); setReviewed(false); cache.setQueryData(privateKey(identity.person.id, event.id, `submission-${team.id}`), fresh.page) }) }
  function acceptLatest(keep: boolean) {
    if (!latest || (keep && !reviewed)) return
    const fresh = latest.draft?.revision ? latest.draft.evidence : latest.page.final?.evidence ?? []
    setFormalRevision(latest.page.revision); setDraftRevision(latest.draft?.revision ?? 0); setSaved(fresh); if (!keep) setItems(fresh)
    setLatest(undefined); setReviewed(false); operation.useCurrent()
  }
  const definition = page.definitions.find(value => value.id === selected) ?? page.definitions[0], item = items.find(value => value.deliverable_id === definition?.id), formal = page.final?.evidence.find(value => value.deliverable_id === definition?.id)
  return <section className="fleet-resource-section" aria-label="Team evidence editor"><h3>{page.can_read_draft ? 'Your team’s working evidence' : 'Organiser amendment'}</h3><p>Private drafts belong to the team. Organisers and reviewers see formal submissions only. Saving a draft does not submit it.</p>{initialDraft?.updated_at && <p>Private draft last saved {localDate(initialDraft.updated_at, event.configuration.timezone)}. The formal submission above is separate.</p>}{locked && <p>Editing is locked. Your unsaved fields remain visible; ask an organiser about an exception.</p>}
    <ol className="fleet-resource-records">{page.definitions.map(value => <li key={value.id}><div><strong>{value.title}</strong><p>{value.required ? 'Required' : 'Optional'} · Due {localDate(value.due_at, event.configuration.timezone)} · Working evidence: {items.some(evidence => evidence.deliverable_id === value.id) ? 'Prepared privately' : 'Not prepared'} · Formal evidence: {page.final?.evidence.some(evidence => evidence.deliverable_id === value.id) ? 'Submitted' : 'Missing'}</p></div><Button icon="list" variant="ghost" disabled={locked || busy} aria-pressed={definition?.id === value.id} onClick={() => setSelected(value.id)}>Edit evidence for {value.title}</Button></li>)}</ol>
    <form className="fleet-resource-form" onSubmit={event => { event.preventDefault(); prepare('formal') }}>{organiser && <><Label htmlFor="amendment-reason">Organiser amendment reason</Label><Textarea id="amendment-reason" value={reason} onChange={event => setReason(event.target.value)} disabled={locked || busy} maxLength={2000} /><p>Required for changes after any deliverable deadline or the shared cutoff. Late amendments retain their history and reset completion review.</p></>}
    {definition && <section className="fleet-resource-section"><fieldset className="fleet-resource-fields" disabled={locked || busy}><legend>{definition.title} · {definition.required ? 'Required' : 'Optional'}</legend><p>{definition.description}</p><p>Owner: {team.name} · Due {localDate(definition.due_at, event.configuration.timezone)} · {now >= new Date(definition.due_at).getTime() ? formal ? 'Deadline passed · Formal evidence recorded' : 'Deadline passed · No formal evidence recorded' : 'Before deadline'}</p>{now >= new Date(definition.due_at).getTime() && !organiser && <p>Changing this deliverable after its deadline requires an organiser exception. Private preparation is still separate from formal acceptance.</p>}<Label htmlFor={`evidence-kind-${definition.id}`}>Evidence type for {definition.title}</Label><Select id={`evidence-kind-${definition.id}`} value={item?.kind ?? ''} onChange={event => { if (!event.target.value) setItems(items.filter(value => value.deliverable_id !== definition.id)); else change({ deliverable_id: definition.id, kind: event.target.value as Evidence['kind'], label: item?.label ?? definition.title, url: event.target.value === 'file' ? '' : item?.url ?? '', file_id: null }) }}><option value="">Not prepared yet</option>{(definition.evidence_kinds?.length ? definition.evidence_kinds : ['file', 'url', 'repository']).map(kind => <option key={kind} value={kind}>{kind === 'url' ? 'Web link' : words(kind)}</option>)}</Select>{item && <><Label htmlFor={`evidence-label-${definition.id}`}>Evidence label for {definition.title}</Label><Input id={`evidence-label-${definition.id}`} value={item.label} maxLength={300} onChange={event => change({ ...item, label: event.target.value })} />{item.kind === 'file' ? item.file_id && <EvidenceList eventID={event.id} items={[item]} /> : <><Label htmlFor={`evidence-url-${definition.id}`}>Reference URL for {definition.title}</Label><Input id={`evidence-url-${definition.id}`} type="url" value={item.url} maxLength={2048} onChange={event => change({ ...item, url: event.target.value })} /><p>Provide a link that reviewers can open. External repository content may change.</p></>}</>}</fieldset>{item?.kind === 'file' && <DeliverableUpload event={event} teamID={team.id} item={item} reason={reason} disabled={locked || operation.pending || (needsReason && !reason.trim())} busy={uploading} setBusy={setUploading} change={change} onDeny={error => { if (resourceDenied(error)) onDeny?.(error) }} />}</section>}
    {problems.length > 0 && <ul aria-label="Evidence problems">{[...new Set(problems)].map(problem => <li key={problem}>{problem}</li>)}</ul>}{missing.length > 0 && <p>Required evidence still missing: {missing.join(', ')}. A valid private draft may be saved before it is complete.</p>}
    <div className="fleet-pattern-actions">{page.can_read_draft && <Button type="button" icon="save" variant="outline" disabled={locked || busy || !page.can_draft || problems.length > 0 || (!dirty && draftRevision > 0)} onClick={() => prepare('draft')}>Save private draft</Button>}<Button type="submit" icon="send" disabled={locked || busy || !page.can_submit || !page.definitions.length || problems.length > 0 || missing.length > 0 || staleFormal || (page.final !== null && sameEvidence(items, page.final.evidence)) || (needsReason && !reason.trim())}>Submit formal version</Button>{page.final && <Button type="button" icon="refresh" variant="outline" disabled={locked || busy} onClick={() => { if (!dirty || window.confirm('Replace your working fields with current formal evidence?')) setItems(page.final!.evidence) }}>Use current formal evidence</Button>}</div></form>
    {staleFormal && !operation.blocked && <section className="fleet-resource-section"><p>The formal submission changed. Your working fields are retained; read and review current evidence before submitting.</p><Button icon="refresh" onClick={reviewLatest}>Review latest evidence</Button></section>}
    {latest && <section className="fleet-resource-section" aria-label="Latest saved evidence"><p>Outcome: {words(latest.page.outcome)} · Formal version {latest.page.final?.revision ?? 'none'} · Private draft {latest.draft?.revision ?? 'unavailable'}</p><h4>Current formal evidence</h4><EvidenceList eventID={event.id} items={latest.page.final?.evidence ?? []} />{latest.draft && <><h4>Private draft</h4><EvidenceList eventID={event.id} items={latest.draft.evidence} /></>}<Label className="check"><Checkbox checked={reviewed} onChange={event => setReviewed(event.target.checked)} />I reviewed the latest saved evidence</Label><div className="fleet-pattern-actions"><Button icon="check" disabled={!reviewed || busy} onClick={() => acceptLatest(true)}>Keep my evidence edits</Button><Button variant="outline" disabled={busy} onClick={() => acceptLatest(false)}>Discard my edits and load latest</Button></div></section>}
    <ResourceOperationResult hideCurrentChoice operation={operation} read={reviewLatest} acceptCurrent={{ label: 'Keep my evidence edits', disabled: !reviewed || !latest || busy, onSelect: () => acceptLatest(true) }} />
    <ConfirmationDialog open={!!review} onOpenChange={open => { if (!open && !operation.pending) setReview(undefined) }} title={review?.kind === 'draft' ? 'Review private evidence draft' : 'Review formal submission'} description={review?.kind === 'draft' ? 'This saves a private team draft. It is not a formal submission and will not be scored or completion reviewed.' : 'Confirm the named team evidence against the current formal state revision. The server decides whether the live window and cutoff allow acceptance.'} confirmLabel={review?.kind === 'draft' ? 'Confirm private draft' : 'Confirm formal submission'} pending={operation.pending} disabled={operation.blocked || paused || uploading} onConfirm={() => void confirm()}><p>{team.name} · Expected revision {review?.revision}</p><EvidenceList eventID={event.id} items={review?.evidence ?? []} />{review?.reason && <p>{review.reason}</p>}</ConfirmationDialog>
  </section>
}

import { useEffect, useState } from 'react'
import { useBlocker } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { api, APIError, privateKey, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { useCurrentTime } from './use-current-time'
import { Button } from './components/ui/button'
import { Failure } from './components/states'
import { localDate } from './event-time'
import { DeliverableUpload } from './deliverable-upload'
import { EvidenceList } from './deliverable-evidence'
import { evidenceProblems, missingRequirements, sameEvidence, words, type Draft, type Evidence, type SubmissionPage, type Team } from './deliverable-model'

type Latest = { page: SubmissionPage; draft?: Draft }
export function DeliverableEditor({ event, team, page, initialDraft, organiser, onDirty }: { event: EventRecord; team: Team; page: SubmissionPage; initialDraft?: Draft; organiser: boolean; onDirty: (dirty: boolean) => void }) {
  const identity = useIdentity(), cache = useQueryClient(), now = useCurrentTime()
  const initialEvidence = initialDraft?.revision ? initialDraft.evidence : page.final?.evidence ?? []
  const [items, setItems] = useState<Evidence[]>(initialEvidence), [saved, setSaved] = useState(initialEvidence), [draftRevision, setDraftRevision] = useState(initialDraft?.revision ?? 0), [formalRevision, setFormalRevision] = useState(page.revision)
  const [reason, setReason] = useState(''), [pending, setPending] = useState(false), [uploading, setUploading] = useState(false), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState(''), [denied, setDenied] = useState(false)
  const [conflict, setConflict] = useState(false), [latest, setLatest] = useState<Latest>(), [reviewed, setReviewed] = useState(false)
  const dirty = !sameEvidence(items, saved) || !!reason, busy = pending || uploading
  const locked = denied || (!page.can_draft && !page.can_submit), problems = evidenceProblems(items), missing = missingRequirements(page.definitions, items)
  const lateChange = page.definitions.some(d => now >= new Date(d.due_at).getTime() && JSON.stringify(items.find(e => e.deliverable_id === d.id)) !== JSON.stringify(page.final?.evidence.find(e => e.deliverable_id === d.id)))
  const needsReason = organiser && (page.outcome !== 'open' || !!page.frozen_at || now >= new Date(page.cutoff).getTime() || lateChange)
  const staleFormal = page.revision !== formalRevision
  useEffect(() => { onDirty(dirty || uploading); return () => onDirty(false) }, [dirty, uploading, onDirty])
  useBlocker({ shouldBlockFn: () => (dirty || uploading) && !window.confirm('Leave these unsaved evidence changes?'), enableBeforeUnload: dirty || uploading })
  function change(item: Evidence) { setItems(old => old.filter(e => e.deliverable_id !== item.deliverable_id).concat(item)); setNotice('') }
  async function refresh() {
    await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, event.id, `submission-${team.id}`) })
    await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, event.id, `submission-history-${team.id}`) })
  }
  async function failed(error: unknown) {
    setFailure(error)
    if (error instanceof APIError && error.code === 'revision_conflict') { setConflict(true); setLatest(undefined); setReviewed(false) }
    if (error instanceof APIError && error.status === 403) setDenied(true)
    if (error instanceof APIError && [403, 409].includes(error.status)) await refresh()
  }
  async function saveDraft() {
    setPending(true); setFailure(undefined); setNotice('')
    try {
      const result = unwrap(await api.PUT('/api/v1/events/{event_id}/teams/{team_id}/submission/draft', { params: { path: { event_id: event.id, team_id: team.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${draftRevision}"` } }, body: { evidence: items } }))
      setDraftRevision(result.revision); setSaved(result.evidence); setNotice('Private draft saved. No formal submission was accepted or reviewed.')
      cache.setQueryData(privateKey(identity.person.id, event.id, `submission-draft-${team.id}`), result)
    } catch (error) { await failed(error) } finally { setPending(false) }
  }
  async function submit() {
    setPending(true); setFailure(undefined); setNotice('')
    try {
      const result = unwrap(await api.PUT('/api/v1/events/{event_id}/teams/{team_id}/submission', { params: { path: { event_id: event.id, team_id: team.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${formalRevision}"` } }, body: { evidence: items, reason: reason.trim() } }))
      setFormalRevision(result.revision); setSaved(items); setReason(''); setNotice(`Formal version ${result.final?.revision} submitted. Completion review is pending.`)
      cache.setQueryData(privateKey(identity.person.id, event.id, `submission-${team.id}`), result)
      await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, event.id, `submission-history-${team.id}`) })
    } catch (error) { await failed(error) } finally { setPending(false) }
  }
  async function reviewLatest() {
    setPending(true)
    try {
      const fresh = unwrap(await api.GET('/api/v1/events/{event_id}/teams/{team_id}/submission', { params: { path: { event_id: event.id, team_id: team.id } } }))
      const draft = fresh.can_read_draft ? unwrap(await api.GET('/api/v1/events/{event_id}/teams/{team_id}/submission/draft', { params: { path: { event_id: event.id, team_id: team.id } } })) : undefined
      setLatest({ page: fresh, draft }); setReviewed(false); cache.setQueryData(privateKey(identity.person.id, event.id, `submission-${team.id}`), fresh)
    } catch (error) { await failed(error) } finally { setPending(false) }
  }
  function acceptLatest(keep: boolean) {
    if (!latest) return
    const fresh = latest.draft?.revision ? latest.draft.evidence : latest.page.final?.evidence ?? []
    setFormalRevision(latest.page.revision); setDraftRevision(latest.draft?.revision ?? 0); setSaved(fresh); if (!keep) setItems(fresh)
    setConflict(false); setLatest(undefined); setReviewed(false); setFailure(undefined); setDenied(false); setNotice(keep ? 'Latest version reviewed. Your edits are retained for an explicit save or submission.' : 'Latest saved evidence loaded.')
  }
  return <form className="deliverable-editor setup-form section-heading" aria-label="Team evidence editor" onSubmit={e => { e.preventDefault(); void submit() }}><h4>{page.can_read_draft ? 'Your team’s working evidence' : 'Organiser amendment'}</h4><p>Private drafts belong to the team. Organisers and reviewers see formal submissions only. Saving a draft does not submit it.</p>{initialDraft?.updated_at && <p className="muted">Private draft last saved {localDate(initialDraft.updated_at, event.configuration.timezone)}. The formal submission above is separate.</p>}{locked && <p className="notice">Editing is locked. Your unsaved fields remain visible; ask an organiser about an exception.</p>}
    {organiser && <><label htmlFor="amendment-reason">Organiser amendment reason</label><textarea id="amendment-reason" value={reason} onChange={e => setReason(e.target.value)} disabled={locked || busy} maxLength={2000} /><p>Required for changes after any deliverable deadline or the shared cutoff. Late amendments retain their history and reset completion review.</p></>}
    {page.definitions.map(d => { const item = items.find(e => e.deliverable_id === d.id), kinds = d.evidence_kinds?.length ? d.evidence_kinds : ['file', 'url', 'repository']; const formal = page.final?.evidence.find(e => e.deliverable_id === d.id); const due = now >= new Date(d.due_at).getTime(); return <fieldset key={d.id} disabled={locked || busy}><legend>{d.title} · {d.required ? 'Required' : 'Optional'}</legend><p>{d.description}</p><p>Owner: {team.name} · Due {localDate(d.due_at, event.configuration.timezone)} · {due ? formal ? 'Deadline passed · Formal evidence recorded' : 'Deadline passed · No formal evidence recorded' : 'Before deadline'}</p>{due && !organiser && <p className="notice">Changing this deliverable after its deadline requires an organiser exception. Private preparation is still separate from formal acceptance.</p>}<p className="muted">Working evidence: {item ? evidenceProblems([item]).length ? 'Incomplete' : 'Prepared privately' : 'Not prepared'} · Formal evidence: {formal ? 'Submitted' : 'Missing'}</p><label htmlFor={`evidence-kind-${d.id}`}>Evidence type for {d.title}</label><select id={`evidence-kind-${d.id}`} value={item?.kind ?? ''} onChange={e => { if (!e.target.value) setItems(items.filter(i => i.deliverable_id !== d.id)); else change({ deliverable_id: d.id, kind: e.target.value as Evidence['kind'], label: item?.label ?? d.title, url: e.target.value === 'file' ? '' : item?.url ?? '', file_id: null }) }}><option value="">Not prepared yet</option>{kinds.map(k => <option key={k} value={k}>{k === 'url' ? 'Web link' : words(k)}</option>)}</select>{item && <><label htmlFor={`evidence-label-${d.id}`}>Evidence label for {d.title}</label><input id={`evidence-label-${d.id}`} value={item.label} maxLength={300} onChange={e => change({ ...item, label: e.target.value })} />{item.kind === 'file' ? <>{item.file_id && <EvidenceList eventID={event.id} items={[item]} />}<DeliverableUpload event={event} teamID={team.id} item={item} reason={reason} disabled={locked || pending || (needsReason && !reason.trim())} busy={uploading} setBusy={setUploading} change={change} /></> : <><label htmlFor={`evidence-url-${d.id}`}>Reference URL for {d.title}</label><input id={`evidence-url-${d.id}`} type="url" value={item.url} maxLength={2048} onChange={e => change({ ...item, url: e.target.value })} /><p className="muted">Provide a link that reviewers can open. External repository content may change.</p></>}</>}</fieldset> })}
    {problems.length > 0 && <ul className="blockers" aria-label="Evidence problems">{[...new Set(problems)].map(p => <li key={p}>{p}</li>)}</ul>}{missing.length > 0 && <p className="notice">Required evidence still missing: {missing.join(', ')}. A valid private draft may be saved before it is complete.</p>}{(conflict || staleFormal) && <section className="panel" aria-label="Evidence conflict"><h4>Review the latest saved evidence</h4><p>Your fields are retained. Another change may have altered the formal submission, draft or outcome.</p><Button type="button" variant="outline" disabled={busy} onClick={() => void reviewLatest()}>Review latest evidence</Button>{latest && <><section aria-label="Latest saved evidence"><p>Outcome: {words(latest.page.outcome)} · Formal version {latest.page.final?.revision ?? 'none'} · Private draft {latest.draft?.revision ?? 'unavailable'}</p><h5>Current formal evidence</h5><EvidenceList eventID={event.id} items={latest.page.final?.evidence ?? []} />{latest.draft && <><h5>Private draft</h5><EvidenceList eventID={event.id} items={latest.draft.evidence} /></>}</section><label className="check"><input type="checkbox" checked={reviewed} onChange={e => setReviewed(e.target.checked)} />I reviewed the latest saved evidence</label><div className="actions"><Button type="button" disabled={!reviewed || busy} onClick={() => acceptLatest(true)}>Keep my evidence edits</Button><Button type="button" variant="outline" disabled={busy} onClick={() => acceptLatest(false)}>Discard my edits and load latest</Button></div></>}</section>}
    {!!failure && <Failure error={failure} />}<div className="actions">{page.can_read_draft && <Button type="button" variant="outline" disabled={locked || busy || !page.can_draft || problems.length > 0 || conflict || (!dirty && draftRevision > 0)} onClick={() => void saveDraft()}>Save private draft</Button>}<Button type="submit" disabled={locked || busy || !page.can_submit || !page.definitions.length || problems.length > 0 || missing.length > 0 || conflict || staleFormal || (page.final !== null && sameEvidence(items, page.final.evidence)) || (needsReason && !reason.trim())}>Submit formal version</Button>{page.final && <Button type="button" variant="outline" disabled={locked || busy} onClick={() => { if (!dirty || window.confirm('Replace your working fields with current formal evidence?')) setItems(page.final!.evidence) }}>Use current formal evidence</Button>}</div>{notice && <p role="status">{notice}</p>}
  </form>
}

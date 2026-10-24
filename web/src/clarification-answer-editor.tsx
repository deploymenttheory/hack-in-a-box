import { useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { api, APIError, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Field, Input, Select, Textarea } from './components/ui/fields'
import { Failure, Loading } from './components/states'
import { SafeMarkdown } from './components/safe-markdown'
import { denied, sameDocument, sameSource, textBytes, uncertain, useAccessFailure, useClarifications, useCurrentPage, type AnswerDocument, type AnswerDraft, type Question, type Source } from './clarification-context'
import { MentorTaskReview, MentorUnknown, useMentorTaskNavigation } from './mentor-task'
import { usePlanningDepartureBarrier, usePlanningUnknownCommands } from './planning-operations'

type Review = components['schemas']['ClarificationReview']
type Action = 'save' | 'approve' | 'publish'
type Pending = { intentID: string; action: Action; version: number; revision: number; document: AnswerDocument; source: Source; id?: string; audience: { label: string; count: number } }
const empty: AnswerDocument = { title: '', body: '', kind: 'clarification' }
const exactDraft = (a: AnswerDraft | undefined | null, b: AnswerDraft | undefined | null) => (!a && !b) || (!!a && !!b && a.id === b.id && a.version === b.version && a.revision === b.revision)
export function OfficialAnswerEditor({ question, enabled }: { question: Question; enabled: boolean }) {
  const { key, event } = useClarifications()
  const query = useQuery({ queryKey: [...key, 'review', question.id], gcTime: 0, retry: false, refetchOnMount: 'always', refetchInterval: 10_000, refetchOnWindowFocus: 'always', queryFn: async ({ signal }) => { const value = unwrap(await api.GET('/api/v1/events/{event_id}/questions/{question_id}/review', { signal, cache: 'no-store', params: { path: { event_id: event.id, question_id: question.id } } })); if (!value.workspace.can_review || !value.workspace.review_audience) throw new APIError(403, 'forbidden'); return value } })
  useAccessFailure(query.error)
  if (denied(query.error)) return null
  if (!query.data || !query.isFetchedAfterMount) return query.error ? <Failure error={query.error} retry={() => void query.refetch()} /> : <Loading />
  if (!query.data.workspace.can_review || !query.data.workspace.review_audience) return <p role="alert">Current official-answer review authority is unavailable. Private editor and review snapshots have been removed.</p>
  return <section className="clarification-private py-6 border-t border-border"><h2>Staff official-answer review</h2>{query.error && <Failure error={query.error} retry={() => void query.refetch()} />}<AnswerForm key={`${question.id}:${query.data.workspace.can_review}`} question={question} review={query.data} readFresh={!query.error} enabled={enabled && !query.error && !query.data.workspace.archived} reload={async () => { const result = await query.refetch(); if (result.error) throw result.error; return result.data! }} /></section>
}
function AnswerForm({ question, review, readFresh, enabled, reload }: { question: Question; review: Review; readFresh: boolean; enabled: boolean; reload: () => Promise<Review> }) {
  const identity = useIdentity(), { event, key, deny } = useClarifications(), cache = useQueryClient(), active = useCurrentPage()
  const history = usePlanningUnknownCommands(`${identity.person.id}:${identity.draft_scope}:${event.id}:${question.id}:official-answer`)
  const [base, setBase] = useState<AnswerDraft | undefined>(review.draft ?? undefined), [document, setDocument] = useState<AnswerDocument>(review.draft?.document ?? empty)
  const [busy, setBusy] = useState(false), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState(''), [pending, setPending] = useState<Pending>(), [prepared, setPrepared] = useState<Pending>(), [comparison, setComparison] = useState<Review>()
  const dirty = !sameDocument(document, base?.document ?? empty), changed = readFresh && !exactDraft(base, review.draft), unknown = !!pending || history.unconfirmed
  const currentSource = review.workspace.source, sourceCurrent = !base || sameSource(base.source, currentSource)
  const root = useRef<HTMLElement>(null), lock = useRef(false), terminal = useRef(false), navigation = useMentorTaskNavigation({ root, dirty: dirty || !!prepared, pending: busy, unknown })
  usePlanningDepartureBarrier(busy)
  const valid = !!document.title.trim() && !!document.body.trim() && textBytes(document.title) <= 200 && textBytes(document.body) <= 16000 && !document.title.includes('\0') && !document.body.includes('\0')
  function prepare(action: Action) {
    if (!enabled || busy || lock.current || terminal.current || unknown || changed || !review.workspace.review_audience || (action === 'save' ? !valid : !base || dirty || !sourceCurrent)) return
    navigation.capture(); setPrepared({ intentID: crypto.randomUUID(), action, version: action === 'save' ? (base?.version ?? 0) + 1 : base!.version, revision: (base?.revision ?? 0) + 1, document: structuredClone(document), source: structuredClone(currentSource), id: base?.id, audience: { ...review.workspace.review_audience } })
  }
  async function write() {
    if (!prepared || !enabled || busy || lock.current || terminal.current || unknown || changed) return
    const intent = structuredClone(prepared), action = intent.action
    lock.current = true; setBusy(true); setFailure(undefined); setNotice(''); setComparison(undefined)
    let attempted = false, confirmed = false
    try {
      const latest = await reload()
      if (!active.current) return
      if (!latest.workspace.can_review || !latest.workspace.review_audience || latest.workspace.archived) { setPrepared(undefined); setNotice('Current review authority or write policy changed. No write was sent.'); return }
      if (!exactDraft(base, latest.draft) || intent.revision !== (latest.draft?.revision ?? 0) + 1 || !sameSource(intent.source, latest.workspace.source)) { setPrepared(undefined); setComparison(latest); setNotice('The server version or common source changed. Compare it with your unsent text and review again before any write.'); return }
      if (action === 'publish' && (latest.workspace.review_audience.count !== intent.audience.count || latest.workspace.review_audience.label !== intent.audience.label)) { setPrepared(undefined); setNotice('The native audience preview changed. Review the current publication audience before sending; no publication was sent.'); return }
      if (action !== 'save' && (!base || !sameSource(base.source, latest.workspace.source))) { setPrepared(undefined); setComparison(latest); setNotice('Current source policy changed. No approval or publication was sent.'); return }
      attempted = true
      const result = action === 'save'
        ? unwrap(await api.PUT('/api/v1/events/{event_id}/questions/{question_id}/answer', { params: { path: { event_id: event.id, question_id: question.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${intent.revision - 1}"` } }, body: intent.document }))
        : unwrap(await api.POST('/api/v1/events/{event_id}/clarifications/{answer_id}/actions', { params: { path: { event_id: event.id, answer_id: intent.id! }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${intent.revision - 1}"` } }, body: { action, version: intent.version } }))
      if (!active.current) return
      if (result.question_id !== question.id || result.version !== intent.version || result.revision !== intent.revision || !sameDocument(result.document, intent.document) || !sameSource(result.source, intent.source) || (action === 'approve' && !result.approved) || (action === 'publish' && !result.published)) throw new Error('Unverified answer response')
      confirmed = true; setBase(result); setDocument(result.document); setPrepared(undefined); setPending(undefined); navigation.restore()
      setNotice(action === 'save' ? `Saved draft version ${result.version}. It is not an official common answer until separately approved and published.` : action === 'approve' ? `Version ${result.version} approved. It has not been published by this action.` : `Official version ${result.version} published. Safe common inbox references are recorded with publication.`)
      void cache.invalidateQueries({ queryKey: [...key, 'question-history', question.id] }); void cache.invalidateQueries({ queryKey: [...key, 'question', question.id] }); void cache.invalidateQueries({ queryKey: [...key, 'questions'] }); void cache.invalidateQueries({ queryKey: [...key, 'common'] }); await reload()
    } catch (error) {
      if (!active.current) return
      setFailure(error); deny(error)
      if (confirmed) { setNotice('The write was confirmed, but the follow-up read failed. Refresh the current review before another decision.'); return }
      if (attempted && uncertain(error)) { terminal.current = true; setPending(intent); history.record({ operation: `Common answer ${action}`, revision: intent.revision - 1, command: { intentID: intent.intentID, questionID: question.id, version: intent.version } }); setNotice('Outcome unknown. No action will be repeated automatically. Read the exact version and immutable transition before another write.') }
      else { setPrepared(undefined); setNotice(attempted ? 'No new saved or published state was confirmed. Your editor text remains only in this page.' : 'Current source could not be verified. No write was sent; your editor text remains unsent.'); if (error instanceof APIError && error.status === 409) setComparison(review) }
    } finally { lock.current = false; if (active.current) setBusy(false) }
  }
  async function reconcile() {
    if (lock.current || busy) return
    lock.current = true; setBusy(true); setFailure(undefined)
    try {
      const latest = await reload(); if (!active.current) return
      const draft = latest.draft
      if (!pending) { setComparison(latest); setNotice('Current review refreshed. Earlier unresolved commands stay unresolved; editor changes remain unsent.'); return }
      const match = draft && draft.version === pending.version && draft.revision >= pending.revision && sameDocument(draft.document, pending.document) && sameSource(draft.source, pending.source) && (pending.action === 'save' || (draft.id === pending.id && (pending.action === 'approve' ? draft.approved : draft.published)))
      let recorded = false, cursor: string | undefined
      if (match) do { const page = unwrap(await api.GET('/api/v1/events/{event_id}/questions/{question_id}/history', { cache: 'no-store', params: { path: { event_id: event.id, question_id: question.id }, query: { cursor, limit: 30 } } })); if (!active.current) return; recorded = page.items.some(t => t.actor_id === identity.person.id && t.version === pending.version && t.revision === pending.revision && t.action === ({ save: 'authored', approve: 'approved', publish: 'published' }[pending.action])); cursor = page.next_cursor ?? undefined; if (recorded) break } while (cursor)
      if (match && recorded && draft) { history.resolve(pending.intentID); terminal.current = false; setBase(draft); setDocument(draft.document); setPending(undefined); setPrepared(undefined); setComparison(undefined); setNotice(`The server confirms matching version ${draft.version} and the exact original transition: ${draft.published ? 'published' : draft.approved ? 'approved, not published' : 'saved draft, not published'}. No action was replayed.`) }
      else { setComparison(latest); setNotice('Compare the authoritative server version with your unsent editor. An unmatched original outcome remains unknown; no write was replayed.') }
    } catch (error) { if (active.current) { setFailure(error); deny(error) } } finally { lock.current = false; if (active.current) setBusy(false) }
  }
  function adoptServer(keepText: boolean) {
    const latest = comparison ?? review
    if ((unknown || (!keepText && dirty)) && !window.confirm(unknown ? 'Keep the original outcome unresolved and deliberately choose a new current base? The earlier command will not be replayed.' : 'Replace this private unsent answer text with the current server draft?')) return
    setBase(latest.draft ?? undefined); if (!keepText) setDocument(latest.draft?.document ?? empty)
    if (unknown) history.selectNewBase(); terminal.current = false; setPending(undefined); setPrepared(undefined); setComparison(undefined); setFailure(undefined); setNotice(keepText ? 'Current server version selected. Your text is still unsent; save explicitly to create a new draft version.' : 'Current server draft loaded. No write was sent.')
  }
  return <section ref={root} tabIndex={-1} aria-label="Official answer task"><p>Authoring, approval and publication are separate decisions. Private source question text and files are never copied into the common answer automatically.</p><p role="status">{unknown ? 'Outcome unknown — writes paused' : dirty ? 'Unsent editor changes — kept only in this page' : base ? `Server draft version ${base.version} · ${base.published ? 'published' : base.approved ? 'approved, not published' : 'not approved or published'}` : 'No server draft yet'}</p>
    {(changed || comparison) && <section className="py-6 border-t border-border"><h3>Current server version comparison</h3><p>{(comparison ?? review).draft ? `Server draft version ${(comparison ?? review).draft!.version}, revision ${(comparison ?? review).draft!.revision}` : 'No server draft is currently recorded.'}</p>{(comparison ?? review).draft && <><h4>{(comparison ?? review).draft!.document.title}</h4><SafeMarkdown>{(comparison ?? review).draft!.document.body}</SafeMarkdown></>}<div className="fleet-pattern-actions"><Button icon="refresh" variant="outline" disabled={busy || !!prepared} onClick={() => adoptServer(false)}>Load current server draft</Button><Button variant="outline" disabled={busy || !!prepared} onClick={() => adoptServer(true)}>Keep my text against the current server version</Button></div></section>}
    <div hidden={!!prepared || unknown} inert={!!prepared || unknown}><form className="grid gap-4 max-w-3xl py-4" onSubmit={e => { e.preventDefault(); prepare('save') }}><fieldset className="grid gap-4" disabled={!enabled || busy || unknown || !!prepared}><legend>Unpublished answer editor</legend>
      <Field id="answer-kind" label="Type of answer"><Select value={document.kind} onChange={e => setDocument({ ...document, kind: e.target.value as AnswerDocument['kind'] })}><option value="clarification">Clarification of the same published version</option><option value="challenge_change">Notice of an already published challenge change</option><option value="rubric_change">Notice of an already reviewed and published rubric change</option></Select></Field>
      <Field id="answer-title" label="Common answer title"><Input value={document.title} maxLength={200} onChange={e => setDocument({ ...document, title: e.target.value })} /></Field>
      <Field id="answer-body" label="Common official answer text" description={`${textBytes(document.body)} / 16,000 UTF-8 bytes. Check that the common answer contains no private question text or attachments.`}><Textarea value={document.body} maxLength={16000} rows={8} onChange={e => setDocument({ ...document, body: e.target.value })} /></Field>
      <Button type="submit" icon="next" disabled={!enabled || busy || unknown || changed || !valid || (!!base && !dirty)}>Review draft version</Button></fieldset></form>
      <div className="fleet-pattern-actions"><Button icon="shield" variant="outline" disabled={!enabled || busy || unknown || changed || !base || dirty || !sourceCurrent || base.approved} onClick={() => prepare('approve')}>Review saved version approval</Button><Button icon="send" disabled={!enabled || busy || unknown || changed || !base?.approved || dirty || !sourceCurrent || base.published} onClick={() => prepare('publish')}>Review approved version publication</Button></div></div>
    <section className="py-6 border-t border-border" aria-label="Actual common audience preview"><h3>{readFresh ? 'Unpublished audience preview' : 'Last validated audience preview — refresh required'}</h3><p>{review.workspace.review_audience?.label}: {review.workspace.review_audience?.count ?? 'Unavailable'} people at this native preview. Publication rechecks current confirmed recipients.</p><p>Current challenge version {currentSource.challenge_version} · Published rubric version {currentSource.rubric.version}</p><p>{document.kind === 'clarification' ? 'Interpretation of the same exact published source.' : 'A material-change notice requires its owning version release, impact review and notifications first; this task cannot change criteria or scores.'}</p><Link to="/events/$eventId/resources/$resourceId" params={{ eventId: event.id, resourceId: currentSource.challenge_id }}>Read the owning common challenge</Link>{!prepared && !unknown && <><h4>{document.title || 'Untitled unsent answer'}</h4><SafeMarkdown>{document.body}</SafeMarkdown></>}</section>
    {prepared && !unknown && <MentorTaskReview title={`Review common answer ${prepared.action === 'save' ? 'draft' : prepared.action === 'approve' ? 'approval' : 'publication'}`} pending={busy} disabled={!enabled || changed} confirmLabel={prepared.action === 'save' ? 'Save a new draft version' : prepared.action === 'approve' ? 'Approve saved version' : 'Publish approved version'} confirm={() => void write()} correct={() => { setPrepared(undefined); navigation.restore() }}><p>Answer {prepared.id ?? 'New draft'} · Version {prepared.version} · Expected answer revision {prepared.revision - 1}</p><p>Challenge {prepared.source.challenge_version} · Rubric {prepared.source.rubric.version}</p><p>{prepared.audience.label}: {prepared.audience.count} people at this native preview; publication rechecks the actual common audience.</p><h4>{prepared.document.title}</h4><SafeMarkdown>{prepared.document.body}</SafeMarkdown><p>{prepared.action === 'save' ? 'Save an unpublished draft only.' : prepared.action === 'approve' ? 'Approve this saved version; this action does not publish it.' : 'Publish this approved version to the native common audience. The source private question and attachments are excluded.'}</p></MentorTaskReview>}
    {!sourceCurrent && <p role="status">This saved draft applies to an earlier source. Review the owning version change and save an applicable draft before approval or publication.</p>}
    {unknown && <MentorUnknown intent={pending?.intentID ?? 'retained common-answer intent'} pending={busy} read={() => void reconcile()}>The original version/action remains unresolved. A matching current flag alone is not its receipt; exact immutable transition, actor, version, document and source are checked.</MentorUnknown>}
    <Button icon="refresh" variant="outline" disabled={busy} onClick={() => void reconcile()}>{unknown ? 'Check the exact server outcome' : 'Refresh current review'}</Button>{base?.published && <Link to="/events/$eventId/clarifications/$answerId" params={{ eventId: event.id, answerId: base.id }}>Read the official published answer</Link>}{notice && <p role="status">{notice}</p>}{!!failure && <Failure error={failure} />}
  </section>
}

import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { api, APIError, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { SafeMarkdown } from './components/safe-markdown'
import { denied, sameDocument, sameSource, textBytes, uncertain, useAccessFailure, useClarifications, useCurrentPage, usePrivateLeaveWarning, type AnswerDocument, type AnswerDraft, type Question, type Source } from './clarification-context'

type Review = components['schemas']['ClarificationReview']
type Pending = { action: 'save' | 'approve' | 'publish'; version: number; revision: number; document: AnswerDocument; source: Source; id?: string }
const empty: AnswerDocument = { title: '', body: '', kind: 'clarification' }
const exactDraft = (a: AnswerDraft | undefined | null, b: AnswerDraft | undefined | null) => (!a && !b) || (!!a && !!b && a.id === b.id && a.version === b.version && a.revision === b.revision)

export function OfficialAnswerEditor({ question, enabled }: { question: Question; enabled: boolean }) {
  const { key, event } = useClarifications()
  const query = useQuery({ queryKey: [...key, 'review', question.id], gcTime: 0, retry: false, refetchInterval: 10_000, refetchOnWindowFocus: 'always',
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/events/{event_id}/questions/{question_id}/review', { signal, cache: 'no-store', params: { path: { event_id: event.id, question_id: question.id } } })) })
  useAccessFailure(query.error)
  if (denied(query.error)) return null
  if (!query.data) return query.error ? <Failure error={query.error} retry={() => void query.refetch()} /> : <Loading />
  return <section className="panel clarification-private"><h2>Staff official-answer review</h2>{query.error && <Failure error={query.error} retry={() => void query.refetch()} />}<AnswerForm key={question.id} question={question} review={query.data} readFresh={!query.error} enabled={enabled && !query.error && query.data.workspace.can_review && !!query.data.workspace.review_audience} reload={async () => { const result = await query.refetch(); if (result.error) throw result.error; return result.data! }} /></section>
}

function AnswerForm({ question, review, readFresh, enabled, reload }: { question: Question; review: Review; readFresh: boolean; enabled: boolean; reload: () => Promise<Review> }) {
  const identity = useIdentity(), { event, key, deny } = useClarifications(), cache = useQueryClient(), active = useCurrentPage()
  const [base, setBase] = useState<AnswerDraft | undefined>(review.draft ?? undefined), [document, setDocument] = useState<AnswerDocument>(review.draft?.document ?? empty)
  const [busy, setBusy] = useState(false), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState(''), [pending, setPending] = useState<Pending>(), [comparison, setComparison] = useState<Review>()
  const dirty = !sameDocument(document, base?.document ?? empty), changed = readFresh && !exactDraft(base, review.draft)
  const currentSource = review.workspace.source, sourceCurrent = !base || sameSource(base.source, currentSource)
  usePrivateLeaveWarning(dirty || !!pending)
  const valid = !!document.title.trim() && !!document.body.trim() && textBytes(document.title) <= 200 && textBytes(document.body) <= 16000 && !document.title.includes('\0') && !document.body.includes('\0')
  async function write(action: 'save' | 'approve' | 'publish') {
    if (!enabled || busy || pending || changed || (action === 'save' ? !valid : !base || dirty || !sourceCurrent)) return
    setBusy(true); setFailure(undefined); setNotice(''); setComparison(undefined)
    let attempted: Pending | undefined
    let confirmed = false
    try {
      const latest = await reload()
      if (!active.current) return
      if (!exactDraft(base, latest.draft)) { setComparison(latest); setNotice('The server version changed. Compare it with your unsent text before another write.'); return }
      if (latest.workspace.archived || (action !== 'save' && !sameSource(base!.source, latest.workspace.source))) { setComparison(latest); setNotice('Current event or source policy changed. Review the authoritative state before another decision.'); return }
      attempted = { action, version: action === 'save' ? (base?.version ?? 0) + 1 : base!.version, revision: (base?.revision ?? 0) + 1, document: { ...document }, source: latest.workspace.source, id: base?.id }
      const result = action === 'save'
        ? unwrap(await api.PUT('/api/v1/events/{event_id}/questions/{question_id}/answer', { params: { path: { event_id: event.id, question_id: question.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${base?.revision ?? 0}"` } }, body: document }))
        : unwrap(await api.POST('/api/v1/events/{event_id}/clarifications/{answer_id}/actions', { params: { path: { event_id: event.id, answer_id: base!.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${base!.revision}"` } }, body: { action, version: base!.version } }))
      if (!active.current) return
      confirmed = true
      setBase(result); setDocument(result.document); setPending(undefined)
      setNotice(action === 'save' ? `Saved draft version ${result.version}. It is not an official common answer until separately approved and published.` : action === 'approve' ? `Version ${result.version} approved. It has not been published by this action.` : `Official version ${result.version} published. Safe common inbox references are recorded with publication.`)
      void cache.invalidateQueries({ queryKey: [...key, 'question-history', question.id] }); void cache.invalidateQueries({ queryKey: [...key, 'question', question.id] }); void cache.invalidateQueries({ queryKey: [...key, 'questions'] }); void cache.invalidateQueries({ queryKey: [...key, 'common'] }); await reload()
    } catch (error) {
      if (!active.current) return
      setFailure(error); deny(error)
      if (confirmed) { setNotice('The write was confirmed, but the follow-up read failed. Refresh the current review before another decision.'); return }
      if (attempted && uncertain(error)) { setPending(attempted); setNotice('Outcome unknown. No action will be repeated automatically. Check the exact current server draft/version before another write.') }
      else { setNotice('No new saved or published state was confirmed. Your editor text remains only in this page.'); if (error instanceof APIError && error.status === 409) setComparison(review) }
    } finally { if (active.current) setBusy(false) }
  }
  async function reconcile() {
    if (busy) return
    setBusy(true); setFailure(undefined)
    try {
      const latest = await reload()
      if (!active.current) return
      const draft = latest.draft
      if (!pending) { setComparison(exactDraft(base, draft) ? undefined : latest); setNotice('Current review refreshed. Any editor changes remain unsent until explicitly saved.'); return }
      const match = pending && draft && draft.version === pending.version && sameDocument(draft.document, pending.document) && sameSource(draft.source, pending.source) && (pending.action === 'save' ? draft.revision === pending.revision : draft.id === pending.id && (pending.action === 'approve' ? draft.approved : draft.published))
      if (match && draft) { setBase(draft); setDocument(draft.document); setPending(undefined); setComparison(undefined); setNotice(`The server confirms matching version ${draft.version}: ${draft.published ? 'published' : draft.approved ? 'approved, not published' : 'saved draft, not published'}. No action was replayed.`) }
      else { setComparison(latest); setNotice('Compare the authoritative server version with your unsent editor. An unmatched outcome remains unknown; no write was replayed.') }
    } catch (error) { if (active.current) { setFailure(error); deny(error) } }
    finally { if (active.current) setBusy(false) }
  }
  function adoptServer(keepText: boolean) {
    const latest = comparison ?? review
    if (!keepText && dirty && !window.confirm('Replace this page’s private unsent answer text with the current server draft?')) return
    setBase(latest.draft ?? undefined); if (!keepText) setDocument(latest.draft?.document ?? empty)
    setPending(undefined); setComparison(undefined); setFailure(undefined); setNotice(keepText ? 'Current server version selected. Your text is still unsent; save explicitly to create a new draft version.' : 'Current server draft loaded. No write was sent.')
  }
  return <><p>Authoring, approval and publication are separate decisions. Private question text and files are never copied into the common answer automatically.</p>
    <p role="status">{pending ? 'Outcome unknown — writes paused' : dirty ? 'Unsent editor changes — kept only in this page' : base ? `Server draft version ${base.version} · ${base.published ? 'published' : base.approved ? 'approved, not published' : 'not approved or published'}` : 'No server draft yet'}</p>
    {(changed || comparison) && <section className="notice"><h3>Current server version comparison</h3><p>{(comparison ?? review).draft ? `Server draft version ${(comparison ?? review).draft!.version}, revision ${(comparison ?? review).draft!.revision}` : 'No server draft is currently recorded.'}</p>{(comparison ?? review).draft && <><h4>{(comparison ?? review).draft!.document.title}</h4><SafeMarkdown>{(comparison ?? review).draft!.document.body}</SafeMarkdown></>}<div className="actions"><Button variant="outline" disabled={busy} onClick={() => adoptServer(false)}>Load current server draft</Button><Button variant="outline" disabled={busy} onClick={() => adoptServer(true)}>Keep my text against the current server version</Button></div></section>}
    <form onSubmit={e => { e.preventDefault(); void write('save') }}><fieldset disabled={!enabled || busy || !!pending}><legend>Unpublished answer editor</legend><label htmlFor="answer-kind">Type of answer</label><select id="answer-kind" value={document.kind} onChange={e => setDocument({ ...document, kind: e.target.value as AnswerDocument['kind'] })}><option value="clarification">Clarification of the same published version</option><option value="challenge_change">Notice of an already published challenge change</option><option value="rubric_change">Notice of an already reviewed and published rubric change</option></select><label htmlFor="answer-title">Common answer title</label><input id="answer-title" value={document.title} maxLength={200} onChange={e => setDocument({ ...document, title: e.target.value })} /><label htmlFor="answer-body">Common official answer text</label><textarea id="answer-body" value={document.body} maxLength={16000} rows={8} onChange={e => setDocument({ ...document, body: e.target.value })} /><p>{textBytes(document.body)} / 16,000 UTF-8 bytes. Check that the answer contains no private question text or attachments.</p></fieldset>
      <Button type="submit" disabled={!enabled || busy || !!pending || changed || !valid || (!!base && !dirty)}>Save a new draft version</Button>
    </form><section aria-label="Actual common audience preview"><h3>{readFresh ? 'Unpublished audience preview' : 'Last validated audience preview — refresh required'}</h3><p>{review.workspace.review_audience?.label}: {review.workspace.review_audience?.count ?? 'Unavailable'} people at this native preview. Publication rechecks current confirmed recipients.</p><p>Current challenge version {currentSource.challenge_version} · Published rubric version {currentSource.rubric.version}</p><p>{document.kind === 'clarification' ? 'This is an interpretation of the same exact published source.' : 'A material-change notice requires its owning version release, impact review and notifications first. Saving validates that evidence; this label cannot change criteria or scores.'}</p><Link to="/events/$eventId/resources/$resourceId" params={{ eventId: event.id, resourceId: currentSource.challenge_id }}>Read the owning common challenge</Link><h4>{document.title || 'Untitled unsent answer'}</h4><SafeMarkdown>{document.body}</SafeMarkdown></section>
    {!sourceCurrent && <p className="notice">This saved draft applies to an earlier source. Review the actual owning version change and save an applicable draft before approval or publication.</p>}
    <div className="actions"><Button variant="outline" disabled={!enabled || busy || !!pending || changed || !base || dirty || !sourceCurrent || base.approved} onClick={() => void write('approve')}>Approve saved version</Button><Button disabled={!enabled || busy || !!pending || changed || !base?.approved || dirty || !sourceCurrent || base.published} onClick={() => void write('publish')}>Publish approved version</Button><Button variant="outline" disabled={busy} onClick={() => void reconcile()}>{pending ? 'Check the exact server outcome' : 'Refresh current review'}</Button></div>
    {base?.published && <Link to="/events/$eventId/clarifications/$answerId" params={{ eventId: event.id, answerId: base.id }}>Read the official published answer</Link>}{notice && <p role="status">{notice}</p>}{!!failure && <Failure error={failure} />}
  </>
}

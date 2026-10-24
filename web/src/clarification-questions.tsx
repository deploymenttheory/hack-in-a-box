import { useEffect, useRef, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, unwrap } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Field, Select, Textarea } from './components/ui/fields'
import { Failure, Loading } from './components/states'
import { localDate } from './event-time'
import { denied, sameSource, textBytes, uncertain, useAccessFailure, useClarifications, useCurrentPage, type Question, type Source } from './clarification-context'
import { QuestionAttachmentUpload, type UploadedQuestionFile } from './clarification-question-files'
import { OfficialAnswerEditor } from './clarification-answer-editor'
import { MentorTaskReview, MentorUnknown, useMentorTaskNavigation } from './mentor-task'
import { usePlanningDepartureBarrier, usePlanningUnknownCommands } from './planning-operations'
import { ContextLink, CollectionPagination } from './components/patterns'

type Payload = { command_id: string; team_id: string; topic: 'challenge' | 'rubric'; body: string; attachments: string[] }
export function AskQuestion({ teamID, onLeaveRisk, onPending }: { teamID: string; onLeaveRisk: (risk: boolean) => void; onPending: (pending: boolean) => void }) {
  const identity = useIdentity(), { event, key, workspace, writable, deny, observe, refresh } = useClarifications(), cache = useQueryClient(), active = useCurrentPage()
  const history = usePlanningUnknownCommands(`${identity.person.id}:${identity.draft_scope}:${event.id}:${teamID}:ask`)
  const [body, setBody] = useState(''), [topic, setTopic] = useState<'challenge' | 'rubric'>('challenge'), [files, setFiles] = useState<UploadedQuestionFile[]>([]), [review, setReview] = useState<{ payload: Payload; source: Source }>()
  const [busy, setBusy] = useState(false), [uploading, setUploading] = useState(false), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState(''), [savedID, setSavedID] = useState<string>(), [attempt, setAttempt] = useState<Payload>()
  const unknown = !!attempt || (!busy && history.unconfirmed), pending = busy || uploading, leaveRisk = !!body || !!files.length || !!review || unknown || pending
  const root = useRef<HTMLElement>(null), lock = useRef(false), terminal = useRef(false), navigation = useMentorTaskNavigation({ root, dirty: !!body || !!files.length || !!review, pending, unknown })
  usePlanningDepartureBarrier(pending)
  useEffect(() => { onLeaveRisk(leaveRisk); onPending(pending); return () => { onLeaveRisk(false); onPending(false) } }, [leaveRisk, pending, onLeaveRisk, onPending])
  function prepare() { if (!writable || !workspace.can_ask || lock.current || unknown || uploading || !body.trim() || textBytes(body) > 8000 || body.includes('\0')) return; navigation.capture(); setReview({ payload: { command_id: crypto.randomUUID(), team_id: teamID, topic, body, attachments: files.map(file => file.id) }, source: structuredClone(workspace.source) }) }
  async function send(retry = false) {
    const payload = retry ? attempt : review?.payload
    if (!active.current() || !payload || !writable || !workspace.can_ask || lock.current || uploading || (!retry && (unknown || terminal.current))) return
    lock.current = true; setBusy(true); setFailure(undefined); setNotice(''); setSavedID(undefined)
    let attempted = false
    try {
      if (!await refresh('ask') || !active.current()) return
      const current = unwrap(await api.GET('/api/v1/events/{event_id}/clarification-workspace', { cache: 'no-store', params: { path: { event_id: event.id } } }))
      if (!active.current()) return
      observe(current)
      if (!active.current() || current.archived || !current.can_ask || current.team_id !== teamID) return
      if (!retry && review && !sameSource(review.source, current.source)) { setReview(undefined); setNotice('The common source changed. Read and review your question against the current challenge/rubric before sending. No question was sent.'); return }
      attempted = true
      if (!retry) history.record({ operation: 'Private team question', revision: 0, command: { intentID: payload.command_id } })
      const result = unwrap(await api.POST('/api/v1/events/{event_id}/questions', { params: { path: { event_id: event.id }, header: { 'X-CSRF-Token': identity.csrf_token } }, body: payload }))
      if (!active.current()) return
      if (result.team_id !== teamID || result.author_id !== identity.person.id || result.body !== payload.body || result.topic !== payload.topic || JSON.stringify(result.attachments) !== JSON.stringify(payload.attachments)) throw new Error('Unverified private question response')
      terminal.current = false; history.resolve(payload.command_id); setAttempt(undefined); setReview(undefined); setBody(''); setFiles([]); setSavedID(result.id); setNotice('Saved as a private team question. It is not a common official answer.'); navigation.restore(); void cache.invalidateQueries({ queryKey: [...key, 'questions'] })
    } catch (error) {
      if (!active.current()) return
      setFailure(error); deny(error)
      if (attempted && uncertain(error)) { terminal.current = true; setAttempt(payload); setNotice('The outcome is unknown. Your original question is retained only in this task. An explicit retry uses its same native command ID; nothing is repeated automatically.') }
      else { if (attempted && !retry) history.resolve(payload.command_id); setReview(undefined); setNotice(attempted ? 'This response did not confirm a saved question. Your text remains unsent in this page.' : 'Current authority/source could not be verified. No question was sent; your text remains unsent.') }
    } finally { lock.current = false; if (active.current()) setBusy(false) }
  }
  return <section ref={root} tabIndex={-1} className="clarification-private py-6 border-t border-border"><h2>Ask a private team question</h2><p>Only your current team and explicit organisers can read the original. This is not a submission or assessment.</p>
    <div hidden={!!review || unknown} inert={!!review || unknown}><form className="grid gap-4 max-w-3xl" onSubmit={e => { e.preventDefault(); prepare() }}><fieldset className="grid gap-4" disabled={!writable || busy || unknown || uploading}>
      <Field id="question-topic" label="About"><Select value={topic} onChange={e => setTopic(e.target.value as 'challenge' | 'rubric')}><option value="challenge">Challenge</option><option value="rubric">Published rubric</option></Select></Field>
      <Field id="question-body" label="Your question" description={`${textBytes(body)} / 8,000 UTF-8 bytes · Unsent text stays only in this task.`}><Textarea value={body} maxLength={8000} onChange={e => setBody(e.target.value)} rows={6} /></Field></fieldset>
      <QuestionAttachmentUpload teamID={teamID} files={files} change={setFiles} disabled={!writable || busy || unknown || !!review} onBusy={setUploading} />
      <Button type="submit" icon="next" disabled={!writable || pending || !body.trim() || textBytes(body) > 8000}>Review private question</Button></form></div>
    {review && !unknown && <MentorTaskReview title="Review private team question" pending={busy} confirmLabel="Send private question" confirm={() => void send()} correct={() => { setReview(undefined); navigation.restore() }}><p>Team {teamID} · Command {review.payload.command_id}</p><p>{review.payload.topic} · Challenge {review.source.challenge_version} · Rubric {review.source.rubric.version}</p><p className="whitespace-pre-wrap">{review.payload.body}</p><p>{review.payload.attachments.length} private attachments. These files and source text do not enter the common answer feed.</p></MentorTaskReview>}
    {unknown && <MentorUnknown intent={attempt?.command_id ?? 'retained private-question intent'} pending={busy} read={() => void refresh('ask')}>The original private question is unresolved. A current question list is not its command receipt.</MentorUnknown>}{attempt && <Button icon="refresh" variant="outline" disabled={!writable || pending} onClick={() => void send(true)}>Retry the same private question</Button>}
    {notice && <p role="status">{notice}</p>}{savedID && <Link to="/events/$eventId/questions/$questionId" params={{ eventId: event.id, questionId: savedID }}>Read the saved private question</Link>}{!!failure && <Failure error={failure} />}
  </section>
}
export function PrivateQuestionReader({ id }: { id: string }) {
  const { event, key, workspace, writable } = useClarifications()
  const query = useQuery({ queryKey: [...key, 'question', id], gcTime: 0, retry: false, refetchOnMount: 'always', refetchInterval: 10_000, refetchOnWindowFocus: 'always', queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/events/{event_id}/questions/{question_id}', { signal, cache: 'no-store', params: { path: { event_id: event.id, question_id: id } } })) })
  useAccessFailure(query.error)
  if (denied(query.error)) return null
  if (!query.data || !query.isFetchedAfterMount) return query.error ? <Failure error={query.error} retry={() => void query.refetch()} /> : <Loading />
  const question = query.data, attachmentsOpen = workspace.team_id === question.team_id || (workspace.can_review && workspace.staff_attachments_open)
  return <section className="clarification-private"><ContextLink back target={{ label: 'Questions and official answers', href: `/events/${event.id}/clarifications` }} />{query.error && <><Failure error={query.error} retry={() => void query.refetch()} /><p>Actions are paused until the authoritative question reloads.</p></>}
    <section className="py-6 border-b border-border"><p className="fleet-scope-label">Private {question.topic} question · {question.status}</p><h2>Original team question</h2><p>Asked {localDate(question.created_at, event.configuration.timezone)} · Question revision {question.revision}</p><p className="whitespace-pre-wrap">{question.body}</p><p>Asked against challenge {question.source.challenge_version} and rubric {question.source.rubric.version}.</p>
      {question.attachments.length > 0 && <section className="py-4"><h3>Private attachments</h3><ul>{question.attachments.map((fileID, index) => <li className="py-2" key={fileID}>{attachmentsOpen ? <a href={`/api/v1/events/${event.id}/questions/${id}/attachments/${fileID}`}>Download private attachment {index + 1}</a> : <span>Private attachment {index + 1} · Staff byte access opens at live start</span>}</li>)}</ul><p>Downloads recheck current question, team and live-start authority.</p></section>}
      {workspace.can_review && <QuestionTriage key={`${id}:${workspace.can_review}`} question={question} enabled={writable && !query.error} refresh={async () => !(await query.refetch()).error} />}
    </section><PrivateQuestionHistory id={id} />{workspace.can_review && <OfficialAnswerEditor key={`${id}:${workspace.can_review}`} question={question} enabled={writable && !query.error} />}
  </section>
}
function QuestionTriage({ question, enabled, refresh }: { question: Question; enabled: boolean; refresh: () => Promise<boolean> }) {
  const identity = useIdentity(), { event, key, deny, observe, workspace } = useClarifications(), active = useCurrentPage(), cache = useQueryClient()
  const [busy, setBusy] = useState(false), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState(''), [original, setOriginal] = useState<{ action: 'triage' | 'close' | 'reopen'; revision: number }>(), [review, setReview] = useState<{ action: 'triage' | 'close' | 'reopen'; revision: number }>()
  const history = usePlanningUnknownCommands(`${identity.person.id}:${identity.draft_scope}:${event.id}:${question.id}:triage`), lock = useRef(false), terminal = useRef(false), root = useRef<HTMLElement>(null)
  const unknown = !!original || (!busy && history.unconfirmed), navigation = useMentorTaskNavigation({ root, dirty: !!review, pending: busy, unknown })
  usePlanningDepartureBarrier(busy)
  async function send() {
    if (!active.current() || !enabled || !workspace.can_review || lock.current || terminal.current || unknown || !review || review.revision !== question.revision) return
    lock.current = true; let attempted = false; setBusy(true); setFailure(undefined); setNotice('')
    try { const current = unwrap(await api.GET('/api/v1/events/{event_id}/clarification-workspace', { params: { path: { event_id: event.id } }, cache: 'no-store' })); if (!active.current()) return; observe(current); if (!active.current() || !current.can_review || current.archived) return
      attempted = true
      history.record({ operation: 'Private question triage', revision: review.revision, command: { intentID: `${question.id}:${review.action}:${review.revision + 1}` } })
      unwrap(await api.POST('/api/v1/events/{event_id}/questions/{question_id}/triage', { params: { path: { event_id: event.id, question_id: question.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${review.revision}"` } }, body: { action: review.action } })); if (!active.current()) return
      history.resolve(`${question.id}:${review.action}:${review.revision + 1}`); setReview(undefined); setNotice('Triage decision recorded.'); navigation.restore(); void cache.invalidateQueries({ queryKey: [...key, 'question-history', question.id] }); await refresh()
    } catch (error) { if (active.current()) { setFailure(error); deny(error); if (attempted && uncertain(error)) { terminal.current = true; setOriginal(review); setNotice('Outcome unknown. Only exact immutable question history can establish the original decision.') } else { if (attempted) history.resolve(`${question.id}:${review.action}:${review.revision + 1}`); setReview(undefined); setNotice('No triage decision was confirmed. Refresh before choosing again.'); await refresh() } } } finally { lock.current = false; if (active.current()) setBusy(false) }
  }
  async function reconcile() {
    if (busy || lock.current || !original) { await refresh(); return }
    lock.current = true; setBusy(true)
    try { let cursor: string | undefined, found = false
      do { const page = unwrap(await api.GET('/api/v1/events/{event_id}/questions/{question_id}/history', { params: { path: { event_id: event.id, question_id: question.id }, query: { cursor, limit: 30 } }, cache: 'no-store' })); if (!active.current()) return; found = page.items.some(t => t.actor_id === identity.person.id && t.revision === original.revision + 1 && t.action === ({ triage: 'triaged', close: 'closed', reopen: 'reopened' }[original.action])); cursor = page.next_cursor ?? undefined; if (found) break } while (cursor)
      if (found) { history.resolve(`${question.id}:${original.action}:${original.revision + 1}`); terminal.current = false; setOriginal(undefined); setReview(undefined); setNotice('Exact original triage transition confirmed from immutable history. No action was replayed.'); await refresh() } else setNotice('No exact original triage transition was confirmed. Current question state is not its receipt; writes remain paused.')
    } catch (error) { if (active.current()) { setFailure(error); deny(error) } } finally { lock.current = false; if (active.current()) setBusy(false) }
  }
  function prepare(action: 'triage' | 'close' | 'reopen') { if (!enabled || unknown || lock.current) return; navigation.capture(); setReview({ action, revision: question.revision }) }
  return <section ref={root} tabIndex={-1} className="py-4 border-t border-border"><h3>Explicit staff triage</h3>{!review && !unknown && <div className="fleet-pattern-actions"><Button icon="next" variant="outline" disabled={!enabled || busy || question.status !== 'open'} onClick={() => prepare('triage')}>Mark triaged</Button><Button icon="close" variant="outline" disabled={!enabled || busy || question.status === 'closed'} onClick={() => prepare('close')}>Close question</Button><Button icon="refresh" variant="outline" disabled={!enabled || busy || question.status !== 'closed'} onClick={() => prepare('reopen')}>Reopen question</Button></div>}
    {review && !unknown && <MentorTaskReview title="Review private question triage" pending={busy} disabled={!enabled || review.revision !== question.revision} confirm={() => void send()} correct={() => { setReview(undefined); navigation.restore() }}><p>Private question {question.id} · Expected revision {review.revision}</p><p>Action: {review.action} · Current status: {question.status}</p><p>This does not approve or publish a common answer.</p></MentorTaskReview>}
    {unknown && <MentorUnknown intent={`${question.id}:triage`} pending={busy} read={() => void reconcile()}>This original triage decision is unresolved. A current question refresh alone never clears the write barrier.</MentorUnknown>}{unknown && <Button icon="refresh" variant="outline" disabled={busy} onClick={() => void reconcile()}>Refresh question outcome</Button>}{notice && <p role="status">{notice}</p>}{!!failure && <Failure error={failure} />}
  </section>
}
function PrivateQuestionHistory({ id }: { id: string }) {
  const { event, key } = useClarifications()
  const query = useInfiniteQuery({ queryKey: [...key, 'question-history', id], gcTime: 0, retry: false, initialPageParam: undefined as string | undefined, queryFn: async ({ pageParam, signal }) => unwrap(await api.GET('/api/v1/events/{event_id}/questions/{question_id}/history', { signal, cache: 'no-store', params: { path: { event_id: event.id, question_id: id }, query: { cursor: pageParam, limit: 30 } } })), getNextPageParam: page => page.next_cursor ?? undefined, refetchInterval: 10_000, refetchOnWindowFocus: 'always' })
  useAccessFailure(query.error)
  if (denied(query.error)) return null
  const items = query.data?.pages.flatMap(page => page.items) ?? []
  return <section className="py-6 border-t border-border"><h2>Private question history</h2>{query.isPending ? <Loading /> : query.error ? <Failure error={query.error} retry={() => void query.refetch()} /> : <><ol>{items.map(item => <li className="py-3 border-b border-border" key={item.id}>{item.action.replaceAll('_', ' ')} · {localDate(item.at, event.configuration.timezone)}{item.version && ` · Answer version ${item.version}`}</li>)}</ol><CollectionPagination source={{ kind: 'cursor', loaded: items.length, busy: query.isFetchingNextPage, next: query.hasNextPage ? () => void query.fetchNextPage() : undefined }} /></>}</section>
}

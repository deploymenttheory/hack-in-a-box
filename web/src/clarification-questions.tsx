import { useEffect, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, unwrap } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { localDate } from './event-time'
import { denied, textBytes, uncertain, useAccessFailure, useClarifications, useCurrentPage, usePrivateLeaveWarning, type Question } from './clarification-context'
import { QuestionAttachmentUpload, type UploadedQuestionFile } from './clarification-question-files'
import { OfficialAnswerEditor } from './clarification-answer-editor'

export function AskQuestion({ teamID, onLeaveRisk }: { teamID: string; onLeaveRisk: (risk: boolean) => void }) {
  const identity = useIdentity(), { event, key, writable, deny, refresh } = useClarifications(), cache = useQueryClient(), active = useCurrentPage()
  const [body, setBody] = useState(''), [topic, setTopic] = useState<'challenge' | 'rubric'>('challenge'), [files, setFiles] = useState<UploadedQuestionFile[]>([])
  const [busy, setBusy] = useState(false), [uploading, setUploading] = useState(false), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState(''), [savedID, setSavedID] = useState<string>()
  const [attempt, setAttempt] = useState<{ command_id: string; team_id: string; topic: 'challenge' | 'rubric'; body: string; attachments: string[] }>()
  const unknown = !!attempt
  const leaveRisk = !!body || !!files.length || unknown || busy || uploading
  usePrivateLeaveWarning(leaveRisk)
  useEffect(() => { onLeaveRisk(leaveRisk); return () => onLeaveRisk(false) }, [leaveRisk, onLeaveRisk])
  async function send() {
    if (!writable || busy || uploading || !body.trim() || textBytes(body) > 8000 || body.includes('\0')) { setNotice('Enter a question up to 8,000 UTF-8 bytes before sending.'); return }
    setBusy(true); setFailure(undefined); setNotice(''); setSavedID(undefined)
    const payload = attempt ?? { command_id: crypto.randomUUID(), team_id: teamID, topic, body, attachments: files.map(file => file.id) }
    try {
      if (!await refresh() || !active.current) return
      const result = unwrap(await api.POST('/api/v1/events/{event_id}/questions', { params: { path: { event_id: event.id }, header: { 'X-CSRF-Token': identity.csrf_token } }, body: payload }))
      if (!active.current) return
      setAttempt(undefined); setBody(''); setFiles([]); setSavedID(result.id); setNotice('Saved as a private team question. It is not a common official answer.')
      void cache.invalidateQueries({ queryKey: [...key, 'questions'] })
    } catch (error) {
      if (!active.current) return
      setFailure(error); deny(error)
      if (uncertain(error)) { setAttempt(payload); setNotice('The outcome is unknown. Your original question is retained only in this page. An explicit retry uses the same command ID; no new question is created for an identical retry.') }
      else { setAttempt(undefined); setNotice('This response did not confirm a saved question. Your text remains unsent in this page.') }
    } finally { if (active.current) setBusy(false) }
  }
  return <section className="panel clarification-private"><h2>Ask a private team question</h2><p>Only your current team and explicit organisers can read the original. This is not a submission or assessment.</p>
    <form onSubmit={e => { e.preventDefault(); void send() }}><fieldset disabled={!writable || busy || unknown || uploading}><legend>Private question</legend><label htmlFor="question-topic">About</label><select id="question-topic" value={topic} onChange={e => setTopic(e.target.value as 'challenge' | 'rubric')}><option value="challenge">Challenge</option><option value="rubric">Published rubric</option></select><label htmlFor="question-body">Your question</label><textarea id="question-body" value={body} maxLength={8000} onChange={e => setBody(e.target.value)} rows={6} /><p className="muted">{textBytes(body)} / 8,000 UTF-8 bytes · Unsent text stays only in this page.</p></fieldset>
      <QuestionAttachmentUpload teamID={teamID} files={files} change={setFiles} disabled={!writable || busy || unknown} onBusy={setUploading} />
      <Button type="submit" disabled={!writable || busy || uploading || !body.trim() || textBytes(body) > 8000}>{busy ? 'Checking and sending…' : unknown ? 'Retry the same private question' : 'Send private question'}</Button>
    </form>{notice && <p role="status">{notice}</p>}{savedID && <Link to="/events/$eventId/questions/$questionId" params={{ eventId: event.id, questionId: savedID }}>Read the saved private question</Link>}{!!failure && <Failure error={failure} />}
  </section>
}

export function PrivateQuestionReader({ id }: { id: string }) {
  const { event, key, workspace, writable } = useClarifications()
  const query = useQuery({ queryKey: [...key, 'question', id], gcTime: 0, retry: false, refetchInterval: 10_000, refetchOnWindowFocus: 'always',
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/events/{event_id}/questions/{question_id}', { signal, cache: 'no-store', params: { path: { event_id: event.id, question_id: id } } })) })
  useAccessFailure(query.error)
  if (denied(query.error)) return null
  if (!query.data) return query.error ? <Failure error={query.error} retry={() => void query.refetch()} /> : <Loading />
  const question = query.data, attachmentsOpen = workspace.team_id === question.team_id || (workspace.can_review && workspace.staff_attachments_open)
  return <section className="clarification-private"><Link to="/events/$eventId/clarifications" params={{ eventId: event.id }}>← Questions and official answers</Link>{query.error && <><Failure error={query.error} retry={() => void query.refetch()} /><p>Actions are paused until the authoritative question reloads.</p></>}
    <article className="panel"><p className="eyebrow">Private {question.topic} question · {question.status}</p><h2>Original team question</h2><p>Asked {localDate(question.created_at, event.configuration.timezone)} · Question revision {question.revision}</p><p className="private-question-text">{question.body}</p><p>Asked against challenge {question.source.challenge_version} and rubric {question.source.rubric.version}.</p>
      {question.attachments.length > 0 && <section><h3>Private attachments</h3><ul>{question.attachments.map((fileID, index) => <li key={fileID}>{attachmentsOpen ? <a href={`/api/v1/events/${event.id}/questions/${id}/attachments/${fileID}`}>Download private attachment {index + 1}</a> : <span>Private attachment {index + 1} · Staff byte access opens at live start</span>}</li>)}</ul><p>Downloads recheck current question, team and live-start authority.</p></section>}
      {workspace.can_review && <QuestionTriage question={question} enabled={writable && !query.error} refresh={async () => !(await query.refetch()).error} />}
    </article><PrivateQuestionHistory id={id} />{workspace.can_review && <OfficialAnswerEditor question={question} enabled={writable && !query.error} />}
  </section>
}

function QuestionTriage({ question, enabled, refresh }: { question: Question; enabled: boolean; refresh: () => Promise<boolean> }) {
  const identity = useIdentity(), { event, key, deny } = useClarifications(), active = useCurrentPage(), cache = useQueryClient()
  const [busy, setBusy] = useState(false), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState(''), [unknown, setUnknown] = useState(false)
  async function act(action: 'triage' | 'close' | 'reopen') {
    if (!enabled || busy || unknown) return
    setBusy(true); setFailure(undefined); setNotice('')
    try {
      unwrap(await api.POST('/api/v1/events/{event_id}/questions/{question_id}/triage', { params: { path: { event_id: event.id, question_id: question.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${question.revision}"` } }, body: { action } }))
      if (!active.current) return
      setNotice('Triage decision recorded.'); void cache.invalidateQueries({ queryKey: [...key, 'question-history', question.id] }); await refresh()
    } catch (error) { if (active.current) { setFailure(error); deny(error); setUnknown(uncertain(error)); setNotice(uncertain(error) ? 'Outcome unknown. Refresh authoritative question state before another triage action.' : 'No triage decision was confirmed. Refresh before trying again.') } }
    finally { if (active.current) setBusy(false) }
  }
  async function reconcile() { const refreshed = await refresh(); if (active.current && refreshed) { setUnknown(false); setNotice('Authoritative question state refreshed. Review its current status before another action.') } }
  return <section><h3>Explicit staff triage</h3><div className="actions"><Button variant="outline" disabled={!enabled || busy || unknown || question.status !== 'open'} onClick={() => void act('triage')}>Mark triaged</Button><Button variant="outline" disabled={!enabled || busy || unknown || question.status === 'closed'} onClick={() => void act('close')}>Close question</Button><Button variant="outline" disabled={!enabled || busy || unknown || question.status !== 'closed'} onClick={() => void act('reopen')}>Reopen question</Button>{unknown && <Button variant="outline" disabled={busy} onClick={() => void reconcile()}>Refresh question outcome</Button>}</div>{notice && <p role="status">{notice}</p>}{!!failure && <Failure error={failure} />}</section>
}

function PrivateQuestionHistory({ id }: { id: string }) {
  const { event, key } = useClarifications()
  const query = useInfiniteQuery({ queryKey: [...key, 'question-history', id], gcTime: 0, retry: false, initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam, signal }) => unwrap(await api.GET('/api/v1/events/{event_id}/questions/{question_id}/history', { signal, cache: 'no-store', params: { path: { event_id: event.id, question_id: id }, query: { cursor: pageParam, limit: 30 } } })), getNextPageParam: page => page.next_cursor ?? undefined, refetchInterval: 10_000, refetchOnWindowFocus: 'always' })
  useAccessFailure(query.error)
  if (denied(query.error)) return null
  return <section className="panel"><h2>Private question history</h2>{query.isPending ? <Loading /> : query.error ? <Failure error={query.error} retry={() => void query.refetch()} /> : <><ol>{query.data?.pages.flatMap(page => page.items).map(item => <li key={item.id}>{item.action.replaceAll('_', ' ')} · {localDate(item.at, event.configuration.timezone)}{item.version && ` · Answer version ${item.version}`}</li>)}</ol>{query.hasNextPage && <Button onClick={() => void query.fetchNextPage()} disabled={query.isFetchingNextPage}>More question history</Button>}</>}</section>
}

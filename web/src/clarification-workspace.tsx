import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useParams } from '@tanstack/react-router'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, privateKey, unwrap } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { localDate } from './event-time'
import { ClarificationContext, denied, useAccessFailure, useClarifications } from './clarification-context'
import { AskQuestion, PrivateQuestionReader } from './clarification-questions'

export function ClarificationWorkspacePage() {
  const { eventId, questionId } = useParams({ strict: false })
  const identity = useIdentity()
  return eventId ? <ClarificationWorkspace key={`${identity.person.id}:${identity.draft_scope}:${eventId}:${questionId ?? ''}`} eventID={eventId} questionID={questionId} /> : <Failure error={new Error('Missing event')} />
}

function ClarificationWorkspace({ eventID, questionID }: { eventID: string; questionID?: string }) {
  const identity = useIdentity(), cache = useQueryClient()
  const key = useMemo(() => [...privateKey(identity.person.id, eventID, 'clarifications'), identity.draft_scope], [identity.person.id, identity.draft_scope, eventID])
  const [failure, setFailure] = useState<unknown>(), [tab, setTab] = useState('common')
  const [privateLeaveRisk, setPrivateLeaveRisk] = useState(false)
  function selectTab(next: string) {
    if (next === tab) return
    if (tab === 'private' && privateLeaveRisk && !window.confirm('Leave this private unsent text or uncertain operation? Text and the retained retry command are kept only in this page and will be cleared.')) return
    setTab(next)
  }
  const workspace = useQuery({ queryKey: [...key, 'access'], gcTime: 0, retry: false, refetchOnWindowFocus: 'always', refetchInterval: 10_000,
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/events/{event_id}/clarification-workspace', { signal, cache: 'no-store', params: { path: { event_id: eventID } } })) })
  const event = useQuery({ queryKey: [...key, 'event'], gcTime: 0, retry: false, refetchOnWindowFocus: 'always', refetchInterval: 10_000,
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/events/{event_id}', { signal, cache: 'no-store', params: { path: { event_id: eventID } } })) })
  const deny = useCallback((error: unknown) => { if (denied(error)) setFailure(error) }, [])
  const inaccessible = failure || (denied(workspace.error) ? workspace.error : denied(event.error) ? event.error : undefined)
  useEffect(() => () => { void cache.cancelQueries({ queryKey: key }); cache.removeQueries({ queryKey: key }) }, [cache, key])
  useEffect(() => { if (inaccessible) cache.removeQueries({ predicate: query => query.queryKey.length > key.length && query.queryKey.slice(0, key.length).every((part, i) => part === key[i]) && !['access', 'event'].includes(String(query.queryKey[key.length])) }) }, [inaccessible, cache, key])
  async function refresh() {
    const [access, currentEvent] = await Promise.all([workspace.refetch(), event.refetch()])
    if (!access.error && !currentEvent.error) { setFailure(undefined); return true }
    return false
  }
  const value = workspace.data, record = event.data
  if (inaccessible) return <section className="workspace"><Link to="/events/$eventId" params={{ eventId: eventID }}>← Back to event</Link><h1>Questions and official clarifications</h1><p>Common challenge questions require actual publication and current access.</p><Failure error={inaccessible} retry={() => void refresh()} /></section>
  if (!value || !record) return workspace.error || event.error ? <Failure error={workspace.error || event.error} retry={() => void refresh()} /> : <Loading />
  return <ClarificationContext.Provider value={{ event: record, workspace: value, key, deny, refresh, writable: !workspace.error && !event.error && !value.archived }}><section className="workspace clarification-workspace">
    <Link to="/events/$eventId" params={{ eventId: eventID }}>← Back to event</Link><h1>{questionID ? 'Private team question' : 'Questions and official clarifications'}</h1>
    <p>{record.name} · Times in {record.configuration.timezone}</p>
    {(workspace.error || event.error) && <><Failure error={workspace.error || event.error} retry={() => void refresh()} /><p role="status">Current access could not be refreshed. Private unsent text remains only in this page; actions are paused.</p></>}
    {value.archived && <p className="notice">This event is read-only. Existing authorised questions and official answers remain available.</p>}
    {questionID ? <PrivateQuestionReader id={questionID} /> : <><nav className="tabs" aria-label="Clarification sections"><Button variant="outline" aria-pressed={tab === 'common'} onClick={() => selectTab('common')}>Official answers</Button>{(value.team_id || value.can_review) && <Button variant="outline" aria-pressed={tab === 'private'} onClick={() => selectTab('private')}>{value.can_review ? 'Staff question queue' : "Your team's private questions"}</Button>}</nav>
      {tab === 'common' || (!value.team_id && !value.can_review) ? <CommonAnswers /> : <section className="clarification-private"><p>Original questions and attachments stay private to the current team and explicit organisers. Only reviewed answers enter the common feed.</p>{value.team_id && (value.can_ask || value.archived) && <AskQuestion key={value.team_id} teamID={value.team_id} onLeaveRisk={setPrivateLeaveRisk} />}<PrivateQuestionList /></section>}
    </>}
  </section></ClarificationContext.Provider>
}

function CommonAnswers() {
  const { key, event, workspace } = useClarifications()
  const [input, setInput] = useState(''), [search, setSearch] = useState('')
  const query = useInfiniteQuery({ queryKey: [...key, 'common', search], gcTime: 0, retry: false, initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam, signal }) => unwrap(await api.GET('/api/v1/events/{event_id}/clarifications', { signal, cache: 'no-store', params: { path: { event_id: event.id }, query: { search: search || undefined, cursor: pageParam, limit: 30 } } })), getNextPageParam: page => page.next_cursor ?? undefined, refetchOnWindowFocus: 'always', refetchInterval: 10_000 })
  useAccessFailure(query.error)
  if (denied(query.error)) return null
  const items = query.data?.pages.flatMap(page => page.items) ?? []
  return <section><h2>Official common answers</h2><p>Every current confirmed attendee receives the same published answers. Private questions are not searched here.</p><p><Link to="/events/$eventId/resources/$resourceId" params={{ eventId: event.id, resourceId: workspace.source.challenge_id }}>Read the current common challenge</Link> · Challenge version {workspace.source.challenge_version} · Published rubric version {workspace.source.rubric.version}</p>
    {!workspace.can_review && <PublishedRubric />}
    <form role="search" onSubmit={e => { e.preventDefault(); setSearch(input.trim()) }}><label htmlFor="clarification-search">Search published answers</label><div className="actions"><input id="clarification-search" type="search" maxLength={200} value={input} onChange={e => setInput(e.target.value)} /><Button type="submit">Search official answers</Button>{search && <Button type="button" variant="outline" onClick={() => { setInput(''); setSearch('') }}>Clear answer search</Button>}</div></form>
    {query.isPending ? <Loading /> : query.error ? <Failure error={query.error} retry={() => void query.refetch()} /> : <><div className="event-grid">{items.map(answer => <article className="panel" key={answer.id}><p className="eyebrow">Official answer · version {answer.version}</p><h3><Link to="/events/$eventId/clarifications/$answerId" params={{ eventId: event.id, answerId: answer.id }}>{answer.document.title}</Link></h3><p>{answer.author_name} · {localDate(answer.published_at, event.configuration.timezone)}</p><p>Challenge {answer.source.challenge_version} · Rubric {answer.source.rubric.version}</p>{!answer.source_current && <p className="notice">Applies to an earlier published challenge or rubric.</p>}{answer.version > 1 && <p>Correction history retained in the canonical answer.</p>}</article>)}</div>{!items.length && <p className="empty-state">No published answers match this search.</p>}{query.hasNextPage && <Button disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>More official answers</Button>}</>}
  </section>
}

function PublishedRubric() {
  const { event, key } = useClarifications(), [open, setOpen] = useState(false)
  const query = useQuery({ queryKey: [...key, 'published-rubric'], enabled: open, gcTime: 0, retry: false, refetchOnWindowFocus: 'always', refetchInterval: 10_000,
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/events/{event_id}/rubric', { signal, cache: 'no-store', params: { path: { event_id: event.id } } })) })
  useAccessFailure(query.error)
  if (denied(query.error)) return null
  const page = query.data
  return <details id="published-rubric" className="panel" onToggle={e => setOpen(e.currentTarget.open)}><summary>Read the current published rubric</summary>{open && (query.isPending ? <Loading /> : query.error ? <Failure error={query.error} retry={() => void query.refetch()} /> : page && <section><h3>{page.definition.name} · version {page.reference.version}</h3><p>Published {localDate(page.published_at, event.configuration.timezone)} · {page.definition.mode === 'all_teams' ? 'All teams assessment' : 'Mentor qualification'}</p>{page.notice && <p className="notice">{page.notice}</p>}<p>Required judges: {page.definition.required_judge_count}{page.definition.mode === 'mentor_qualification' && ` · Finalist places ${page.definition.finalist_count} · Reserves ${page.definition.reserve_count}`}</p>
    {page.definition.categories.map(category => <section key={category.id}><h4>{category.label}</h4><p>Range {category.min_score}–{category.max_score} · Weight {category.weight}</p><p>{category.guidance}</p><ul>{category.anchors.map(anchor => <li key={anchor.score}>{anchor.score}: {anchor.description}</li>)}</ul></section>)}
    {page.definition.prizes.length > 0 && <section><h4>Published prizes</h4><ul>{page.definition.prizes.map(prize => <li key={prize.id}>{prize.name} · {prize.description}</li>)}</ul></section>}{page.definition.side_quests.length > 0 && <section><h4>Side quests</h4><p>Evidence guidance only; this does not change assessment scores.</p><ul>{page.definition.side_quests.map(quest => <li key={quest.id}>{quest.title} · {quest.evidence_guidance}</li>)}</ul></section>}</section>)}</details>
}

function PrivateQuestionList() {
  const { key, event, workspace } = useClarifications()
  const query = useInfiniteQuery({ queryKey: [...key, 'questions'], gcTime: 0, retry: false, initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam, signal }) => unwrap(await api.GET('/api/v1/events/{event_id}/questions', { signal, cache: 'no-store', params: { path: { event_id: event.id }, query: { cursor: pageParam, limit: 30 } } })), getNextPageParam: page => page.next_cursor ?? undefined, refetchOnWindowFocus: 'always', refetchInterval: 10_000 })
  useAccessFailure(query.error)
  if (denied(query.error)) return null
  const items = query.data?.pages.flatMap(page => page.items) ?? []
  return <section><h2>{workspace.can_review ? 'Authorised private question queue' : "Your team's questions"}</h2>{query.isPending ? <Loading /> : query.error ? <Failure error={query.error} retry={() => void query.refetch()} /> : <><ul className="item-list">{items.map(question => <li key={question.id}><div><Link to="/events/$eventId/questions/$questionId" params={{ eventId: event.id, questionId: question.id }}>{question.topic === 'rubric' ? 'Rubric question' : 'Challenge question'} · {localDate(question.created_at, event.configuration.timezone)}</Link><p>{question.status} · Question revision {question.revision}</p></div></li>)}</ul>{!items.length && <p className="empty-state">No private questions are available to your current role and team.</p>}{query.hasNextPage && <Button disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>More private questions</Button>}</>}</section>
}

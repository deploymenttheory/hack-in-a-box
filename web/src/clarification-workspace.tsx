import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useParams } from '@tanstack/react-router'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, privateKey, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { localDate } from './event-time'
import { ClarificationContext, denied, useAccessFailure, useClarifications } from './clarification-context'
import { AskQuestion, PrivateQuestionReader } from './clarification-questions'
import { ActionQueue, CollectionHeading, CollectionPagination, CollectionRecords, CollectionToolbar, ContextLink } from './components/patterns'
import { usePrivateRequestScope } from './private-request-scope'
import { PlanningOperations } from './planning-operations'

type Workspace = components['schemas']['ClarificationWorkspace']
export function ClarificationWorkspacePage() {
  const { eventId, questionId } = useParams({ strict: false }), identity = useIdentity()
  return eventId ? <PlanningOperations key={`${identity.person.id}:${identity.draft_scope}:${eventId}`}><ClarificationWorkspace key={`${identity.person.id}:${identity.draft_scope}:${eventId}`} eventID={eventId} questionID={questionId} /></PlanningOperations> : <Failure error={new Error('Missing event')} />
}
function ClarificationWorkspace({ eventID, questionID }: { eventID: string; questionID?: string }) {
  const identity = useIdentity(), cache = useQueryClient()
  const key = useMemo(() => [...privateKey(identity.person.id, eventID, 'clarifications'), identity.draft_scope], [identity.person.id, identity.draft_scope, eventID])
  const scope = usePrivateRequestScope(key.join(':'))
  const [failure, setFailure] = useState<unknown>(), [tab, setTab] = useState('common'), [privateEpoch, setPrivateEpoch] = useState(0), [privateLeaveRisk, setPrivateLeaveRisk] = useState(false), [privatePending, setPrivatePending] = useState(false)
  const privateScope = usePrivateRequestScope(`${key.join(':')}:private:${privateEpoch}`, scope)
  const scrub = useCallback((includeCommon = false) => {
    const predicate = (query: { queryKey: readonly unknown[] }) => query.queryKey.length > key.length && query.queryKey.slice(0, key.length).every((part, i) => part === key[i]) && !(includeCommon ? ['access', 'event'] : ['access', 'event', 'common', 'published-rubric']).includes(String(query.queryKey[key.length]))
    privateScope.invalidate(); void cache.cancelQueries({ predicate }); cache.removeQueries({ predicate }); setPrivateEpoch(value => value + 1); setPrivateLeaveRisk(false); setTab('common')
  }, [cache, key, privateScope])
  const observe = useCallback((value: Workspace) => {
    const previous = cache.getQueryData<Workspace>([...key, 'access'])
    if (previous && ((previous.can_review && !value.can_review) || previous.team_id !== value.team_id)) scrub()
    cache.setQueryData([...key, 'access'], value)
  }, [cache, key, scrub])
  const workspace = useQuery({ queryKey: [...key, 'access'], gcTime: 0, retry: false, refetchOnMount: 'always', refetchOnWindowFocus: 'always', refetchInterval: 10_000, queryFn: async ({ signal }) => {
    const ticket = scope.capture()
    try { const value = unwrap(await api.GET('/api/v1/events/{event_id}/clarification-workspace', { signal, cache: 'no-store', params: { path: { event_id: eventID } } })); ticket.assert(); observe(value); return value } catch (error) { if (ticket.current() && denied(error)) scrub(true); throw error }
  } })
  const event = useQuery({ queryKey: [...key, 'event'], gcTime: 0, retry: false, refetchOnWindowFocus: 'always', refetchInterval: 10_000, queryFn: async ({ signal }) => { const ticket = scope.capture(); const value = unwrap(await api.GET('/api/v1/events/{event_id}', { signal, cache: 'no-store', params: { path: { event_id: eventID } } })); ticket.assert(); return value } })
  const deny = useCallback((error: unknown) => { if (denied(error)) { scrub(true); setFailure(error) } }, [scrub])
  const inaccessible = failure || (denied(workspace.error) ? workspace.error : denied(event.error) ? event.error : undefined)
  useEffect(() => () => { void cache.cancelQueries({ queryKey: key }); cache.removeQueries({ queryKey: key }) }, [cache, key])
  async function refresh(capability?: 'ask' | 'review') {
    const [access, currentEvent] = await Promise.all([workspace.refetch(), event.refetch()])
    if (!access.error && !currentEvent.error) { setFailure(undefined); return !!access.data && !access.data.archived && (capability === 'ask' ? access.data.can_ask : capability === 'review' ? access.data.can_review : true) }
    return false
  }
  function selectTab(next: string) { if (privatePending || next === tab) return; if (tab === 'private' && privateLeaveRisk && !window.confirm('Leave this private unsent text or uncertain operation? Text and the retained retry command are kept only in this page and will be cleared.')) return; setTab(next) }
  const value = workspace.data, record = event.data
  if (inaccessible) return <section><ContextLink back target={{ label: 'Back to event', href: `/events/${eventID}` }} /><CollectionHeading title="Questions and official clarifications" scope={{ entity: 'Current event' }} description="Common challenge questions require actual publication and current access." /><Failure error={inaccessible} retry={() => void refresh()} /></section>
  if (!value || !record || !workspace.isFetchedAfterMount) return workspace.error || event.error ? <Failure error={workspace.error || event.error} retry={() => void refresh()} /> : <Loading />
  return <ClarificationContext.Provider value={{ event: record, workspace: value, key, privateScope, deny, observe, refresh, writable: !workspace.error && !event.error && !value.archived }}><section className="fleet-clarification-workspace">
    <ContextLink back target={{ label: 'Back to event', href: `/events/${eventID}` }} /><CollectionHeading title={questionID ? 'Private team question' : 'Questions and official clarifications'} scope={{ entity: record.name, event: record.configuration.timezone }} description="Private source questions and reviewed common answers have separate audiences and actions." />
    {(workspace.error || event.error) && <><Failure error={workspace.error || event.error} retry={() => void refresh()} /><p role="status">Current access could not be refreshed. Private unsent text remains only in this page; actions are paused.</p></>}{value.archived && <p role="status">This event is read-only. Existing authorised questions and official answers remain available.</p>}
    <div key={privateEpoch}>{questionID ? value.team_id || value.can_review ? <PrivateQuestionReader key={questionID} id={questionID} /> : <p role="alert">Current private question authority is unavailable. No former private source is restored.</p> : <><nav className="fleet-pattern-actions py-4 border-b border-border" aria-label="Clarification sections"><Button icon="book" variant="ghost" disabled={privatePending} aria-pressed={tab === 'common'} onClick={() => selectTab('common')}>Official answers</Button>{(value.team_id || value.can_review) && <Button icon="help" variant="ghost" disabled={privatePending} aria-pressed={tab === 'private'} onClick={() => selectTab('private')}>{value.can_review ? 'Staff question queue' : "Your team's private questions"}</Button>}</nav>
      {tab === 'common' || (!value.team_id && !value.can_review) ? <CommonAnswers /> : <section className="clarification-private"><p>Original questions and attachments stay private to the current team and explicit organisers. Only reviewed answers enter the common feed.</p>{value.team_id && (value.can_ask || value.archived) && <AskQuestion key={value.team_id} teamID={value.team_id} onLeaveRisk={setPrivateLeaveRisk} onPending={setPrivatePending} />}<PrivateQuestionList /></section>}
    </>}</div>
  </section></ClarificationContext.Provider>
}
function CommonAnswers() {
  const { event, key, workspace } = useClarifications(), [input, setInput] = useState(''), [search, setSearch] = useState('')
  const query = useInfiniteQuery({ queryKey: [...key, 'common', search], gcTime: 0, retry: false, initialPageParam: undefined as string | undefined, queryFn: async ({ pageParam, signal }) => unwrap(await api.GET('/api/v1/events/{event_id}/clarifications', { signal, cache: 'no-store', params: { path: { event_id: event.id }, query: { search: search || undefined, cursor: pageParam, limit: 30 } } })), getNextPageParam: page => page.next_cursor ?? undefined, refetchOnWindowFocus: 'always', refetchInterval: 10_000 })
  useAccessFailure(query.error)
  if (denied(query.error)) return null
  const items = query.data?.pages.flatMap(page => page.items) ?? []
  return <section className="py-6"><h2>Official common answers</h2><p>Every current confirmed attendee receives the same published answers. Private questions are not searched here.</p><p><Link to="/events/$eventId/resources/$resourceId" params={{ eventId: event.id, resourceId: workspace.source.challenge_id }}>Read the current common challenge</Link> · Challenge version {workspace.source.challenge_version} · Published rubric version {workspace.source.rubric.version}</p>{!workspace.can_review && <PublishedRubric />}
    <CollectionToolbar source="cursor" summary={`${items.length} loaded published answers`} search={{ kind: 'server', label: 'Search published answers', maxLength: 200, value: input, onChange: setInput, onSearch: () => setSearch(input.trim()), onClear: () => { setInput(''); setSearch('') } }} />
    {query.isPending ? <Loading /> : query.error ? <Failure error={query.error} retry={() => void query.refetch()} /> : <><CollectionRecords label="Official common answers" rows={items} rowKey={a => a.id} rowLabel={a => a.document.title} destination={a => ({ label: a.document.title, href: `/events/${event.id}/clarifications/${a.id}` })} columns={[{ id: 'version', label: 'Published source', cell: a => `Answer ${a.version} · Challenge ${a.source.challenge_version} · Rubric ${a.source.rubric.version}` }, { id: 'author', label: 'Published by', cell: a => `${a.author_name} · ${localDate(a.published_at, event.configuration.timezone)}` }, { id: 'current', label: 'Applicability', cell: a => a.source_current ? 'Current released source' : 'Earlier released challenge or rubric' }]} />{!items.length && <p>No published answers match this search.</p>}<CollectionPagination source={{ kind: 'cursor', loaded: items.length, busy: query.isFetchingNextPage, next: query.hasNextPage ? () => void query.fetchNextPage() : undefined }} /></>}
  </section>
}
function PrivateQuestionList() {
  const { key, event, workspace } = useClarifications()
  const query = useInfiniteQuery({ queryKey: [...key, 'questions'], gcTime: 0, retry: false, initialPageParam: undefined as string | undefined, queryFn: async ({ pageParam, signal }) => unwrap(await api.GET('/api/v1/events/{event_id}/questions', { signal, cache: 'no-store', params: { path: { event_id: event.id }, query: { cursor: pageParam, limit: 30 } } })), getNextPageParam: page => page.next_cursor ?? undefined, refetchOnWindowFocus: 'always', refetchInterval: 10_000 })
  useAccessFailure(query.error)
  if (denied(query.error)) return null
  const items = query.data?.pages.flatMap(page => page.items) ?? []
  return <section className="py-6">{query.isPending ? <Loading /> : query.error ? <Failure error={query.error} retry={() => void query.refetch()} /> : <><ActionQueue title={workspace.can_review ? 'Authorised private question queue' : "Your team's questions"} emptyMessage="No private questions are available to your current role and team." items={items.map(q => ({ id: q.id, icon: 'help', title: q.topic === 'rubric' ? 'Rubric question' : 'Challenge question', description: `Asked ${localDate(q.created_at, event.configuration.timezone)} · Question revision ${q.revision}`, scope: { entity: `Team ${q.team_id.slice(0, 8)}` }, status: { label: q.status }, action: { label: `${q.topic === 'rubric' ? 'Rubric question' : 'Challenge question'} · ${localDate(q.created_at, event.configuration.timezone)}`, href: `/events/${event.id}/questions/${q.id}`, icon: 'next' } }))} /><CollectionPagination source={{ kind: 'cursor', loaded: items.length, busy: query.isFetchingNextPage, next: query.hasNextPage ? () => void query.fetchNextPage() : undefined }} /></>}</section>
}
function PublishedRubric() {
  const { event, key } = useClarifications(), [open, setOpen] = useState(false)
  const query = useQuery({ queryKey: [...key, 'published-rubric'], enabled: open, gcTime: 0, retry: false, refetchOnWindowFocus: 'always', refetchInterval: 10_000,
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/events/{event_id}/rubric', { signal, cache: 'no-store', params: { path: { event_id: event.id } } })) })
  useAccessFailure(query.error)
  if (denied(query.error)) return null
  const page = query.data
  return <details id="published-rubric" className="py-6 border-t border-border" onToggle={e => setOpen(e.currentTarget.open)}><summary>Read the current published rubric</summary>{open && (query.isPending ? <Loading /> : query.error ? <Failure error={query.error} retry={() => void query.refetch()} /> : page && <section><h3>{page.definition.name} · version {page.reference.version}</h3><p>Published {localDate(page.published_at, event.configuration.timezone)} · {page.definition.mode === 'all_teams' ? 'All teams assessment' : 'Mentor qualification'}</p>{page.notice && <p className="py-3 text-muted-foreground">{page.notice}</p>}<p>Required judges: {page.definition.required_judge_count}{page.definition.mode === 'mentor_qualification' && ` · Finalist places ${page.definition.finalist_count} · Reserves ${page.definition.reserve_count}`}</p>
    {page.definition.categories.map(category => <section key={category.id}><h4>{category.label}</h4><p>Range {category.min_score}–{category.max_score} · Weight {category.weight}</p><p>{category.guidance}</p><ul>{category.anchors.map(anchor => <li key={anchor.score}>{anchor.score}: {anchor.description}</li>)}</ul></section>)}
    {page.definition.prizes.length > 0 && <section><h4>Published prizes</h4><ul>{page.definition.prizes.map(prize => <li key={prize.id}>{prize.name} · {prize.description}</li>)}</ul></section>}{page.definition.side_quests.length > 0 && <section><h4>Side quests</h4><p>Evidence guidance only; this does not change assessment scores.</p><ul>{page.definition.side_quests.map(quest => <li key={quest.id}>{quest.title} · {quest.evidence_guidance}</li>)}</ul></section>}</section>)}</details>
}

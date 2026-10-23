import { useEffect, useMemo } from 'react'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from '@tanstack/react-router'
import { api, privateKey, unwrap } from './api/client'
import { useIdentity } from './session'
import { Failure, Loading } from './components/states'
import { SafeMarkdown } from './components/safe-markdown'
import { Button } from './components/ui/button'

// This canonical read-only destination belongs to the native publication contract.
// The complete private-question and staff review workspace is owned by #162.
export function ClarificationReader() {
  const { eventId, answerId } = useParams({ strict: false }) as { eventId: string; answerId: string }
  const identity = useIdentity(), cache = useQueryClient()
  const key = useMemo(() => [...privateKey(identity.person.id, eventId, 'official-clarification'), answerId], [identity.person.id, eventId, answerId])
  const answer = useQuery({ queryKey: key, gcTime: 0, staleTime: 0, retry: false, refetchOnWindowFocus: 'always', refetchInterval: 15_000,
    queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/clarifications/{answer_id}', { params: { path: { event_id: eventId, answer_id: answerId } } })) })
  const older = useInfiniteQuery({ queryKey: [...key, 'history', answer.data?.answer.version ?? 0], enabled: false, gcTime: 0, retry: false,
    initialPageParam: answer.data?.history.at(-1)?.version ?? 0,
    queryFn: async ({ pageParam }) => unwrap(await api.GET('/api/v1/events/{event_id}/clarifications/{answer_id}/history', { params: { path: { event_id: eventId, answer_id: answerId }, query: { cursor: pageParam || undefined, limit: 20 } } })),
    getNextPageParam: page => page.next_cursor ?? undefined })
  useEffect(() => () => { cache.removeQueries({ queryKey: key }) }, [cache, key])
  useEffect(() => { if (answer.isError || older.isError) cache.removeQueries({ queryKey: key }) }, [answer.isError, older.isError, cache, key])
  if (answer.isPending || !answer.isFetchedAfterMount) return <Loading />
  if (answer.error || older.error) return <Failure error={answer.error || older.error} retry={() => { void answer.refetch(); if (older.isError) void older.refetch() }} />
  const page = answer.data!, current = page.answer
  const history = [...page.history, ...(older.data?.pages.flatMap(page => page.items) ?? [])]
  return <section className="workspace"><Link to="/inbox">← Your inbox</Link><p><Link to="/events/$eventId/clarifications" params={{ eventId }}>Search common official answers</Link></p><article className="panel">
    <p className="eyebrow">Reviewed common clarification · version {current.version}</p><h1>{current.document.title}</h1>
    <p>Published {new Date(current.published_at).toLocaleString()} · Author {current.author_name}</p>
    <p>Challenge version {current.source.challenge_version} · Rubric version {current.source.rubric.version}</p>
    <p><Link to="/events/$eventId/resources/$resourceId" params={{ eventId, resourceId: current.source.challenge_id }}>Read the current owning challenge</Link> · <Link to="/events/$eventId/clarifications" params={{ eventId }} hash="published-rubric">Current published rubric</Link></p>
    {!current.source_current && <p role="status">This answer applies to an earlier published challenge or rubric. Check the current common resources.</p>}
    {current.document.kind !== 'clarification' && <p>This answer announces a change already published through the challenge or rubric review process.</p>}
    {page.archived && <p>This event is archived. Published answers remain read-only.</p>}
    <SafeMarkdown>{current.document.body}</SafeMarkdown>
    {history.length > 1 && <details><summary>Published correction history</summary>{history.filter(item => !item.current).map(item => <article key={item.version}>
      <h2>Superseded answer · version {item.version}</h2><p>{new Date(item.published_at).toLocaleString()} · Challenge version {item.source.challenge_version} · Rubric version {item.source.rubric.version}</p><SafeMarkdown>{item.document.body}</SafeMarkdown>
    </article>)}{page.history_has_more && (!older.data || older.hasNextPage) && <Button variant="outline" disabled={older.isFetching} onClick={() => void older.fetchNextPage()}>More published revisions</Button>}</details>}
  </article></section>
}

import { useState } from 'react'
import { useInfiniteQuery, useQueryClient } from '@tanstack/react-query'
import { api, privateKey, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { ResourceReader } from './resource-reader'
export { ResourceReader } from './resource-reader'
import { EventGuidance } from './event-guidance'
import { localDate } from './event-time'

export function ResourceLibrary({ event }: { event: EventRecord }) {
  const identity = useIdentity(), cache = useQueryClient()
  const [search, setSearch] = useState(''), [query, setQuery] = useState(''), [selected, setSelected] = useState<string>()
  const key = privateKey(identity.person.id, event.id, 'resource-library')
  const pages = useInfiniteQuery({ queryKey: [...key, query], initialPageParam: undefined as string | undefined, queryFn: async ({ pageParam }) => unwrap(await api.GET('/api/v1/events/{event_id}/content', { params: { path: { event_id: event.id }, query: { q: query || undefined, limit: 30, cursor: pageParam } } })), getNextPageParam: p => p.next_cursor ?? undefined, refetchOnWindowFocus: 'always', refetchInterval: 15_000 })
  const items = pages.data?.pages.flatMap(p => p.items) ?? []
  function refresh() { void cache.invalidateQueries({ queryKey: key }); void cache.invalidateQueries({ queryKey: privateKey(identity.person.id, event.id, 'resource-reader') }) }
  return <section className="resource-library"><div className="preparation-heading"><div><p className="eyebrow">Your event library</p><h2>Resources</h2><p>Released pages and attachments available to your current role and team.</p></div><Button variant="outline" onClick={refresh}>Refresh resources</Button></div>
    {selected ? <ResourceReader key={selected} id={selected} event={event} back={() => setSelected(undefined)} /> : <>
      <EventGuidance event={event} open={setSelected} />
      <form className="resource-search" role="search" onSubmit={e => { e.preventDefault(); setQuery(search.trim()) }}><label htmlFor="resource-search">Search resources</label><div className="actions"><input id="resource-search" type="search" maxLength={200} value={search} onChange={e => setSearch(e.target.value)} placeholder="Search page titles and text" /><Button type="submit">Search</Button>{query && <Button type="button" variant="outline" onClick={() => { setQuery(''); setSearch('') }}>Clear search</Button>}</div></form>
      {pages.isPending ? <Loading /> : pages.error ? <Failure error={pages.error} retry={() => void pages.refetch()} /> : <><nav aria-label="Released resources" className="event-grid resource-directory">{items.map(p => <article key={p.id} className="panel"><span className="badge">{p.kind === 'challenge' ? 'Challenge' : p.kind === 'brief' ? 'Role brief' : 'Resource'}</span><h3>{p.document.title}</h3><p>Released version {p.version} · {localDate(p.released_at, event.configuration.timezone)}</p>{p.document.require_acknowledgement && <p className={p.acknowledged_at ? 'muted' : 'notice'}>{p.acknowledged_at ? `Version ${p.version} acknowledged` : 'Acknowledgement required'}</p>}<Button variant="outline" onClick={() => setSelected(p.id)}>Read {p.document.title}</Button></article>)}</nav>{!items.length && <p className="empty-state">{query ? 'No released resources match your search.' : 'No released resources are available to you yet.'}</p>}{pages.hasNextPage && <Button disabled={pages.isFetchingNextPage} onClick={() => void pages.fetchNextPage()}>More resources</Button>}</>}
    </>}
  </section>
}

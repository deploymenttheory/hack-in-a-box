import { useEffect, useRef, useState } from 'react'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, APIError, privateKey, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { SafeMarkdown } from './components/safe-markdown'
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
      <form className="resource-search" role="search" onSubmit={e => { e.preventDefault(); setQuery(search.trim()) }}><label htmlFor="resource-search">Search resources</label><div className="actions"><input id="resource-search" type="search" maxLength={200} value={search} onChange={e => setSearch(e.target.value)} placeholder="Search page titles and text" /><Button type="submit">Search</Button>{query && <Button type="button" variant="outline" onClick={() => { setQuery(''); setSearch('') }}>Clear search</Button>}</div></form>
      {pages.isPending ? <Loading /> : pages.error ? <Failure error={pages.error} retry={() => void pages.refetch()} /> : <><nav aria-label="Released resources" className="event-grid resource-directory">{items.map(p => <article key={p.id} className="panel"><span className="badge">{p.kind === 'challenge' ? 'Challenge' : p.kind === 'brief' ? 'Role brief' : 'Resource'}</span><h3>{p.document.title}</h3><p>Released version {p.version} · {localDate(p.released_at, event.configuration.timezone)}</p>{p.document.require_acknowledgement && <p className={p.acknowledged_at ? 'muted' : 'notice'}>{p.acknowledged_at ? `Version ${p.version} acknowledged` : 'Acknowledgement required'}</p>}<Button variant="outline" onClick={() => setSelected(p.id)}>Read {p.document.title}</Button></article>)}</nav>{!items.length && <p className="empty-state">{query ? 'No released resources match your search.' : 'No released resources are available to you yet.'}</p>}{pages.hasNextPage && <Button disabled={pages.isFetchingNextPage} onClick={() => void pages.fetchNextPage()}>More resources</Button>}</>}
    </>}
  </section>
}

function ResourceReader({ id, event, back }: { id: string; event: EventRecord; back: () => void }) {
  const identity = useIdentity(), cache = useQueryClient(), heading = useRef<HTMLHeadingElement>(null)
  const [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>(), [reloadRequired, setReloadRequired] = useState(false)
  const key = [...privateKey(identity.person.id, event.id, 'resource-reader'), id]
  const resource = useQuery({ queryKey: key, queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/content/{content_id}', { params: { path: { event_id: event.id, content_id: id } } })), refetchOnWindowFocus: 'always', refetchInterval: 15_000 })
  useEffect(() => { heading.current?.focus() }, [resource.data?.id])
  async function reload() { const result = await resource.refetch(); if (!result.error) { setFailure(undefined); setReloadRequired(false) } }
  async function acknowledge() {
    if (!resource.data) return
    setPending(true); setFailure(undefined)
    try {
      const result = unwrap(await api.POST('/api/v1/events/{event_id}/content/{content_id}/acknowledgements', { params: { path: { event_id: event.id, content_id: id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${resource.data.version}"` } } }))
      cache.setQueryData(key, result)
      await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, event.id, 'resource-library') })
    } catch (error) { setFailure(error); if (error instanceof APIError && [403, 404, 409].includes(error.status)) setReloadRequired(true) } finally { setPending(false) }
  }
  const page = resource.data
  return <section className="resource-reader"><Button variant="outline" onClick={back}>Back to resources</Button>
    {resource.isPending ? <Loading /> : resource.error ? <Failure error={resource.error} retry={() => void reload()} /> : reloadRequired ? <div className="panel"><Failure error={failure} /><p>This resource or your access changed. Load the current released version before continuing.</p><Button onClick={() => void reload()}>Reload latest resource</Button></div> : page && <article className="panel"><p className="eyebrow">Released version {page.version}</p><h2 ref={heading} tabIndex={-1}>{page.document.title}</h2><p className="muted">Released {localDate(page.released_at, event.configuration.timezone)} · {event.configuration.timezone}</p>
      {page.kind === 'challenge' && <p className="notice">The common challenge is released. Your team may start building now. Formal submissions and mentor or judge review open at live start: {localDate(event.configuration.live_start, event.configuration.timezone)}.</p>}
      <SafeMarkdown>{page.document.markdown}</SafeMarkdown>
      {page.attachment_details.length > 0 && <section aria-label="Resource attachments"><h3>Attachments</h3><ul className="item-list">{page.attachment_details.map(a => <li key={a.id}><div>{a.can_download ? <a className="content-link" href={`/api/v1/events/${event.id}/files/${a.id}`}>{a.filename}</a> : <strong>{a.filename} · Unavailable</strong>}<p>{a.media_type} · {Math.ceil(a.size / 1024)} KiB</p></div></li>)}</ul><p className="muted">Downloads check your current access. Previously downloaded copies cannot be recalled.</p></section>}
      {page.document.require_acknowledgement && <section className="resource-acknowledgement" aria-label="Required acknowledgement"><h3>Required reading</h3>{page.acknowledged_at ? <p role="status">You acknowledged version {page.version} on {localDate(page.acknowledged_at, event.configuration.timezone)}.</p> : <><p>Confirm that you have read this version and its attachments. A material update can require a new acknowledgement.</p><Button disabled={pending || ['archived', 'cancelled'].includes(event.stage)} onClick={() => void acknowledge()}>I have read version {page.version}</Button>{['archived', 'cancelled'].includes(event.stage) && <p>This event no longer accepts acknowledgements.</p>}</>}{!!failure && <Failure error={failure} />}</section>}
    </article>}
  </section>
}

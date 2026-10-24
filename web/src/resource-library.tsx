import { useRef, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useInfiniteQuery, useQueryClient } from '@tanstack/react-query'
import { api, APIError, privateKey, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { useResourceReturnFocus } from './resource-focus'
import { usePrivateRequestScope } from './private-request-scope'
import { Button } from './components/ui/button'
import { StatusBadge } from './components/ui/feedback'
import { CollectionFrame, CollectionHeading, CollectionRecords, CollectionPagination, CollectionToolbar, type CollectionState } from './components/patterns'
import { ResourceReader } from './resource-reader'
export { ResourceReader } from './resource-reader'
import { EventGuidance } from './event-guidance'
import { localDate } from './event-time'

export function ResourceLibrary({ event }: { event: EventRecord }) {
  const identity = useIdentity()
  return <LibraryLifetime key={`${identity.person.id}:${identity.draft_scope}:${event.id}`} event={event} />
}
function LibraryLifetime({ event }: { event: EventRecord }) {
  const identity = useIdentity(), cache = useQueryClient(), scope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${event.id}:library`)
  const [search, setSearch] = useState(''), [query, setQuery] = useState(''), [selected, setSelected] = useState<string>()
  const root = useRef<HTMLElement>(null), focus = useResourceReturnFocus(root), key = privateKey(identity.person.id, event.id, 'resource-library')
  const pages = useInfiniteQuery({ queryKey: [...key, query], initialPageParam: undefined as string | undefined, retry: false, queryFn: async ({ pageParam, signal }) => { const ticket = scope.capture(); const result = unwrap(await api.GET('/api/v1/events/{event_id}/content', { signal, params: { path: { event_id: event.id }, query: { q: query || undefined, limit: 30, cursor: pageParam } } })); ticket.assert(); return result }, getNextPageParam: p => p.next_cursor ?? undefined, refetchOnWindowFocus: 'always', refetchInterval: 15_000 })
  const items = pages.data?.pages.flatMap(p => p.items) ?? [], denied = pages.error instanceof APIError && [401, 403, 404].includes(pages.error.status)
  const state: CollectionState = denied ? { kind: 'denied', message: 'You do not have access to this resource library.' } : pages.isPending ? { kind: 'loading' } : pages.error ? { kind: pages.data ? 'paused' : 'failed', message: 'Current resource access could not be checked. Read the server again before acting.', read: { label: 'Check resource access', onSelect: () => void pages.refetch() } } : !items.length ? query ? { kind: 'no-match', message: 'No released resources match your search.', clear: () => { setSearch(''); setQuery('') } } : { kind: 'empty', message: 'No released resources are available to you yet.' } : { kind: 'ready' }
  function open(id: string, opener?: HTMLElement) { focus.capture(opener); setSelected(id) }
  function back() { setSelected(undefined); focus.restore() }
  function refresh() { void cache.invalidateQueries({ queryKey: key }); void cache.invalidateQueries({ queryKey: privateKey(identity.person.id, event.id, 'resource-reader') }) }
  return <section ref={root} className="fleet-resource-workspace"><CollectionHeading title="Resources" scope={{ event: event.name, entity: 'Released library' }} description="Released pages and attachments available to your current role and team." secondary={[{ label: 'Refresh resources', icon: 'refresh', onSelect: refresh }]} /><p><Link to="/events/$eventId/clarifications" params={{ eventId: event.id }}>Questions and official clarifications</Link></p>
    {selected && !denied ? <ResourceReader key={selected} id={selected} event={event} back={back} /> : <>{!denied && <EventGuidance event={event} open={open} />}<CollectionFrame scopeKey={scope.key} state={state} heading={<CollectionHeading title="Released resources" scope={{ entity: 'Current audience', event: event.name }} />} toolbar={<CollectionToolbar source="cursor" search={{ kind: 'server', label: 'Search resources', maxLength: 200, value: search, onChange: setSearch, onSearch: () => setQuery(search.trim()), onClear: () => { setSearch(''); setQuery('') } }} />}><CollectionRecords label="Released resources" rows={items} rowKey={page => page.id} rowLabel={page => page.document.title} destination={page => ({ label: page.document.title, href: `/events/${event.id}/resources/${page.id}`, onNavigate: () => open(page.id) })} columns={[{ id: 'kind', label: 'Type', cell: page => page.kind === 'challenge' ? 'Challenge' : page.kind === 'brief' ? 'Role brief' : 'Resource' }, { id: 'version', label: 'Released version', cell: page => page.version }, { id: 'released', label: 'Released', cell: page => localDate(page.released_at, event.configuration.timezone) }, { id: 'reading', label: 'Required reading', cell: page => <StatusBadge tone={page.acknowledged_at ? 'success' : page.document.require_acknowledgement ? 'warning' : 'neutral'}>{page.acknowledged_at ? `Version ${page.version} acknowledged` : page.document.require_acknowledgement ? 'Acknowledgement required' : 'Optional'}</StatusBadge> }]} actions={page => <Button data-record-focus={page.id} icon="book" variant="ghost" onClick={event => open(page.id, event.currentTarget)}>Read {page.document.title}</Button>} /><CollectionPagination source={{ kind: 'cursor', loaded: items.length, busy: pages.isFetchingNextPage, next: pages.hasNextPage ? () => void pages.fetchNextPage() : undefined }} label="Resource pages" /></CollectionFrame></>}
  </section>
}

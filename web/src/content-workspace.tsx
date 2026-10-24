import { useCallback, useEffect, useRef, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, privateKey, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Select, Label } from './components/ui/fields'
import { StatusBadge } from './components/ui/feedback'
import { CollectionHeading, CollectionRecords, CollectionPagination, CollectionToolbar } from './components/patterns'
import { Failure, Loading } from './components/states'
import { ContentEditor } from './content-editor'
import { contentTemplates, draftPage, type ContentPage } from './content-model'
import { EventGuidance } from './event-guidance'
import { localDate } from './event-time'
import { usePrivateRequestScope } from './private-request-scope'
import { planningDepartureAllowed } from './planning-operations'
import { useResourceReturnFocus } from './resource-focus'
import { resourceDenied } from './resource-operation'

export function ContentWorkspace(props: { event: EventRecord; onDirty: (value: boolean) => void; openSection: (section: string) => void }) {
  const identity = useIdentity()
  return <AuthoringLifetime key={`${identity.person.id}:${identity.draft_scope}:${props.event.id}`} {...props} />
}
function AuthoringLifetime({ event, onDirty, openSection }: { event: EventRecord; onDirty: (value: boolean) => void; openSection: (section: string) => void }) {
  const cache = useQueryClient(), cleared = useRef(false)
  const identity = useIdentity(), scope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${event.id}:authoring`), root = useRef<HTMLElement>(null), focus = useResourceReturnFocus(root)
  const [selected, setSelected] = useState<ContentPage>(), [template, setTemplate] = useState('faq'), [search, setSearch] = useState(''), [query, setQuery] = useState(''), [dirty, setDirty] = useState(false), [denied, setDenied] = useState<unknown>()
  const markDirty = useCallback((value: boolean) => { setDirty(value); onDirty(value) }, [onDirty])
  const pages = useInfiniteQuery({ queryKey: [...privateKey(identity.person.id, event.id, 'content-authoring'), query], initialPageParam: undefined as string | undefined, enabled: !denied, retry: false, queryFn: async ({ pageParam, signal }) => { const ticket = scope.capture(); const result = unwrap(await api.GET('/api/v1/events/{event_id}/content', { signal, params: { path: { event_id: event.id }, query: { preview: true, q: query || undefined, cursor: pageParam, limit: 50 } } })); ticket.assert(); return result }, getNextPageParam: page => page.next_cursor ?? undefined, refetchOnWindowFocus: 'always', refetchInterval: 15_000 })
  const teams = useQuery({ queryKey: privateKey(identity.person.id, event.id, 'teams'), enabled: !denied, retry: false, queryFn: async ({ signal }) => { const ticket = scope.capture(); const result = unwrap(await api.GET('/api/v1/events/{event_id}/teams', { signal, params: { path: { event_id: event.id } } })); ticket.assert(); return result } })
  const definitions = useQuery({ queryKey: privateKey(identity.person.id, event.id, 'content-templates'), enabled: !denied, retry: false, queryFn: async ({ signal }) => { const ticket = scope.capture(); const value = unwrap(await api.GET('/api/v1/events/{event_id}/configuration-snapshot', { signal, params: { path: { event_id: event.id } } })); ticket.assert(); return value } })
  const readiness = useQuery({ queryKey: privateKey(identity.person.id, event.id, 'content-readiness'), enabled: !denied, retry: false, queryFn: async ({ signal }) => { const ticket = scope.capture(); const value = unwrap(await api.GET('/api/v1/events/{event_id}/content-readiness', { signal, params: { path: { event_id: event.id } } })); ticket.assert(); return value }, refetchInterval: 15_000, refetchOnWindowFocus: 'always' })
  const items = pages.data?.pages.flatMap(page => page.items) ?? [], templates = contentTemplates(definitions.data?.definitions.role_briefs ?? [], items), chosen = templates.find(item => item.id === template)
  function choose(page?: ContentPage) { if (planningDepartureAllowed() && (!dirty || window.confirm('Leave this page with unsaved content?'))) { if (page) focus.capture(); setSelected(page); if (!page) focus.restore() } }
  function leave(section: string) { if (planningDepartureAllowed() && (!dirty || window.confirm('Leave this page with unsaved content?'))) openSection(section) }
  const revoked = !!denied || resourceDenied(pages.error) || resourceDenied(teams.error) || resourceDenied(definitions.error)
  useEffect(() => {
    if (!revoked || cleared.current) return
    cleared.current = true
    setDenied(denied ?? pages.error ?? teams.error ?? definitions.error)
    scope.invalidate()
    setSelected(undefined); markDirty(false)
    for (const name of ['content-authoring', 'content-templates', 'content-readiness', ...(selected ? [`content-version-${selected.id}`, `content-history-${selected.id}`] : [])]) {
      const key = privateKey(identity.person.id, event.id, name)
      void cache.cancelQueries({ queryKey: key }); cache.removeQueries({ queryKey: key })
    }
  }, [revoked, denied, pages.error, teams.error, definitions.error, selected, markDirty, scope, cache, identity.person.id, event.id])
  if (revoked) return <Failure error={denied ?? pages.error ?? teams.error} />
  return <section ref={root} className="fleet-resource-workspace"><CollectionHeading title="Content" scope={{ event: event.name, entity: 'Briefs and resources' }} description="Prepare a private document, review its exact version and audience, then deliberately approve or release it." />{event.can_edit && <p><Link to="/events/$eventId/clarifications" params={{ eventId: event.id }}>Review private questions and common official answers</Link></p>}
    {!!pages.error && <Failure error={pages.error} retry={() => void pages.refetch()} />}
    {selected ? teams.data ? <ContentEditor key={selected.id} initial={selected} event={{ ...event, can_edit: event.can_edit && !pages.error && !teams.error }} teams={teams.data.teams} readiness={readiness.error ? undefined : readiness.data} onDirty={markDirty} onDeny={setDenied} close={() => choose()} openSection={leave} /> : teams.isPending ? <Loading /> : <Failure error={teams.error} retry={() => void teams.refetch()} /> : <>
      <section className="fleet-resource-section"><h2>Start a page</h2><Label htmlFor="content-template">Starting template</Label><Select id="content-template" value={template} onChange={event => setTemplate(event.target.value)}>{templates.map(item => <option key={item.id} value={item.id} disabled={item.id === 'challenge' && items.some(page => page.kind === 'challenge')}>{item.name}{item.id === 'challenge' && items.some(page => page.kind === 'challenge') ? ' (already exists)' : ''}</option>)}</Select><p>Event-template role briefs are reusable. Copies start as private drafts and require their own attachments and approval.</p><Button data-resource-opener="start-template" icon="add" disabled={!event.can_edit || !!pages.error || !chosen || (chosen.id === 'challenge' && items.some(page => page.kind === 'challenge'))} onClick={() => chosen && choose(draftPage(chosen.input))}>Start from template</Button>{definitions.isPending && <p role="status">Loading event templates…</p>}{!!definitions.error && <Failure error={definitions.error} retry={() => void definitions.refetch()} />}</section>
      <CollectionToolbar source="cursor" search={{ kind: 'server', label: 'Find authored pages', maxLength: 200, value: search, onChange: setSearch, onSearch: () => setQuery(search.trim()), onClear: () => { setSearch(''); setQuery('') } }} />{pages.isPending ? <Loading /> : pages.data && <><CollectionRecords label="Authored pages" rows={items} rowKey={page => page.id} rowLabel={page => page.document.title} columns={[{ id: 'state', label: 'Review state', cell: page => <StatusBadge tone={page.state === 'released' ? 'success' : 'neutral'}>{page.state}</StatusBadge> }, { id: 'version', label: 'Current version', cell: page => page.version }, { id: 'released', label: 'Released version', cell: page => page.released_version ?? 'Not released' }, { id: 'schedule', label: 'Publication', cell: page => page.document.scheduled_at ? localDate(page.document.scheduled_at, event.configuration.timezone) : 'Manual release after approval' }]} actions={page => <Button data-record-focus={page.id} icon="next" variant="ghost" disabled={!!pages.error} onClick={() => choose(page)}>{event.can_edit ? 'Edit' : 'View'} {page.document.title}</Button>} />{!items.length && <p>{query ? 'No authorised authored pages match this search.' : 'No pages yet. Start with a challenge, role brief, FAQ or resource page.'}</p>}<CollectionPagination label="Authored content pages" source={{ kind: 'cursor', loaded: items.length, busy: pages.isFetchingNextPage, next: pages.hasNextPage ? () => void pages.fetchNextPage() : undefined }} /></>}
      <EventGuidance event={event} readiness />
    </>}
  </section>
}

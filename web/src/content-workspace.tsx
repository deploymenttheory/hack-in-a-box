import { useCallback, useState } from 'react'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { api, privateKey, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { ContentEditor } from './content-editor'
import { contentTemplates, draftPage, type ContentPage } from './content-model'
import { EventGuidance } from './event-guidance'
import { localDate } from './event-time'

export function ContentWorkspace({ event, onDirty, openSection }: { event: EventRecord; onDirty: (value: boolean) => void; openSection: (section: string) => void }) {
  const identity = useIdentity(), [selected, setSelected] = useState<ContentPage>(), [template, setTemplate] = useState('faq'), [search, setSearch] = useState(''), [dirty, setDirty] = useState(false)
  const markDirty = useCallback((value: boolean) => { setDirty(value); onDirty(value) }, [onDirty])
  const pages = useInfiniteQuery({ queryKey: privateKey(identity.person.id, event.id, 'content-authoring'), initialPageParam: undefined as string | undefined, queryFn: async ({ pageParam }) => unwrap(await api.GET('/api/v1/events/{event_id}/content', { params: { path: { event_id: event.id }, query: { preview: true, cursor: pageParam, limit: 50 } } })), getNextPageParam: p => p.next_cursor ?? undefined, refetchOnWindowFocus: 'always' })
  const teams = useQuery({ queryKey: privateKey(identity.person.id, event.id, 'teams'), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/teams', { params: { path: { event_id: event.id } } })) })
  const definitions = useQuery({ queryKey: privateKey(identity.person.id, event.id, 'content-templates'), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/configuration-snapshot', { params: { path: { event_id: event.id } } })) })
  const readiness = useQuery({ queryKey: privateKey(identity.person.id, event.id, 'content-readiness'), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/content-readiness', { params: { path: { event_id: event.id } } })), refetchInterval: 15_000, refetchOnWindowFocus: 'always' })
  const items = pages.data?.pages.flatMap(p => p.items) ?? []
  const templates = contentTemplates(definitions.data?.definitions.role_briefs ?? [], items), chosen = templates.find(t => t.id === template)
  function choose(page?: ContentPage) { if (!dirty || window.confirm('Leave this page with unsaved content?')) setSelected(page) }
  function leave(section: string) { if (!dirty || window.confirm('Leave this page with unsaved content?')) openSection(section) }
  return <section className="content-workspace"><div className="preparation-heading"><div><p className="eyebrow">Briefs and resources</p><h2>Content</h2><p>Write once, review the version, then release it to the right people.</p></div></div>
    {selected ? <>{teams.error ? <Failure error={teams.error} retry={() => void teams.refetch()} /> : teams.isPending ? <Loading /> : <ContentEditor key={selected.id} initial={selected} event={event} teams={teams.data.teams} readiness={readiness.data} onDirty={markDirty} close={() => choose()} openSection={leave} />}{readiness.error && <Failure error={readiness.error} retry={() => void readiness.refetch()} />}</> : <>
      <EventGuidance event={event} readiness />
      {event.can_edit && <section className="panel content-start"><h3>Start a page</h3><label htmlFor="content-template">Starting template</label><select id="content-template" value={template} onChange={e => setTemplate(e.target.value)}>{templates.map(t => <option key={t.id} value={t.id} disabled={t.id === 'challenge' && items.some(p => p.kind === 'challenge')}>{t.name}{t.id === 'challenge' && items.some(p => p.kind === 'challenge') ? ' (already exists)' : ''}</option>)}</select><p>Event-template role briefs are reusable. Copies start as private drafts and require their own attachments and approval.</p><Button disabled={!chosen || (chosen.id === 'challenge' && items.some(p => p.kind === 'challenge'))} onClick={() => chosen && choose(draftPage(chosen.input))}>Start from template</Button>{definitions.isPending && <p role="status">Loading event templates…</p>}{definitions.error && <Failure error={definitions.error} retry={() => void definitions.refetch()} />}</section>}
      <label htmlFor="content-search">Find authored pages</label><input id="content-search" type="search" value={search} onChange={e => setSearch(e.target.value)} />{pages.isPending ? <Loading /> : pages.error ? <Failure error={pages.error} retry={() => void pages.refetch()} /> : <><div className="event-grid content-directory">{items.filter(p => p.document.title.toLowerCase().includes(search.toLowerCase())).map(p => <article className="panel" key={p.id}><span className="badge">{p.state}</span><h3>{p.document.title}</h3><p>Version {p.version}{p.released_version ? ` · Released version ${p.released_version}` : ' · Not released'}</p><p>{p.document.scheduled_at ? localDate(p.document.scheduled_at, event.configuration.timezone) : 'Manual release after approval'}</p><Button variant="outline" onClick={() => choose(p)}>{event.can_edit ? 'Edit' : 'View'} {p.document.title}</Button></article>)}</div>{!items.length && <p className="empty-state">No pages yet. Start with a challenge, role brief, FAQ or resource page.</p>}{pages.hasNextPage && <Button disabled={pages.isFetchingNextPage} onClick={() => void pages.fetchNextPage()}>Load more authored pages</Button>}</>}
    </>}
  </section>
}

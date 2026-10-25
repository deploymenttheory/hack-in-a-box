import { useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useParams, useSearch } from '@tanstack/react-router'
import { useInfiniteQuery, useQueryClient } from '@tanstack/react-query'
import { api, privateKey, unwrap } from './api/client'
import { useIdentity } from './session'
import { usePrivateRequestScope } from './private-request-scope'
import { useOutcomeRead } from './outcome-read'
import { outcomeDenied } from './outcome-command'
import { CollectionHeading, CollectionPagination, CollectionToolbar } from './components/patterns/collection'
import { Button, buttonVariants } from './components/ui/button'
import { Field, Select } from './components/ui/fields'
import { Icon } from './components/ui/icon'
import { Failure, Loading } from './components/states'
import { followUpLabels, ShowcaseReading, ShowcaseRecords, type ReviewedProject } from './showcase-view'

export type ShowcaseSearch = { search?: string; event?: string; status?: keyof typeof followUpLabels }
const uuid = /^[a-f\d]{8}-[a-f\d]{4}-[a-f\d]{4}-[a-f\d]{4}-[a-f\d]{12}$/i
export function showcaseSearch(value: Record<string, unknown>): ShowcaseSearch {
 return { search: typeof value.search === 'string' ? value.search.slice(0, 200) : undefined, event: typeof value.event === 'string' && uuid.test(value.event) ? value.event : undefined, status: typeof value.status === 'string' && Object.hasOwn(followUpLabels, value.status) ? value.status as keyof typeof followUpLabels : undefined }
}

export function PublishedProjects() {
 const identity = useIdentity()
 return <ProjectDirectory key={`${identity.person.id}:${identity.draft_scope}`} />
}
function ProjectDirectory() {
 const identity = useIdentity(), cache = useQueryClient(), navigate = useNavigate(), filters = showcaseSearch(useSearch({ strict: false }))
 const [blocked, setBlocked] = useState<unknown>(), [mount] = useState(() => crypto.randomUUID()), [generation, setGeneration] = useState(0)
 const scope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:showcase`)
 const prefix = useMemo(() => [...privateKey(identity.person.id, 'organisation', 'showcase'), identity.draft_scope, mount], [identity.person.id, identity.draft_scope, mount])
 useEffect(() => () => { void cache.cancelQueries({ queryKey: prefix }); cache.removeQueries({ queryKey: prefix }) }, [cache, prefix])
 const projects = useInfiniteQuery({ queryKey: [...prefix, generation, filters], gcTime: 0, enabled: !blocked, initialPageParam: undefined as string | undefined,
  queryFn: async ({ pageParam, signal }) => {
   await Promise.resolve(); const ticket = scope.capture(); ticket.assert()
   try {
    const value = unwrap(await api.GET('/api/v1/projects', { signal, cache: 'no-store', params: { query: { search: filters.search, event_id: filters.event, status: filters.status, cursor: pageParam, limit: 25 } } }))
    ticket.assert(); return value
   } catch (error) {
    if (ticket.current() && outcomeDenied(error)) { scope.invalidate(); setBlocked(error); void cache.cancelQueries({ queryKey: prefix }); cache.removeQueries({ queryKey: prefix }); void navigate({ to: '/projects', search: {}, replace: true }) }
    throw error
   }
  }, getNextPageParam: page => page.next_cursor ?? undefined, retry: false, refetchInterval: 15_000, refetchOnWindowFocus: 'always' })
 const rows = projects.data?.pages.flatMap(page => page.items) ?? [], events = projects.data?.pages[0]?.events ?? []
 function change(next: ShowcaseSearch) { scope.renew(); setGeneration(value => value + 1); void navigate({ to: '/projects', search: next }) }
 function retry() { scope.renew(); setBlocked(undefined); setGeneration(value => value + 1) }
 return <section className="workspace fleet-showcase">
  <Link className="fleet-showcase-back" to="/ideas"><Icon name="back" />Your ideas</Link>
  <CollectionHeading title="Reviewed projects" scope={{ entity: 'Organisation showcase · Current approved audience' }} description={<p>Discover useful work from every team. Projects appear only after separate organiser publication; submissions, awards and event completion do not publish them.</p>} secondary={[{ label: 'Read current projects', icon: 'refresh', onSelect: () => { scope.renew(); setGeneration(value => value + 1) }, disabled: projects.isFetching || !!blocked }]} />
  {blocked ? <><Failure error={blocked} /><Button icon="refresh" variant="outline" onClick={retry}>Check current showcase access</Button></> : <>
   <ShowcaseToolbar key={filters.search ?? ''} filters={filters} events={events} change={change} />
   {projects.isPending ? <Loading /> : projects.error ? <Failure error={projects.error} retry={() => void projects.refetch()} /> : <>
    <ShowcaseRecords projects={rows} destination={project => ({ href: `/projects/${project.id}`, label: project.problem, onNavigate: () => void navigate({ to: '/projects/$outcomeId', params: { outcomeId: project.id }, search: filters }) })} action={project => <Link className={buttonVariants({ variant: 'ghost' })} to="/projects/$outcomeId" params={{ outcomeId: project.id }} search={filters} aria-label={`Open ${project.problem}`}><Icon name="next" />Open project</Link>} />
    {!rows.length && <p role="status">No currently permitted projects match these filters. Change the search or event to continue.</p>}
    <CollectionPagination label="Showcase pages" source={{ kind: 'cursor', loaded: rows.length, busy: projects.isFetchingNextPage, next: projects.hasNextPage ? () => void projects.fetchNextPage() : undefined }} />
   </>}
  </>}
 </section>
}
function ShowcaseToolbar({ filters, events, change }: { filters: ShowcaseSearch; events: { id: string; name: string }[]; change: (filters: ShowcaseSearch) => void }) {
 const [draft, setDraft] = useState(filters.search ?? '')
 return (<CollectionToolbar source="cursor" search={{ kind: 'server', label: 'Search reviewed problem and solution', value: draft, maxLength: 200, onChange: setDraft, onSearch: () => change({ ...filters, search: draft || undefined }), onClear: () => { setDraft(''); change({ ...filters, search: undefined }) } }} filters={<div className="fleet-showcase-filters"><Field label="Source event"><Select value={filters.event ?? ''} onChange={event => change({ ...filters, event: event.target.value || undefined })}><option value="">All reviewed events</option>{events.map(event => <option key={event.id} value={event.id}>{event.name}</option>)}{filters.event && !events.some(event => event.id === filters.event) && <option value={filters.event}>Selected event unavailable</option>}</Select></Field><Field label="Reviewed follow-up"><Select value={filters.status ?? ''} onChange={event => change({ ...filters, status: event.target.value as ShowcaseSearch['status'] || undefined })}><option value="">All follow-up statuses</option>{Object.entries(followUpLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</Select></Field></div>} />)
}
export function PublishedProjectPage() {
 const { outcomeId } = useParams({ strict: false }), identity = useIdentity()
 return <ProjectReader key={`${identity.person.id}:${identity.draft_scope}:${outcomeId}`} id={outcomeId!} />
}
function ProjectReader({ id }: { id: string }) {
 const filters = showcaseSearch(useSearch({ strict: false }))
 const { current, blocked, checkAccess } = useOutcomeRead(id, 'showcase-project', async signal => unwrap(await apiRead(id, signal)))
 return <section className="workspace fleet-showcase"><Link className="fleet-showcase-back" to="/projects" search={filters}><Icon name="back" />Reviewed projects</Link>
  {blocked ? <><CollectionHeading title="Project unavailable" scope={{ entity: 'Current reviewed publication' }} /><Failure error={blocked} /><Button icon="refresh" variant="outline" onClick={checkAccess}>Check current publication access</Button></> : current.isPending ? <Loading /> : current.error ? <Failure error={current.error} retry={() => void current.refetch()} /> : <>
   <CollectionHeading title={current.data.problem} scope={{ entity: `Reviewed publication · Version ${current.data.version}` }} description={<p>Published {new Date(current.data.published_at).toLocaleString()}. Only reviewed fields and currently permitted published awards or media are shown.</p>} secondary={[{ label: 'Read current project', icon: 'refresh', onSelect: () => void current.refetch(), disabled: current.isFetching }]} />
   <ShowcaseReading project={current.data} source={current.data.source_readable ? <Link to="/ideas/$outcomeId" params={{ outcomeId: id }} className={buttonVariants({ variant: 'outline' })}><Icon name="book" />Read current outcome record</Link> : <p>Source outcome version {current.data.source_version} was reviewed for this publication. Its restricted record is not shared with this audience.</p>} references={current.data.links.length ? <ul className="fleet-showcase-links">{current.data.links.map((link, index) => <li key={`${link.url}:${index}`}><ProjectReference link={link} /></li>)}</ul> : <p>No approved reference is currently available.</p>} />
  </>}
 </section>
}
async function apiRead(id: string, signal: AbortSignal) { return api.GET('/api/v1/projects/{outcome_id}', { signal, cache: 'no-store', params: { path: { outcome_id: id } } }) }
const mediaDestination = /^\/events\/([a-f\d-]{36})\/media\/([a-f\d-]{36})$/i
function ProjectReference({ link }: { link: ReviewedProject['links'][number] }) {
 if (!link.media) return <a href={link.url} rel="noopener noreferrer" target="_blank"><Icon name="external" />{link.label}</a>
 const destination = mediaDestination.exec(link.url)
 if (!destination || !uuid.test(destination[1]) || !uuid.test(destination[2])) return <span><Icon name="warning" />{link.label} · Current media unavailable</span>
 return <Link to="/events/$eventId/media/$mediaId" params={{ eventId: destination[1], mediaId: destination[2] }}><Icon name="book" />{link.label} (current authorised media)</Link>
}

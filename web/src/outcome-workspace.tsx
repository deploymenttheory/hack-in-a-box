import { CollectionHeading, CollectionRecords, CollectionPagination } from './components/patterns/collection'
import { StatusBadge } from './components/ui/feedback'
import { Icon } from './components/ui/icon'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams, useSearch } from '@tanstack/react-router'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, APIError, privateKey, unwrap } from './api/client'
import { useIdentity } from './session'
import { usePrivateRequestScope } from './private-request-scope'
import { Button, buttonVariants } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { readOutcome, readReview, type Outcome, type OutcomeDefinition } from './outcome-api'
import { CommandState, outcomeDenied, useOutcomeCommand, useOutcomeUnsent } from './outcome-command'
import { OutcomeControls, OutcomeEvidence, OutcomeHistory, OutcomeReviews } from './outcome-controls'
import { OutcomePublication } from './outcome-publication'

const emptyDefinition = (): OutcomeDefinition => ({ kind: 'pitch', problem: '', audience: '', proposed_value: '', solution: '', backlog_url: '', theme_sensitive: false })
export function OutcomeWorkspace() {
  const identity=useIdentity(),search=useSearch({strict:false}) as {event?:string}
 return <OutcomeDirectory key={`${identity.person.id}:${identity.draft_scope}:${search.event??'general'}`} eventID={search.event}/>
}
function OutcomeDirectory({ eventID }: { eventID?: string }) {
  const identity = useIdentity(), cache = useQueryClient(), navigate = useNavigate()
  const scope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:ideas:${eventID ?? 'general'}`)
  const [mount] = useState(() => crypto.randomUUID()), [generation, setGeneration] = useState(() => crypto.randomUUID())
  const [mode, setMode] = useState<'own' | 'review'>('own'), [blocked, setBlocked] = useState<unknown>(), [creating, setCreating] = useState(false)
  const proposal = useRef<HTMLDivElement>(null)
  const prefix = useMemo(() => [...privateKey(identity.person.id, eventID ?? 'general', 'outcome-list'), identity.draft_scope, mount], [identity.person.id, identity.draft_scope, eventID, mount])
  const deny = useCallback((error: unknown) => {
    scope.invalidate(); setBlocked(error)
    void cache.cancelQueries({ queryKey: prefix }); cache.removeQueries({ queryKey: prefix })
    const teams = privateKey(identity.person.id, eventID ?? 'general', 'outcome-own-team')
    void cache.cancelQueries({ queryKey: teams }); cache.removeQueries({ queryKey: teams })
  }, [scope, cache, prefix, identity.person.id, eventID])
  useEffect(() => () => { void cache.cancelQueries({ queryKey: prefix }); cache.removeQueries({ queryKey: prefix }) }, [cache, prefix])
  useEffect(() => {
    if (creating) { const heading = proposal.current?.querySelector('h2'); if (heading) { heading.tabIndex = -1; heading.focus() } }
  }, [creating])
  const listing = useInfiniteQuery({ queryKey: [...prefix, mode, generation], gcTime: 0, enabled: !blocked, initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam, signal }) => {
      await Promise.resolve()
      const ticket = scope.capture(); ticket.assert()
      try {
        const value = unwrap(await api.GET('/api/v1/outcomes', { signal, params: { query: { mode, event_id: mode === 'review' ? eventID : undefined, cursor: pageParam, limit: 25 } } }))
        ticket.assert(); return value
      } catch (error) { if (ticket.current() && outcomeDenied(error)) deny(error); throw error }
    }, getNextPageParam: page => page.next_cursor ?? undefined, refetchInterval: 15_000, refetchOnWindowFocus: 'always', retry: false })
  function chooseMode(next: 'own' | 'review') { scope.renew(); setBlocked(undefined); setMode(next); setGeneration(crypto.randomUUID()) }
  const records = listing.data?.pages.flatMap(page => page.items) ?? []
  return <section className="workspace fleet-outcomes fleet-outcomes--private">
    <CollectionHeading title="Ideas" scope={{ entity: mode === 'review' ? 'Current event organiser review' : 'Your ideas and follow-through' }} description={<><p>Propose an idea, review its current source and follow the evidence. Estimates and self-reports remain distinct from demonstrated adoption.</p><Link className="fleet-outcome-navigation" to="/projects"><Icon name="external" />Browse reviewed projects</Link></>} primary={!blocked ? { label: creating ? 'New idea form open' : 'Propose an idea', icon: 'add', disabled: creating, onSelect: () => setCreating(true) } : undefined} />
    {import.meta.env.MODE === 'admin' && eventID && <nav className="fleet-pattern-actions fleet-outcome-views" aria-label="Idea directory view"><Button icon="user" variant="ghost" disabled={creating} aria-pressed={mode === 'own'} onClick={() => chooseMode('own')}>My ideas</Button><Button icon="shield" variant="ghost" disabled={creating} aria-pressed={mode === 'review'} onClick={() => chooseMode('review')}>Organiser review for this event</Button></nav>}
    {blocked ? <Failure error={blocked} /> : <>
      {creating && <div ref={proposal}><NewIdea key={`${identity.person.id}:${identity.draft_scope}:${eventID ?? 'general'}`} eventID={eventID} scope={scope} deny={deny} /></div>}
      <div className="fleet-outcome-directory" aria-busy={listing.isFetching || undefined}>
        <p className="fleet-outcome-evidence-note"><Icon name="info" />A review status records a deliberate decision. It does not establish achieved impact, an award or a published project.</p>
        {listing.isPending ? <Loading /> : listing.error ? <Failure error={listing.error} retry={() => void listing.refetch()} /> : <>
          <CollectionRecords label="Ideas and follow-through" rows={records} rowKey={value => value.id} rowLabel={value => value.definition.problem} destination={value => ({ href: `/ideas/${value.id}`, label: value.definition.problem, onNavigate: () => void navigate({ to: '/ideas/$outcomeId', params: { outcomeId: value.id } }) })} columns={[
            { id: 'status', label: 'Review status', essential: true, cell: value => <StatusBadge>{value.status.replaceAll('_', ' ')}</StatusBadge> },
            { id: 'source', label: 'Source', cell: value => value.definition.kind === 'pitch' ? 'Future-theme pitch' : 'Team outcome' },
            { id: 'owner', label: 'Follow-through', cell: value => value.owner_id ? 'Owner assigned' : 'No owner assigned' },
            { id: 'version', label: 'Current version', cell: value => value.current_version },
          ]} actions={value => <Link className={buttonVariants({ variant: 'ghost' })} to="/ideas/$outcomeId" params={{ outcomeId: value.id }} aria-label={`Open ${value.definition.problem}`}><Icon name="next" />Open</Link>} />
          {!records.length && <p>No ideas are available in this view.</p>}
          <CollectionPagination source={{ kind: 'cursor', loaded: records.length, busy: listing.isFetchingNextPage, next: listing.hasNextPage ? () => void listing.fetchNextPage() : undefined }} />
        </>}
      </div>
    </>}
  </section>
}

function NewIdea({ eventID, scope, deny }: { eventID?: string; scope: ReturnType<typeof usePrivateRequestScope>; deny: (e: unknown) => void }) {
  const identity = useIdentity(), navigate = useNavigate(), [id] = useState(() => crypto.randomUUID()), [definition, setDefinition] = useState(emptyDefinition), [reason, setReason] = useState(''), [team, setTeam] = useState(''), [created, setCreated] = useState<string>()
  const teams = useQuery({ queryKey: privateKey(identity.person.id, eventID ?? 'general', 'outcome-own-team'), enabled: !!eventID, queryFn: async ({ signal }) => {const ticket=scope.capture();ticket.assert();try{const value=unwrap(await api.GET('/api/v1/events/{event_id}/teams',{signal,params:{path:{event_id:eventID!}}}));ticket.assert();return value}catch(error){if(ticket.current()&&outcomeDenied(error))deny(error);throw error}}, retry: false, refetchInterval: 15_000 })
  const command = useOutcomeCommand(`${identity.person.id}:${identity.draft_scope}:new:${id}`, scope, async (intent, ticket) => { try { ticket.assert(); await readOutcome(id, ticket.signal); ticket.assert(); throw new APIError(409, 'conflict') } catch (error) { if (!(error instanceof APIError && error.status === 404)) throw error; ticket.assert() }; if(intent.kind==='outcome'&&intent.command.provenance?.event_id&&intent.command.provenance.team_id){const eventID=intent.command.provenance.event_id; ticket.assert();const teams=unwrap(await api.GET('/api/v1/events/{event_id}/teams',{signal:ticket.signal,params:{path:{event_id:eventID}}}));ticket.assert();if(!teams.teams.some(t=>t.state==='published'&&t.id===intent.command.provenance!.team_id))throw new APIError(403,'forbidden');const event=unwrap(await api.GET('/api/v1/events/{event_id}',{signal:ticket.signal,params:{path:{event_id:eventID}}}));ticket.assert();if(!event.published_at||Date.now()<Date.parse(event.configuration.live_start)||Date.now()<Date.parse(event.published_at))throw new APIError(409,'event_blocked')} }, value => { setCreated('id' in value ? value.id : id) }, deny)
  const dirty = !created && (!!definition.problem || !!definition.audience || !!definition.proposed_value || !!definition.solution || !!definition.backlog_url || !!reason || !!command.intent)
  useOutcomeUnsent(dirty)
  useEffect(() => { if (created) void navigate({ to: '/ideas/$outcomeId', params: { outcomeId: created } }) }, [created, navigate])
  function change(name: keyof OutcomeDefinition, value: string | boolean) { setDefinition({ ...definition, [name]: value }); command.clearNotice() }
  return <section className="panel"><h2>Propose an idea</h2><p>Private to you, your current source team where applicable, and authorised organisers. This does not publish a project or claim an achieved result.</p><p>Unsent input stays only in memory in this tab. It is not recovered after leaving, reloading or signing out.</p><form onSubmit={e => { e.preventDefault(); void command.write({ kind: 'outcome', outcomeID: id, expected: 0, command: { id: crypto.randomUUID(), action: 'propose', reason, definition: { ...definition, kind: team ? 'team_outcome' : 'pitch' }, provenance: team && eventID ? { event_id: eventID, team_id: team } : undefined } }) }}><fieldset disabled={command.busy || !!command.intent}>
    {eventID && teams.data && <><label htmlFor="idea-team">Source</label><select id="idea-team" value={team} onChange={e => setTeam(e.target.value)}><option value="">General organisation idea (no event association)</option>{teams.data.teams.filter(t => t.state === 'published').map(t => <option key={t.id} value={t.id}>Team outcome: {t.name}</option>)}</select><p>Team competition output can only be captured after the actual common release and scheduled live start. General pitches remain available independently.</p></>}
    {teams.error && <p role="status">Team context is unavailable. You can still submit a general organisation idea.</p>}
    <DefinitionFields value={definition} change={change} prefix="new-idea" /><label htmlFor="idea-submit-reason">Why submit this idea?</label><textarea id="idea-submit-reason" required maxLength={2000} value={reason} onChange={e => setReason(e.target.value)} /><Button type="submit" disabled={!navigator.onLine || !definition.problem.trim() || !definition.audience.trim() || !definition.proposed_value.trim() || !reason.trim()}>Submit idea for review</Button></fieldset></form><CommandState value={command} /></section>
}

export function DefinitionFields({ value, change, prefix }: { value: OutcomeDefinition; change: (name: keyof OutcomeDefinition, value: string | boolean) => void; prefix: string }) {
  return <>{([['problem', 'Problem or opportunity', 10000], ['audience', 'Who benefits?', 4000], ['proposed_value', 'Proposed value or estimate (not achieved results)', 10000], ['solution', 'Proposed solution (optional)', 10000], ['backlog_url', 'External backlog reference (optional HTTPS link)', 2000]] as const).map(([name, label, max]) => <div key={name}><label htmlFor={`${prefix}-${name}`}>{label}</label>{name === 'backlog_url' ? <input id={`${prefix}-${name}`} type="url" maxLength={max} value={value[name]} onChange={e => change(name, e.target.value)} /> : <textarea id={`${prefix}-${name}`} maxLength={max} required={name !== 'solution'} value={value[name]} onChange={e => change(name, e.target.value)} />}</div>)}<label className="checkbox-row"><input type="checkbox" checked={value.theme_sensitive} onChange={e => change('theme_sensitive', e.target.checked)} />This idea contains theme-sensitive information (actual release embargo applies)</label></>
}

export function OutcomePage() {
 const { outcomeId } = useParams({ strict: false }), identity = useIdentity()
 return <OutcomeReader key={`${identity.person.id}:${identity.draft_scope}:${outcomeId}`} id={outcomeId!} />
}
function OutcomeReader({id}:{id:string}) {
 const identity=useIdentity(),cache=useQueryClient(),[blocked,setBlocked]=useState<unknown>()
  const prefix = [...privateKey(identity.person.id, id, 'outcome')], scope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${id}`)
  const deny = useCallback((error: unknown) => { scope.invalidate(); setBlocked(error); void cache.cancelQueries({ queryKey: privateKey(identity.person.id, id, 'outcome') }); cache.removeQueries({ queryKey: privateKey(identity.person.id, id, 'outcome') }) }, [cache, identity.person.id, id, scope])
  const current = useQuery({ queryKey: [...prefix, 'current'], enabled: !blocked, queryFn: async ({ signal }) => { const ticket = scope.capture(); ticket.assert(); try { const value = await readOutcome(id, signal); ticket.assert(); return value } catch (error) { if (ticket.current() && outcomeDenied(error)) deny(error); throw error } }, retry: false, refetchInterval: 15_000, refetchOnWindowFocus: 'always' })
  const refresh = () => { void cache.invalidateQueries({ queryKey: prefix }); void cache.invalidateQueries({ queryKey: ['private', identity.person.id, 'general', 'outcome-list'] }) }
  if (blocked) return <section className="workspace"><h1>Idea unavailable</h1><p>Current access changed. Private inputs and cached outcome data were cleared.</p><Failure error={blocked} /><Button onClick={() => { scope.renew(); setBlocked(undefined) }}>Check current outcome access</Button></section>
  if (!current.data) return current.error ? <Failure error={current.error} retry={() => void current.refetch()} /> : <Loading />
  const value = current.data
  return <section className="workspace outcome-workspace"><Link to="/ideas">← Ideas</Link><div className="page-heading"><div><p className="eyebrow">Restricted follow-through · {value.capability.archived ? 'Archived event: outcome updates only' : 'Current resource'}</p><h1>Idea and follow-through</h1><p>Reasoned status: {value.status.replaceAll('_', ' ')} · Version {value.current_version}</p><Button variant="outline" onClick={refresh}>Refresh current outcome</Button></div><Link to="/projects/$outcomeId" params={{ outcomeId: id }}>Check separately published project</Link></div>
    {current.error && <p role="status">Current authority could not be refreshed. Private saved evidence is hidden and saving is disabled; unsent input remains in this tab until access is checked.</p>}
    {!current.error && <OutcomeSummary value={value} />}
    <OutcomeControls key={`${identity.person.id}:${identity.draft_scope}:${id}`} value={value} scope={scope} deny={deny} available={!current.error} refresh={refresh} />
    <OutcomeEvidence value={value} scope={scope} deny={deny} available={!current.error} refresh={refresh} />
    <OutcomeReviews value={value} scope={scope} deny={deny} available={!current.error} refresh={refresh} />
    {import.meta.env.MODE === 'admin' && value.capability.can_moderate && <OutcomePublication value={value} scope={scope} deny={deny} available={!current.error} refresh={refresh} />}
    {!current.error && <OutcomeHistory value={value} scope={scope} deny={deny} />}
  </section>
}
function OutcomeSummary({ value }: { value: Outcome }) { return <section className="panel"><h2>{value.definition.problem}</h2><dl className="dates"><div><dt>Who benefits?</dt><dd>{value.definition.audience}</dd></div><div><dt>Proposed value</dt><dd>{value.definition.proposed_value}</dd></div><div><dt>Solution</dt><dd>{value.definition.solution || 'Not recorded'}</dd></div><div><dt>Owner</dt><dd className="identifier">{value.owner_id || 'Not assigned'}</dd></div><div><dt>Review date</dt><dd>{value.review_at ? new Date(value.review_at).toLocaleString() : 'Not assigned'}</dd></div></dl>{value.definition.backlog_url && <a href={value.definition.backlog_url} rel="noopener noreferrer" target="_blank">External backlog reference</a>}<p>Proposed value and private evidence are not automatically published. Scores, submissions and awards remain unchanged.</p></section> }

export function OutcomeReviewPage() {
  const { reviewId } = useParams({ strict: false }), identity = useIdentity(), scope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:review:${reviewId}`)
  const current = useQuery({ queryKey: privateKey(identity.person.id, reviewId!, 'outcome-review'), queryFn: async ({ signal }) => { const ticket = scope.capture(); ticket.assert(); const value = await readReview(reviewId!, signal); ticket.assert(); return value }, refetchInterval: 15_000, refetchOnWindowFocus: 'always', retry: false })
  return <section className="workspace outcome-workspace"><Link to="/ideas">← Ideas</Link><h1>Outcome review</h1>{current.isPending ? <Loading /> : current.error ? <Failure error={current.error} /> : <section className="panel"><h2>{current.data.schedule.days}-day checkpoint</h2><p>{current.data.effective_state.replaceAll('_', ' ')} · {new Date(current.data.due_at).toLocaleString(undefined, { timeZone: current.data.schedule.timezone })} ({current.data.schedule.timezone})</p><p>No action or completion is submitted automatically by opening this reminder.</p><Link className="primary-link" to="/ideas/$outcomeId" params={{ outcomeId: current.data.outcome_id }}>Open your current follow-through</Link></section>}</section>
}

export function PublishedProjects() {
  const identity = useIdentity(), [search, setSearch] = useState(''), [term, setTerm] = useState('')
  const projects = useInfiniteQuery({ queryKey: [...privateKey(identity.person.id, 'organisation', 'published-projects'), term], initialPageParam: undefined as string | undefined, queryFn: async ({ pageParam, signal }) => unwrap(await api.GET('/api/v1/projects', { signal, params: { query: { search: term, cursor: pageParam, limit: 25 } } })), getNextPageParam: p => p.next_cursor ?? undefined, retry: false, refetchInterval: 15_000, refetchOnWindowFocus: 'always' })
  return <section className="workspace outcome-workspace"><Link to="/ideas">← Your ideas</Link><h1>Reviewed projects</h1><p>Only separately reviewed publications available to your current audience appear here. Event completion, submissions or awards do not publish projects.</p><form onSubmit={e => { e.preventDefault(); setTerm(search) }}><label htmlFor="project-search">Search reviewed problem and solution</label><input id="project-search" value={search} maxLength={200} onChange={e => setSearch(e.target.value)} /><Button type="submit">Search projects</Button></form>{projects.isPending ? <Loading /> : projects.error ? <Failure error={projects.error} retry={() => void projects.refetch()} /> : <><div className="event-grid">{projects.data.pages.flatMap(p => p.items).map(project => <article className="panel" key={project.id}><h2><Link to="/projects/$outcomeId" params={{ outcomeId: project.id }}>{project.problem}</Link></h2><p>{project.solution}</p><p>Reviewed follow-up status: {project.follow_up_status.replaceAll('_', ' ')}</p></article>)}</div>{!projects.data.pages[0].items.length && <p>No permitted reviewed projects match this search.</p>}{projects.hasNextPage && <Button onClick={() => void projects.fetchNextPage()}>More reviewed projects</Button>}</>}</section>
}
export function PublishedProjectPage() {
  const { outcomeId } = useParams({ strict: false }), identity = useIdentity()
  const project = useQuery({ queryKey: privateKey(identity.person.id, outcomeId!, 'published-project'), queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/projects/{outcome_id}', { signal, params: { path: { outcome_id: outcomeId! } } })), retry: false, refetchInterval: 15_000, refetchOnWindowFocus: 'always' })
  return <section className="workspace outcome-workspace"><Link to="/projects">← Reviewed projects</Link>{project.isPending ? <Loading /> : project.error ? <Failure error={project.error} /> : <article className="panel"><p className="eyebrow">Separately reviewed publication · Version {project.data.version}</p><h1>{project.data.problem}</h1><h2>Solution</h2><p className="outcome-prose">{project.data.solution}</p><p>Published follow-up status: {project.data.follow_up_status.replaceAll('_', ' ')}. This is the status reviewed for this publication; later private updates do not silently republish it.</p>{project.data.event_name && <p>Source event: {project.data.event_name}</p>}{project.data.team_name && <p>Source team: {project.data.team_name}</p>}<ul>{project.data.links.map((link, i) => <li key={`${link.url}:${i}`}><a href={link.url} rel="noopener noreferrer" target={link.media ? undefined : '_blank'}>{link.label}{link.media ? ' (current authorised media)' : ''}</a></li>)}</ul><p>Published {new Date(project.data.published_at).toLocaleString()}. Private evidence, estimates and original assessment records are excluded.</p></article>}</section>
}

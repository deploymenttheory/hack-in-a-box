import { useRef, useState } from 'react'
import { useParams } from '@tanstack/react-router'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { APIError, privateKey } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { localDate } from './event-time'
import { checkpointPage, currentCard } from './scorecard-api'
import type { Checkpoint } from './scorecard-model'
import { CardEditor, type ScorecardRisk } from './mentor-scorecard-task'
import { CollectionFrame, CollectionHeading, CollectionPagination, CollectionRecords, ContextLink } from './components/patterns'
import { CoverageDetail } from './coverage-detail'
import { useCoverageFocus, type CoverageFocusTarget } from './coverage-navigation'
import { usePrivateRequestScope } from './private-request-scope'
import { scrubMentorPrivateQueries } from './mentor-private-lifecycle'
import { PlanningOperations } from './planning-operations'

function invalidatesEditor(error: unknown) { return error instanceof APIError && [401, 403, 404, 409].includes(error.status) }
export function ScorecardWorkspace() {
  const { eventId, teamId } = useParams({ strict: false }), identity = useIdentity()
  return <PlanningOperations key={`${identity.person.id}:${identity.draft_scope}:${eventId}:${teamId}`}><ScorecardTeam eventID={eventId!} teamID={teamId!} /></PlanningOperations>
}
function ScorecardTeam({ eventID, teamID }: { eventID: string; teamID: string }) {
  const identity = useIdentity(), cache = useQueryClient(), [selected, select] = useState<string>(), [risk, setRisk] = useState<ScorecardRisk>({ dirty: false, pending: false, unknown: false })
  const root = useRef<HTMLElement>(null), focus = useCoverageFocus(root), origin = useRef<CoverageFocusTarget | undefined>(undefined)
  const scopeKey = `${identity.person.id}:${identity.draft_scope}:${eventID}:${teamID}:checkpoints`, scope = usePrivateRequestScope(scopeKey)
  const query = useInfiniteQuery({ queryKey: [...privateKey(identity.person.id, eventID, 'scorecard-checkpoints'), teamID], initialPageParam: undefined as string | undefined, queryFn: async ({ pageParam, signal }) => { const ticket = scope.capture(); let page; try { page = await checkpointPage(eventID, teamID, pageParam, signal) } catch (error) { if (ticket.current() && invalidatesEditor(error)) { select(undefined); scrubMentorPrivateQueries(cache, identity.person.id, eventID, teamID, 'scorecards') }; throw error }; ticket.assert(); return page }, getNextPageParam: page => page.next_cursor ?? undefined, refetchInterval: 15_000, refetchOnWindowFocus: 'always' })
  const checkpoints = query.data?.pages.flatMap(p => p.items) ?? [], checkpoint = checkpoints.find(c => `${c.definition.id}:${c.version}:${c.definition.run_id}` === selected)
  const denied = invalidatesEditor(query.error)
  const state = denied ? { kind: 'denied' as const, message: 'Scorecards require current assignment or review authority and a released live assessment source. Private entry has been removed.' } : query.isPending ? { kind: 'loading' as const } : query.error ? { kind: query.data ? 'paused' as const : 'failed' as const, message: 'Current checkpoint authority could not be verified. Private entry is hidden and paused.', read: { label: 'Read current checkpoints', onSelect: () => void query.refetch() } } : { kind: 'ready' as const }
  function close() { if (risk.pending || ((risk.dirty || risk.unknown) && !window.confirm(risk.unknown ? 'Leave the original assessment outcome unresolved? It will not be replayed.' : 'Discard the private unsent scorecard changes?'))) return; select(undefined); focus.restore(origin.current, true) }
  return <section ref={root} tabIndex={-1} className="fleet-scorecard-workspace"><ContextLink back target={{ label: 'Team mentor workspace', href: `/events/${eventID}/mentor/${teamID}` }} /><CollectionFrame scopeKey={scopeKey} state={state} heading={<CollectionHeading title="Your independent scorecards" scope={{ entity: `Team ${teamID}` }} description="Your private assessment has its own source and author. Huddles, moderation, sign-off and announced results are separate decisions. Unsent scores stay in this task’s memory; they are never stored for offline submission." />}>
    <div hidden={!!query.error} inert={!!query.error}>{checkpoint ? <CoverageDetail title={checkpoint.definition.name} close={close} disabled={risk.pending}><CardLoader key={`${selected}:${identity.draft_scope}`} eventID={eventID} teamID={teamID} checkpoint={checkpoint} onRisk={setRisk} /></CoverageDetail> : <><h2>Checkpoint schedule and rubric</h2>{selected && <p role="alert">The selected checkpoint/run changed or is no longer available. Its private editor has been cleared; choose a current checkpoint.</p>}<CollectionRecords label="Published checkpoint schedule" rows={checkpoints} rowKey={c => `${c.definition.id}:${c.version}:${c.definition.run_id}`} rowLabel={c => c.definition.name} columns={[{ id: 'purpose', label: 'Assessment source', cell: c => c.purpose === 'checkpoint' ? 'Live checkpoint · provisional assessment' : 'Final mentor qualification · frozen formal output' }, { id: 'window', label: 'Native checkpoint window', cell: c => `${localDate(c.opens_at, c.timezone)} — ${localDate(c.closes_at, c.timezone)} · ${c.timezone}` }, { id: 'version', label: 'Source version', cell: c => `Rubric ${c.rubric.version} · Checkpoint ${c.version}` }]} actions={c => <Button data-record-focus={`${c.definition.id}:${c.version}:${c.definition.run_id}`} icon="next" variant="ghost" onClick={() => { origin.current = focus.capture(); select(`${c.definition.id}:${c.version}:${c.definition.run_id}`) }}>Open {c.definition.name}</Button>} />{!checkpoints.length && <p>No current published checkpoints are available.</p>}<CollectionPagination source={{ kind: 'cursor', loaded: checkpoints.length, busy: query.isFetchingNextPage, next: query.hasNextPage ? () => void query.fetchNextPage() : undefined }} /></>}</div>
  </CollectionFrame>{denied && <Button icon="refresh" variant="outline" onClick={() => void query.refetch()}>Read current checkpoints</Button>}</section>
}
function CardLoader({ eventID, teamID, checkpoint, onRisk }: { eventID: string; teamID: string; checkpoint: Checkpoint; onRisk: (risk: ScorecardRisk) => void }) {
  const identity = useIdentity(), scope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${eventID}:${teamID}:${checkpoint.definition.id}:${checkpoint.version}:${checkpoint.definition.run_id}`)
  const query = useQuery({ queryKey: [...privateKey(identity.person.id, eventID, 'scorecard-current'), teamID, checkpoint.definition.id, checkpoint.version], gcTime: 0, refetchOnMount: 'always', queryFn: async ({ signal }) => { const ticket = scope.capture(); const value = await currentCard(eventID, teamID, checkpoint, identity.person.id, signal); ticket.assert(); return value }, refetchInterval: 15_000, refetchOnWindowFocus: 'always' })
  if (query.isPending || !query.isFetchedAfterMount) return <Loading />
  if (query.error && (!query.data || invalidatesEditor(query.error))) return <><p>Entry is unavailable until current authority and the assessment source are confirmed. Final qualification requires actual post-cutoff frozen formal output.</p><Failure error={query.error} retry={() => void query.refetch()} /></>
  return <>{query.error && <><p>Current card/source read is unavailable. Unsent input remains in memory; saved evidence and entry are hidden until a fresh read succeeds.</p><Failure error={query.error} retry={() => void query.refetch()} /></>}<div hidden={!!query.error} inert={!!query.error}><CardEditor eventID={eventID} teamID={teamID} checkpoint={checkpoint} initial={query.data!} refresh={() => query.refetch()} onRisk={onRisk} /></div></>
}

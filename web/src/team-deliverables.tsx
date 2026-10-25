import { useEffect, useId, useLayoutEffect, useRef, useState } from 'react'
import { useBlocker } from '@tanstack/react-router'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, privateKey, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { ActionLink, Button } from './components/ui/button'
import { Select, Textarea, Label, Checkbox } from './components/ui/fields'
import { ConfirmationDialog } from './components/ui/dialog'
import { CollectionHeading } from './components/patterns'
import { StatusBadge } from './components/ui/feedback'
import { Failure, Loading } from './components/states'
import { localDate } from './event-time'
import { PersonalPreparation } from './personal-preparation'
import { DeliverableEditor } from './deliverable-editor'
import { EvidenceList, OrganiserHelp, SubmissionHistory } from './deliverable-evidence'
import { words, type SubmissionPage, type Team } from './deliverable-model'
import { usePrivateRequestScope, type PrivateRequestScope } from './private-request-scope'
import { useEventReadBoundary } from './event-authority'
import { useCommunicationCommands } from './communication-tests'
import { planningDepartureAllowed } from './planning-operations'
import { resourceDenied, ResourceOperationResult, useResourceOperation } from './resource-operation'

export function TeamDeliverables(props: { event: EventRecord; onDirty: (dirty: boolean) => void }) {
  const identity = useIdentity(), boundary = useEventReadBoundary(props.event.id)
  return <DeliverablesLifetime key={`${identity.person.id}:${identity.draft_scope}:${props.event.id}:${boundary?.generation ?? 0}`} {...props} parentScope={boundary?.scope} authorityPaused={!!boundary?.paused} />
}
function DeliverablesLifetime({ event, onDirty, parentScope, authorityPaused }: { event: EventRecord; onDirty: (dirty: boolean) => void; parentScope?: PrivateRequestScope; authorityPaused: boolean }) {
  const identity = useIdentity(), cache = useQueryClient(), [selected, setSelected] = useState(''), [personalDirty, setPersonalDirty] = useState(false), [teamDirty, setTeamDirty] = useState(false), [denied, setDenied] = useState<unknown>(), [generation, setGeneration] = useState(0), signature = useRef<string | undefined>(undefined), scope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${event.id}:working-teams`, parentScope)
  const selectedTeam = useRef<string | undefined>(undefined)
  function clearPrivate() { for (const query of cache.getQueryCache().findAll({ queryKey: ['private', identity.person.id, event.id] })) { const name = query.queryKey[3]; if (typeof name === 'string' && name.startsWith('submission-')) { void cache.cancelQueries({ queryKey: query.queryKey, exact: true }); cache.removeQueries({ queryKey: query.queryKey, exact: true }) } } }
  useEffect(() => { onDirty(personalDirty || teamDirty); return () => onDirty(false) }, [personalDirty, teamDirty, onDirty])
  const teams = useQuery({ queryKey: privateKey(identity.person.id, event.id, 'teams'), retry: false, enabled: !denied, queryFn: async ({ signal }) => {
    const ticket = scope.capture()
    try {
      const value = unwrap(await api.GET('/api/v1/events/{event_id}/teams', { signal, params: { path: { event_id: event.id } } }))
      ticket.assert()
      // Unrelated team additions do not revoke the selected owner's unsent work.
      const eligible = value.teams.filter(team => team.state === 'published' && (value.can_manage || team.published.some(person => person.id === identity.person.id)))
      const current = JSON.stringify([value.can_manage, selectedTeam.current ? eligible.some(team => team.id === selectedTeam.current) : true])
      if (signature.current !== undefined && signature.current !== current) { scope.renew(); clearPrivate(); setGeneration(value => value + 1) }
      signature.current = current
      return value
    } catch (error) {
      if (resourceDenied(error)) { scope.invalidate(); clearPrivate(); setDenied(error) }
      throw error
    }
  }, refetchOnWindowFocus: 'always', refetchInterval: 15_000 })
  const manageable = teams.data?.can_manage ?? false, active = teams.data?.teams.filter(team => team.state === 'published' && (manageable || team.published.some(person => person.id === identity.person.id))) ?? [], team = active.find(item => item.id === selected) ?? active[0]
  useLayoutEffect(() => { selectedTeam.current = team?.id }, [team?.id])
  if (denied || resourceDenied(teams.error)) return <Failure error={denied ?? teams.error} retry={() => { scope.renew(); setDenied(undefined); void teams.refetch() }} />
  return <section className="fleet-resource-workspace"><CollectionHeading title="Deliverables" scope={{ event: event.name, entity: 'Preparation and competition evidence' }} description="Personal preparation, private team drafts and formal competition evidence have separate owners and acceptance rules." /><PersonalPreparation event={event} onDirty={setPersonalDirty} />
    <section className="fleet-resource-section"><h2>Team deliverables</h2>{teams.isPending || !teams.isFetchedAfterMount ? <Loading /> : !teams.data ? <Failure error={teams.error} retry={() => void teams.refetch()} /> : <>{!!teams.error && <Failure error={teams.error} retry={() => void teams.refetch()} />}{active.length > 1 && <><Label htmlFor="deliverable-team">Team</Label><Select id="deliverable-team" value={team?.id ?? ''} onChange={event => { if (planningDepartureAllowed() && (!teamDirty || window.confirm('Switch teams with unsaved evidence?'))) setSelected(event.target.value) }}>{active.map(item => <option key={item.id} value={item.id}>{item.name} · {item.id.slice(0, 8)}</option>)}</Select></>}{team ? <TeamSubmission key={`${team.id}:${generation}`} event={event} team={team} organiser={manageable} parentScope={scope} paused={authorityPaused || !!teams.error} onDirty={setTeamDirty} /> : <p>Your published team deliverables will appear here after allocation. Draft assignments cannot submit competition evidence.</p>}{teams.data.teams.filter(item => item.state === 'disbanded').map(item => <p key={item.id}>{item.name} is disbanded. Its entry is withdrawn, unused and unscored; there is no submission to complete.</p>)}</>}</section>
  </section>
}
type TeamSubmissionProps = { event: EventRecord; team: Team; organiser: boolean; paused: boolean; onDirty: (dirty: boolean) => void; parentScope: PrivateRequestScope }
function TeamSubmission(props: TeamSubmissionProps) {
  const [generation, setGeneration] = useState(0)
  return <TeamSubmissionLifetime key={generation} {...props} retry={() => setGeneration(value => value + 1)} />
}
function TeamSubmissionLifetime({ event, team, organiser, paused, onDirty, parentScope, retry }: TeamSubmissionProps & { retry: () => void }) {
  const identity = useIdentity(), cache = useQueryClient(), [editorDirty, setEditorDirty] = useState(false), [reviewDirty, setReviewDirty] = useState(false), [editorDenied, setEditorDenied] = useState<unknown>(), [reviewDenied, setReviewDenied] = useState<unknown>(), [pageDenied, setPageDenied] = useState<unknown>()
  const mount = useId(), scope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${event.id}:${team.id}:submission`, parentScope), pageKey = [...privateKey(identity.person.id, event.id, `submission-${team.id}`), mount], draftKey = [...privateKey(identity.person.id, event.id, `submission-draft-${team.id}`), mount]
  useEffect(() => { onDirty(editorDirty || reviewDirty); return () => onDirty(false) }, [editorDirty, reviewDirty, onDirty])
  const page = useQuery({ queryKey: pageKey, gcTime: 0, retry: false, enabled: !pageDenied, queryFn: async ({ signal }) => {
    const ticket = scope.capture()
    try {
      const value = unwrap(await api.GET('/api/v1/events/{event_id}/teams/{team_id}/submission', { signal, params: { path: { event_id: event.id, team_id: team.id } } }))
      ticket.assert(); return value
    } catch (error) {
      if (resourceDenied(error)) {
        scope.invalidate(); setPageDenied(error)
        for (const resource of [`submission-${team.id}`, `submission-draft-${team.id}`, `submission-history-${team.id}`]) {
          const key = privateKey(identity.person.id, event.id, resource)
          void cache.cancelQueries({ queryKey: key }); cache.removeQueries({ queryKey: key })
        }
      }
      throw error
    }
  }, refetchOnWindowFocus: 'always', refetchInterval: 15_000 })
  const draftScope = usePrivateRequestScope(`${scope.key}:draft:${page.data?.can_read_draft === true}`, scope)
  const draft = useQuery({ queryKey: draftKey, gcTime: 0, enabled: page.data?.can_read_draft === true, retry: false, queryFn: async () => { const ticket = draftScope.capture(); const value = unwrap(await api.GET('/api/v1/events/{event_id}/teams/{team_id}/submission/draft', { signal: ticket.signal, params: { path: { event_id: event.id, team_id: team.id } } })); ticket.assert(); return value }, refetchOnWindowFocus: 'always' })
  const privateCleared = useRef(false)
  const mayReadDraft = page.data?.can_read_draft
  useLayoutEffect(() => {
    if (mayReadDraft !== false && !resourceDenied(draft.error) && !editorDenied && !resourceDenied(page.error)) { privateCleared.current = false; return }
    if (privateCleared.current) return
    privateCleared.current = true
    draftScope.invalidate()
    const key = privateKey(identity.person.id, event.id, `submission-draft-${team.id}`)
    void cache.cancelQueries({ queryKey: key })
    cache.removeQueries({ queryKey: key })
  }, [mayReadDraft, draft.error, editorDenied, page.error, cache, identity.person.id, event.id, team.id, draftScope])
  if (pageDenied) return <Failure error={pageDenied} retry={retry} />
  if (page.isPending || !page.isFetchedAfterMount) return <Loading />
  if (!page.data || resourceDenied(page.error)) return <Failure error={page.error} retry={() => void page.refetch()} />
  const value = page.data, captain = team.published.find(person => person.id === team.published_captain), editable = value.can_read_draft || value.can_submit
  return <section aria-label="Team submission workspace">{!!page.error && <Failure error={page.error} retry={() => void page.refetch()} />}<CollectionHeading title={team.name} scope={{ entity: 'Competition entry', event: event.name }} description={<>Owner: {team.name}{captain ? ` · Captain ${captain.name}` : ' · Organiser must appoint a captain'}</>} status={<StatusBadge tone={value.outcome === 'frozen' ? 'success' : 'neutral'}>{words(value.outcome)}</StatusBadge>} /><dl className="fleet-resource-facts"><div><dt>Formal submissions and review open</dt><dd>{localDate(value.live_start, event.configuration.timezone)}</dd></div><div><dt>Shared submission cutoff</dt><dd>{localDate(value.cutoff, event.configuration.timezone)}</dd></div><div><dt>Recorded outcome</dt><dd>{words(value.outcome)}{value.frozen_at && ` · Frozen ${localDate(value.frozen_at, event.configuration.timezone)}`}</dd></div></dl>
    {!value.can_submit && value.outcome === 'open' && <p>{value.can_draft ? 'You may prepare and save a private draft. Formal submissions and review open at live start.' : 'Formal submission is unavailable. The challenge must be published and the live submission window must be open.'}</p>}{value.outcome !== 'open' && <p>{value.outcome === 'frozen' ? 'Final evidence is frozen at the cutoff.' : value.outcome === 'not_submitted' ? 'No formal submission was recorded by the cutoff. A private draft does not count as a submission.' : 'This entry is withdrawn and will not be scored.'} Ask an organiser about any exception.</p>}
    <ActionLink icon="code" href={`/events/${event.id}/teams/${team.id}/build`}>Review submitted build and code cutoff evidence</ActionLink><section className="fleet-resource-section" aria-label="Current formal submission"><h3>Current formal evidence</h3>{value.final ? <><p>Formal version {value.final.revision} · Submitted {localDate(value.final.submitted_at, event.configuration.timezone)}{value.final.late ? ' · Late amendment' : ''} · Completion review: {words(value.final.review_status)}</p><EvidenceList eventID={event.id} items={value.final.evidence} /></> : <p>No formal submission accepted.</p>}</section>
    {editorDenied || resourceDenied(draft.error) ? <Failure error={editorDenied ?? draft.error} retry={retry} /> : editable ? value.can_read_draft && draft.isPending ? <Loading /> : value.can_read_draft && !draft.data ? <Failure error={draft.error} retry={() => void draft.refetch()} /> : <DeliverableEditor key={value.can_read_draft ? 'team-draft' : 'formal-amendment'} event={event} team={team} page={value} parentScope={value.can_read_draft ? draftScope : scope} pageKey={pageKey} draftKey={draftKey} initialDraft={value.can_read_draft ? draft.data : undefined} organiser={organiser} paused={paused || !!page.error || (value.can_read_draft && !!draft.error)} onDeny={setEditorDenied} onDirty={setEditorDirty} /> : !organiser && !value.can_read_draft ? <section className="fleet-resource-section"><h3>Competition requirements</h3><p>Competition requirements will appear after the common challenge is published. Your personal preparation remains available above.</p></section> : <section className="fleet-resource-section"><h3>Required and optional work</h3><ul>{value.definitions.map(definition => <li key={definition.id}><strong>{definition.title}</strong> · {definition.required ? 'Required' : 'Optional'} · Due {localDate(definition.due_at, event.configuration.timezone)}<p>{definition.description}</p></li>)}</ul></section>}
    <OrganiserHelp page={value} team={team} event={event} />{value.final && <SubmissionHistory event={event} teamID={team.id} parentScope={scope} />}{reviewDenied ? <Failure error={reviewDenied} retry={retry} /> : value.can_review && <CompletionReview event={event} teamID={team.id} parentScope={scope} pageKey={pageKey} page={value} paused={paused || !!page.error} onDeny={setReviewDenied} onDirty={setReviewDirty} />}
  </section>
}
function CompletionReview({ event, teamID, page, paused, onDeny, onDirty, parentScope, pageKey }: { parentScope: PrivateRequestScope; pageKey: readonly unknown[]; event: EventRecord; teamID: string; page: SubmissionPage; paused: boolean; onDeny: (error: unknown) => void; onDirty: (dirty: boolean) => void }) {
  const identity = useIdentity(), cache = useQueryClient(), [status, setStatus] = useState<'pending' | 'accepted' | 'changes_requested'>('changes_requested'), [reason, setReason] = useState(''), [revision, setRevision] = useState(page.revision), [formal, setFormal] = useState(page.final!.revision), [latest, setLatest] = useState<SubmissionPage>(), [reviewed, setReviewed] = useState(false)
  const [review, setReview] = useState<{ status: typeof status; reason: string; revision: number; formal: number }>()
  const owner = `${identity.person.id}:${identity.draft_scope}:${event.id}:team-deliverables:${teamID}:completion`, safeCommands = useCommunicationCommands(owner)
  const operation = useResourceOperation(owner, onDeny, { parentScope, safeCommands, resourceID: teamID }), changed = revision !== page.revision || formal !== page.final?.revision
  useEffect(() => { onDirty(!!reason || !!review || operation.blocked); return () => onDirty(false) }, [reason, review, operation.blocked, onDirty])
  useBlocker({ shouldBlockFn: () => operation.isBlocked() || (!!reason && !window.confirm('Leave this unsaved completion feedback?')), enableBeforeUnload: !!reason || operation.blocked })
  async function save() {
    if (!review || paused || changed || !page.can_review) return
    const intent = review, ticket = operation.capture()
    await operation.run({ operation: 'Record completion feedback', revision: intent.revision, command: { submission_revision: intent.formal, status: intent.status, reason: intent.reason } }, async () => {
      const result = unwrap(await api.POST('/api/v1/events/{event_id}/teams/{team_id}/submission/review', { params: { path: { event_id: event.id, team_id: teamID }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${intent.revision}"` } }, body: { submission_revision: intent.formal, status: intent.status, reason: intent.reason } }))
      if (result.team_id !== teamID || result.revision <= intent.revision || result.final?.revision !== intent.formal || result.final.review_status !== intent.status) throw new Error('Unexpected completion review response')
      return result
    }, async (result, current) => { setRevision(result.revision); setReason(''); setReview(undefined); cache.setQueryData(pageKey, result); await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, event.id, `submission-history-${teamID}`) }); current.assert(); return `Completion feedback recorded for formal version ${intent.formal}.` })
    if (ticket.current()) setReview(undefined)
  }
  function readCurrent() { void operation.read(async signal => unwrap(await api.GET('/api/v1/events/{event_id}/teams/{team_id}/submission', { signal, params: { path: { event_id: event.id, team_id: teamID } } })), value => { setLatest(value); setReviewed(false); cache.setQueryData(pageKey, value) }) }
  function useCurrent() { if (!latest?.can_review || !latest.final || !reviewed) return; setRevision(latest.revision); setFormal(latest.final.revision); setLatest(undefined); setReviewed(false); operation.useCurrent() }
  return <section className="fleet-resource-section" aria-label="Organiser completion review"><h3>Organiser completion review</h3><p>Review formal version {formal}. This records completion feedback, separately from mentor checkpoint scores and final judging.</p><form className="fleet-resource-form" onSubmit={event => { event.preventDefault(); if (!paused && !changed && !operation.blocked && reason.trim()) setReview({ status, reason, revision, formal }) }}><fieldset className="fleet-resource-fields" disabled={paused || operation.blocked || changed}><legend>Completion decision</legend><Label htmlFor="completion-status">Completion decision</Label><Select id="completion-status" value={status} onChange={event => setStatus(event.target.value as typeof status)}><option value="changes_requested">Changes requested</option><option value="accepted">Complete</option><option value="pending">Pending</option></Select><Label htmlFor="completion-feedback">Feedback and reason</Label><Textarea id="completion-feedback" value={reason} onChange={event => setReason(event.target.value)} maxLength={2000} /></fieldset><Button type="submit" icon="check" disabled={paused || operation.blocked || changed || !reason.trim()}>Record completion feedback</Button></form>
    {changed && !operation.blocked && <><p>This submission changed. Review the current formal evidence before using a fresh review base.</p><Button icon="refresh" variant="outline" onClick={readCurrent}>Review current formal evidence</Button></>}{latest && <section className="fleet-resource-section" aria-label="Current completion review evidence"><p>Current formal version {latest.final?.revision ?? 'none'} · State revision {latest.revision}</p><EvidenceList eventID={event.id} items={latest.final?.evidence ?? []} /><Label className="check"><Checkbox checked={reviewed} onChange={event => setReviewed(event.target.checked)} />I reviewed the current formal evidence</Label><Button icon="check" disabled={!reviewed || !latest.can_review || !latest.final} onClick={useCurrent}>Use latest review state</Button></section>}
    <ResourceOperationResult hideCurrentChoice operation={operation} read={readCurrent} /><ConfirmationDialog printExcluded open={!!review} onOpenChange={open => { if (!open && !operation.pending) setReview(undefined) }} title="Review completion feedback" description="This records a reasoned organiser decision for the exact named formal submission. It is not mentor scoring or final judging." confirmLabel="Confirm completion feedback" pending={operation.pending} disabled={operation.blocked || paused || changed} onConfirm={() => void save()}><p>Formal version {review?.formal} · State revision {review?.revision} · {words(review?.status ?? '')}</p><p>{review?.reason}</p></ConfirmationDialog>
  </section>
}

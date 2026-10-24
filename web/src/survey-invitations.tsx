import { useCommunicationCommands, CommunicationCommandHistory } from './communication-tests'
import { usePlanningDepartureBarrier } from './planning-operations'
import { CommunicationStep } from './communication-layout'
import { CollectionRecords } from './components/patterns/collection'
import { StatusBadge } from './components/ui/feedback'
import { usePrivateRequestScope } from './private-request-scope'
import { communicationRead } from './communication-private'
import { Input, Select, Textarea, Label } from './components/ui/fields'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { api, privateKey, unwrap, type EventRecord } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { localDate } from './event-time'
import { CampaignWorkspace } from './campaign-composer'
import { nativeDenied, unknownOutcome, useActivePage, useSurveyLeaveWarning, type SurveyConfiguration } from './survey-context'

type Command = components['schemas']['AutomationCommand']
type Intent = { command: Command; eventRevision: number; previousReview?: number }
type InvitationProps = { event: EventRecord; configuration: SurveyConfiguration; writable: boolean; onDirty: (value: boolean) => void; deny: (error: unknown) => void }
export function SurveyInvitations(props: InvitationProps) {
  const identity = useIdentity()
  return <InvitationWorkspace key={`${identity.person.id}:${identity.draft_scope}:${props.event.id}:${props.configuration.id}:${props.configuration.version}:${props.writable}`} {...props} />
}
function InvitationWorkspace({ event, configuration, writable, onDirty, deny }: InvitationProps) {
  const identity = useIdentity(), cache = useQueryClient(), active = useActivePage()
  const scope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${event.id}:${configuration.id}:${configuration.version}:invitation`), writing = useRef(false)
  const [compose, setCompose] = useState(false), [composeDirty, setComposeDirty] = useState(false), [campaignId, setCampaignId] = useState(''), [bindingId] = useState(() => crypto.randomUUID()), [lateness, setLateness] = useState(60), [reason, setReason] = useState(''), [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState(''), [uncertain, setUncertain] = useState<Intent>()
  const history = useCommunicationCommands(`${identity.person.id}:${identity.draft_scope}:${event.id}:${configuration.id}:invitation`), [currentRead, setCurrentRead] = useState(false)
  usePlanningDepartureBarrier(pending)
  const key = useMemo(() => [...privateKey(identity.person.id, event.id, 'survey-invitations'), identity.draft_scope, configuration.id], [identity.person.id, identity.draft_scope, event.id, configuration.id])
  const automation = useInfiniteQuery({ queryKey: [...key, 'automation'], gcTime: 0, retry: false, refetchOnWindowFocus: 'always', refetchInterval: 15_000, initialPageParam: {} as { bindings_after?: string; occurrences_after?: string }, queryFn: ({ pageParam }) => communicationRead(scope, async signal => unwrap(await api.GET('/api/v1/events/{event_id}/communications/automation', { signal, params: { path: { event_id: event.id }, query: { ...pageParam, limit: 50 } } }))),
    getNextPageParam: (last, _all, previous) => last.next_binding_cursor || last.next_occurrence_cursor ? { bindings_after: last.next_binding_cursor ?? last.bindings.at(-1)?.definition.id ?? previous.bindings_after, occurrences_after: last.next_occurrence_cursor ?? last.occurrences.at(-1)?.id ?? previous.occurrences_after } : undefined })
  const campaigns = useQuery({ queryKey: [...key, 'campaigns'], gcTime: 0, retry: false, refetchOnWindowFocus: 'always', refetchInterval: 15_000, queryFn: () => communicationRead(scope, async signal => unwrap(await api.GET('/api/v1/events/{event_id}/communications/campaigns', { signal, params: { path: { event_id: event.id } } }))) })
  useEffect(() => () => { cache.removeQueries({ queryKey: key }) }, [cache, key])
  useEffect(() => { if (nativeDenied(automation.error || campaigns.error)) deny(automation.error || campaigns.error) }, [automation.error, campaigns.error, deny])
  const dirty = composeDirty || !!uncertain || history.unconfirmed || !!reason.trim()
  useEffect(() => { onDirty(dirty); return () => onDirty(false) }, [dirty, onDirty])
  useSurveyLeaveWarning(dirty)
  const page = !automation.error ? automation.data?.pages[0] : undefined, allBindings = automation.data?.pages.flatMap(p => p.bindings) ?? [], allOccurrences = automation.data?.pages.flatMap(p => p.occurrences) ?? []
  const bindings = allBindings.filter(binding => binding.definition.trigger === 'survey.open' && binding.definition.source_id === configuration.id && binding.definition.source_version === configuration.version)
  const occurrences = allOccurrences.filter(occurrence => occurrence.source_kind === 'survey.open' && occurrence.source_id === configuration.id && occurrence.source_revision === configuration.version)
  async function refresh() { const [a, c] = await Promise.all([automation.refetch(), campaigns.refetch()]); if (a.error) throw a.error; if (c.error) throw c.error; return a.data?.pages.flatMap(p => p.bindings) ?? [] }
  async function act(command: Command, previousReview?: number) {
    if (!page || writing.current || uncertain || history.unconfirmed || !writable) return
    const ticket = scope.capture(); if (!ticket.current()) return
    writing.current = true
    const intent = { command, eventRevision: page.revision, previousReview }
    const operation = history.begin({ operation: `Invitation ${command.action}`, revision: intent.eventRevision, command: { intentID: command.definition?.id ?? command.binding_id ?? command.occurrence_id, action: command.action } })
    setPending(true); setFailure(undefined); setNotice('')
    try {
      const value = unwrap(await api.POST('/api/v1/events/{event_id}/communications/automation', { params: { path: { event_id: event.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${intent.eventRevision}"` } }, body: command }))
      history.complete(operation)
      if (!active.current || !ticket.current()) return
      setUncertain(undefined); setReason(''); setNotice(`Native invitation operation ${command.action.replaceAll('_', ' ')} recorded. This is not a provider receipt.`)
      cache.setQueryData([...key, 'automation'], { pages: [value.page], pageParams: [{}] }); await refresh().catch(error => { if (nativeDenied(error)) deny(error); else setFailure(error) })
    } catch (error) {
      if (unknownOutcome(error)) history.uncertain(operation); else history.complete(operation)
      if (!active.current || !ticket.current()) return
      if (nativeDenied(error)) { deny(error); return }
      setFailure(error); if (unknownOutcome(error)) { setUncertain(intent); setCurrentRead(false) } else { setUncertain(undefined); try { await refresh() } catch (currentError) { if (nativeDenied(currentError)) deny(currentError) } }
    } finally { writing.current = false; if (active.current && ticket.current()) setPending(false) }
  }
  async function reconcile() {
    setPending(true); setFailure(undefined)
    try {
      await refresh(); if (!active.current || (!uncertain && !history.unconfirmed)) return
      setCurrentRead(true)
      setNotice('Current native bindings and occurrences were refreshed. They are current facts, not the original command acknowledgement. The original outcome remains unknown; no additional operation was sent.')
    } catch (error) { if (nativeDenied(error)) deny(error); else setFailure(error) } finally { if (active.current) setPending(false) }
  }
  const approved = campaigns.data?.campaigns.filter(item => item.status === 'approved' && !!item.campaign.approval && !item.campaign.cancelled) ?? [], selected = approved.find(item => item.campaign.definition.id === campaignId)
  const blocked = pending || !!uncertain || history.unconfirmed || !writable || !reason.trim() || !page
  function command(action: Command['action'], binding?: components['schemas']['AutomationBinding'], occurrence?: components['schemas']['AutomationOccurrence']): Command { return { action, reason, binding_id: binding?.definition.id, version: binding?.version ?? 0, occurrence_id: occurrence?.id } }
  return <section className="fleet-communications"><h2>Approved survey invitations</h2><CommunicationCommandHistory attempts={history.attempts} /><p>Survey publication does not send a message. Review an approved campaign, then approve a binding for this exact survey version. Eligibility is checked again after answering, close, withdrawal or archive.</p>
    {!configuration.published_at && <p role="status">Publish the fixed survey invitations before creating its native communication binding.</p>}
    <Button variant="outline" onClick={() => { if (!composeDirty || window.confirm('Leave unsaved campaign text? It is kept only in the campaign editor and will be cleared.')) { setCompose(!compose); setComposeDirty(false) } }}>{compose ? 'Close invitation composer' : 'Compose and approve invitation message'}</Button>
    {compose && <CampaignWorkspace key={`${event.id}:${writable}`} event={event} onDirty={setComposeDirty} />}
    {automation.error || campaigns.error ? <Failure error={automation.error || campaigns.error} retry={() => void refresh().catch(error => { if (nativeDenied(error)) deny(error); else setFailure(error) })} /> : !page || campaigns.isPending ? <Loading /> : <>
      {page.paused && <p className="notice">Event communication automation is paused. Approval does not resume it or claim delivery.</p>}
      <CommunicationStep title="Native survey-opening binding" icon="calendar" description="Bind an approved campaign version to this survey opening, with an explicit lateness limit and recorded reason."><fieldset disabled={pending || !!uncertain || history.unconfirmed || !writable}><legend>Reviewed campaign and schedule</legend><Label htmlFor="survey-invitation-campaign">Approved invitation campaign and exact version</Label><Select id="survey-invitation-campaign" value={campaignId} onChange={e => setCampaignId(e.target.value)}><option value="">Choose a currently approved campaign</option>{approved.map(item => <option key={item.campaign.definition.id} value={item.campaign.definition.id}>{item.campaign.definition.name} · version {item.campaign.version}</option>)}</Select>
        <Label htmlFor="survey-invitation-lateness">Explicit maximum lateness (seconds)</Label><Input id="survey-invitation-lateness" type="number" min={1} max={86400} value={lateness} onChange={e => setLateness(e.target.valueAsNumber)} /><Label htmlFor="survey-invitation-reason">Reason for this invitation decision</Label><Textarea id="survey-invitation-reason" maxLength={2000} value={reason} onChange={e => setReason(e.target.value)} />
      </fieldset><p>The opening may already have occurred. Late or changed work requires native occurrence review; no timer or generic retry bypasses the source/provider guards. Times remain {event.configuration.timezone}.</p>
      <Button icon="save" disabled={blocked || !configuration.published_at || configuration.cancelled || !selected || !Number.isInteger(lateness) || lateness < 1 || lateness > 86400 || allBindings.some(b => b.definition.id === bindingId)} onClick={() => { if (selected) void act({ ...command('save'), definition: { id: bindingId, campaign_id: selected.campaign.definition.id, campaign_version: selected.campaign.version, trigger: 'survey.open', source_id: configuration.id, source_version: configuration.version, maximum_lateness_seconds: lateness, retain_after_archive: false, retain_until: null } }) }}>Save exact survey invitation binding</Button>
      </CommunicationStep><CollectionRecords label="Survey invitation bindings" rows={bindings} rowKey={binding => binding.definition.id} rowLabel={binding => `Binding version ${binding.version}`} columns={[{ id: 'campaign', label: 'Campaign version', cell: binding => binding.definition.campaign_version }, { id: 'state', label: 'State', essential: true, cell: binding => <StatusBadge>{binding.cancelled ? 'Cancelled' : binding.approved_at ? 'Approved' : 'Not approved'}</StatusBadge> }]} actions={binding => <div className="fleet-pattern-actions"><Button icon="shield" variant="outline" disabled={blocked || binding.cancelled || !!binding.approved_at} onClick={() => void act(command('approve', binding))}>Approve exact invitation binding</Button><Button icon="remove" variant="ghost" disabled={blocked || binding.cancelled} onClick={() => void act(command('cancel', binding))}>Cancel invitation binding</Button></div>} />
      <h3>Native invitation occurrences</h3><CollectionRecords label="Native invitation occurrences" rows={occurrences} rowKey={occurrence => occurrence.id} rowLabel={() => 'Survey opening'} columns={[{ id: 'state', label: 'State', essential: true, cell: occurrence => <StatusBadge tone={['missed','blocked','partially_queued'].includes(occurrence.state) ? 'warning' : 'neutral'}>{occurrence.state.replaceAll('_', ' ')}</StatusBadge> }, { id: 'due', label: 'Due', cell: occurrence => localDate(occurrence.due_at, event.configuration.timezone) }, { id: 'review', label: 'Review revision', cell: occurrence => occurrence.review_revision }, { id: 'evidence', label: 'Native channel evidence', cell: occurrence => <><p>{occurrence.reason || 'No failure reason'}</p>{Object.entries(occurrence.channel_reports).map(([channel, report]) => <p key={channel}>{channel}: {report.queued} newly queued intents, {report.local_persisted} local inbox records, {report.unavailable} unavailable recipients. Queued/provider acceptance is not human receipt.</p>)}</> }]} actions={occurrence => <div className="fleet-pattern-actions"><Button icon="shield" variant="outline" disabled={blocked || occurrence.state === 'cancelled' || !['missed', 'blocked', 'partially_queued'].includes(occurrence.state)} onClick={() => void act(command('review_occurrence', undefined, occurrence), occurrence.review_revision)}>Review this invitation occurrence</Button><Button icon="remove" variant="ghost" disabled={blocked || occurrence.state === 'cancelled'} onClick={() => void act(command('cancel_occurrence', undefined, occurrence))}>Cancel this invitation occurrence</Button></div>} />{automation.hasNextPage && <Button variant="outline" disabled={automation.isFetchingNextPage} onClick={() => void automation.fetchNextPage()}>More invitation history</Button>}
      <p><Link to="/events/$eventId/communications/deliveries" params={{ eventId: event.id }}>Review actual provider history and uncertain delivery outcomes</Link></p>
    </>}
    <p role="status" aria-label="Survey invitation status">{notice || (uncertain || history.unconfirmed ? 'The operation outcome is unknown. Check the native binding/occurrence before another write.' : '')}</p>{!!failure && <Failure error={failure} />}{(uncertain || history.unconfirmed) && <div className="actions"><Button variant="outline" disabled={pending} onClick={() => void reconcile()}>Read current invitation facts</Button><Button variant="outline" disabled={pending || history.pending || !currentRead} onClick={() => { if (window.confirm('The original outcome remains unconfirmed. Use the current facts as a new base for a different explicitly reviewed command? The original command will not be replayed.')) { history.selectNewBase(); setUncertain(undefined); setReason(''); setCurrentRead(false) } }}>Use current invitation facts as a new base</Button></div>}
  </section>
}

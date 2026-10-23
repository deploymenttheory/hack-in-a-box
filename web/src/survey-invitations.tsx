import { useEffect, useMemo, useState } from 'react'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { api, privateKey, unwrap, type EventRecord } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { localDate } from './event-time'
import { CampaignWorkspace } from './campaign-composer'
import { same, nativeDenied, unknownOutcome, useActivePage, useSurveyLeaveWarning, type SurveyConfiguration } from './survey-context'

type Command = components['schemas']['AutomationCommand']
type Page = components['schemas']['AutomationPage']
type Intent = { command: Command; eventRevision: number; previousReview?: number }
export function SurveyInvitations({ event, configuration, writable, onDirty, deny }: { event: EventRecord; configuration: SurveyConfiguration; writable: boolean; onDirty: (value: boolean) => void; deny: (error: unknown) => void }) {
  const identity = useIdentity(), cache = useQueryClient(), active = useActivePage()
  const [compose, setCompose] = useState(false), [composeDirty, setComposeDirty] = useState(false), [campaignId, setCampaignId] = useState(''), [bindingId] = useState(() => crypto.randomUUID()), [lateness, setLateness] = useState(60), [reason, setReason] = useState(''), [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState(''), [uncertain, setUncertain] = useState<Intent>()
  const key = useMemo(() => [...privateKey(identity.person.id, event.id, 'survey-invitations'), identity.draft_scope, configuration.id], [identity.person.id, identity.draft_scope, event.id, configuration.id])
  const automation = useInfiniteQuery({ queryKey: [...key, 'automation'], gcTime: 0, retry: false, refetchOnWindowFocus: 'always', refetchInterval: 15_000, initialPageParam: {} as { bindings_after?: string; occurrences_after?: string }, queryFn: async ({ pageParam }) => unwrap(await api.GET('/api/v1/events/{event_id}/communications/automation', { params: { path: { event_id: event.id }, query: { ...pageParam, limit: 50 } } })),
    getNextPageParam: (last, _all, previous) => last.next_binding_cursor || last.next_occurrence_cursor ? { bindings_after: last.next_binding_cursor ?? last.bindings.at(-1)?.definition.id ?? previous.bindings_after, occurrences_after: last.next_occurrence_cursor ?? last.occurrences.at(-1)?.id ?? previous.occurrences_after } : undefined })
  const campaigns = useQuery({ queryKey: [...key, 'campaigns'], gcTime: 0, retry: false, refetchOnWindowFocus: 'always', refetchInterval: 15_000, queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/communications/campaigns', { params: { path: { event_id: event.id } } })) })
  useEffect(() => () => { cache.removeQueries({ queryKey: key }) }, [cache, key])
  useEffect(() => { if (nativeDenied(automation.error || campaigns.error)) deny(automation.error || campaigns.error) }, [automation.error, campaigns.error, deny])
  const dirty = composeDirty || !!uncertain || !!reason.trim()
  useEffect(() => { onDirty(dirty); return () => onDirty(false) }, [dirty, onDirty])
  useSurveyLeaveWarning(dirty)
  const page = !automation.error ? automation.data?.pages[0] : undefined, allBindings = automation.data?.pages.flatMap(p => p.bindings) ?? [], allOccurrences = automation.data?.pages.flatMap(p => p.occurrences) ?? []
  const bindings = allBindings.filter(binding => binding.definition.trigger === 'survey.open' && binding.definition.source_id === configuration.id && binding.definition.source_version === configuration.version)
  const occurrences = allOccurrences.filter(occurrence => occurrence.source_kind === 'survey.open' && occurrence.source_id === configuration.id && occurrence.source_revision === configuration.version)
  async function refresh() { const [a, c] = await Promise.all([automation.refetch(), campaigns.refetch()]); if (a.error) throw a.error; if (c.error) throw c.error; return a.data?.pages.flatMap(p => p.bindings) ?? [] }
  async function act(command: Command, previousReview?: number) {
    if (!page) return
    const intent = { command, eventRevision: page.revision, previousReview }
    setPending(true); setFailure(undefined); setNotice('')
    try {
      const value = unwrap(await api.POST('/api/v1/events/{event_id}/communications/automation', { params: { path: { event_id: event.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${intent.eventRevision}"` } }, body: command }))
      if (!active.current) return
      setUncertain(undefined); setReason(''); setNotice(`Native invitation operation ${command.action.replaceAll('_', ' ')} recorded. This is not a provider receipt.`)
      cache.setQueryData([...key, 'automation'], { pages: [value.page], pageParams: [{}] }); await refresh().catch(error => { if (nativeDenied(error)) deny(error); else setFailure(error) })
    } catch (error) {
      if (!active.current) return
      if (nativeDenied(error)) { deny(error); return }
      setFailure(error); if (unknownOutcome(error)) setUncertain(intent); else { setUncertain(undefined); try { await refresh() } catch (currentError) { if (nativeDenied(currentError)) deny(currentError) } }
    } finally { if (active.current) setPending(false) }
  }
  async function reconcile() {
    setPending(true); setFailure(undefined)
    try {
      await refresh(); if (!active.current || !uncertain) return
      const fresh = cache.getQueryData<{ pages: Page[] }>([...key, 'automation'])?.pages
      const command = uncertain.command, binding = fresh?.flatMap(p => p.bindings).find(b => b.definition.id === (command.definition?.id ?? command.binding_id)), occurrence = fresh?.flatMap(p => p.occurrences).find(o => o.id === command.occurrence_id)
      const confirmed = command.action === 'save' ? !!binding && binding.version === (command.version ?? 0) + 1 && same(binding.definition, command.definition) : command.action === 'approve' ? !!binding && binding.version === (command.version ?? 0) + 1 && !!binding.approved_at : command.action === 'cancel' ? !!binding?.cancelled : command.action === 'cancel_occurrence' ? occurrence?.state === 'cancelled' : command.action === 'review_occurrence' && occurrence?.review_revision === (uncertain.previousReview ?? 0) + 1
      if (confirmed) { setUncertain(undefined); setReason(''); setNotice('The current native binding or occurrence confirms this effect. No second operation was sent.') }
      else setNotice('This effect is not confirmed. Keep the original command identity or deliberately discard it; no new revision will be retried automatically.')
    } catch (error) { if (nativeDenied(error)) deny(error); else setFailure(error) } finally { if (active.current) setPending(false) }
  }
  const approved = campaigns.data?.campaigns.filter(item => item.status === 'approved' && !!item.campaign.approval && !item.campaign.cancelled) ?? [], selected = approved.find(item => item.campaign.definition.id === campaignId)
  const blocked = pending || !!uncertain || !writable || !reason.trim() || !page
  function command(action: Command['action'], binding?: components['schemas']['AutomationBinding'], occurrence?: components['schemas']['AutomationOccurrence']): Command { return { action, reason, binding_id: binding?.definition.id, version: binding?.version ?? 0, occurrence_id: occurrence?.id } }
  return <section><h2>Approved survey invitations</h2><p>Survey publication does not send a message. Save and approve the real message, audience and channels, then separately approve a native invitation binding for this exact survey version. Current survey eligibility stops sends after answering, close, withdrawal or archive.</p>
    {!configuration.published_at && <p role="status">Publish the fixed survey invitations before creating its native communication binding.</p>}
    <Button variant="outline" onClick={() => { if (!composeDirty || window.confirm('Leave unsaved campaign text? It is kept only in the campaign editor and will be cleared.')) { setCompose(!compose); setComposeDirty(false) } }}>{compose ? 'Close invitation composer' : 'Compose and approve invitation message'}</Button>
    {compose && <CampaignWorkspace key={`${event.id}:${writable}`} event={event} onDirty={setComposeDirty} />}
    {automation.error || campaigns.error ? <Failure error={automation.error || campaigns.error} retry={() => void refresh().catch(error => { if (nativeDenied(error)) deny(error); else setFailure(error) })} /> : !page || campaigns.isPending ? <Loading /> : <>
      {page.paused && <p className="notice">Event communication automation is paused. Approval does not resume it or claim delivery.</p>}
      <fieldset disabled={pending || !!uncertain || !writable}><legend>Native survey-opening binding</legend><label htmlFor="survey-invitation-campaign">Approved invitation campaign and exact version</label><select id="survey-invitation-campaign" value={campaignId} onChange={e => setCampaignId(e.target.value)}><option value="">Choose a currently approved campaign</option>{approved.map(item => <option key={item.campaign.definition.id} value={item.campaign.definition.id}>{item.campaign.definition.name} · version {item.campaign.version}</option>)}</select>
        <label htmlFor="survey-invitation-lateness">Explicit maximum lateness (seconds)</label><input id="survey-invitation-lateness" type="number" min={1} max={86400} value={lateness} onChange={e => setLateness(e.target.valueAsNumber)} /><label htmlFor="survey-invitation-reason">Reason for this invitation decision</label><textarea id="survey-invitation-reason" maxLength={2000} value={reason} onChange={e => setReason(e.target.value)} />
      </fieldset><p>The opening may already have occurred. Late or changed work requires native occurrence review; no timer or generic retry bypasses the source/provider guards. Times remain {event.configuration.timezone}.</p>
      <Button disabled={blocked || !configuration.published_at || configuration.cancelled || !selected || !Number.isInteger(lateness) || lateness < 1 || lateness > 86400 || allBindings.some(b => b.definition.id === bindingId)} onClick={() => { if (selected) void act({ ...command('save'), definition: { id: bindingId, campaign_id: selected.campaign.definition.id, campaign_version: selected.campaign.version, trigger: 'survey.open', source_id: configuration.id, source_version: configuration.version, maximum_lateness_seconds: lateness, retain_after_archive: false, retain_until: null } }) }}>Save exact survey invitation binding</Button>
      <ul>{bindings.map(binding => <li key={binding.definition.id}><p>Binding version {binding.version} · Campaign version {binding.definition.campaign_version} · {binding.cancelled ? 'Cancelled' : binding.approved_at ? 'Approved' : 'Not approved'}</p><Button variant="outline" disabled={blocked || binding.cancelled || !!binding.approved_at} onClick={() => void act(command('approve', binding))}>Approve exact invitation binding</Button><Button variant="outline" disabled={blocked || binding.cancelled} onClick={() => void act(command('cancel', binding))}>Cancel invitation binding</Button></li>)}</ul>
      <h3>Native invitation occurrences</h3><ul>{occurrences.map(occurrence => <li key={occurrence.id}><p>{occurrence.state.replaceAll('_', ' ')} · {occurrence.reason || 'No failure reason'} · due {localDate(occurrence.due_at, event.configuration.timezone)} · review revision {occurrence.review_revision}</p>{Object.entries(occurrence.channel_reports).map(([channel, report]) => <p key={channel}>{channel}: {report.queued} newly queued intents, {report.local_persisted} local inbox records, {report.unavailable} unavailable recipients. Queued/provider acceptance is not human receipt.</p>)}<Button variant="outline" disabled={blocked || occurrence.state === 'cancelled' || !['missed', 'blocked', 'partially_queued'].includes(occurrence.state)} onClick={() => void act(command('review_occurrence', undefined, occurrence), occurrence.review_revision)}>Review this invitation occurrence</Button><Button variant="outline" disabled={blocked || occurrence.state === 'cancelled'} onClick={() => void act(command('cancel_occurrence', undefined, occurrence))}>Cancel this invitation occurrence</Button></li>)}</ul>{automation.hasNextPage && <Button variant="outline" disabled={automation.isFetchingNextPage} onClick={() => void automation.fetchNextPage()}>More invitation history</Button>}
      <p><Link to="/events/$eventId/communications/deliveries" params={{ eventId: event.id }}>Review actual provider history and uncertain delivery outcomes</Link></p>
    </>}
    <p role="status" aria-label="Survey invitation status">{notice || (uncertain ? 'The operation outcome is unknown. Check the native binding/occurrence before another write.' : '')}</p>{!!failure && <Failure error={failure} />}{uncertain && <div className="actions"><Button variant="outline" disabled={pending} onClick={() => void reconcile()}>Check exact invitation operation</Button><Button variant="outline" disabled={pending} onClick={() => { if (window.confirm('Discard this uncertain page intent? The native effect may already exist; no additional operation will be sent.')) { setUncertain(undefined); setReason(''); void refresh() } }}>Discard uncertain invitation intent</Button></div>}
  </section>
}

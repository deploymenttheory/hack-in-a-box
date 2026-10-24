import { useCommunicationTests, useCommunicationCommands, CommunicationCommandHistory } from './communication-tests'
import { Tabs, TabList, Tab, TabPanel } from './components/ui/tabs'
import { usePlanningDepartureBarrier } from './planning-operations'
import { ConfirmationDialog } from './components/ui/dialog'
import { useCallback, useMemo } from 'react'
import { usePrivateRequestScope, type PrivateRequestScope } from './private-request-scope'
import { communicationRead } from './communication-private'
import { CollectionRecords } from './components/patterns/collection'
import { StatusBadge } from './components/ui/feedback'
import { Icon } from './components/ui/icon'
import { Input, Label } from './components/ui/fields'
import { CommunicationStep } from './communication-layout'
import { Link } from '@tanstack/react-router'
import { SMTPTestDelivery } from './smtp-test-delivery'
import { useEffect, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, APIError, privateKey, unwrap, type EventRecord } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { MessageEditor, AudienceEditor } from './campaign-source-editors'
import { CampaignSettings, ProviderSetup } from './campaign-settings'
import { CampaignPreview } from './campaign-preview'
import { TeamsWorkflowSetup } from './teams-workflow-setup'
import { blankAudience, blankCampaign, blankTemplate, blockerText, channelNames, same, type CampaignDetail, type CampaignPage, type Channel, type ComposerPage } from './campaign-model'

type CampaignCommand = components['schemas']['CampaignCommand']
type SelfTestPreview = components['schemas']['CampaignSelfTestPreview']
type SourceCommand = components['schemas']['CommunicationCommand']
export function CampaignWorkspace({ event, onDirty }: { event: EventRecord; onDirty: (value: boolean) => void }) {
  const identity = useIdentity()
  return <CampaignScope key={`${identity.person.id}:${identity.draft_scope}:${event.id}`} event={event} onDirty={onDirty} />
}
function CampaignScope({ event, onDirty }: { event: EventRecord; onDirty: (value: boolean) => void }) {
  const identity = useIdentity(), cache = useQueryClient(), [denied, setDenied] = useState(false)
  const key = useMemo(() => [...privateKey(identity.person.id, event.id, 'communications-workspace'), identity.draft_scope], [identity.person.id, identity.draft_scope, event.id])
  const scope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${event.id}:communications`)
  const deny = useCallback(() => { scope.invalidate(); setDenied(true) }, [scope])
  useEffect(() => {
    const events = ['session-ending', 'session-expired', 'mentor-draft-invalidated']
    for (const name of events) window.addEventListener(name, deny)
    return () => { for (const name of events) window.removeEventListener(name, deny) }
  }, [deny])
  function read<T>(load: (signal: AbortSignal) => Promise<T>) {
    return communicationRead(scope, async signal => { try { return await load(signal) } catch (error) { if (scope.capture().current() && error instanceof APIError && [401,403,404].includes(error.status)) deny(); throw error } })
  }
  const source = useQuery({ queryKey: [...key, 'source'], gcTime: 0, queryFn: () => read(async signal => unwrap(await api.GET('/api/v1/events/{event_id}/communications', { signal, params: { path: { event_id: event.id } } }))), retry: false, refetchOnWindowFocus: 'always' })
  const campaigns = useQuery({ queryKey: [...key, 'campaigns'], gcTime: 0, queryFn: () => read(async signal => unwrap(await api.GET('/api/v1/events/{event_id}/communications/campaigns', { signal, params: { path: { event_id: event.id } } }))), retry: false, refetchInterval: 15_000, refetchOnWindowFocus: 'always' })
  const teams = useQuery({ queryKey: [...key, 'teams'], gcTime: 0, queryFn: () => read(async signal => unwrap(await api.GET('/api/v1/events/{event_id}/teams', { signal, params: { path: { event_id: event.id } } }))), retry: false })
  const error = source.error || campaigns.error || teams.error
  const inaccessible = denied || (error instanceof APIError && [401, 403, 404].includes(error.status))
  useEffect(() => { if (inaccessible) { scope.invalidate(); cache.removeQueries({ queryKey: key }) } }, [inaccessible, cache, scope, key])
  if (inaccessible) return <p role="alert">Campaign content is no longer available to your current access. Private edits and retained previews have been cleared.</p>
  if ((!source.data || !campaigns.data || !teams.data) && !error) return <Loading />
  if (!source.data || !campaigns.data || !teams.data) return <Failure error={error} retry={() => { void source.refetch(); void campaigns.refetch(); void teams.refetch() }} />
  return <>{!!error && <Failure error={error} retry={() => { void source.refetch(); void campaigns.refetch(); void teams.refetch() }} />}<Composer key={`${campaigns.data?.can_manage}:${event.can_edit}`} event={event} initialSource={source.data!} initialPage={campaigns.data!} teams={teams.data!.teams.filter(t => t.state === 'published')} onDirty={onDirty} scope={scope} deny={deny} readFresh={!error} /></>
}

function Composer({ event, initialSource, initialPage, teams, onDirty, scope: parentScope, deny, readFresh }: { readFresh: boolean; scope: PrivateRequestScope; deny: () => void; event: EventRecord; initialSource: ComposerPage; initialPage: CampaignPage; teams: { id: string; name: string }[]; onDirty: (value: boolean) => void }) {
  const scope = usePrivateRequestScope('campaign-editor', parentScope), tests = useCommunicationTests(event.id)
  const identity = useIdentity(), cache = useQueryClient(), [editing, setEditing] = useState(false), [step, setStep] = useState('schedule'), [source, setSource] = useState(initialSource), [page, setPage] = useState(initialPage), [revision, setRevision] = useState(Math.max(initialSource.revision, initialPage.revision)), [message, setMessage] = useState(initialSource.templates[0] ?? blankTemplate), [savedMessage, setSavedMessage] = useState(message), [audience, setAudience] = useState(initialSource.audiences[0] ?? blankAudience), [savedAudience, setSavedAudience] = useState(audience), [definition, setDefinition] = useState(() => blankCampaign(initialSource)), [savedDefinition, setSavedDefinition] = useState(definition), [detail, setDetail] = useState<CampaignDetail>(), [reason, setReason] = useState(''), [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState(''), [conflict, setConflict] = useState(false), [latest, setLatest] = useState<{ source: ComposerPage; page: CampaignPage; detail?: CampaignDetail }>(), [reviewed, setReviewed] = useState(false)
  const dirtySources = !same(message, savedMessage) || !same(audience, savedAudience), dirtyDefinition = !same(definition, savedDefinition), dirty = dirtySources || dirtyDefinition
  const frame = useRef<HTMLElement>(null), entryFocus = useRef(false), opener = useRef<string | undefined>(undefined), destinationFocus = useRef<{ selector: string; current: () => boolean } | undefined>(undefined)
  const [reviewTest, setReviewTest] = useState<Channel>(), [selfReview, setSelfReview] = useState<SelfTestPreview>(), [reviewPending, setReviewPending] = useState(false), [reviewFailure, setReviewFailure] = useState<unknown>(), testTrigger = useRef<HTMLButtonElement>(null), reviewSequence = useRef(0)
  const writing = useRef(false), [unknownNotice, setUnknownCommand] = useState<string>()
  const history = useCommunicationCommands(`${identity.person.id}:${identity.draft_scope}:${event.id}:communications`)
  const unknownCommand = unknownNotice || (history.unconfirmed ? 'An earlier campaign command in this event session remains unconfirmed. Current facts cannot prove its original acknowledgement; no command will be replayed.' : undefined)
  usePlanningDepartureBarrier(pending)
  useEffect(() => {
    if (editing && !pending && entryFocus.current) { const heading = frame.current?.querySelector<HTMLElement>('[data-campaign-heading]'); heading?.focus(); entryFocus.current = false }
    if (!editing && opener.current) { [...(frame.current?.querySelectorAll<HTMLElement>('[data-record-focus]') ?? [])].find(control => control.dataset.recordFocus === opener.current)?.focus(); opener.current = undefined }
  }, [editing, pending])
  useEffect(() => {
    const target = destinationFocus.current
    if (!target || pending) return
    let scroll = 0
    // Let the newly active panel settle before overriding browser scroll anchoring.
    const focus = requestAnimationFrame(() => { if (!target.current()) return; const heading = frame.current?.querySelector<HTMLElement>(target.selector); if (!heading) return; heading.tabIndex = -1; heading.focus(); scroll = requestAnimationFrame(() => { if (target.current()) heading.scrollIntoView({ block: 'start' }); destinationFocus.current = undefined }) })
    return () => { cancelAnimationFrame(focus); cancelAnimationFrame(scroll) }
  }, [step, pending])
  const externalChanged = initialPage.revision > revision
  const disabled = !readFresh || pending || !!unknownCommand || conflict || externalChanged || !page.can_manage || !event.can_edit, actionDisabled = disabled || !reason.trim()
  useEffect(() => { onDirty(dirty || !!unknownCommand || pending); return () => onDirty(false) }, [dirty, unknownCommand, pending, onDirty])
  const path = { event_id: event.id }, header = { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${revision}"` }
  async function current(id?: string) {
    const ticket = scope.capture(); ticket.assert()
    const [freshSource, freshPage, freshDetail] = await Promise.all([api.GET('/api/v1/events/{event_id}/communications', { params: { path } }).then(unwrap), api.GET('/api/v1/events/{event_id}/communications/campaigns', { params: { path } }).then(unwrap), id ? api.GET('/api/v1/events/{event_id}/communications/campaigns/{campaign_id}', { params: { path: { ...path, campaign_id: id } } }).then(unwrap) : Promise.resolve(undefined)])
    ticket.assert(); cache.setQueryData([...privateKey(identity.person.id, event.id, 'communications-workspace'), identity.draft_scope, 'campaigns'], freshPage)
    return { source: freshSource, page: freshPage, detail: freshDetail }
  }
  async function reject(error: unknown) { if (!scope.capture().current()) return; if (error instanceof APIError && [401, 403, 404].includes(error.status)) { deny(); return } setFailure(error); if (error instanceof APIError && error.status === 409) { setConflict(true); setReviewed(false); try { setLatest(await current(detail?.campaign.campaign.definition.id)) } catch (refreshError) { setFailure(refreshError) } } }
  function settleFailure(error: unknown, operation: string) {
    if (!(error instanceof APIError) || error.status >= 500 || error.status === 408) { history.uncertain(operation); if (scope.capture().current()) setUnknownCommand('The previous campaign operation has an unknown outcome. Refresh current facts for review; no operation will be replayed.') }
    else history.complete(operation)
  }
  function openStep(value: string, headingSelector: string) {
    const ticket = scope.capture()
    destinationFocus.current = { selector: headingSelector, current: ticket.current }
    setStep(value)
  }
  async function sourceChange(command: Omit<SourceCommand, 'reason'>) {
    if (writing.current || unknownCommand) return
    const ticket = scope.capture(); if (!ticket.current()) return
    writing.current = true
    const operation = history.begin({ operation: `Campaign ${command.action}`, revision, command: { intentID: crypto.randomUUID(), action: command.action } })
    let acknowledged = false
    setPending(true); setFailure(undefined); setNotice('')
    try {
      const fresh = unwrap(await api.POST('/api/v1/events/{event_id}/communications', { params: { path, header }, body: { ...command, reason } }))
      acknowledged = true; history.complete(operation)
      ticket.assert(); setSource(fresh); setRevision(fresh.revision)
      if (command.action === 'save_template') { const saved = fresh.templates.find(t => t.id === message.id)!; setMessage(saved); setSavedMessage(saved) }
      else { const saved = fresh.audiences.find(a => a.id === audience.id)!; setAudience(saved); setSavedAudience(saved) }
      const latest = await current(detail?.campaign.campaign.definition.id); setPage(latest.page); if (latest.detail) setDetail(latest.detail)
      if (latest.page.revision !== fresh.revision) { setLatest(latest); setConflict(true); setReviewed(false) } else setRevision(fresh.revision); setNotice('Source version saved. Review a new campaign preview before approval.')
    } catch (error) { if (!acknowledged) settleFailure(error, operation); await reject(error) } finally { writing.current = false; setPending(false) }
  }
  function receive(fresh: CampaignDetail, resetDefinition = false) { setDetail(fresh); setRevision(fresh.revision); setPage(page => ({ ...page, revision: fresh.revision, providers: fresh.providers, can_manage: fresh.can_manage, campaigns: page.campaigns.filter(c => c.campaign.definition.id !== fresh.campaign.campaign.definition.id).concat(fresh.campaign) })); if (resetDefinition) { setDefinition(fresh.campaign.campaign.definition); setSavedDefinition(fresh.campaign.campaign.definition) } }
  async function campaignChange(action: CampaignCommand['action'], channel?: Channel, reviewed?: SelfTestPreview) {
    if (action === 'test_self' && channel === 'smtp' && tests.blocked()) return
    if (action === 'test_self' && (!reviewed || reviewed.actor_id !== identity.person.id || reviewed.channel !== channel || reviewed.revision !== revision || reviewed.campaign_version !== detail?.campaign.campaign.version)) return
    if (writing.current || unknownCommand) return
    const ticket = scope.capture(); if (!ticket.current()) return
    writing.current = true
    const operation = history.begin({ operation: `Campaign ${action}`, revision, command: { intentID: crypto.randomUUID(), action } })
    setPending(true); setFailure(undefined); setNotice('')
    try {
      const fresh = unwrap(await api.POST('/api/v1/events/{event_id}/communications/campaigns/{campaign_id}', { params: { path: { ...path, campaign_id: definition.id }, header }, body: { action, version: detail?.campaign.campaign.version ?? 0, reason, ...(action === 'save' ? { definition } : {}), ...(channel ? { channel } : {}), ...(reviewed ? { reviewed_test_fingerprint: reviewed.review_fingerprint } : {}) } }))
      if (action === 'test_self' && channel === 'smtp' && fresh.test_request_id && reviewed) tests.queued({ testID: fresh.test_request_id, campaignID: definition.id, campaignVersion: reviewed.campaign_version, providerRevision: reviewed.provider_revision })
      history.complete(operation)
      ticket.assert(); receive(fresh, action === 'save'); if (action === 'save' || action === 'preview') openStep('review', '#campaign-preview h3')
      setNotice(action === 'test_self' ? 'Test queued for the requesting organiser only. Provider acceptance and delivery are not confirmed.' : action === 'approve' ? 'Campaign definition approved. This is not a delivery receipt.' : action === 'preview' ? 'Retained audience preview generated.' : action === 'cancel' ? 'Campaign cancelled. Obsolete queued self-tests have been cancelled.' : 'Campaign saved as a draft. Approval is required.')
    } catch (error) { settleFailure(error, operation); await reject(error) } finally { writing.current = false; setPending(false) }
  }
  async function reviewSelf(channel: Channel, trigger: HTMLButtonElement) {
    if (actionDisabled || !detail || channel === 'teams' || (channel === 'smtp' && tests.blocked())) return
    const ticket = scope.capture(); if (!ticket.current()) return
    const sequence = ++reviewSequence.current
    testTrigger.current = trigger; setReviewTest(channel); setSelfReview(undefined); setReviewFailure(undefined); setReviewPending(true)
    try {
      const value = unwrap(await api.GET('/api/v1/events/{event_id}/communications/campaigns/{campaign_id}/self-test-preview', { signal: ticket.signal, cache: 'no-store', params: { path: { ...path, campaign_id: definition.id }, query: { channel } } }))
      ticket.assert(); if (sequence !== reviewSequence.current) return
      if (value.actor_id !== identity.person.id || value.channel !== channel || value.revision !== revision || value.campaign_version !== detail.campaign.campaign.version) throw new Error('Review changed; reload current campaign facts')
      setSelfReview(value)
    } catch (error) { if (ticket.current() && sequence === reviewSequence.current) { setReviewFailure(error); if (error instanceof APIError && [401,403,404].includes(error.status)) deny() } }
    finally { if (ticket.current() && sequence === reviewSequence.current) setReviewPending(false) }
  }
  function resetSources(next: ComposerPage, campaign: typeof definition) {
    const savedTemplate = next.templates.find(t => t.id === campaign.template_id) ?? next.templates[0] ?? blankTemplate()
    const savedSelector = next.audiences.find(a => a.id === campaign.audience_id) ?? next.audiences[0] ?? blankAudience()
    setMessage(savedTemplate); setSavedMessage(savedTemplate); setAudience(savedSelector); setSavedAudience(savedSelector)
  }
  async function choose(id?: string) {
    if (pending || (!id && unknownCommand)) return
    if (dirty && !window.confirm('Leave this campaign with unsaved message, audience or schedule edits?')) return
    opener.current = id ?? 'new-campaign'; entryFocus.current = true
    setEditing(true); setStep(id ? 'review' : source.templates.length && source.audiences.length ? 'schedule' : 'message')
    setFailure(undefined); setNotice(''); setConflict(false); setLatest(undefined)
    if (!id) { const empty = blankCampaign(source); setDefinition(empty); setSavedDefinition(empty); setDetail(undefined); resetSources(source, empty); return }
    setPending(true)
    try { const fresh = await current(id); setSource(fresh.source); setPage(fresh.page); setRevision(fresh.page.revision); if (fresh.detail) { receive(fresh.detail, true); resetSources(fresh.source, fresh.detail.campaign.campaign.definition) } } catch (error) { await reject(error) } finally { setPending(false) }
  }
  async function refresh() {
    setPending(true); setFailure(undefined)
    try { const fresh = await current(detail?.campaign.campaign.definition.id); if (fresh.page.revision !== revision) { setLatest(fresh); setConflict(true); setReviewed(false) } else { setPage(fresh.page); if (fresh.detail) setDetail(fresh.detail) } } catch (error) { await reject(error) } finally { setPending(false) }
  }
  function acceptLatest() {
    if (!latest || !reviewed) return
    setSource(latest.source); setPage(latest.page); setRevision(latest.page.revision)
    const freshMessage = latest.source.templates.find(t => t.id === message.id), freshAudience = latest.source.audiences.find(a => a.id === audience.id)
    if (freshMessage) { setMessage({ ...message, version: freshMessage.version }); setSavedMessage(freshMessage) }
    if (freshAudience) { setAudience({ ...audience, version: freshAudience.version }); setSavedAudience(freshAudience) }
    if (latest.detail) setDetail(latest.detail)
    setConflict(false); setLatest(undefined); setFailure(undefined)
  }
  const versionSaved = !!detail && !dirtySources && !dirtyDefinition, preview = detail?.preview
  const selectedTemplate = source.templates.find(t => t.id === definition.template_id), selectedAudience = source.audiences.find(a => a.id === definition.audience_id)
  const currentPreview = preview && preview.template_version === selectedTemplate?.version && preview.audience_version === selectedAudience?.version
  const approvalBlocked = detail?.campaign.blockers.some(code => !['approval_outdated', 'provider_configuration_changed'].includes(code)) ?? true
  return <section ref={frame} id="communications" className="campaign-composer fleet-communications"><h2>Campaign composer</h2><CommunicationCommandHistory attempts={history.attempts} /><p><Link to="/events/$eventId/communications/deliveries" params={{ eventId: event.id }}>Delivery history and failure review</Link></p><p>Review the content, current eligible audience, channels and schedule before explicitly approving. Approval does not publish teams, challenges or results.</p>
    <section hidden={editing} className="fleet-campaign-collection" aria-label="Saved campaigns"><div className="fleet-pattern-actions"><h3>Campaigns</h3><Button icon="shield" variant="outline" disabled={pending} data-record-focus="channel-setup" onClick={() => { opener.current = 'channel-setup'; entryFocus.current = true; setEditing(true); setStep('channels') }}>Channel setup</Button><Button data-record-focus="new-campaign" icon="add" disabled={pending || !!unknownCommand} onClick={() => void choose()}>New campaign</Button></div><CollectionRecords label="Saved campaigns" rows={page.campaigns} rowKey={row => row.campaign.definition.id} rowLabel={row => row.campaign.definition.name} columns={[{ id: 'status', label: 'State', essential: true, cell: row => <StatusBadge>{row.status.replaceAll('_', ' ')}</StatusBadge> }, { id: 'version', label: 'Version', cell: row => row.campaign.version }, { id: 'channels', label: 'Channels', cell: row => row.campaign.definition.channels.map(channel => channelNames[channel]).join(', ') }]} actions={row => <Button icon="next" variant="ghost" data-record-focus={row.campaign.definition.id} disabled={pending} onClick={() => void choose(row.campaign.definition.id)} aria-label={`Review ${row.campaign.definition.name}`}>Review</Button>} />{!page.campaigns.length && <p>No campaigns saved yet. Choose New campaign to review its message, audience and schedule.</p>}</section>
    {tests.blocked() && <p role="alert">An earlier Email TEST is pending or its submission remains uncertain. A replacement Email TEST is blocked until the original native request has a confirmed submission outcome. Changing campaigns, templates or delivery setup does not clear this barrier.</p>}
    {page.can_manage && tests.items.map(test => <SMTPTestDelivery key={test.testID} eventID={event.id} testID={test.testID} observed={tests.observed} denied={deny} />)}
    {externalChanged && <p role="alert">Current campaign facts have changed. Refresh and review the current message, audience, channel and schedule before another command.</p>}
    {unknownCommand && <div role="alert"><p>{unknownCommand}</p><Button icon="refresh" variant="outline" disabled={pending} onClick={() => void refresh()}>Read current campaign facts</Button><p>Current facts do not prove the acknowledgement of the original operation. Reopen this workspace only after reconciling any TEST in delivery history.</p></div>}
    {conflict && <section role="alert" className="panel"><h3>Review changed campaign facts</h3><p>Your edits are preserved and nothing is retried automatically. Review the latest message, audience and saved schedule before using its revision.</p>{latest && <><p>Latest messages: {latest.source.templates.map(t => `${t.name}: ${t.subject}`).join(' · ')}</p><p>Latest audiences: {latest.source.audiences.map(a => `${a.name} (version ${a.version})`).join(' · ')}</p>{latest.detail && <p>Latest saved campaign: {latest.detail.campaign.campaign.definition.name} · {latest.detail.campaign.status}</p>}<label className="check"><input type="checkbox" checked={reviewed} onChange={e => setReviewed(e.target.checked)} />I reviewed the current message, audience and schedule</label><Button disabled={!reviewed || pending} onClick={acceptLatest}>Use current revision and keep my edits</Button></>}</section>}
    {!page.can_manage && <p role="alert">This event does not permit campaign changes. Your draft is preserved.</p>}
    {editing && <><Button icon="back" variant="ghost" disabled={pending} onClick={() => { if (!dirty || window.confirm('Leave unsaved campaign edits? The event session retains unknown command metadata.')) { setMessage(savedMessage); setAudience(savedAudience); setDefinition(savedDefinition); setReason(''); setEditing(false) } }}>Back to campaigns</Button><h3 data-campaign-heading tabIndex={-1}>{step === 'channels' ? 'Channel setup' : definition.name || 'New campaign'}</h3><p className="fleet-scope-label">{detail ? `Campaign version ${detail.campaign.campaign.version}` : 'Unsaved campaign'} · {dirty ? 'Unsaved edits' : detail?.campaign.status || 'Draft'}</p><Tabs value={step} onValueChange={value => setStep(String(value))}><TabList aria-label="Campaign workflow" activateOnFocus={false} className="fleet-communication-tabs"><Tab value="message" disabled={pending}><Icon name="type" />Message</Tab><Tab value="audience" disabled={pending}><Icon name="team" />Audience</Tab><Tab value="schedule" disabled={pending}><Icon name="calendar" />Schedule</Tab><Tab value="review" disabled={pending}><Icon name="shield" />Review &amp; TEST</Tab><Tab value="channels" disabled={pending}><Icon name="send" />Channels</Tab></TabList>
    <div hidden={step === 'channels'} className="fleet-field"><Label htmlFor="campaign-reason">Reason for campaign changes</Label><Input id="campaign-reason" maxLength={2000} value={reason} onChange={e => setReason(e.target.value)} /></div>
    <TabPanel value="message" keepMounted><MessageEditor source={source} value={message} change={setMessage} disabled={disabled} saveDisabled={actionDisabled} save={() => { if (!actionDisabled) void sourceChange({ action: 'save_template', template: message }) }} choose={id => { const t = source.templates.find(t => t.id === id); if (t && (same(message, savedMessage) || window.confirm('Discard unsaved message changes?'))) { setMessage(t); setSavedMessage(t) } }} create={() => { if (same(message, savedMessage) || window.confirm('Discard unsaved message changes?')) { const t = blankTemplate(); setMessage(t); setSavedMessage(t) } }} />
    </TabPanel><TabPanel value="audience" keepMounted><AudienceEditor source={source} teams={teams} value={audience} change={setAudience} disabled={disabled} saveDisabled={actionDisabled} save={() => { if (!actionDisabled) void sourceChange({ action: 'save_audience', audience }) }} choose={id => { const a = source.audiences.find(a => a.id === id); if (a && (same(audience, savedAudience) || window.confirm('Discard unsaved audience changes?'))) { setAudience(a); setSavedAudience(a) } }} create={() => { if (same(audience, savedAudience) || window.confirm('Discard unsaved audience changes?')) { const a = blankAudience(); setAudience(a); setSavedAudience(a) } }} />
    </TabPanel>{step !== 'channels' && !reason.trim() && <p className="muted">Record a reason before saving, previewing, approving, cancelling or queuing a self-test.</p>}
    <TabPanel value="schedule" keepMounted><CampaignSettings setup={() => openStep('channels', '#campaign-provider-setup h3')} value={definition} change={setDefinition} source={source} page={page} detail={detail} disabled={disabled} saveDisabled={actionDisabled} save={() => void campaignChange('save')} />
    </TabPanel><TabPanel value="review" keepMounted><CampaignPreview detail={detail} template={selectedTemplate} />
    <CommunicationStep id="campaign-approval" title="Campaign approval and tests" icon="shield" description="Approve only after reviewing the current saved message, retained audience and schedule. TEST sends are separate from approval and delivery receipts.">{detail && <p>Saved state: <strong>{detail.campaign.status}</strong></p>}{!versionSaved && <p>Save all edited source versions and the campaign draft before preview or approval.</p>}<div className="actions"><Button icon="user" disabled={actionDisabled || !versionSaved || detail?.campaign.campaign.cancelled} onClick={() => void campaignChange('preview')}>Generate audience preview</Button><Button icon="shield" disabled={actionDisabled || !versionSaved || !currentPreview || !!preview?.blockers.length || approvalBlocked || detail?.campaign.campaign.cancelled} onClick={() => void campaignChange('approve')}>{detail?.campaign.campaign.approval ? 'Explicitly reapprove campaign' : 'Approve campaign'}</Button><Button icon="remove" variant="outline" disabled={actionDisabled || !detail || detail.campaign.campaign.cancelled} onClick={() => void campaignChange('cancel')}>Cancel campaign</Button></div>
    <p>Self-tests target only you, {identity.person.display_name}. They never reuse a selected recipient's personal preview.</p>{definition.channels.map(channel => { const provider = page.providers.find(p => p.channel === channel); return <div key={channel}>{provider?.supports_self_test ? <><Button icon="send" variant="outline" disabled={actionDisabled || (channel === 'smtp' && tests.blocked()) || !versionSaved || !provider.self_test_available || detail?.campaign.campaign.cancelled} onClick={event => void reviewSelf(channel, event.currentTarget)}>Queue {channelNames[channel]} test to myself</Button>{!provider.self_test_available && <p>{blockerText(provider.self_test_reason)}. <Button icon="shield" variant="link" onClick={() => openStep('channels', '#campaign-provider-setup h3')}>Review setup</Button>.</p>}</> : <p>{channelNames[channel]} is a shared broadcast and cannot send a self-test only to you.</p>}</div> })}</CommunicationStep>
    <ConfirmationDialog open={!!reviewTest} onOpenChange={open => { if (!open) { reviewSequence.current++; setReviewTest(undefined); setSelfReview(undefined); setReviewFailure(undefined) } }} title="Review organiser-only TEST" description="Review the exact current personalised output for your own account. Confirmation queues one TEST; it does not confirm delivery." confirmLabel="Confirm organiser-only TEST" pending={pending} disabled={actionDisabled || (reviewTest === 'smtp' && tests.blocked()) || reviewPending || !selfReview} finalFocus={testTrigger} onConfirm={() => { if (reviewTest && selfReview && !writing.current) void campaignChange('test_self', reviewTest, selfReview).finally(() => { if (scope.capture().current()) { setReviewTest(undefined); setSelfReview(undefined) } }) }}>
      {reviewPending ? <Loading /> : reviewFailure ? <Failure error={reviewFailure} /> : selfReview && <><dl><dt>Recipient</dt><dd>{identity.person.display_name} · requesting organiser only</dd><dt>Channel</dt><dd>{channelNames[selfReview.channel]}</dd><dt>Delivery connection</dt><dd>Current reviewed {channelNames[selfReview.channel]} connection</dd><dt>Saved campaign</dt><dd>Version {selfReview.campaign_version}</dd><dt>Subject</dt><dd className="fleet-communication-reading">{selfReview.subject}</dd><dt>Full plain-text message</dt><dd className="fleet-communication-reading">{selfReview.delivery_plain_text}</dd><dt>Reason</dt><dd>{reason}</dd></dl><p>Changes to your account, message, event details or delivery connection require a fresh review before the TEST can be queued. Opening this preview sends nothing.</p>{selfReview.calendar && <div><h4>Reviewed calendar attachment</h4><p>{selfReview.calendar.summary}</p><p>{new Date(selfReview.calendar.start).toLocaleString(undefined, { timeZone: selfReview.calendar.timezone })} – {new Date(selfReview.calendar.end).toLocaleString(undefined, { timeZone: selfReview.calendar.timezone })} · {selfReview.calendar.timezone}</p><p className="fleet-communication-reading">{selfReview.calendar.workspace_url}</p></div>}<p>The message above is the full plain-text content. Any calendar attachment uses the reviewed event details shown here. Branded email layout and delivery tracking are separate from this text.</p></>}
    </ConfirmationDialog>
    </TabPanel><TabPanel value="channels" keepMounted><ProviderSetup page={page} refresh={() => void refresh()}><TeamsWorkflowSetup eventID={event.id} canEdit={page.can_manage && event.can_edit} changed={() => void refresh()} /></ProviderSetup></TabPanel></Tabs></>}{pending && <p role="status">Updating reviewed campaign state…</p>}{notice && <p role="status">{notice}</p>}{!!failure && <Failure error={failure} />}
  </section>
}

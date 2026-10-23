import { Link } from '@tanstack/react-router'
import { SMTPTestDelivery } from './smtp-test-delivery'
import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
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
type SourceCommand = components['schemas']['CommunicationCommand']
export function CampaignWorkspace({ event, onDirty }: { event: EventRecord; onDirty: (value: boolean) => void }) {
  const identity = useIdentity(), source = useQuery({ queryKey: privateKey(identity.person.id, event.id, 'communications'), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/communications', { params: { path: { event_id: event.id } } })), retry: false })
  const campaigns = useQuery({ queryKey: privateKey(identity.person.id, event.id, 'campaigns'), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/communications/campaigns', { params: { path: { event_id: event.id } } })), retry: false })
  const teams = useQuery({ queryKey: privateKey(identity.person.id, event.id, 'campaign-published-teams'), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/teams', { params: { path: { event_id: event.id } } })), retry: false })
  if (source.isPending || campaigns.isPending || teams.isPending) return <Loading />
  if (source.error || campaigns.error || teams.error) return <Failure error={source.error || campaigns.error || teams.error} retry={() => { void source.refetch(); void campaigns.refetch(); void teams.refetch() }} />
  return <Composer key={event.id} event={event} initialSource={source.data!} initialPage={campaigns.data!} teams={teams.data!.teams.filter(t => t.state === 'published')} onDirty={onDirty} />
}
function Composer({ event, initialSource, initialPage, teams, onDirty }: { event: EventRecord; initialSource: ComposerPage; initialPage: CampaignPage; teams: { id: string; name: string }[]; onDirty: (value: boolean) => void }) {
  const identity = useIdentity(), [source, setSource] = useState(initialSource), [page, setPage] = useState(initialPage), [revision, setRevision] = useState(Math.max(initialSource.revision, initialPage.revision)), [message, setMessage] = useState(initialSource.templates[0] ?? blankTemplate), [savedMessage, setSavedMessage] = useState(message), [audience, setAudience] = useState(initialSource.audiences[0] ?? blankAudience), [savedAudience, setSavedAudience] = useState(audience), [definition, setDefinition] = useState(() => blankCampaign(initialSource)), [savedDefinition, setSavedDefinition] = useState(definition), [detail, setDetail] = useState<CampaignDetail>(), [reason, setReason] = useState(''), [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState(''), [testChannel, setTestChannel] = useState<Channel>(), [conflict, setConflict] = useState(false), [latest, setLatest] = useState<{ source: ComposerPage; page: CampaignPage; detail?: CampaignDetail }>(), [reviewed, setReviewed] = useState(false)
  const dirtySources = !same(message, savedMessage) || !same(audience, savedAudience), dirtyDefinition = !same(definition, savedDefinition), dirty = dirtySources || dirtyDefinition
  const disabled = pending || conflict || !page.can_manage || !event.can_edit, actionDisabled = disabled || !reason.trim()
  useEffect(() => { onDirty(dirty); return () => onDirty(false) }, [dirty, onDirty])
  const path = { event_id: event.id }, header = { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${revision}"` }
  async function current(id?: string) {
    const [freshSource, freshPage, freshDetail] = await Promise.all([api.GET('/api/v1/events/{event_id}/communications', { params: { path } }).then(unwrap), api.GET('/api/v1/events/{event_id}/communications/campaigns', { params: { path } }).then(unwrap), id ? api.GET('/api/v1/events/{event_id}/communications/campaigns/{campaign_id}', { params: { path: { ...path, campaign_id: id } } }).then(unwrap) : Promise.resolve(undefined)])
    return { source: freshSource, page: freshPage, detail: freshDetail }
  }
  async function reject(error: unknown) { setFailure(error); if (error instanceof APIError && error.status === 403) setPage(page => ({ ...page, can_manage: false })); if (error instanceof APIError && error.status === 409) { setConflict(true); setReviewed(false); try { setLatest(await current(detail?.campaign.campaign.definition.id)) } catch (refreshError) { setFailure(refreshError) } } }
  async function sourceChange(command: Omit<SourceCommand, 'reason'>) {
    setPending(true); setFailure(undefined); setNotice('')
    try {
      const fresh = unwrap(await api.POST('/api/v1/events/{event_id}/communications', { params: { path, header }, body: { ...command, reason } }))
      setSource(fresh); setRevision(fresh.revision); setTestChannel(undefined)
      if (command.action === 'save_template') { const saved = fresh.templates.find(t => t.id === message.id)!; setMessage(saved); setSavedMessage(saved) }
      else { const saved = fresh.audiences.find(a => a.id === audience.id)!; setAudience(saved); setSavedAudience(saved) }
      const latest = await current(detail?.campaign.campaign.definition.id); setPage(latest.page); if (latest.detail) setDetail(latest.detail)
      if (latest.page.revision !== fresh.revision) { setLatest(latest); setConflict(true); setReviewed(false) } else setRevision(fresh.revision); setNotice('Source version saved. Review a new campaign preview before approval.')
    } catch (error) { await reject(error) } finally { setPending(false) }
  }
  function receive(fresh: CampaignDetail, resetDefinition = false) { setDetail(fresh); setRevision(fresh.revision); setPage(page => ({ ...page, revision: fresh.revision, providers: fresh.providers, can_manage: fresh.can_manage, campaigns: page.campaigns.filter(c => c.campaign.definition.id !== fresh.campaign.campaign.definition.id).concat(fresh.campaign) })); if (resetDefinition) { setDefinition(fresh.campaign.campaign.definition); setSavedDefinition(fresh.campaign.campaign.definition) } }
  async function campaignChange(action: CampaignCommand['action'], channel?: Channel) {
    setPending(true); setFailure(undefined); setNotice('')
    try {
      const fresh = unwrap(await api.POST('/api/v1/events/{event_id}/communications/campaigns/{campaign_id}', { params: { path: { ...path, campaign_id: definition.id }, header }, body: { action, version: detail?.campaign.campaign.version ?? 0, reason, ...(action === 'save' ? { definition } : {}), ...(channel ? { channel } : {}) } }))
      receive(fresh, action === 'save'); setTestChannel(action === 'test_self' ? channel : undefined)
      setNotice(action === 'test_self' ? 'Test queued for the requesting organiser only. Provider acceptance and delivery are not confirmed.' : action === 'approve' ? 'Campaign definition approved. This is not a delivery receipt.' : action === 'preview' ? 'Retained audience preview generated.' : action === 'cancel' ? 'Campaign cancelled. Obsolete queued self-tests have been cancelled.' : 'Campaign saved as a draft. Approval is required.')
    } catch (error) { await reject(error) } finally { setPending(false) }
  }
  function resetSources(next: ComposerPage, campaign: typeof definition) {
    const savedTemplate = next.templates.find(t => t.id === campaign.template_id) ?? next.templates[0] ?? blankTemplate()
    const savedSelector = next.audiences.find(a => a.id === campaign.audience_id) ?? next.audiences[0] ?? blankAudience()
    setMessage(savedTemplate); setSavedMessage(savedTemplate); setAudience(savedSelector); setSavedAudience(savedSelector)
  }
  async function choose(id?: string) {
    if (dirty && !window.confirm('Leave this campaign with unsaved message, audience or schedule edits?')) return
    setFailure(undefined); setNotice(''); setTestChannel(undefined); setConflict(false); setLatest(undefined)
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
  return <section id="communications" className="campaign-composer"><h2>Campaign composer</h2><p><Link to="/events/$eventId/communications/deliveries" params={{ eventId: event.id }}>Delivery history and failure review</Link></p><p>Review the content, current eligible audience, channels and schedule before explicitly approving. Approval does not publish teams, challenges or results.</p>
    <section className="panel"><h3>Campaigns</h3><div className="actions"><Button variant="outline" disabled={pending} onClick={() => void choose()}>New campaign</Button>{page.campaigns.map(c => <Button variant="outline" key={c.campaign.definition.id} disabled={pending} onClick={() => void choose(c.campaign.definition.id)}>{c.campaign.definition.name} · {c.status}</Button>)}</div>{!page.campaigns.length && <p>No campaigns saved yet.</p>}</section>
    {conflict && <section role="alert" className="panel"><h3>Review changed campaign facts</h3><p>Your edits are preserved and nothing is retried automatically. Review the latest message, audience and saved schedule before using its revision.</p>{latest && <><p>Latest messages: {latest.source.templates.map(t => `${t.name}: ${t.subject}`).join(' · ')}</p><p>Latest audiences: {latest.source.audiences.map(a => `${a.name} (version ${a.version})`).join(' · ')}</p>{latest.detail && <p>Latest saved campaign: {latest.detail.campaign.campaign.definition.name} · {latest.detail.campaign.status}</p>}<label className="check"><input type="checkbox" checked={reviewed} onChange={e => setReviewed(e.target.checked)} />I reviewed the current message, audience and schedule</label><Button disabled={!reviewed || pending} onClick={acceptLatest}>Use current revision and keep my edits</Button></>}</section>}
    {!page.can_manage && <p role="alert">This event does not permit campaign changes. Your draft is preserved.</p>}
    <label htmlFor="campaign-reason">Reason for campaign changes</label><input id="campaign-reason" maxLength={2000} value={reason} onChange={e => setReason(e.target.value)} />
    <MessageEditor source={source} value={message} change={setMessage} disabled={disabled} saveDisabled={actionDisabled} save={() => { if (!actionDisabled) void sourceChange({ action: 'save_template', template: message }) }} choose={id => { const t = source.templates.find(t => t.id === id); if (t && (same(message, savedMessage) || window.confirm('Discard unsaved message changes?'))) { setMessage(t); setSavedMessage(t) } }} create={() => { if (same(message, savedMessage) || window.confirm('Discard unsaved message changes?')) { const t = blankTemplate(); setMessage(t); setSavedMessage(t) } }} />
    <AudienceEditor source={source} teams={teams} value={audience} change={setAudience} disabled={disabled} saveDisabled={actionDisabled} save={() => { if (!actionDisabled) void sourceChange({ action: 'save_audience', audience }) }} choose={id => { const a = source.audiences.find(a => a.id === id); if (a && (same(audience, savedAudience) || window.confirm('Discard unsaved audience changes?'))) { setAudience(a); setSavedAudience(a) } }} create={() => { if (same(audience, savedAudience) || window.confirm('Discard unsaved audience changes?')) { const a = blankAudience(); setAudience(a); setSavedAudience(a) } }} />
    {!reason.trim() && <p className="muted">Record a reason before saving, previewing, approving, cancelling or queuing a self-test.</p>}
    <CampaignSettings value={definition} change={setDefinition} source={source} page={page} detail={detail} disabled={disabled} saveDisabled={actionDisabled} save={() => void campaignChange('save')} />
    <CampaignPreview detail={detail} template={selectedTemplate} />
    <section className="panel" aria-label="Campaign approval and tests"><h3>Approval and test controls</h3>{detail && <p>Saved state: <strong>{detail.campaign.status}</strong></p>}{!versionSaved && <p>Save all edited source versions and the campaign draft before preview or approval.</p>}<div className="actions"><Button disabled={actionDisabled || !versionSaved || detail?.campaign.campaign.cancelled} onClick={() => void campaignChange('preview')}>Generate audience preview</Button><Button disabled={actionDisabled || !versionSaved || !currentPreview || !!preview?.blockers.length || approvalBlocked || detail?.campaign.campaign.cancelled} onClick={() => void campaignChange('approve')}>{detail?.campaign.campaign.approval ? 'Explicitly reapprove campaign' : 'Approve campaign'}</Button><Button variant="outline" disabled={actionDisabled || !detail || detail.campaign.campaign.cancelled} onClick={() => void campaignChange('cancel')}>Cancel campaign</Button></div>
    <p>Self-tests target only you, {identity.person.display_name}. They never reuse a selected recipient's personal preview.</p>{definition.channels.map(channel => { const provider = page.providers.find(p => p.channel === channel); return <div key={channel}>{provider?.supports_self_test ? <><Button variant="outline" disabled={actionDisabled || !versionSaved || !provider.self_test_available || detail?.campaign.campaign.cancelled} onClick={() => void campaignChange('test_self', channel)}>Queue {channelNames[channel]} test to myself</Button>{!provider.self_test_available && <p>{blockerText(provider.self_test_reason)}. <a href="#campaign-provider-setup">Review setup</a>.</p>}</> : <p>{channelNames[channel]} is a shared broadcast and cannot send a self-test only to you.</p>}</div> })}</section>
    {page.can_manage && testChannel === 'smtp' && detail?.test_request_id && definition.channels.includes('smtp') && <SMTPTestDelivery eventID={event.id} testID={detail.test_request_id} />}
    <ProviderSetup page={page} refresh={() => void refresh()}><TeamsWorkflowSetup eventID={event.id} canEdit={page.can_manage && event.can_edit} changed={() => void refresh()} /></ProviderSetup>{pending && <p role="status">Updating reviewed campaign state…</p>}{notice && <p role="status">{notice}</p>}{!!failure && <Failure error={failure} />}
  </section>
}

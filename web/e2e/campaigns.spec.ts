import { openEventSection } from './event-navigation-fixture'
import { test, expect, type Page } from '@playwright/test'
import { mockWorkspace, firstID, person } from './fixtures'
import type { components } from '../src/api/schema.gen'

type Definition = components['schemas']['CampaignDefinition']
type Detail = components['schemas']['CampaignDetail']
type Source = components['schemas']['CommunicationPage']
type Provider = components['schemas']['ChannelReadiness']
const templateID = 'c16fd14c-343f-4f3c-b656-cb065502ce95', audienceID = '1864ae7c-7e78-441b-afbc-6a39b096227b', previewID = '1e9d7260-4c36-4277-8d83-155fe87cc560'
const attendeeID = '3263f548-bc52-41cf-8d76-3e2a2cb19565', waitlistID = 'fac4ac6a-d95b-4622-93f7-785a0077cb9e'
const escape = (v: string) => v.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&#34;').replaceAll("'", '&#39;')
async function prepare(page: Page, ready = true, receipt: 'pending' | 'accepted' | 'ambiguous' = 'pending') {
  await mockWorkspace(page)
  let revision = 1, authorized = true
  const source: Source = { revision, variables: ['event.name', 'person.name', 'event.live_start', 'event.submission_cutoff'], preview: null, templates: [{ id: templateID, version: 1, name: 'Synthetic reminder', purpose: 'reminder', format: 'html', subject: 'Hello {{person.name}}', body: 'Synthetic public message for {{person.name}}', fact_id: null }], audiences: [{ id: audienceID, version: 1, name: 'Confirmed competitors', roles: ['attendee'], registration_states: [], teams: [], outstanding: '', fact_id: null }] }
  const providers: Provider[] = ['in_app', 'smtp', 'teams'].map(channel => ({ channel: channel as Provider['channel'], available: ready && channel === 'smtp', config_revision: ready && channel === 'smtp' ? 'synthetic-v1' : '', supports_self_test: channel !== 'teams', self_test_available: ready && channel === 'smtp', reason: ready && channel === 'smtp' ? 'ready' : 'not_configured', self_test_reason: channel === 'teams' ? 'broadcast_only' : ready && channel === 'smtp' ? 'ready' : 'not_configured', setup_url: '#campaign-provider-setup' }))
  const state = { dropTest: false, receipt: receipt as components['schemas']['SMTPTestDeliveryStatus']['state'] }
  const records = new Map<string, Detail>(), requests: { path: string; body: Record<string, unknown>; revision: string }[] = []
  function view(detail: Detail) {
    detail.revision = revision; detail.providers = providers; detail.campaign.blockers = []
    const c = detail.campaign.campaign, t = source.templates.find(t => t.id === c.definition.template_id)!, a = source.audiences.find(a => a.id === c.definition.audience_id)!
    for (const channel of c.definition.channels) if (!providers.find(p => p.channel === channel)?.available) detail.campaign.blockers.push('provider_unavailable_' + channel)
    if (c.definition.channels.includes('teams')) {
      if (a.roles.length !== 1 || a.roles[0] !== 'attendee' || a.registration_states.length || a.teams.length || a.outstanding) detail.campaign.blockers.push('teams_requires_common_audience')
      if (t.subject.includes('person.name') || t.body.includes('person.name')) detail.campaign.blockers.push('teams_requires_broadcast_content')
    }
    if (c.approval && (c.approval.template_version !== t.version || c.approval.audience_version !== a.version)) detail.campaign.blockers.push('approval_outdated')
    detail.campaign.status = c.cancelled ? 'cancelled' : c.approval ? detail.campaign.blockers.includes('approval_outdated') ? 'draft' : detail.campaign.blockers.length ? 'blocked' : 'approved' : 'draft'
    return detail
  }
  const index = () => ({ revision, timezone: 'Europe/London', can_manage: true, campaigns: [...records.values()].map(d => view(d).campaign), providers })
  function selfPreview(campaignID: string): components['schemas']['CampaignSelfTestPreview'] {
    const saved = records.get(campaignID)!.campaign.campaign, template = source.templates.find(value => value.id === saved.definition.template_id)!
    const render = (value: string) => value.replaceAll('{{person.name}}', person.display_name).replaceAll('{{event.name}}', 'North Star Hack')
    return { revision, campaign_version: saved.version, actor_id: person.id, channel: 'smtp', subject: '[TEST] ' + render(template.subject), plain_text: 'This is a test for the requesting organiser only.\n\n' + render(template.body), delivery_plain_text: 'This is a test for the requesting organiser only.\n\n' + render(template.body), calendar: null, include_calendar: saved.definition.include_calendar, provider_revision: 'synthetic-v1', review_fingerprint: 'a'.repeat(64) }
  }
  await page.route(`**/events/${firstID}/communications**`, async route => {
    const req = route.request(), path = new URL(req.url()).pathname
    const fail = (status: number, code: string) => route.fulfill({ status, json: { error: { code, message: 'Synthetic reviewed-state rejection', request_id: 'synthetic' } } })
    if (!authorized) return fail(403, 'forbidden')
    if (path.endsWith('/delivery')) return route.fulfill({ json: { state: state.receipt, reason: state.receipt === 'accepted' ? 'relay_accepted' : state.receipt === 'ambiguous' ? 'acceptance_unknown' : 'awaiting_submission', updated_at: '2026-10-22T12:00:00Z' } })
    if (path.endsWith('/self-test-preview')) {
      return route.fulfill({ json: selfPreview(path.split('/').at(-2)!) })
    }
    const campaignPath = path.includes('/campaigns'), id = path.split('/').at(-1)!
    if (req.method() === 'GET') return route.fulfill({ json: campaignPath ? id === 'campaigns' ? index() : view(records.get(id)!) : { ...source, revision } })
    const body = req.postDataJSON(), reviewed = req.headers()['if-match']
    requests.push({ path, body, revision: reviewed })
    if (reviewed !== `"${revision}"`) return fail(409, 'revision_conflict')
    if (!body.reason || !req.headers()['x-csrf-token']) return fail(400, 'invalid')
    if (!campaignPath) {
      if (body.action === 'save_template') { const t = body.template; t.version++; source.templates = source.templates.filter(v => v.id !== t.id).concat(t) }
      else { const a = body.audience; a.version++; source.audiences = source.audiences.filter(v => v.id !== a.id).concat(a) }
      revision++; return route.fulfill({ json: { ...source, revision } })
    }
    let detail = records.get(id)
    if (body.action === 'save') {
      const definition = body.definition as Definition
      detail = { revision, timezone: 'Europe/London', can_manage: true, providers, test_request_id: null, preview: null, campaign: { campaign: { definition, version: (detail?.campaign.campaign.version ?? 0) + 1, preview_id: null, approval: null, cancelled: false }, status: 'draft', due_at: '2027-06-19T09:00:00Z', blockers: [] } }
      records.set(id, detail)
    } else {
      if (!detail) return fail(404, 'not_found')
      const c = detail.campaign.campaign
      if (body.version !== c.version) return fail(409, 'revision_conflict')
      if (body.action === 'preview') {
        const t = source.templates.find(t => t.id === c.definition.template_id)!, a = source.audiences.find(a => a.id === c.definition.audience_id)!
        const people = [...(a.roles.includes('attendee') ? [{ person_id: attendeeID, name: 'Confirmed synthetic competitor' }] : []), ...(a.registration_states.includes('waitlisted') ? [{ person_id: waitlistID, name: 'Waitlisted synthetic applicant' }] : [])]
        detail.preview = { id: previewID, template_id: t.id, template_version: t.version, audience_id: a.id, audience_version: a.version, recipients: people.map(p => ({ ...p, subject: t.subject.replaceAll('{{person.name}}', p.name), body: t.format === 'html' ? escape(t.body.replaceAll('{{person.name}}', p.name)) : t.body.replaceAll('{{person.name}}', p.name), unresolved: [] })), blockers: people.length ? [] : ['empty_audience'], fingerprint: 'synthetic-safe-fingerprint', approved: false }
        c.preview_id = previewID; c.approval = null; c.version++
      } else if (body.action === 'approve') {
        view(detail); if (detail.campaign.blockers.length || !detail.preview || detail.preview.blockers.length) return fail(409, 'event_blocked')
        c.approval = { preview_id: previewID, template_version: detail.preview.template_version, audience_version: detail.preview.audience_version, fingerprint: detail.preview.fingerprint, binding: 'synthetic-exact-binding', actor_id: person.id, approved_at: '2026-10-22T12:00:00Z', due_at: detail.campaign.due_at!, channel_revisions: { smtp: 'synthetic-v1' } }; c.version++
      } else if (body.action === 'cancel') { c.cancelled = true; c.approval = null; c.version++ }
      else if (body.action === 'test_self') { if (body.channel !== 'smtp' || !ready) return fail(409, 'event_blocked'); detail.test_request_id = 'd4c31a6d-f9d9-4207-b5f9-1594de1ccf32' }
    }
    revision++; if (body.action === 'test_self' && state.dropTest) { state.dropTest = false; return route.abort('failed') } return route.fulfill({ json: view(detail!) })
  })
  await page.goto(`/events/${firstID}`)
  await openEventSection(page, 'Campaigns')
  await expect(page.getByRole('heading', { name: 'Campaign composer' })).toBeVisible()
  await page.getByRole('button', { name: 'New campaign', exact: true }).click()
  await page.getByLabel('Reason for campaign changes').fill('Synthetic explicit review')
  await page.getByLabel('Campaign name', { exact: true }).fill('Synthetic event reminder')
  return { source, providers, records, requests, state, selfPreview, externalChange: () => { revision++ }, revoke: () => { authorized = false } }
}
async function campaignStep(page: Page, name: string) { await page.getByRole('tab', { name, exact: true }).click() }
async function saveAndPreview(page: Page) { await campaignStep(page, 'Schedule'); await page.getByRole('button', { name: 'Save campaign draft', exact: true }).click(); await expect(page.getByRole('status')).toContainText('Campaign saved as a draft'); await page.getByRole('button', { name: 'Generate audience preview' }).click(); await expect(page.getByRole('status')).toContainText('Retained audience preview generated') }

test('approval binds saved content and timing, with edits requiring a new preview and approval', async ({ page }, info) => {
  const data = await prepare(page)
  await saveAndPreview(page)
  await expect(page.getByRole('region', { name: 'Personalised campaign preview' })).toContainText('1 eligible recipients')
  await page.getByRole('button', { name: 'Approve campaign', exact: true }).click()
  await expect(page.getByText('Saved state: approved')).toBeVisible()
  await campaignStep(page, 'Schedule')
  await page.getByLabel('Days from anchor (negative means before)').fill('-2')
  await campaignStep(page, 'Review & TEST')
  await expect(page.getByRole('button', { name: 'Explicitly reapprove campaign' })).toBeDisabled()
  await campaignStep(page, 'Schedule')
  await page.getByRole('button', { name: 'Save campaign draft', exact: true }).click()
  await expect(page.getByText('Saved state: draft')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Approve campaign', exact: true })).toBeDisabled()
  expect([...data.records.values()][0].campaign.campaign.definition.timing.days).toBe(-2)
  await campaignStep(page, 'Message')
  await page.getByLabel('Message body', { exact: true }).fill('Changed approved content {{person.name}}')
  await page.getByRole('button', { name: 'Save message version', exact: true }).click()
  await expect(page.getByRole('status')).toContainText('Source version saved')
  await campaignStep(page, 'Review & TEST')
  await page.getByRole('button', { name: 'Generate audience preview' }).click()
  await page.getByRole('button', { name: 'Approve campaign', exact: true }).click()
  await expect(page.getByText('Saved state: approved')).toBeVisible()
  expect([...data.records.values()][0].campaign.campaign.approval?.template_version).toBe(2)
  await page.screenshot({ path: info.outputPath('campaign-approved.png'), fullPage: true })
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
})

test('recipient preview prevents confusing confirmed competitors with waitlisted applicants', async ({ page }) => {
  const data = await prepare(page)
  await campaignStep(page, 'Audience')
  await page.getByRole('checkbox', { name: 'Confirmed competitors', exact: true }).uncheck()
  await page.getByRole('checkbox', { name: 'waitlisted', exact: true }).check()
  await page.getByRole('button', { name: 'Save audience version' }).click()
  await expect(page.getByRole('status')).toContainText('Source version saved')
  await saveAndPreview(page)
  await expect(page.getByLabel('Preview recipient')).toHaveText('Waitlisted synthetic applicant')
  await expect(page.getByRole('region', { name: 'Personalised campaign preview' })).not.toContainText('Confirmed synthetic competitor')
  expect(data.source.audiences[0].roles).toEqual([]); expect(data.source.audiences[0].registration_states).toEqual(['waitlisted'])
})

test('retains unavailable selected channels, links working setup and never offers a Teams self-test', async ({ page }, info) => {
  const data = await prepare(page, false)
  await page.getByRole('checkbox', { name: 'Teams broadcast · Unavailable', exact: true }).check()
  await saveAndPreview(page)
  await campaignStep(page, 'Schedule')
  await expect(page.getByRole('checkbox', { name: 'Email · Unavailable', exact: true })).toBeChecked()
  await expect(page.getByRole('checkbox', { name: 'Teams broadcast · Unavailable', exact: true })).toBeChecked()
  await campaignStep(page, 'Review & TEST')
  await expect(page.getByRole('button', { name: 'Approve campaign', exact: true })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Queue Email test to myself' })).toBeDisabled()
  await expect(page.getByRole('button', { name: /Teams.*test to myself/ })).toHaveCount(0)
  await campaignStep(page, 'Schedule')
  await expect(page.getByText('Teams cannot broadcast personal variables', { exact: false })).toBeVisible()
  await page.getByRole('button', { name: 'Review Teams broadcast setup' }).click()
  await expect(page.getByRole('heading', { name: 'Channel setup and readiness' })).toBeInViewport()
  expect([...data.records.values()][0].campaign.campaign.definition.channels).toEqual(['smtp', 'teams'])
  await page.screenshot({ path: info.outputPath('campaign-unavailable-channels.png'), fullPage: true })
})

test('self-tests are explicit own-target intents and cancellation is separate from provider delivery', async ({ page }) => {
  const data = await prepare(page)
  await saveAndPreview(page)
  await page.getByRole('button', { name: 'Queue Email test to myself' }).click(); await page.getByRole('button', { name: 'Confirm organiser-only TEST', exact: true }).click()
  await expect(page.getByRole('status')).toContainText('requesting organiser only')
  await expect(page.getByRole('status')).toContainText('delivery are not confirmed')
  const request = data.requests.at(-1)!.body
  expect(request).toMatchObject({ action: 'test_self', channel: 'smtp', reviewed_test_fingerprint: 'a'.repeat(64) })
  expect(Object.keys(request).sort()).toEqual(['action', 'channel', 'reason', 'reviewed_test_fingerprint', 'version'])
  await page.getByRole('button', { name: 'Cancel campaign', exact: true }).click()
  await expect(page.getByText('Saved state: cancelled')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Queue Email test to myself' })).toBeDisabled()
})

test('preserves conflicting schedule edits and requires explicit current-fact review before retry', async ({ page }, info) => {
  const data = await prepare(page)
  await page.getByRole('button', { name: 'Save campaign draft', exact: true }).click()
  await expect(page.getByRole('status')).toContainText('Campaign saved as a draft')
  await campaignStep(page, 'Schedule')
  await page.getByLabel('Campaign name', { exact: true }).fill('My preserved campaign edit')
  data.externalChange()
  await page.getByRole('button', { name: 'Save campaign draft', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Review changed campaign facts' })).toBeVisible()
  await expect(page.getByLabel('Campaign name', { exact: true })).toHaveValue('My preserved campaign edit')
  await expect(page.getByRole('button', { name: 'Save campaign draft', exact: true })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Use current revision and keep my edits' })).toBeDisabled()
  await page.screenshot({ path: info.outputPath('campaign-conflict-preserved.png'), fullPage: true })
  await page.getByRole('checkbox', { name: 'I reviewed the current message, audience and schedule' }).check()
  await page.getByRole('button', { name: 'Use current revision and keep my edits' }).click()
  await page.getByRole('button', { name: 'Save campaign draft', exact: true }).click()
  await expect(page.getByRole('status')).toContainText('Campaign saved as a draft')
  expect(data.requests.map(r => r.revision)).toEqual(['"1"', '"2"', '"3"'])
})

test('hostile long message previews remain inert text and access loss clears private drafts', async ({ page }, info) => {
  const data = await prepare(page)
  await campaignStep(page, 'Message')
  await page.getByLabel('Message body', { exact: true }).fill('<img src="https://example.invalid/secret" onerror="window.__campaignInjected=true">' + 'longsyntheticword'.repeat(80))
  await page.getByRole('button', { name: 'Save message version' }).click()
  await expect(page.getByRole('status')).toContainText('Source version saved')
  await saveAndPreview(page)
  await expect(page.locator('.campaign-message-preview img')).toHaveCount(0)
  expect(await page.evaluate(() => (window as unknown as { __campaignInjected?: boolean }).__campaignInjected)).toBeUndefined()
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
  await page.screenshot({ path: info.outputPath('campaign-hostile-preview.png'), fullPage: true })
  await campaignStep(page, 'Schedule')
  await page.getByLabel('Campaign name', { exact: true }).fill('Preserved after role loss')
  data.revoke()
  await page.getByRole('button', { name: 'Save campaign draft', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('Private edits and retained previews have been cleared')
  await expect(page.getByLabel('Campaign name', { exact: true })).toHaveCount(0)
  await expect(page.getByText('Preserved after role loss', { exact: true })).toHaveCount(0)
})

for (const state of ['accepted', 'ambiguous'] as const) test(`SMTP test shows ${state} honestly without exposing its private envelope`, async ({ page }) => {
  const data = await prepare(page, true, state)
  await page.getByRole('button', { name: 'Save campaign draft', exact: true }).click()
  await expect(page.getByRole('status')).toContainText('Campaign saved as a draft')
  await page.getByRole('button', { name: 'Queue Email test to myself', exact: true }).click(); await page.getByRole('button', { name: 'Confirm organiser-only TEST', exact: true }).click()
  const receipt = page.getByRole('region', { name: 'SMTP test submission' })
  await expect(receipt).toContainText(state === 'accepted' ? 'Inbox delivery and reading are unconfirmed.' : 'Submission is uncertain.')
  if (state === 'ambiguous') await expect(receipt).toContainText('will not be retried automatically')
  expect(data.requests.filter(request => request.body.action === 'test_self')).toHaveLength(1)
  const body = data.requests.find(request => request.body.action === 'test_self')!.body
  expect(body).not.toHaveProperty('email'); expect(body).not.toHaveProperty('recipient_id')
  expect(await page.evaluate(() => JSON.stringify(localStorage) + JSON.stringify(sessionStorage) + location.href)).not.toContain('@example.test')
})


test('lost organiser TEST retains metadata across event tabs and never enables a replacement send', async ({ page }) => {
  const data = await prepare(page)
  await page.getByLabel('Reason for campaign changes').fill('PRIVATE-UNKNOWN-TEST-REASON')
  await page.getByRole('button', { name: 'Save campaign draft', exact: true }).click()
  await expect(page.getByRole('status')).toContainText('Campaign saved as a draft')
  data.state.dropTest = true
  await page.getByRole('button', { name: 'Queue Email test to myself', exact: true }).click()
  const review = page.getByRole('alertdialog')
  await expect(review).toContainText('[TEST] Hello Synthetic Person')
  expect(data.requests.filter(request => request.body.action === 'test_self')).toHaveLength(0)
  await review.getByRole('button', { name: 'Confirm organiser-only TEST', exact: true }).click()
  await expect(page.getByText('The previous campaign operation has an unknown outcome.', { exact: false })).toBeVisible()
  const history = page.getByRole('complementary', { name: 'Unconfirmed command history' })
  await expect(history).toContainText('Campaign test_self')
  await expect(history).not.toContainText('PRIVATE-UNKNOWN-TEST-REASON')
  page.on('dialog', dialog => dialog.accept())
  await openEventSection(page, 'Today'); await openEventSection(page, 'Campaigns')
  await expect(page.getByRole('button', { name: 'New campaign', exact: true })).toBeDisabled()
  await expect(history).toContainText('Original outcome remains unconfirmed')
  expect(data.requests.filter(request => request.body.action === 'test_self')).toHaveLength(1)
  expect(await page.evaluate(() => JSON.stringify(localStorage) + JSON.stringify(sessionStorage))).not.toContain('PRIVATE-UNKNOWN-TEST-REASON')
})

test('late closed TEST preview cannot overwrite a newer reviewed output or fingerprint', async ({ page }) => {
  const data = await prepare(page)
  await page.getByRole('button', { name: 'Save campaign draft', exact: true }).click()
  await expect(page.getByRole('status')).toContainText('Campaign saved as a draft')
  const id = [...data.records.keys()][0], first = { ...data.selfPreview(id), subject: '[TEST] OLD PREVIEW', review_fingerprint: 'a'.repeat(64) }, second = { ...data.selfPreview(id), subject: '[TEST] CURRENT PREVIEW', review_fingerprint: 'b'.repeat(64) }
  let release: () => void = () => {}, calls = 0
  const held = new Promise<void>(resolve => { release = resolve })
  await page.route('**/self-test-preview?channel=smtp', async route => { calls++; if (calls === 1) { await held; await route.fulfill({ json: first }) } else await route.fulfill({ json: second }) })
  try {
    await page.getByRole('button', { name: 'Queue Email test to myself', exact: true }).click()
    await expect(page.getByRole('alertdialog')).toBeVisible()
    await page.getByRole('alertdialog').getByRole('button', { name: 'Cancel', exact: true }).click()
    await page.getByRole('button', { name: 'Queue Email test to myself', exact: true }).click()
    await expect(page.getByRole('alertdialog')).toContainText('[TEST] CURRENT PREVIEW')
    release()
    await expect(page.getByRole('alertdialog')).not.toContainText('[TEST] OLD PREVIEW')
    await page.getByRole('button', { name: 'Confirm organiser-only TEST', exact: true }).click()
    await expect(page.getByRole('status')).toContainText('Test queued for the requesting organiser only')
    expect(data.requests.at(-1)?.body.reviewed_test_fingerprint).toBe('b'.repeat(64))
  } finally { release() }
})

test('current access loss while TEST is pending clears previews and ignores the late private response', async ({ page }) => {
  await page.clock.install()
  const data = await prepare(page)
  await saveAndPreview(page)
  const id = [...data.records.keys()][0], late = structuredClone(data.records.get(id)!)
  late.preview!.recipients[0].body = 'PRIVATE-LATE-RESPONSE-CANARY'
  let release: () => void = () => {}, attempted = false
  const held = new Promise<void>(resolve => { release = resolve })
  await page.route(`**/communications/campaigns/${id}`, async route => { if (route.request().method() !== 'POST') return route.fallback(); attempted = true; await held; await route.fulfill({ json: late }) })
  try {
    await page.getByRole('button', { name: 'Queue Email test to myself', exact: true }).click()
    await page.getByRole('button', { name: 'Confirm organiser-only TEST', exact: true }).click()
    await expect.poll(() => attempted).toBe(true)
    data.revoke(); await page.clock.fastForward(16_000)
    await expect(page.getByText('Private edits and retained previews have been cleared.', { exact: false })).toBeVisible()
    release()
    await expect(page.getByRole('region', { name: 'Personalised campaign preview' })).toHaveCount(0)
    await expect(page.getByText('PRIVATE-LATE-RESPONSE-CANARY')).toHaveCount(0)
    await expect(page.getByRole('button', { name: 'Save campaign draft', exact: true })).toHaveCount(0)
  } finally { release() }
})


test('retains the original queued Email TEST barrier through navigation and setup changes until exact native settlement', async ({ page }) => {
  const data = await prepare(page)
  await saveAndPreview(page)
  const queue = page.getByRole('button', { name: 'Queue Email test to myself', exact: true })
  await queue.click()
  await page.getByRole('button', { name: 'Confirm organiser-only TEST', exact: true }).click()
  await expect(queue).toBeDisabled()
  await expect(page.getByRole('region', { name: 'SMTP test submission' })).toContainText('Test queued')
  // Known acknowledgement survives the actual event section unmount, independently
  // of PlanningOperations' unknown-command capsules and all campaign editor keys.
  await openEventSection(page, 'Overview')
  data.providers.find(provider => provider.channel === 'smtp')!.config_revision = 'synthetic-v2'
  data.state.receipt = 'retryable'
  await openEventSection(page, 'Campaigns')
  await expect(page.getByRole('region', { name: 'SMTP test submission' })).toContainText('bounded retry is scheduled')
  await page.getByRole('button', { name: 'Review Synthetic event reminder', exact: true }).click()
  await page.getByLabel('Reason for campaign changes').fill('Synthetic next review')
  await expect(queue).toBeDisabled()
  data.state.receipt = 'ambiguous'
  await expect(page.getByRole('region', { name: 'SMTP test submission' })).toContainText('Submission is uncertain')
  await campaignStep(page, 'Channels')
  await campaignStep(page, 'Review & TEST')
  await expect(queue).toBeDisabled()
  expect(data.requests.filter(request => request.body.action === 'test_self')).toHaveLength(1)
  await expect(page.getByRole('heading', { name: 'Unconfirmed earlier commands', exact: true })).toHaveCount(0)
  // Re-read this same acknowledged native TEST. A different campaign GET or
  // provider configuration never settles its provider effect.
  data.state.receipt = 'accepted'
  await page.getByRole('button', { name: 'Check original TEST', exact: true }).click()
  await expect(page.getByRole('region', { name: 'SMTP test submission' })).toContainText('Inbox delivery and reading are unconfirmed')
  await expect(queue).toBeEnabled()
  expect(data.requests.filter(request => request.body.action === 'test_self')).toHaveLength(1)
})


test('invalid and empty schedule fields retain edits and do not issue a save command', async ({ page }) => {
  const data = await prepare(page)
  const save = page.getByRole('button', { name: 'Save campaign draft', exact: true })
  const days = page.getByLabel('Days from anchor (negative means before)', { exact: true })
  await days.fill('')
  await expect(days).toHaveAttribute('aria-invalid', 'true')
  await expect(page.getByText('Enter a whole number of days from −366 to 366.', { exact: true })).toBeVisible()
  await expect(save).toBeDisabled()
  await days.fill('-367')
  await expect(save).toBeDisabled()
  await days.fill('-2')
  await page.getByLabel('Additional minutes', { exact: true }).fill('1.5')
  await expect(save).toBeDisabled()
  await page.getByLabel('Additional minutes', { exact: true }).fill('0')
  await expect(save).toBeEnabled()
  await page.getByLabel('Schedule type', { exact: true }).selectOption('fixed')
  await expect(save).toBeDisabled()
  await expect(page.getByLabel('Scheduled local time (Europe/London)', { exact: true })).toHaveAttribute('aria-invalid', 'true')
  expect(data.requests).toHaveLength(0)
  await page.getByLabel('Scheduled local time (Europe/London)', { exact: true }).fill('2027-06-18T09:30')
  await expect(save).toBeEnabled()
  await save.click()
  await expect(page.getByRole('status')).toContainText('Campaign saved as a draft')
  expect(data.requests).toHaveLength(1)
  expect(data.requests[0].body.definition).toMatchObject({ timing: { kind: 'fixed', wall_time: '2027-06-18T09:30:00' } })
})

for (const receipt of ['pending', 'ambiguous'] as const) test(`original ${receipt} Email TEST survives the actual delivery sibling route`, async ({ page }) => {
  const data = await prepare(page, true, receipt)
  await page.route(`**/events/${firstID}/communications/deliveries**`, route => route.fulfill({ json: { items: [], next_cursor: null, archived: false, paused: false } }))
  await saveAndPreview(page)
  await page.getByRole('button', { name: 'Queue Email test to myself', exact: true }).click()
  await page.getByRole('button', { name: 'Confirm organiser-only TEST', exact: true }).click()
  await expect(page.getByRole('region', { name: 'SMTP test submission' })).toContainText(receipt === 'pending' ? 'Test queued' : 'Submission is uncertain')
  await page.getByRole('link', { name: 'Delivery history and failure review', exact: true }).click()
  await expect(page).toHaveURL(/communications\/deliveries$/)
  await page.getByRole('link', { name: '← Back to event', exact: true }).click()
  await openEventSection(page, 'Campaigns')
  await page.getByRole('button', { name: 'Review Synthetic event reminder', exact: true }).click()
  await page.getByLabel('Reason for campaign changes').fill('Review the original TEST')
  await expect(page.getByRole('button', { name: 'Queue Email test to myself', exact: true })).toBeDisabled()
  expect(data.requests.filter(request => request.body.action === 'test_self')).toHaveLength(1)
  data.state.receipt = 'accepted'
  await page.getByRole('button', { name: 'Check original TEST', exact: true }).click()
  await expect(page.getByRole('region', { name: 'SMTP test submission' })).toContainText('Inbox delivery and reading are unconfirmed')
  await expect(page.getByRole('button', { name: 'Queue Email test to myself', exact: true })).toBeEnabled()
})

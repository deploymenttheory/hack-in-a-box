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
  await page.route(`**/events/${firstID}/communications**`, async route => {
    const req = route.request(), path = new URL(req.url()).pathname
    const fail = (status: number, code: string) => route.fulfill({ status, json: { error: { code, message: 'Synthetic reviewed-state rejection', request_id: 'synthetic' } } })
    if (!authorized) return fail(403, 'forbidden')
    if (path.endsWith('/delivery')) return route.fulfill({ json: { state: receipt, reason: receipt === 'accepted' ? 'relay_accepted' : receipt === 'ambiguous' ? 'acceptance_unknown' : 'awaiting_submission', updated_at: '2026-10-22T12:00:00Z' } })
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
    revision++; return route.fulfill({ json: view(detail!) })
  })
  await page.goto(`/events/${firstID}`)
  await page.getByRole('button', { name: 'Campaigns', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Campaign composer' })).toBeVisible()
  await page.getByLabel('Reason for campaign changes').fill('Synthetic explicit review')
  await page.getByLabel('Campaign name', { exact: true }).fill('Synthetic event reminder')
  return { source, providers, records, requests, externalChange: () => { revision++ }, revoke: () => { authorized = false } }
}
async function saveAndPreview(page: Page) { await page.getByRole('button', { name: 'Save campaign draft', exact: true }).click(); await expect(page.getByRole('status')).toContainText('Campaign saved as a draft'); await page.getByRole('button', { name: 'Generate audience preview' }).click(); await expect(page.getByRole('status')).toContainText('Retained audience preview generated') }

test('approval binds saved content and timing, with edits requiring a new preview and approval', async ({ page }, info) => {
  const data = await prepare(page)
  await saveAndPreview(page)
  await expect(page.getByRole('region', { name: 'Personalised campaign preview' })).toContainText('1 eligible recipients')
  await page.getByRole('button', { name: 'Approve campaign', exact: true }).click()
  await expect(page.getByText('Saved state: approved')).toBeVisible()
  await page.getByLabel('Days from anchor (negative means before)').fill('-2')
  await expect(page.getByRole('button', { name: 'Explicitly reapprove campaign' })).toBeDisabled()
  await page.getByRole('button', { name: 'Save campaign draft', exact: true }).click()
  await expect(page.getByText('Saved state: draft')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Approve campaign', exact: true })).toBeDisabled()
  expect([...data.records.values()][0].campaign.campaign.definition.timing.days).toBe(-2)
  await page.getByText('Message editing', { exact: true }).click()
  await page.getByLabel('Message body', { exact: true }).fill('Changed approved content {{person.name}}')
  await page.getByRole('button', { name: 'Save message version', exact: true }).click()
  await expect(page.getByRole('status')).toContainText('Source version saved')
  await page.getByRole('button', { name: 'Generate audience preview' }).click()
  await page.getByRole('button', { name: 'Approve campaign', exact: true }).click()
  await expect(page.getByText('Saved state: approved')).toBeVisible()
  expect([...data.records.values()][0].campaign.campaign.approval?.template_version).toBe(2)
  await page.getByText('Message editing', { exact: true }).click()
  await page.screenshot({ path: info.outputPath('campaign-approved.png'), fullPage: true })
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
})

test('recipient preview prevents confusing confirmed competitors with waitlisted applicants', async ({ page }) => {
  const data = await prepare(page)
  await page.getByText('Audience selector editing', { exact: true }).click()
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
  await expect(page.getByRole('checkbox', { name: 'Email · Unavailable', exact: true })).toBeChecked()
  await expect(page.getByRole('checkbox', { name: 'Teams broadcast · Unavailable', exact: true })).toBeChecked()
  await expect(page.getByRole('button', { name: 'Approve campaign', exact: true })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Queue Email test to myself' })).toBeDisabled()
  await expect(page.getByRole('button', { name: /Teams.*test to myself/ })).toHaveCount(0)
  await expect(page.getByText('Teams cannot broadcast personal variables', { exact: false })).toBeVisible()
  await page.getByRole('link', { name: 'Review Teams broadcast setup' }).click()
  await expect(page.getByRole('heading', { name: 'Channel setup and readiness' })).toBeInViewport()
  expect([...data.records.values()][0].campaign.campaign.definition.channels).toEqual(['smtp', 'teams'])
  await page.screenshot({ path: info.outputPath('campaign-unavailable-channels.png'), fullPage: true })
})

test('self-tests are explicit own-target intents and cancellation is separate from provider delivery', async ({ page }) => {
  const data = await prepare(page)
  await saveAndPreview(page)
  await page.getByRole('button', { name: 'Queue Email test to myself' }).click()
  await expect(page.getByRole('status')).toContainText('requesting organiser only')
  await expect(page.getByRole('status')).toContainText('delivery are not confirmed')
  const request = data.requests.at(-1)!.body
  expect(request).toMatchObject({ action: 'test_self', channel: 'smtp' })
  expect(Object.keys(request).sort()).toEqual(['action', 'channel', 'reason', 'version'])
  await page.getByRole('button', { name: 'Cancel campaign', exact: true }).click()
  await expect(page.getByText('Saved state: cancelled')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Queue Email test to myself' })).toBeDisabled()
})

test('preserves conflicting schedule edits and requires explicit current-fact review before retry', async ({ page }, info) => {
  const data = await prepare(page)
  await page.getByRole('button', { name: 'Save campaign draft', exact: true }).click()
  await expect(page.getByRole('status')).toContainText('Campaign saved as a draft')
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

test('hostile long message previews remain inert text and access loss preserves the draft', async ({ page }, info) => {
  const data = await prepare(page)
  await page.getByText('Message editing', { exact: true }).click()
  await page.getByLabel('Message body', { exact: true }).fill('<img src="https://example.invalid/secret" onerror="window.__campaignInjected=true">' + 'longsyntheticword'.repeat(80))
  await page.getByRole('button', { name: 'Save message version' }).click()
  await expect(page.getByRole('status')).toContainText('Source version saved')
  await page.getByText('Message editing', { exact: true }).click()
  await saveAndPreview(page)
  await expect(page.locator('.campaign-message-preview img')).toHaveCount(0)
  expect(await page.evaluate(() => (window as unknown as { __campaignInjected?: boolean }).__campaignInjected)).toBeUndefined()
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
  await page.screenshot({ path: info.outputPath('campaign-hostile-preview.png'), fullPage: true })
  await page.getByLabel('Campaign name', { exact: true }).fill('Preserved after role loss')
  data.revoke()
  await page.getByRole('button', { name: 'Save campaign draft', exact: true }).click()
  await expect(page.getByText('This event does not permit campaign changes. Your draft is preserved.', { exact: true })).toBeVisible()
  await expect(page.getByLabel('Campaign name', { exact: true })).toHaveValue('Preserved after role loss')
  await expect(page.getByRole('button', { name: 'Save campaign draft', exact: true })).toBeDisabled()
})

for (const state of ['accepted', 'ambiguous'] as const) test(`SMTP test shows ${state} honestly without exposing its private envelope`, async ({ page }) => {
  const data = await prepare(page, true, state)
  await page.getByRole('button', { name: 'Save campaign draft', exact: true }).click()
  await expect(page.getByRole('status')).toContainText('Campaign saved as a draft')
  await page.getByRole('button', { name: 'Queue Email test to myself', exact: true }).click()
  const receipt = page.getByRole('region', { name: 'SMTP test submission' })
  await expect(receipt).toContainText(state === 'accepted' ? 'Inbox delivery and reading are unconfirmed.' : 'Submission is uncertain.')
  if (state === 'ambiguous') await expect(receipt).toContainText('will not be retried automatically')
  expect(data.requests.filter(request => request.body.action === 'test_self')).toHaveLength(1)
  const body = data.requests.find(request => request.body.action === 'test_self')!.body
  expect(body).not.toHaveProperty('email'); expect(body).not.toHaveProperty('recipient_id')
  expect(await page.evaluate(() => JSON.stringify(localStorage) + JSON.stringify(sessionStorage) + location.href)).not.toContain('@example.test')
})

import { uploadedFile } from './file-fixtures'
import { openEventSection } from './event-navigation-fixture'
import { test, expect, type Page } from '@playwright/test'
import { mockWorkspace, firstID, person } from './fixtures'
import { defaultBranding } from '../src/branding'
import type { components } from '../src/api/schema.gen'

type Content = components['schemas']['ContentPage']
type Document = components['schemas']['ContentDocument']
const pageID = '5883f880-1c49-4d89-b498-005df3f30986', challengeID = 'ddcc2a2d-aa18-49f5-a5a3-233f8fdd1ed0', teamID = '164a3cd9-7b9e-45da-b6c7-1b797f11f70c', fileID = 'e7c04139-482e-4b02-bbb2-2674f6acfbbe'
function contentDocument(title = 'Team handbook'): Document { return { title, require_acknowledgement: false, markdown: '# Welcome\n\nBuild together.\n\n[Guide](https://example.test/guide)', audience: { confirmed: true, roles: [], teams: [] }, attachments: [], scheduled_at: null } }
function item(id = pageID): Content { return { id, kind: 'page', revision: 3, version: 1, state: 'approved', document: contentDocument(), released_version: null, released_at: null, attachment_details: [], acknowledged_at: null, can_manage: true, message: '' } }
async function setup(page: Page) {
  const workspace = await mockWorkspace(page), event = workspace.events[0]
  event.stage = 'preparation'; event.confirmed_attendees = 32
  const items = [item(), { ...item(challengeID), kind: 'challenge' as const, state: 'draft' as const, revision: 1, document: { ...contentDocument('Common challenge'), scheduled_at: event.challenge_publication } }]
  const versions = new Map(items.map(i => [`${i.id}:1`, structuredClone(i)])), writes: { method: string; path: string; body: unknown; revision?: string }[] = []
  const blockers: components['schemas']['Blocker'][] = [{ field: 'team_freeze', code: 'teams_unready', message: 'Freeze every confirmed team before publication.' }]
  let denied = false, missingFile = false
  const files: Content['attachment_details'] = []
  await page.route(`**/events/${firstID}/**`, async route => {
    const r = route.request(), path = new URL(r.url()).pathname, query = new URL(r.url()).searchParams
    const fail = (status: number, code: string, message = 'Synthetic rejection') => route.fulfill({ status, json: { error: { code, message, request_id: 'synthetic', blockers: code === 'event_blocked' ? [{ field: 'content', code: 'attachment_unavailable', message }] : undefined } } })
    if (path.endsWith('/configuration-snapshot')) return route.fulfill({ json: { event: event.configuration, definitions: { forms: [], tasks: [], deliverables: [], role_briefs: [{ id: '54b5e312-49b4-424b-b181-85c404b29478', role: 'mentor', body: '# Mentor\n\nHelp teams ask better questions.' }], messages: [], rubrics: [], branding: defaultBranding } } })
    if (path.endsWith('/teams')) return route.fulfill({ json: { revision: event.revision, can_manage: true, can_create: false, frozen: false, freeze_at: event.configuration.team_freeze, teams: [{ id: teamID, name: 'Aurora', state: 'published', frozen: false, captain: null, published_captain: null, invitation_code: null, draft: [], published: [] }], people: [], requests: [] } })
    if (path.endsWith('/content-readiness')) return route.fulfill({ json: { published_at: event.published_at, blockers } })
    if (path.endsWith('/files') && r.method() === 'POST') {
      const body = r.postDataJSON(); writes.push({ method: r.method(), path, body, revision: r.headers()['if-match'] })
      files.push({ id: fileID, filename: body.filename, media_type: body.media_type, size: 14, can_download: true })
      return route.fulfill({ status: 201, json: uploadedFile(fileID, body) })
    }
    if (path.endsWith(`/files/${fileID}`)) return route.fulfill({ contentType: 'text/plain', body: 'Synthetic attachment' })
    if (!path.includes('/content')) return route.fallback()
    if (path.endsWith('/content')) return route.fulfill({ json: { items, next_cursor: null } })
    const id = path.split('/content/')[1]?.split('/')[0]
    const current = items.find(i => i.id === id)
    if (path.endsWith('/history')) return route.fulfill({ json: { items: [{ revision: current?.revision ?? 1, version: current?.version ?? 1, action: current?.state ?? 'draft', actor_id: person.id, reason: 'Synthetic review', scheduled_at: current?.document.scheduled_at, at: '2027-06-01T09:00:00Z' }], next_cursor: null } })
    if (r.method() === 'GET') return current ? route.fulfill({ json: query.has('version') ? versions.get(`${id}:${query.get('version')}`) ?? current : current }) : fail(404, 'not_found')
    const body = r.postDataJSON(); writes.push({ method: r.method(), path, body, revision: r.headers()['if-match'] })
    if (denied) return fail(403, 'forbidden')
    if (r.headers()['if-match'] !== `"${current?.revision ?? 0}"`) return fail(409, 'revision_conflict')
    if (r.method() === 'PUT') {
      const saved: Content = { ...(current ?? { ...item(id), revision: 0, version: 0 }), kind: body.kind, document: body.document, revision: (current?.revision ?? 0) + 1, version: (current?.version ?? 0) + 1, state: 'draft', attachment_details: body.document.attachments.map((id: string) => files.find(f => f.id === id)).filter(Boolean) }
      if (current) Object.assign(current, saved); else items.push(saved)
      versions.set(`${id}:${saved.version}`, structuredClone(saved)); return route.fulfill({ json: saved })
    }
    if (!current) return fail(404, 'not_found')
    if (body.action === 'approve' && missingFile) return fail(409, 'event_blocked', 'An attachment is missing. Restore or replace it before approval.')
    current.revision++
    if (body.action === 'review') current.state = 'review'
    if (body.action === 'approve') current.state = 'approved'
    if (body.action === 'release') { current.state = 'released'; current.released_version = current.version; current.released_at = '2027-06-01T10:00:00Z' }
    if (body.action === 'cancel') current.state = 'cancelled'
    if (body.action === 'withdraw') { current.state = 'withdrawn'; current.released_version = null; current.message = 'Future access removed. Previously downloaded copies cannot be recalled.' }
    return route.fulfill({ json: current })
  })
  await page.goto(`/events/${firstID}`); await openEventSection(page, 'Content')
  await expect(page.getByRole('heading', { name: 'Content', exact: true })).toBeVisible()
  return { items, writes, versions, workspace, blockers, deny: () => { denied = true }, missingFile: () => { missingFile = true } }
}
async function contentSection(page: Page, name: string) { await page.getByRole('navigation', { name: 'Content editor sections' }).getByRole('button', { name, exact: true }).click() }
const editor = (page: Page) => page.getByRole('form', { name: 'Content editor' })

test('editing approved content creates a draft and requires review again', async ({ page }, info) => {
  const { writes } = await setup(page)
  await page.getByRole('button', { name: 'Edit Team handbook', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Release approved version' })).toBeEnabled()
  await page.getByLabel('Markdown', { exact: true }).fill('# Updated handbook\n\nBring your curiosity.')
  await page.getByLabel('Require readers to acknowledge this version').check()
  await expect(page.getByRole('button', { name: 'Release approved version' })).toBeDisabled()
  await page.getByRole('button', { name: 'Save new draft', exact: true }).click()
  await expect(page.getByText('Draft version 2 saved.', { exact: false })).toBeVisible()
  expect(writes.at(-1)?.revision).toBe('"3"')
  expect((writes.at(-1)?.body as { document: Document }).document.require_acknowledgement).toBe(true)
  await page.getByRole('button', { name: 'Mark ready for review' }).click()
  await page.getByRole('button', { name: 'Approve version 2' }).click()
  await expect(page.getByRole('button', { name: 'Release approved version' })).toBeEnabled()
  await page.getByText('Version comparison and history', { exact: true }).click()
  await expect(page.getByRole('region', { name: 'Saved version 1' })).toContainText('Build together.')
  await expect(page.getByRole('region', { name: 'Current editor version' })).toContainText('Bring your curiosity.')
  await page.getByRole('button', { name: 'Release approved version' }).click()
  await expect(page.getByText('Version released to its audience.', { exact: true })).toBeVisible()
  await page.screenshot({ path: info.outputPath('content-authoring.png'), fullPage: true })
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
})

test('role and team previews never grant access and dangerous Markdown stays inert', async ({ page }) => {
  const { writes } = await setup(page)
  await page.getByRole('button', { name: 'Edit Team handbook' }).click()
  await page.getByLabel('Markdown', { exact: true }).fill('# Safe title\n\n[Good](https://example.test/guide)\n\n[Unsafe](javascript:alert%281%29)\n\n![tracker](https://example.test/tracker.png)\n\n<script>window.bad=true</script>')
  const preview = page.getByRole('region', { name: 'Audience preview' })
  await expect(preview.getByRole('link', { name: 'Good' })).toBeVisible()
  await expect(preview.getByRole('link', { name: 'Unsafe' })).toHaveCount(0)
  await expect(preview.locator('script,iframe,img')).toHaveCount(0)
  await page.getByLabel('Preview as').selectOption('mentor')
  await expect(preview).toContainText('Outside the release audience')
  await contentSection(page, 'Release audience')
  await editor(page).getByLabel('Mentor', { exact: true }).check()
  await expect(preview).toContainText('Included in the release audience')
  await editor(page).getByLabel('All confirmed attendees').uncheck()
  await editor(page).getByLabel('Aurora', { exact: true }).check()
  await page.getByLabel('Preview as').selectOption(`team:${teamID}`)
  await expect(preview).toContainText('Aurora · Included in the release audience')
  await expect(preview).toContainText('unsaved draft')
  expect(writes).toHaveLength(0)
})

test('templates, placeholder validation, file upload and missing-file approval', async ({ page }) => {
  const state = await setup(page)
  await page.getByLabel('Starting template').selectOption('faq')
  await page.getByRole('button', { name: 'Start from template' }).click()
  await page.getByRole('button', { name: 'Save new draft' }).click()
  await page.getByRole('button', { name: 'Mark ready for review' }).click()
  await expect(page.getByRole('button', { name: 'Approve version 1' })).toBeDisabled()
  await expect(page.getByLabel('Approval blockers')).toContainText('Resolve every {{placeholder}}')
  await page.getByLabel('Markdown', { exact: true }).fill('# FAQ\n\nAsk the organising team.')
  await contentSection(page, 'Attachments')
  await page.getByLabel('Choose an attachment').setInputFiles({ name: 'guide.txt', mimeType: 'text/plain', buffer: Buffer.from('Synthetic guide') })
  await page.getByRole('button', { name: 'Upload attachment' }).click()
  await expect(page.getByRole('link', { name: 'guide.txt' })).toBeVisible()
  await page.getByRole('button', { name: 'Save new draft' }).click()
  await page.getByRole('button', { name: 'Mark ready for review' }).click()
  state.missingFile()
  await page.getByRole('button', { name: 'Approve version 2' }).click()
  await expect(page.getByRole('alert')).toContainText('An attachment is missing')
  await contentSection(page, 'Page content')
  await expect(page.getByLabel('Markdown', { exact: true })).toHaveValue('# FAQ\n\nAsk the organising team.')
  expect(state.writes.some(w => w.path.endsWith('/files'))).toBe(true)
})

test('challenge controls show common timing and readiness without a manual bypass', async ({ page }) => {
  const state = await setup(page)
  await page.getByRole('button', { name: 'Edit Common challenge' }).click()
  const readiness = page.getByRole('region', { name: 'Challenge publication' })
  await expect(readiness).toContainText('Seven calendar days before live start')
  await expect(readiness).toContainText('32 confirmed competitors · Minimum 30')
  await expect(readiness).toContainText('Freeze every confirmed team before publication.')
  await contentSection(page, 'Release audience')
  await expect(page.getByLabel('All confirmed attendees')).toBeDisabled()
  await expect(page.getByLabel('Release method')).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Release approved version' })).toHaveCount(0)
  await page.getByRole('button', { name: 'Mark ready for review' }).click()
  await page.getByRole('button', { name: 'Approve version 1' }).click()
  await expect(page.getByRole('button', { name: 'Release approved version' })).toHaveCount(0)
  expect(state.items.find(p => p.id === challengeID)?.state).toBe('approved')
  await page.getByRole('button', { name: 'Cancel pending release' }).click()
  await expect(page.getByText('Pending release cancelled.', { exact: false })).toBeVisible()
  await expect(page.getByText('Active and waitlisted competitors must explicitly withdraw', { exact: false })).toBeVisible()
  await page.getByRole('button', { name: 'Manage privileged access' }).click()
  await expect(page.getByRole('heading', { name: 'Assign event staff' })).toBeVisible()
})

test('conflicts preserve edits until the latest version is reviewed', async ({ page }) => {
  const { items } = await setup(page)
  await page.getByRole('button', { name: 'Edit Team handbook' }).click()
  await page.getByLabel('Page title').fill('My handbook edits')
  items[0].revision = 4; items[0].document.title = 'Colleague handbook'
  await page.getByRole('button', { name: 'Save new draft' }).click()
  await expect(page.getByLabel('Page title')).toHaveValue('My handbook edits')
  await expect(page.getByRole('button', { name: 'Save new draft' })).toBeDisabled()
  await page.getByRole('button', { name: 'Review latest saved version' }).click()
  await expect(page.getByRole('region', { name: 'Latest saved content' })).toContainText('Colleague handbook')
  await expect(page.getByRole('button', { name: 'Keep my edits for a new draft' })).toBeDisabled()
  await page.getByLabel('I reviewed the latest saved version').check()
  await page.getByRole('button', { name: 'Keep my edits for a new draft' }).click()
  await page.getByRole('button', { name: 'Save new draft' }).click()
  await expect(page.getByText('Draft version 2 saved.', { exact: false })).toBeVisible()
})

test('uses reusable event briefs, protects unsaved work, and disables revoked mutations', async ({ page }) => {
  const state = await setup(page)
  await page.getByLabel('Starting template').selectOption({ label: 'Mentor brief from event template' })
  await page.getByRole('button', { name: 'Start from template' }).click()
  await expect(page.getByLabel('Markdown', { exact: true })).toHaveValue('# Mentor\n\nHelp teams ask better questions.')
  page.once('dialog', dialog => dialog.dismiss()); await openEventSection(page, 'Overview')
  await expect(page.getByRole('form', { name: 'Content editor' })).toBeVisible()
  state.deny(); await page.getByRole('button', { name: 'Save new draft' }).click()
  await expect(page.getByRole('alert')).toContainText('do not have access')
  await expect(editor(page)).toHaveCount(0)
})

test('resolves event-local schedules and requires a reason to withdraw released pages', async ({ page }) => {
  const { writes } = await setup(page)
  await page.getByRole('button', { name: 'Edit Team handbook' }).click()
  await contentSection(page, 'Release timing')
  await page.getByLabel('Release method').selectOption('scheduled')
  await page.getByLabel('Release time in Europe/London').fill('2027-06-10T10:30')
  await expect(page.getByRole('region', { name: 'Audience preview' })).toContainText('2027-06-10 10:30 · Europe/London (unsaved; resolved on save)')
  await page.getByRole('button', { name: 'Save new draft' }).click()
  await expect(page.getByText('Draft version 2 saved.', { exact: false })).toBeVisible()
  expect((writes.at(-1)?.body as { document: Document }).document.scheduled_at).toBe('2027-06-10T10:30:00Z')
  await expect(page.getByRole('button', { name: 'Save new draft' })).toBeDisabled()
  await page.getByLabel('Repeated local time').selectOption('earlier')
  await expect(page.getByRole('button', { name: 'Save new draft' })).toBeEnabled()
  await page.getByLabel('Release method').selectOption('manual')
  await page.getByRole('button', { name: 'Save new draft' }).click(); await page.getByRole('button', { name: 'Mark ready for review' }).click(); await page.getByRole('button', { name: 'Approve version 3' }).click(); await page.getByRole('button', { name: 'Release approved version' }).click()
  await expect(page.getByRole('button', { name: 'Withdraw released content' })).toBeDisabled()
  await page.getByLabel('Reason for withdrawal').fill('Replace obsolete instructions')
  await page.getByRole('button', { name: 'Withdraw released content' }).click()
  await expect(page.getByText('Previously downloaded copies cannot be recalled.', { exact: false }).first()).toBeVisible()
})


test('invalid and past schedules cannot approve and keep the organiser draft', async ({ page }) => {
  const { items, writes } = await setup(page)
  items[0].state = 'review'; items[0].document.scheduled_at = '2020-01-01T10:00:00Z'
  await page.reload(); await openEventSection(page, 'Content')
  await page.getByRole('button', { name: 'Edit Team handbook' }).click()
  await expect(page.getByRole('button', { name: 'Approve version 1' })).toBeDisabled()
  await expect(page.getByRole('alert')).toContainText('The release schedule is in the past.')
  await page.route('**/api/v1/time/resolve', route => route.fulfill({ status: 409, json: { error: { code: 'event_blocked', message: 'Scheduling needs attention.', request_id: 'synthetic', blockers: [{ field: 'dst_choice', code: 'ambiguous_time', message: 'Choose the earlier or later occurrence of this repeated local time.' }] } } }))
  await contentSection(page, 'Release timing')
  await page.getByLabel('Release time in Europe/London').fill('2027-10-31T01:30')
  await page.getByRole('button', { name: 'Save new draft' }).click()
  await expect(page.getByText('Choose the earlier or later occurrence', { exact: false })).toBeVisible()
  await expect(page.getByLabel('Release time in Europe/London')).toHaveValue('2027-10-31T01:30')
  await expect(page.getByRole('button', { name: 'Approve version 1' })).toBeDisabled()
  expect(writes).toHaveLength(0)
})

test('lost release response preserves unknown command history after reading the native released version', async ({ page }) => {
  const state = await setup(page)
  let releases = 0
  await page.route(`**/content/${pageID}/actions`, async route => {
    const body = route.request().postDataJSON()
    if (body.action !== 'release') return route.fallback()
    releases++
    expect(route.request().headers()['if-match']).toBe('"3"')
    expect(route.request().headers()['x-csrf-token']).toBe('synthetic-csrf-token-32-characters')
    state.items[0].state = 'released'; state.items[0].revision = 4; state.items[0].released_version = 1
    state.items[0].released_at = '2027-06-01T10:00:00Z'
    return route.fulfill({ status: 503, json: { error: { code: 'unavailable', request_id: 'synthetic' } } })
  })
  await page.getByRole('button', { name: 'Edit Team handbook', exact: true }).click()
  await page.getByRole('button', { name: 'Release approved version', exact: true }).click()
  await expect(page.getByText('Release action outcome is unconfirmed.', { exact: false })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Close editor', exact: true })).toBeDisabled()
  await page.getByRole('button', { name: 'Review latest saved version', exact: true }).click()
  await expect(page.getByRole('region', { name: 'Latest saved content' })).toContainText('released')
  expect(releases).toBe(1)
  await page.getByLabel('I reviewed the latest saved version').check()
  await page.getByRole('button', { name: 'Keep my edits for a new draft', exact: true }).click()
  await expect(page.getByRole('complementary', { name: 'Unconfirmed command history' })).toContainText('Content release · Expected revision 3 · Original outcome remains unconfirmed')
  await expect(page.getByText('Version released to its audience.', { exact: true })).toHaveCount(0)
  expect(releases).toBe(1)
})

test('denied privileged history clears the private editor and its audience preview', async ({ page }) => {
  await setup(page)
  await page.getByRole('button', { name: 'Edit Team handbook', exact: true }).click()
  await page.getByLabel('Markdown', { exact: true }).fill('SYNTHETIC-PRIVATE-DRAFT-CANARY')
  await page.route(`**/content/${pageID}/history*`, route => route.fulfill({ status: 403, json: { error: { code: 'forbidden', request_id: 'synthetic' } } }))
  await page.getByText('Version comparison and history', { exact: true }).click()
  await expect(editor(page)).toHaveCount(0)
  await expect(page.getByRole('region', { name: 'Audience preview' })).toHaveCount(0)
  await expect(page.getByText('SYNTHETIC-PRIVATE-DRAFT-CANARY', { exact: false })).toHaveCount(0)
})


test('closing a new template restores the actual template action for keyboard users', async ({ page }, info) => {
  await setup(page)
  await page.getByRole('button', { name: 'Start from template', exact: true }).focus(); await page.keyboard.press('Enter')
  await expect(editor(page)).toBeVisible()
  await page.setViewportSize(info.project.name === 'mobile' ? { width: 1280, height: 800 } : { width: 390, height: 844 })
  page.once('dialog', dialog => dialog.accept())
  await page.getByRole('button', { name: 'Close editor', exact: true }).focus(); await page.keyboard.press('Enter')
  await expect(page.getByRole('button', { name: 'Start from template', exact: true })).toBeFocused()
})

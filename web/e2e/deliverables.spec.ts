import { test, expect, type Page } from '@playwright/test'
import { mockWorkspace, firstID, person } from './fixtures'
import type { components } from '../src/api/schema.gen'

type Submission = components['schemas']['SubmissionPage']
type Draft = components['schemas']['SubmissionDraft']
type Evidence = components['schemas']['SubmissionEvidence']
type Prep = components['schemas']['PreparationPage']
const teamID = '164a3cd9-7b9e-45da-b6c7-1b797f11f70c', repoID = '621b317a-b992-44d6-a52c-42e49b7c0fa1', notesID = 'fae73e34-8238-45da-b72c-b23a998ddc9f', fileID = 'e7c04139-482e-4b02-bbb2-2674f6acfbbe', taskID = '400407c1-e157-41db-8604-c7c540c215ec'
const otherPerson = '735d0abd-716c-4a07-8a2f-8af13818eaa8'
function repository(label = 'Aurora repository', url = 'https://example.test/aurora'): Evidence { return { deliverable_id: repoID, kind: 'repository', label, url, file_id: null } }
async function setup(page: Page, { live = true, organiser = false, frozen = false, disbanded = false, noTeam = false, published = true } = {}) {
  const workspace = await mockWorkspace(page), event = workspace.events[0]
  event.stage = live ? 'live' : 'preparation'; event.published_at = published ? '2027-06-13T09:00:00Z' : null; event.confirmed_attendees = 32; event.can_edit = organiser; event.can_read_staff = organiser; event.can_transition = organiser; event.can_manage_owner = organiser
  const team: components['schemas']['HackTeam'] = { id: teamID, name: 'Aurora', state: disbanded ? 'disbanded' : noTeam ? 'draft' : 'published', frozen: true, captain: person.id, published_captain: person.id, invitation_code: null, draft: [], published: [{ id: person.id, name: person.display_name }] }
  const value: Submission = { event_revision: event.revision, revision: 0, entry_id: 'b42bf250-a254-4497-ac23-20b591656d30', team_id: teamID, outcome: frozen ? 'frozen' : 'open', frozen_at: frozen ? event.configuration.submission_cutoff : null, can_draft: published && !organiser && !frozen, can_read_draft: published && !organiser, can_submit: published && live && (!frozen || organiser), can_review: false, live_start: event.configuration.live_start, cutoff: event.configuration.submission_cutoff, organisers: [{ id: otherPerson, name: 'Jane Organiser' }], definitions: [{ id: repoID, title: 'Repository', description: 'Share your project reference.', required: true, due_at: event.configuration.submission_cutoff, evidence_kinds: ['repository'] }, { id: notesID, title: 'Technical notes', description: 'Explain your choices.', required: false, due_at: event.configuration.submission_cutoff, evidence_kinds: ['file', 'url'] }], final: null }
  if (frozen) { value.revision = 2; value.final = { revision: 1, actor_id: person.id, submitted_at: event.configuration.live_start, late: false, reason: '', review_status: 'changes_requested', definitions: value.definitions, evidence: [repository()] }; value.can_review = organiser }
  const draft: Draft = { revision: 0, evidence: [], updated_at: null }
  const prep: Prep = { revision: event.revision, timezone: event.configuration.timezone, can_manage: organiser, items: [{ definition: { id: taskID, title: 'Prepare your laptop', description: 'Check the development tools.', owner_id: person.id, owner_role: 'attendee', status: 'not_started', schedule_id: null, fixed_due_at: '2027-06-19T09:00:00Z', depends_on: [], checklist: [{ id: '7c54d99b-9d9a-494d-b412-702cc0f7dc84', title: 'Install tooling', complete: false }], evidence: [] }, reason: '', override_dependencies: false, revision: 1, due_at: '2027-06-19T09:00:00Z', blocked: false, overdue: false, scheduling_issue: false, unowned: false, can_edit: true, restricted_evidence_count: 1, hidden_dependencies: 0 }] }
  const history: components['schemas']['SubmissionChange'][] = frozen ? [{ revision: 2, actor_id: otherPerson, action: 'reviewed', active_revision: 1, review_status: 'changes_requested', reason: 'Add installation instructions.', at: '2027-06-20T12:00:00Z', submission: null }] : []
  const writes: { path: string; body: unknown; revision?: string; key?: string }[] = [], reads: string[] = [], files = new Map<string, string>()
  let failUpload = false, denied = false, cutoff = false
  await page.route(`**/events/${firstID}/**`, async route => {
    const r = route.request(), path = new URL(r.url()).pathname
    if (r.method() === 'GET') reads.push(path)
    const fail = (status: number, code: string, blockers?: components['schemas']['Blocker'][]) => route.fulfill({ status, json: { error: { code, message: 'Synthetic rejection', request_id: 'synthetic', blockers } } })
    if (path.endsWith('/teams')) return route.fulfill({ json: { revision: event.revision, can_manage: organiser, can_create: false, frozen: true, freeze_at: event.configuration.team_freeze, teams: [team], people: [], requests: [] } })
    if (path.includes('/preparation/tasks')) {
      if (r.method() === 'GET') return route.fulfill({ json: prep })
      const body = r.postDataJSON(); writes.push({ path, body, revision: r.headers()['if-match'] })
      if (r.headers()['if-match'] !== `"${prep.revision}"`) return fail(409, 'revision_conflict')
      Object.assign(prep.items[0], body); prep.revision++; prep.items[0].revision++; event.revision = prep.revision
      return route.fulfill({ json: prep })
    }
    if (path.endsWith('/files') && r.method() === 'POST') {
      const body = r.postDataJSON(), key = r.headers()['idempotency-key']; writes.push({ path, body, revision: r.headers()['if-match'], key })
      if (!files.has(key)) { files.set(key, fileID); event.revision++ }
      if (failUpload) { failUpload = false; return fail(503, 'unavailable') }
      return route.fulfill({ status: 201, json: { id: files.get(key), filename: body.filename, media_type: body.media_type, size: 23, checksum: 'synthetic' } })
    }
    if (!path.includes('/submission')) return route.fallback()
    if (denied) return fail(403, 'forbidden')
    if (path.endsWith('/history')) return route.fulfill({ json: { items: history, next_cursor: null } })
    if (r.method() === 'GET') return route.fulfill({ json: path.endsWith('/draft') ? draft : value })
    const body = r.postDataJSON(); writes.push({ path, body, revision: r.headers()['if-match'] })
    if (cutoff) { value.outcome = value.final ? 'frozen' : 'not_submitted'; value.frozen_at = value.cutoff; value.can_draft = false; value.can_submit = false; value.revision++; cutoff = false; return fail(409, 'event_blocked', [{ field: 'submission_cutoff', code: 'deadline_locked', message: 'The shared submission cutoff has passed. Contact an organiser about an exception.' }]) }
    const isDraft = path.endsWith('/draft')
    if (r.headers()['if-match'] !== `"${isDraft ? draft.revision : value.revision}"`) return fail(409, 'revision_conflict')
    if (isDraft) { draft.revision++; draft.evidence = body.evidence; draft.updated_at = '2027-06-14T09:00:00Z'; return route.fulfill({ json: draft }) }
    value.revision++
    if (path.endsWith('/review')) { value.final!.review_status = body.status; history.unshift({ revision: value.revision, actor_id: person.id, action: 'reviewed', active_revision: value.final!.revision, review_status: body.status, reason: body.reason, at: event.configuration.live_start, submission: null }) }
    else { value.final = { revision: value.revision, actor_id: person.id, submitted_at: event.configuration.live_start, late: !!value.frozen_at, reason: body.reason, review_status: 'pending', definitions: value.definitions, evidence: body.evidence }; value.can_review = organiser; history.unshift({ revision: value.revision, actor_id: person.id, action: 'submitted', active_revision: value.revision, review_status: 'pending', reason: body.reason, at: event.configuration.live_start, submission: value.final }) }
    return route.fulfill({ json: value })
  })
  await page.goto(`/events/${firstID}`); await page.getByRole('button', { name: 'Deliverables', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Deliverables', exact: true })).toBeVisible()
  return { value, draft, prep, team, event, writes, reads, files, failUpload: () => { failUpload = true }, deny: () => { denied = true }, cutoff: () => { cutoff = true } }
}
async function fillRepository(page: Page, url = 'https://example.test/aurora') {
  await page.getByLabel('Evidence type for Repository', { exact: true }).selectOption('repository')
  await page.getByLabel('Reference URL for Repository', { exact: true }).fill(url)
}

test('shows required work, submits labelled file and repository evidence, and retries a lost upload response safely', async ({ page }, info) => {
  const state = await setup(page)
  await expect(page.getByRole('button', { name: 'Submit formal version' })).toBeDisabled()
  await expect(page.getByText('Required evidence still missing: Repository.', { exact: false })).toBeVisible()
  await fillRepository(page); await page.getByLabel('Evidence label for Repository', { exact: true }).fill('Our project repository')
  await page.getByLabel('Evidence type for Technical notes', { exact: true }).selectOption('file')
  await page.getByLabel('File for Technical notes', { exact: true }).setInputFiles({ name: 'architecture.txt', mimeType: 'text/plain', buffer: Buffer.from('Synthetic design notes') })
  state.failUpload(); await page.getByRole('button', { name: 'Upload Technical notes', exact: true }).click()
  await expect(page.getByText('Your other fields are retained.', { exact: false })).toBeVisible()
  await expect(page.getByLabel('Evidence label for Repository', { exact: true })).toHaveValue('Our project repository')
  await expect(page.getByLabel('Reference URL for Repository', { exact: true })).toHaveValue('https://example.test/aurora')
  await page.getByRole('button', { name: 'Upload Technical notes', exact: true }).click()
  await expect(page.getByText('architecture.txt uploaded.', { exact: false })).toBeVisible()
  const uploads = state.writes.filter(w => w.path.endsWith('/files'))
  expect(uploads).toHaveLength(2); expect(uploads[0].key).toBe(uploads[1].key); expect(uploads.map(w => w.revision)).toEqual(['"1"', '"2"']); expect(state.files.size).toBe(1)
  expect((uploads[0].body as { team_id: string }).team_id).toBe(teamID)
  await page.getByRole('button', { name: 'Submit formal version' }).click()
  await expect(page.getByRole('region', { name: 'Current formal submission' })).toContainText('Formal version 1')
  await expect(page.getByRole('region', { name: 'Current formal submission' }).getByRole('link', { name: 'Technical notes' })).toHaveAttribute('href', `/api/v1/events/${firstID}/files/${fileID}`)
  await expect(page.getByRole('button', { name: 'Submit formal version' })).toBeDisabled()
  expect(state.value.final?.evidence).toHaveLength(2)
  await page.screenshot({ path: info.outputPath('team-deliverables.png'), fullPage: true })
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
})

test('pre-live preparation stays a private draft and never calls formal review', async ({ page }) => {
  const state = await setup(page, { live: false })
  await fillRepository(page)
  await expect(page.getByRole('button', { name: 'Submit formal version' })).toBeDisabled()
  await page.getByRole('button', { name: 'Save private draft' }).click()
  await expect(page.getByText('Private draft saved. No formal submission was accepted or reviewed.')).toBeVisible()
  expect(state.writes.map(w => w.path.endsWith('/draft'))).toEqual([true]); expect(state.value.final).toBeNull()
  expect(state.reads.some(p => p.endsWith('/history') || p.endsWith('/review'))).toBe(false)
})

test('cutoff locks preserve unsaved fields and identify organisers for an exception', async ({ page }) => {
  const state = await setup(page); await fillRepository(page)
  state.cutoff(); await page.getByRole('button', { name: 'Submit formal version' }).click()
  await expect(page.getByText('No formal submission was recorded by the cutoff.', { exact: false })).toBeVisible()
  await expect(page.getByLabel('Reference URL for Repository', { exact: true })).toHaveValue('https://example.test/aurora')
  await expect(page.getByLabel('Reference URL for Repository', { exact: true })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Submit formal version' })).toBeDisabled()
  await page.getByText('Contact an organiser about an exception', { exact: true }).click()
  await expect(page.locator('.organiser-help')).toContainText('Jane Organiser')
  await expect(page.locator('.organiser-help')).toContainText(teamID)
  await expect(page.locator('.organiser-help')).toContainText('not submitted')
})

test('frozen evidence and completion feedback are visible without soliciting replacements', async ({ page }) => {
  const state = await setup(page, { frozen: true })
  await expect(page.getByText('Final evidence is frozen at the cutoff.', { exact: false })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Current formal submission' })).toContainText('Completion review: changes requested')
  await expect(page.getByRole('region', { name: 'Submission history and feedback' })).toContainText('Add installation instructions.')
  await expect(page.getByRole('button', { name: 'Submit formal version' })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Save private draft' })).toBeDisabled()
  expect(state.writes).toHaveLength(0)
})

test('conflicts require reviewing the latest formal evidence before preserving and resubmitting edits', async ({ page }) => {
  const state = await setup(page); await fillRepository(page)
  state.value.revision = 1; state.value.final = { revision: 1, actor_id: otherPerson, submitted_at: state.value.live_start, late: false, reason: '', review_status: 'pending', definitions: state.value.definitions, evidence: [repository('Colleague repository', 'https://example.test/colleague')] }
  await page.getByRole('button', { name: 'Submit formal version' }).click()
  await expect(page.getByRole('button', { name: 'Submit formal version' })).toBeDisabled()
  await expect(page.getByLabel('Reference URL for Repository', { exact: true })).toHaveValue('https://example.test/aurora')
  await page.getByRole('button', { name: 'Review latest evidence' }).click()
  await expect(page.getByRole('region', { name: 'Latest saved evidence' })).toContainText('Colleague repository')
  await expect(page.getByRole('button', { name: 'Keep my evidence edits' })).toBeDisabled()
  await page.getByLabel('I reviewed the latest saved evidence').check(); await page.getByRole('button', { name: 'Keep my evidence edits' }).click()
  await page.getByRole('button', { name: 'Submit formal version' }).click()
  await expect(page.getByText('Formal version 2 submitted.', { exact: false })).toBeVisible()
  expect(state.writes.map(w => w.revision)).toEqual(['"0"', '"1"'])
})

test('disbanded teams have no submission solicitation or draft requests', async ({ page }) => {
  const state = await setup(page, { disbanded: true })
  await expect(page.getByText('Aurora is disbanded.', { exact: false })).toBeVisible()
  await expect(page.getByRole('form', { name: 'Team evidence editor' })).toHaveCount(0)
  expect(state.reads.some(p => p.includes('/submission'))).toBe(false)
})

test('personal preparation updates only assigned progress and references without privileged queries', async ({ page }) => {
  const state = await setup(page, { noTeam: true })
  await page.getByRole('button', { name: 'Open Prepare your laptop' }).click()
  await expect(page.getByRole('button', { name: 'Save preparation progress' })).toBeDisabled()
  await page.getByLabel('Preparation status').selectOption('complete'); await page.getByLabel('Install tooling').check()
  await page.getByLabel('Progress reason').fill('Tools checked locally.')
  await page.getByRole('button', { name: 'Add preparation reference' }).click()
  await page.getByLabel('Reference label 1', { exact: true }).fill('Tooling guide'); await page.getByLabel('Preparation reference URL 1').fill('https://example.test/tooling')
  await page.getByRole('button', { name: 'Save preparation progress' }).click()
  await expect(page.getByText('Preparation progress saved.')).toBeVisible()
  const body = state.writes[0].body as components['schemas']['PreparationUpdate']
  expect(body.override_dependencies).toBe(false); expect(body.definition.title).toBe('Prepare your laptop'); expect(body.definition.owner_id).toBe(person.id); expect(body.definition.status).toBe('complete')
  expect(state.reads.some(p => /\/(staff|logistics|registrations)/.test(p))).toBe(false)
})

test('organiser review and late amendment require recorded reasons and cannot read private drafts', async ({ page }) => {
  const state = await setup(page, { organiser: true, frozen: true })
  await expect(page.getByRole('button', { name: 'Save private draft' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Record completion feedback' })).toBeDisabled()
  await page.getByLabel('Feedback and reason').fill('Documentation now complete.'); await page.getByLabel('Completion decision').selectOption('accepted')
  await page.getByRole('button', { name: 'Record completion feedback' }).click()
  await expect(page.getByText('Completion feedback recorded.')).toBeVisible()
  await page.getByRole('button', { name: 'Review latest evidence' }).click(); await page.getByLabel('I reviewed the latest saved evidence').check(); await page.getByRole('button', { name: 'Keep my evidence edits' }).click()
  await page.getByLabel('Reference URL for Repository', { exact: true }).fill('https://example.test/late-correction')
  await expect(page.getByRole('button', { name: 'Submit formal version' })).toBeDisabled()
  await page.getByLabel('Organiser amendment reason').fill('Approved outage exception.')
  await page.getByRole('button', { name: 'Submit formal version' }).click()
  await expect(page.getByRole('region', { name: 'Current formal submission' })).toContainText('Late amendment')
  expect(state.reads.some(p => p.endsWith('/draft'))).toBe(false)
  expect((state.writes.at(-1)?.body as { reason: string }).reason).toBe('Approved outage exception.')
})

test('revoked membership disables mutations and hides the former private editor', async ({ page }) => {
  const state = await setup(page); await fillRepository(page); state.deny()
  await page.getByRole('button', { name: 'Save private draft' }).click()
  await expect(page.getByRole('alert')).toContainText('You do not have access')
  await expect(page.getByRole('form', { name: 'Team evidence editor' })).toHaveCount(0)
})


test('unpublished challenge permits viewing requirements but no draft or formal evidence requests', async ({ page }) => {
  const state = await setup(page, { live: false, published: false })
  await expect(page.getByText('The challenge must be published', { exact: false })).toBeVisible()
  await expect(page.getByRole('form', { name: 'Team evidence editor' })).toHaveCount(0)
  await expect(page.getByRole('heading', { name: 'Required and optional work' })).toBeVisible()
  expect(state.reads.some(p => p.endsWith('/draft') || p.endsWith('/history'))).toBe(false)
})

test('private draft conflicts preserve fields and use the reviewed draft revision on retry', async ({ page }) => {
  const state = await setup(page, { live: false }); await fillRepository(page)
  state.draft.revision = 1; state.draft.evidence = [repository('Colleague draft', 'https://example.test/draft')]
  await page.getByRole('button', { name: 'Save private draft' }).click()
  await expect(page.getByLabel('Reference URL for Repository', { exact: true })).toHaveValue('https://example.test/aurora')
  await page.getByRole('button', { name: 'Review latest evidence' }).click()
  await expect(page.getByRole('region', { name: 'Latest saved evidence' })).toContainText('Colleague draft')
  await page.getByLabel('I reviewed the latest saved evidence').check(); await page.getByRole('button', { name: 'Keep my evidence edits' }).click()
  await page.getByRole('button', { name: 'Save private draft' }).click()
  await expect(page.getByText('Private draft saved.', { exact: false })).toBeVisible()
  expect(state.writes.map(w => w.revision)).toEqual(['"0"', '"1"']); expect(state.value.final).toBeNull()
})

test('personal preparation conflict retains progress while preserving changed assignment definitions', async ({ page }) => {
  const state = await setup(page, { noTeam: true })
  await page.getByRole('button', { name: 'Open Prepare your laptop' }).click()
  await page.getByLabel('Preparation status').selectOption('in_progress')
  state.prep.revision = 2; state.prep.items[0].definition.title = 'Prepare the updated toolchain'
  await page.getByRole('button', { name: 'Save preparation progress' }).click()
  await page.getByRole('button', { name: 'Review latest preparation' }).click()
  await expect(page.getByRole('region', { name: 'Latest preparation task' })).toContainText('Prepare the updated toolchain')
  await page.getByLabel('I reviewed the latest preparation task').check(); await page.getByRole('button', { name: 'Keep my preparation progress' }).click()
  await page.getByRole('button', { name: 'Save preparation progress' }).click()
  await expect(page.getByText('Preparation progress saved.')).toBeVisible()
  const body = state.writes.at(-1)!.body as components['schemas']['PreparationUpdate']
  expect(body.definition.title).toBe('Prepare the updated toolchain'); expect(body.definition.status).toBe('in_progress'); expect(state.writes.at(-1)?.revision).toBe('"2"')
})

import { uploadedFile } from './file-fixtures'
import { openEventSection } from './event-navigation-fixture'
import { expect, type Page } from '@playwright/test'
import { mockWorkspace, firstID, person } from './fixtures'
import type { components } from '../src/api/schema.gen'

type Submission = components['schemas']['SubmissionPage']
type Draft = components['schemas']['SubmissionDraft']
type Evidence = components['schemas']['SubmissionEvidence']
type Prep = components['schemas']['PreparationPage']
export const teamID = '164a3cd9-7b9e-45da-b6c7-1b797f11f70c', repoID = '621b317a-b992-44d6-a52c-42e49b7c0fa1', notesID = 'fae73e34-8238-45da-b72c-b23a998ddc9f', fileID = 'e7c04139-482e-4b02-bbb2-2674f6acfbbe', taskID = '400407c1-e157-41db-8604-c7c540c215ec'
export const otherPerson = '735d0abd-716c-4a07-8a2f-8af13818eaa8'
export function repository(label = 'Aurora repository', url = 'https://example.test/aurora'): Evidence { return { deliverable_id: repoID, kind: 'repository', label, url, file_id: null } }
export async function setup(page: Page, { live = true, organiser = false, frozen = false, disbanded = false, noTeam = false, published = true, canary = false, participant = false } = {}) {
  const workspace = await mockWorkspace(page), event = workspace.events[0]
  event.stage = live ? 'live' : 'preparation'; event.published_at = published ? '2027-06-13T09:00:00Z' : null; event.confirmed_attendees = 32; event.can_edit = organiser; event.can_read_staff = organiser; event.can_transition = organiser; event.can_manage_owner = organiser
  const team: components['schemas']['HackTeam'] = { id: teamID, name: 'Aurora', state: disbanded ? 'disbanded' : noTeam ? 'draft' : 'published', frozen: true, captain: person.id, published_captain: person.id, invitation_code: null, draft: [], published: [{ id: person.id, name: person.display_name }] }
  const value: Submission = { event_revision: event.revision, revision: 0, entry_id: 'b42bf250-a254-4497-ac23-20b591656d30', team_id: teamID, outcome: frozen ? 'frozen' : 'open', frozen_at: frozen ? event.configuration.submission_cutoff : null, can_draft: published && !organiser && !frozen, can_read_draft: published && !organiser, can_submit: published && live && (!frozen || organiser), can_review: false, live_start: event.configuration.live_start, cutoff: event.configuration.submission_cutoff, organisers: [{ id: otherPerson, name: 'Jane Organiser' }], definitions: [{ id: repoID, title: 'Repository', description: 'Share your project reference.', required: true, due_at: event.configuration.submission_cutoff, evidence_kinds: ['repository'] }, { id: notesID, title: 'Technical notes', description: 'Explain your choices.', required: false, due_at: event.configuration.submission_cutoff, evidence_kinds: ['file', 'url'] }], final: null }
  if (canary) { value.definitions[0].title = 'SYNTHETIC-THEME-CANARY'; value.definitions[0].description = 'SYNTHETIC-EVIDENCE-INSTRUCTIONS-CANARY' }
  if (participant) await page.route('**/api/v1/session', route => route.fulfill({ json: { person: { ...person, platform_admin: false }, csrf_token: 'synthetic-csrf-token-32-characters', can_create_events: false } }))
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
      return route.fulfill({ status: 201, json: uploadedFile(files.get(key)!, body) })
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
  await page.goto(`/events/${firstID}`); await openEventSection(page, 'Deliverables')
  await expect(page.getByRole('heading', { name: 'Deliverables', exact: true })).toBeVisible()
  return { value, draft, prep, team, event, writes, reads, files, failUpload: () => { failUpload = true }, deny: () => { denied = true }, cutoff: () => { cutoff = true } }
}

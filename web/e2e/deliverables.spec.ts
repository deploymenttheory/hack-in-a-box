import { test, expect, type Page } from '@playwright/test'
import { firstID, person } from './fixtures'
import type { components } from '../src/api/schema.gen'
import { setup, repository, teamID, fileID, otherPerson } from './deliverable-fixtures'
async function fillRepository(page: Page, url = 'https://example.test/aurora') {
  await page.getByLabel('Evidence type for Repository', { exact: true }).selectOption('repository')
  await page.getByLabel('Reference URL for Repository', { exact: true }).fill(url)
}

test('shows required work, submits labelled file and repository evidence, and retries a lost upload response safely', async ({ page }, info) => {
  const state = await setup(page)
  await expect(page.getByRole('button', { name: 'Submit formal version' })).toBeDisabled()
  await expect(page.getByText('Required evidence still missing: Repository.', { exact: false })).toBeVisible()
  await fillRepository(page); await page.getByLabel('Evidence label for Repository', { exact: true }).fill('Our project repository')
  await page.getByRole('button', { name: 'Edit evidence for Technical notes', exact: true }).click()
  await page.getByLabel('Evidence type for Technical notes', { exact: true }).selectOption('file')
  await page.getByLabel('File for Technical notes', { exact: true }).setInputFiles({ name: 'architecture.txt', mimeType: 'text/plain', buffer: Buffer.from('Synthetic design notes') })
  state.failUpload(); await page.getByRole('button', { name: 'Upload Technical notes', exact: true }).click()
  await expect(page.getByText('Upload outcome unknown', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Edit evidence for Repository', exact: true })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Submit formal version' })).toBeDisabled()
  await page.getByRole('button', { name: 'Upload Technical notes', exact: true }).click()
  await expect(page.getByText(`File ${fileID} uploaded.`, { exact: false })).toBeVisible()
  await page.getByRole('button', { name: 'Edit evidence for Repository', exact: true }).click()
  await expect(page.getByLabel('Evidence label for Repository', { exact: true })).toHaveValue('Our project repository')
  await expect(page.getByLabel('Reference URL for Repository', { exact: true })).toHaveValue('https://example.test/aurora')
  const uploads = state.writes.filter(w => w.path.endsWith('/files'))
  expect(uploads).toHaveLength(2); expect(uploads[0].body).toEqual(uploads[1].body); expect(uploads[0].key).toBe(uploads[1].key); expect(uploads.map(w => w.revision)).toEqual(['"1"', '"1"']); expect(state.files.size).toBe(1)
  expect((uploads[0].body as { team_id: string }).team_id).toBe(teamID)
  await page.getByRole('button', { name: 'Submit formal version' }).click(); await page.getByRole('button', { name: 'Confirm formal submission', exact: true }).click()
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
  await page.getByRole('button', { name: 'Save private draft' }).click(); await page.getByRole('button', { name: 'Confirm private draft', exact: true }).click()
  await expect(page.getByText('Private draft saved. No formal submission was accepted or reviewed.')).toBeVisible()
  expect(state.writes.map(w => w.path.endsWith('/draft'))).toEqual([true]); expect(state.value.final).toBeNull()
  expect(state.reads.some(p => p.endsWith('/history') || p.endsWith('/review'))).toBe(false)
})

test('cutoff locks preserve unsaved fields and identify organisers for an exception', async ({ page }) => {
  const state = await setup(page); await fillRepository(page)
  state.cutoff(); await page.getByRole('button', { name: 'Submit formal version' }).click(); await page.getByRole('button', { name: 'Confirm formal submission', exact: true }).click()
  await expect(page.getByText('No formal submission was recorded by the cutoff.', { exact: false })).toBeVisible()
  await expect(page.getByLabel('Reference URL for Repository', { exact: true })).toHaveValue('https://example.test/aurora')
  await expect(page.getByLabel('Reference URL for Repository', { exact: true })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Submit formal version' })).toBeDisabled()
  await page.getByText('Contact an organiser about an exception', { exact: true }).click()
  await expect(page.locator('details').filter({ has: page.getByText('Contact an organiser about an exception', { exact: true }) })).toContainText('Jane Organiser')
  await expect(page.locator('details').filter({ has: page.getByText('Contact an organiser about an exception', { exact: true }) })).toContainText(teamID)
  await expect(page.locator('details').filter({ has: page.getByText('Contact an organiser about an exception', { exact: true }) })).toContainText('not submitted')
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
  await page.getByRole('button', { name: 'Submit formal version' }).click(); await page.getByRole('button', { name: 'Confirm formal submission', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Submit formal version' })).toBeDisabled()
  await expect(page.getByLabel('Reference URL for Repository', { exact: true })).toHaveValue('https://example.test/aurora')
  await page.getByRole('button', { name: 'Read current state' }).click()
  await expect(page.getByRole('region', { name: 'Latest saved evidence' })).toContainText('Colleague repository')
  await expect(page.getByRole('button', { name: 'Keep my evidence edits' })).toBeDisabled()
  await page.getByLabel('I reviewed the latest saved evidence').check(); await page.getByRole('button', { name: 'Keep my evidence edits' }).click()
  await page.getByRole('button', { name: 'Submit formal version' }).click(); await page.getByRole('button', { name: 'Confirm formal submission', exact: true }).click()
  await expect(page.getByText('Formal version 2 submitted.', { exact: false })).toBeVisible()
  expect(state.writes.map(w => w.revision)).toEqual(['"0"', '"1"'])
})

test('disbanded teams have no submission solicitation or draft requests', async ({ page }) => {
  const state = await setup(page, { disbanded: true })
  await expect(page.getByText('Aurora is disbanded.', { exact: false })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Team evidence editor' })).toHaveCount(0)
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
  await page.getByRole('button', { name: 'Save preparation progress' }).click(); await page.getByRole('button', { name: 'Confirm preparation progress', exact: true }).click()
  await expect(page.getByText('Preparation progress saved', { exact: false })).toBeVisible()
  const body = state.writes[0].body as components['schemas']['PreparationUpdate']
  expect(body.override_dependencies).toBe(false); expect(body.definition.title).toBe('Prepare your laptop'); expect(body.definition.owner_id).toBe(person.id); expect(body.definition.status).toBe('complete')
  expect(state.reads.some(p => /\/(staff|logistics|registrations)/.test(p))).toBe(false)
})

test('organiser review and late amendment require recorded reasons and cannot read private drafts', async ({ page }) => {
  const state = await setup(page, { organiser: true, frozen: true })
  await expect(page.getByRole('button', { name: 'Save private draft' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Record completion feedback' })).toBeDisabled()
  await page.getByLabel('Feedback and reason').fill('Documentation now complete.'); await page.getByLabel('Completion decision').selectOption('accepted')
  await page.getByRole('button', { name: 'Record completion feedback' }).click(); await page.getByRole('button', { name: 'Confirm completion feedback', exact: true }).click()
  await expect(page.getByText('Completion feedback recorded', { exact: false })).toBeVisible()
  await page.getByRole('button', { name: 'Review latest evidence' }).click(); await page.getByLabel('I reviewed the latest saved evidence').check(); await page.getByRole('button', { name: 'Keep my evidence edits' }).click()
  await page.getByLabel('Reference URL for Repository', { exact: true }).fill('https://example.test/late-correction')
  await expect(page.getByRole('button', { name: 'Submit formal version' })).toBeDisabled()
  await page.getByLabel('Organiser amendment reason').fill('Approved outage exception.')
  await page.getByRole('button', { name: 'Submit formal version' }).click(); await page.getByRole('button', { name: 'Confirm formal submission', exact: true }).click()
  await expect(page.getByRole('region', { name: 'Current formal submission' })).toContainText('Late amendment')
  expect(state.reads.some(p => p.endsWith('/draft'))).toBe(false)
  expect((state.writes.at(-1)?.body as { reason: string }).reason).toBe('Approved outage exception.')
})

test('revoked membership disables mutations and hides the former private editor', async ({ page }) => {
  const state = await setup(page); await fillRepository(page); state.deny()
  await page.getByRole('button', { name: 'Save private draft' }).click(); await page.getByRole('button', { name: 'Confirm private draft', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('You do not have access')
  await expect(page.getByRole('region', { name: 'Team evidence editor' })).toHaveCount(0)
})


test('unpublished challenge withholds competition prose without hiding personal preparation', async ({ page }) => {
  const state = await setup(page, { live: false, published: false, canary: true })
  await expect(page.getByText('The challenge must be published', { exact: false })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Team evidence editor' })).toHaveCount(0)
  await expect(page.getByText('Competition requirements will appear after the common challenge is published.', { exact: false })).toBeVisible()
  await expect(page.getByText(/SYNTHETIC-THEME-CANARY|SYNTHETIC-EVIDENCE-INSTRUCTIONS-CANARY/)).toHaveCount(0)
  await expect(page.getByText('Prepare your laptop', { exact: true })).toBeVisible()
  expect(state.reads.some(p => p.endsWith('/draft') || p.endsWith('/history'))).toBe(false)
})

test('private draft conflicts preserve fields and use the reviewed draft revision on retry', async ({ page }) => {
  const state = await setup(page, { live: false }); await fillRepository(page)
  state.draft.revision = 1; state.draft.evidence = [repository('Colleague draft', 'https://example.test/draft')]
  await page.getByRole('button', { name: 'Save private draft' }).click(); await page.getByRole('button', { name: 'Confirm private draft', exact: true }).click()
  await expect(page.getByLabel('Reference URL for Repository', { exact: true })).toHaveValue('https://example.test/aurora')
  await page.getByRole('button', { name: 'Read current state' }).click()
  await expect(page.getByRole('region', { name: 'Latest saved evidence' })).toContainText('Colleague draft')
  await page.getByLabel('I reviewed the latest saved evidence').check(); await page.getByRole('button', { name: 'Keep my evidence edits' }).click()
  await page.getByRole('button', { name: 'Save private draft' }).click(); await page.getByRole('button', { name: 'Confirm private draft', exact: true }).click()
  await expect(page.getByText('Private draft saved.', { exact: false })).toBeVisible()
  expect(state.writes.map(w => w.revision)).toEqual(['"0"', '"1"']); expect(state.value.final).toBeNull()
})

test('personal preparation conflict retains progress while preserving changed assignment definitions', async ({ page }) => {
  const state = await setup(page, { noTeam: true })
  await page.getByRole('button', { name: 'Open Prepare your laptop' }).click()
  await page.getByLabel('Preparation status').selectOption('in_progress')
  state.prep.revision = 2; state.prep.items[0].definition.title = 'Prepare the updated toolchain'
  await page.getByRole('button', { name: 'Save preparation progress' }).click(); await page.getByRole('button', { name: 'Confirm preparation progress', exact: true }).click()
  await page.getByRole('button', { name: 'Read current state' }).click()
  await expect(page.getByRole('region', { name: 'Latest preparation task' })).toContainText('Prepare the updated toolchain')
  await page.getByLabel('I reviewed the latest preparation task').check(); await page.getByRole('button', { name: 'Keep my preparation progress' }).click()
  await page.getByRole('button', { name: 'Save preparation progress' }).click(); await page.getByRole('button', { name: 'Confirm preparation progress', exact: true }).click()
  await expect(page.getByText('Preparation progress saved', { exact: false })).toBeVisible()
  const body = state.writes.at(-1)!.body as components['schemas']['PreparationUpdate']
  expect(body.definition.title).toBe('Prepare the updated toolchain'); expect(body.definition.status).toBe('in_progress'); expect(state.writes.at(-1)?.revision).toBe('"2"')
})

for (const formal of [false, true]) test(`unknown ${formal ? 'formal submission' : 'private draft'} retains evidence and requires a reviewed native base without replay`, async ({ page }) => {
  const state = await setup(page)
  await fillRepository(page)
  const endpoint = `/api/v1/events/${firstID}/teams/${teamID}/submission${formal ? '' : '/draft'}`
  const attempts: { revision: string; body: unknown; csrf: string }[] = []
  await page.route(`**${endpoint}`, async route => {
    if (route.request().method() !== 'PUT') return route.fallback()
    const request = route.request(), body = request.postDataJSON()
    attempts.push({ revision: request.headers()['if-match'], body, csrf: request.headers()['x-csrf-token'] })
    if (attempts.length > 1) return route.fallback()
    if (formal) {
      state.value.revision = 1
      state.value.final = { revision: 1, actor_id: person.id, submitted_at: state.value.live_start, late: false, reason: '', review_status: 'pending', definitions: state.value.definitions, evidence: body.evidence }
    } else { state.draft.revision = 1; state.draft.evidence = body.evidence; state.draft.updated_at = state.value.live_start }
    return route.fulfill({ status: 503, json: { error: { code: 'unavailable', request_id: 'synthetic' } } })
  })
  const action = formal ? 'Submit formal version' : 'Save private draft', confirm = formal ? 'Confirm formal submission' : 'Confirm private draft'
  await page.getByRole('button', { name: action, exact: true }).click()
  expect(attempts).toHaveLength(0)
  await page.getByRole('button', { name: confirm, exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Outcome unknown', exact: true })).toBeVisible()
  await expect(page.getByLabel('Reference URL for Repository', { exact: true })).toHaveValue('https://example.test/aurora')
  await expect(page.getByLabel('Reference URL for Repository', { exact: true })).toBeDisabled()
  await page.getByRole('button', { name: 'Read current state', exact: true }).click()
  await expect(page.getByRole('region', { name: 'Latest saved evidence' })).toContainText(formal ? 'Formal version 1' : 'Private draft 1')
  expect(attempts).toHaveLength(1)
  await expect(page.getByRole('button', { name: 'Keep my evidence edits', exact: true })).toBeDisabled()
  await page.getByLabel('I reviewed the latest saved evidence').check()
  await page.getByRole('button', { name: 'Keep my evidence edits', exact: true }).click()
  await expect(page.getByRole('region', { name: 'Earlier unconfirmed resource commands' })).toContainText('Expected revision 0 · Original outcome remains unconfirmed')
  await page.getByLabel('Reference URL for Repository', { exact: true }).fill('https://example.test/reviewed-change')
  await page.getByRole('button', { name: action, exact: true }).click()
  await page.getByRole('button', { name: confirm, exact: true }).click()
  await expect(page.getByText(formal ? 'Formal version 2 submitted.' : 'Private draft saved.', { exact: false })).toBeVisible()
  expect(attempts.map(value => value.revision)).toEqual(['"0"', '"1"'])
  expect(attempts.every(value => value.csrf === 'synthetic-csrf-token-32-characters')).toBe(true)
  expect((attempts[0].body as { evidence: { url: string }[] }).evidence[0].url).toBe('https://example.test/aurora')
  expect((attempts[1].body as { evidence: { url: string }[] }).evidence[0].url).toBe('https://example.test/reviewed-change')
})

test('a deferred file read cannot send a new upload after its session scope ends', async ({ page }) => {
  const state = await setup(page)
  await page.evaluate(() => {
    const original = FileReader.prototype.readAsDataURL
    FileReader.prototype.readAsDataURL = function(blob) { this.addEventListener('loadend', () => { (window as unknown as { fileDone: boolean }).fileDone = true }); (window as unknown as { finishFile: () => void }).finishFile = () => original.call(this, blob) }
  })
  await page.getByRole('button', { name: 'Edit evidence for Technical notes', exact: true }).click()
  await page.getByLabel('Evidence type for Technical notes', { exact: true }).selectOption('file')
  await page.getByLabel('File for Technical notes', { exact: true }).setInputFiles({ name: 'private-notes.txt', mimeType: 'text/plain', buffer: Buffer.from('SYNTHETIC-PRIVATE-FILE-CANARY') })
  await page.getByRole('button', { name: 'Upload Technical notes', exact: true }).click()
  await expect.poll(() => page.evaluate(() => typeof (window as unknown as { finishFile?: () => void }).finishFile)).toBe('function')
  await page.evaluate(() => { window.dispatchEvent(new Event('session-ending')); (window as unknown as { finishFile: () => void }).finishFile() })
  await expect.poll(() => page.evaluate(() => (window as unknown as { fileDone: boolean }).fileDone)).toBe(true)
  expect(state.writes.filter(value => value.path.endsWith('/files'))).toHaveLength(0)
  expect(await page.evaluate(() => JSON.stringify({ ...localStorage, ...sessionStorage }))).not.toContain('SYNTHETIC-PRIVATE-FILE-CANARY')
})

test('a second organiser formal revision requires reviewing new evidence and retains feedback', async ({ page }) => {
  const state = await setup(page, { organiser: true, frozen: true })
  await page.getByLabel('Feedback and reason').fill('Retained review reason.')
  await page.getByLabel('Completion decision', { exact: true }).selectOption('accepted')
  await page.getByRole('button', { name: 'Record completion feedback', exact: true }).click()
  expect(state.writes).toHaveLength(0)
  state.value.revision = 3; state.value.final!.revision = 3; state.value.final!.evidence = [repository('New formal evidence', 'https://example.test/new-formal')]
  await page.getByRole('button', { name: 'Confirm completion feedback', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Current version needed', exact: true })).toBeVisible()
  await expect(page.getByLabel('Feedback and reason')).toHaveValue('Retained review reason.')
  await page.getByRole('button', { name: 'Read current state', exact: true }).click()
  await expect(page.getByRole('region', { name: 'Current completion review evidence' })).toContainText('New formal evidence')
  await expect(page.getByRole('button', { name: 'Use latest review state', exact: true })).toBeDisabled()
  await page.getByLabel('I reviewed the current formal evidence').check()
  await page.getByRole('button', { name: 'Use latest review state', exact: true }).click()
  await expect(page.getByLabel('Feedback and reason')).toHaveValue('Retained review reason.')
  await page.getByRole('button', { name: 'Record completion feedback', exact: true }).click()
  await page.getByRole('button', { name: 'Confirm completion feedback', exact: true }).click()
  await expect(page.getByText('Completion feedback recorded for formal version 3.', { exact: true })).toBeVisible()
  expect(state.writes.map(value => value.revision)).toEqual(['"2"', '"3"'])
  expect(state.writes.map(value => (value.body as { submission_revision: number }).submission_revision)).toEqual([1, 3])
})

test('denied completion write clears private feedback and review without replay', async ({ page }) => {
  const state = await setup(page, { organiser: true, frozen: true })
  await page.getByLabel('Feedback and reason').fill('SYNTHETIC-PRIVATE-REVIEW-CANARY')
  await page.getByRole('button', { name: 'Record completion feedback', exact: true }).click()
  state.deny()
  await page.getByRole('button', { name: 'Confirm completion feedback', exact: true }).click()
  await expect(page.getByRole('region', { name: 'Organiser completion review' })).toHaveCount(0)
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(page.getByText('SYNTHETIC-PRIVATE-REVIEW-CANARY', { exact: false })).toHaveCount(0)
  expect(state.writes).toHaveLength(0)
})

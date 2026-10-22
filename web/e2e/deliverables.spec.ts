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


test('unpublished challenge withholds competition prose without hiding personal preparation', async ({ page }) => {
  const state = await setup(page, { live: false, published: false, canary: true })
  await expect(page.getByText('The challenge must be published', { exact: false })).toBeVisible()
  await expect(page.getByRole('form', { name: 'Team evidence editor' })).toHaveCount(0)
  await expect(page.getByText('Competition requirements will appear after the common challenge is published.', { exact: false })).toBeVisible()
  await expect(page.getByText(/SYNTHETIC-THEME-CANARY|SYNTHETIC-EVIDENCE-INSTRUCTIONS-CANARY/)).toHaveCount(0)
  await expect(page.getByText('Prepare your laptop', { exact: true })).toBeVisible()
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

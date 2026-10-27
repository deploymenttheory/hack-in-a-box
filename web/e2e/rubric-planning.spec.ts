import { test, expect, type Page } from '@playwright/test'
import { firstID } from './fixtures'
import { openEventSection } from './event-navigation-fixture'
import { rubricPlanningFixture } from './rubric-planning-fixture'

async function open(page: Page) { await page.goto(`/events/${firstID}?section=judging%20criteria`); await expect(page.getByRole('heading', { name: 'Judging criteria', exact: true })).toBeVisible() }
const dialog = (page: Page) => page.getByRole('alertdialog')
async function draft(page: Page, name = 'Reviewed common event criteria') { await page.getByLabel('Rubric name', { exact: true }).fill(name); await page.getByLabel('Rubric decision reason', { exact: true }).fill('Review exact common assessment model'); await page.getByRole('button', { name: 'Review draft save', exact: true }).click() }

test('rubric draft and exact approval are separate reviewed versioned commands', async ({ page }, info) => {
  test.skip(info.project.metadata.portal !== 'administrator', 'Authoring belongs to the administrator portal')
  const state = await rubricPlanningFixture(page); await open(page)
  await page.getByLabel('Rubric name', { exact: true }).fill('Reviewed common event criteria')
  await page.getByLabel('Judging mode', { exact: true }).selectOption('mentor_qualification')
  await page.getByRole('spinbutton', { name: 'Finalist count', exact: true }).fill('4')
  await page.getByRole('spinbutton', { name: 'Reserve count', exact: true }).fill('2')
  await page.getByLabel('Rubric decision reason', { exact: true }).fill('Common criteria independently reviewed')
  await page.getByRole('button', { name: 'Review draft save', exact: true }).press('Enter')
  await expect(dialog(page)).toContainText('Starting revision 0 · Version 0'); await expect(dialog(page)).toContainText('Finalists 4 · Reserves 2'); expect(state.posts).toHaveLength(0)
  await dialog(page).getByRole('button', { name: 'Cancel', exact: true }).click(); await expect(page.getByRole('button', { name: 'Review draft save', exact: true })).toBeFocused()
  await page.getByRole('button', { name: 'Review draft save', exact: true }).click(); await dialog(page).getByRole('button', { name: 'Save reviewed draft', exact: true }).click()
  await expect(dialog(page)).toHaveCount(0); await expect(page.getByRole('status').filter({ hasText: 'Private draft saved' })).toBeVisible()
  expect(state.posts).toHaveLength(1); expect(state.posts[0].revision).toBe('"0"'); expect(state.configuration.definition.finalist_count).toBe(4); expect(state.configuration.approved).toBe(false)
  await page.getByLabel('Rubric decision reason', { exact: true }).fill('Approve exact saved version one')
  await page.getByRole('button', { name: 'Review version approval', exact: true }).click(); await expect(dialog(page)).toContainText('Starting revision 1 · Version 1'); expect(state.posts).toHaveLength(1)
  await dialog(page).getByRole('button', { name: 'Approve reviewed version', exact: true }).click(); await expect(page.getByRole('status').filter({ hasText: 'Exact rubric version approved' })).toBeVisible()
  expect(state.posts).toHaveLength(2); expect(state.posts[1]).toEqual({ revision: '"1"', body: { action: 'approve', version: 1, reason: 'Approve exact saved version one' } }); expect(state.configuration.published_version).toBeNull()
  await page.reload(); await expect(page.getByLabel('Rubric name', { exact: true })).toHaveValue('Reviewed common event criteria'); await expect(page.getByRole('button', { name: 'Review version approval', exact: true })).toBeDisabled()
})

test('rubric validation reveals the invalid category and retains entered bounds and evidence', async ({ page }, info) => {
  test.skip(info.project.metadata.portal !== 'administrator', 'Administrator editor')
  const state = await rubricPlanningFixture(page); await open(page)
  await page.getByRole('navigation', { name: 'Rubric definition sections', exact: true }).getByRole('button', { name: 'Categories', exact: true }).click()
  await page.getByText('Demo', { exact: true }).click()
  await page.getByLabel('Maximum score for category 1', { exact: true }).fill('0')
  await page.getByLabel('Public evidence guidance for category 1', { exact: true }).fill('Retained local evidence guidance')
  await page.getByRole('navigation', { name: 'Rubric definition sections', exact: true }).getByRole('button', { name: 'Model', exact: true }).click()
  await page.getByLabel('Rubric decision reason', { exact: true }).fill('Validate before save')
  await page.getByRole('button', { name: 'Review draft save', exact: true }).click()
  await expect(page.getByLabel('Maximum score for category 1', { exact: true })).toBeFocused(); await expect(page.getByLabel('Maximum score for category 1', { exact: true })).toHaveAttribute('aria-invalid', 'true')
  await expect(page.getByLabel('Public evidence guidance for category 1', { exact: true })).toHaveValue('Retained local evidence guidance'); expect(state.posts).toHaveLength(0)
})

test('a changed rubric revision cannot submit its old review; explicit current read retains local input', async ({ page }, info) => {
  test.skip(info.project.metadata.portal !== 'administrator', 'Administrator editor')
  const state = await rubricPlanningFixture(page); await open(page); await draft(page)
  state.configuration = { ...state.configuration, revision: 1, reference: { ...state.configuration.reference, version: 1 }, definition: { ...state.configuration.definition, name: 'Other organiser draft' } }
  await dialog(page).getByRole('button', { name: 'Save reviewed draft', exact: true }).click()
  await expect(dialog(page)).toHaveCount(0); await expect(page.getByRole('heading', { name: 'Current version needed', exact: true })).toBeVisible(); expect(state.posts).toHaveLength(0)
  await page.getByRole('button', { name: 'Check current status', exact: true }).click(); await expect(page.getByLabel('Rubric name', { exact: true })).toHaveValue('Reviewed common event criteria')
  await page.getByRole('button', { name: 'Review draft save', exact: true }).click(); await expect(dialog(page)).toContainText('Starting revision 1 · Version 1')
  await dialog(page).getByRole('button', { name: 'Save reviewed draft', exact: true }).click(); await expect(dialog(page)).toHaveCount(0); expect(state.posts).toHaveLength(1); expect(state.posts[0].revision).toBe('"1"')
})

test('a lost rubric response stays unknown after current reads and section remount without any replay', async ({ page }, info) => {
  test.skip(info.project.metadata.portal !== 'administrator', 'Administrator editor')
  const state = await rubricPlanningFixture(page); await open(page); await draft(page); state.lose = true
  await dialog(page).getByRole('button', { name: 'Save reviewed draft', exact: true }).click(); await expect(dialog(page)).toHaveCount(0)
  await expect(page.getByRole('heading', { name: 'Outcome unknown', exact: true })).toBeVisible(); expect(state.posts).toHaveLength(1)
  await page.getByRole('button', { name: 'Check current status', exact: true }).click(); await expect(page.getByRole('heading', { name: 'Outcome unknown', exact: true })).toBeVisible(); await expect(page.getByRole('button', { name: 'Review version approval', exact: true })).toBeDisabled(); expect(state.posts).toHaveLength(1)
  page.once('dialog', dialog => dialog.accept()); await openEventSection(page, 'Overview'); await openEventSection(page, 'Judging criteria')
  await expect(page.getByRole('heading', { name: 'Outcome unknown', exact: true })).toBeVisible(); await expect(page.getByRole('button', { name: 'Review draft save', exact: true })).toBeDisabled(); expect(state.posts).toHaveLength(1)
})

test('event authority loss fences a held rubric preflight and clears private guidance before any POST', async ({ page }, info) => {
  test.skip(info.project.metadata.portal !== 'administrator', 'Administrator editor')
  const state = await rubricPlanningFixture(page); await open(page); await draft(page)
  let release!: () => void, started!: () => void; state.held = new Promise<void>(resolve => { release = resolve }); const held = new Promise<void>(resolve => { started = resolve }); state.started = started
  await dialog(page).getByRole('button', { name: 'Save reviewed draft', exact: true }).click(); await held
  state.event = { ...state.event, can_edit: false, can_read_staff: false }; await page.evaluate(() => { window.dispatchEvent(new Event('focus')); window.dispatchEvent(new Event('visibilitychange')) })
  await expect(page.getByLabel('Rubric name', { exact: true })).toHaveCount(0); release(); state.held = undefined
  await expect(page.locator('body')).not.toContainText('Synthetic private guidance. Never use this fixture as native authority.'); expect(state.posts).toHaveLength(0)
})

test('regular portal cannot turn a copied rubric planning section into organiser configuration discovery', async ({ page }, info) => {
  test.skip(info.project.metadata.portal !== 'regular', 'Regular portal boundary')
  const state = await rubricPlanningFixture(page); await page.goto(`/events/${firstID}?section=judging%20criteria`)
  await expect(page.getByLabel('Rubric name', { exact: true })).toHaveCount(0); await expect(page.getByRole('button', { name: 'Review draft save', exact: true })).toHaveCount(0); expect(state.reads).toBe(0); expect(state.posts).toHaveLength(0)
})

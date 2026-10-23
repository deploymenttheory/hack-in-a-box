import { expect, test, type Page } from '@playwright/test'
import { firstID, fixtureEvent } from './fixtures'
import { mockObjectives, objectiveID, measureID, plannedDefinition, resourceID } from './objective-fixtures'
import { defaultBranding } from '../src/branding'
import type { components } from '../src/api/schema.gen'

async function templateFixture(page: Page) {
  await mockObjectives(page)
  const initial = { id: resourceID, name: 'Reusable objective process', revision: 1, version: 1, latest_version: 1, published_version: null, configuration: { event: fixtureEvent().configuration, definitions: { forms: [], tasks: [], deliverables: [], role_briefs: [], rubrics: [], messages: [], branding: defaultBranding, objectives: [{ id: objectiveID, definition: plannedDefinition('Template private objective') }] } } }
  const state = { current: structuredClone(initial), denied: false, writes: [] as { expected: string; body: unknown }[] }
  await page.route(/\/api\/v1\/templates\//, async route => {
    const request = route.request()
    if (state.denied) return route.fulfill({ status: 403, json: { error: { code: 'forbidden', message: 'Current authority removed', request_id: 'synthetic' } } })
    if (request.method() === 'PUT') {
      const body = request.postDataJSON(); state.writes.push({ expected: request.headers()['if-match'], body: structuredClone(body) })
      state.current = { ...state.current, name: body.name, configuration: body.configuration, revision: state.current.revision + 1, version: state.current.version + 1, latest_version: state.current.latest_version + 1 }
    }
    return route.fulfill({ json: state.current })
  })
  await page.goto(`/templates/${resourceID}`)
  await page.getByText('Objectives and success measures', { exact: true }).click()
  return state
}

test('template unit changes require fresh explicit version review and retain safe edits', async ({ page }) => {
  const state = await templateFixture(page)
  await page.getByLabel('Unit', { exact: true }).fill('my intended unit')
  state.current.revision = 2; state.current.version = 2; state.current.latest_version = 2
  state.current.configuration.definitions.objectives[0].definition.measures[0].unit = 'new server unit'
  await page.getByRole('button', { name: 'Save new draft version' }).click()
  const comparison = page.getByRole('region', { name: 'Current template comparison' })
  await expect(comparison).toContainText('new server unit'); await expect(comparison).toContainText('my intended unit')
  await expect(page.getByLabel('Unit', { exact: true })).toHaveValue('my intended unit'); expect(state.writes).toHaveLength(0)
  await page.getByRole('button', { name: 'Use current template revision and keep my edits' }).click()
  expect(state.writes).toHaveLength(0)
  await page.getByRole('button', { name: 'Save new draft version' }).click()
  await expect(page.getByRole('status')).toContainText('Version 3 saved as a draft')
  expect(state.writes).toHaveLength(1); expect(state.writes[0].expected).toBe('"2"')
  expect(state.current.configuration.definitions.objectives[0].definition.measures[0].unit).toBe('my intended unit')
})

test('native template denial clears unsent private objective inputs while the page stays active', async ({ page }) => {
  await page.clock.install()
  const state = await templateFixture(page)
  await page.getByLabel('Objective description', { exact: true }).fill('PRIVATE TEMPLATE REVOKED INPUT')
  state.denied = true; await page.clock.fastForward(16_000)
  await expect(page.getByRole('heading', { name: 'Template unavailable' })).toBeVisible()
  await expect(page.getByLabel('Objective description', { exact: true })).toHaveCount(0)
  await expect(page.getByText('PRIVATE TEMPLATE REVOKED INPUT', { exact: true })).toHaveCount(0)
  expect(state.writes).toHaveLength(0)
  expect(await page.evaluate(() => [...Object.values(sessionStorage), ...Object.values(localStorage)].some(value => value.includes('PRIVATE TEMPLATE REVOKED INPUT')))).toBe(false)
})

async function contentFixture(page: Page, retained = false) {
  const objectives = await mockObjectives(page, true)
  type Content = components['schemas']['ContentPage']
  const current: Content = { id: resourceID, kind: 'page', revision: 1, version: 1, state: 'draft', released_version: null, released_at: null, acknowledged_at: null, can_manage: true, attachment_details: [], message: '', document: { title: 'Objective guidance', markdown: 'Approved guidance text', require_acknowledgement: false, audience: { confirmed: true, roles: [], teams: [] }, attachments: [], scheduled_at: null, objectives: retained ? [{ objective: { id: objectiveID, version: 1 }, measure_ids: [measureID] }] : [] } }
  const writes: components['schemas']['ContentDocument'][] = []
  await page.route(new RegExp(`/api/v1/events/${firstID}/(?:content(?:[/?]|$)|configuration-snapshot(?:[?]|$)|content-readiness(?:[?]|$))`), async route => {
    const path = new URL(route.request().url()).pathname
    if (path.endsWith('/configuration-snapshot')) return route.fulfill({ json: { event: fixtureEvent().configuration, definitions: { forms: [], tasks: [], deliverables: [], role_briefs: [], messages: [], rubrics: [], branding: defaultBranding } } })
    if (path.endsWith('/content-readiness')) return route.fulfill({ json: { published_at: null, blockers: [] } })
    if (path.endsWith('/content')) return route.fulfill({ json: { items: [current], next_cursor: null } })
    if (path.endsWith('/history')) return route.fulfill({ json: { items: [], next_cursor: null } })
    if (route.request().method() === 'PUT') {
      const body = route.request().postDataJSON(); writes.push(structuredClone(body.document)); current.document = body.document; current.revision++; current.version++
    }
    return route.fulfill({ json: current })
  })
  return { ...objectives, current, writes, open: async () => { await page.goto(`/events/${firstID}`); await page.getByRole('button', { name: 'Content', exact: true }).click(); await page.getByRole('button', { name: 'Edit Objective guidance', exact: true }).click() } }
}

test('content author deliberately selects immutable objective/measure references without copying private planning inputs', async ({ page }, info) => {
  const state = await contentFixture(page)
  const privateDefinition = plannedDefinition('Private objective selected for reviewed disclosure')
  privateDefinition.owner_id = '1ca7d59b-3160-48b7-99b7-aad2cb648d64'
  privateDefinition.measures[0].baseline = { value: '98765', basis: 'self_report' }
  state.advance(firstID, objectiveID, privateDefinition)
  await state.open()
  await page.getByText('Select objective versions for disclosure', { exact: true }).click()
  await page.getByRole('button', { name: 'Select version 2 for disclosure' }).click()
  await page.getByLabel('Include measure: Working examples', { exact: true }).check()
  const preview = page.getByRole('region', { name: 'Selected disclosure fields' })
  await expect(preview).toContainText('Planning target: 0 prototype')
  await expect(preview).not.toContainText('98765'); await expect(preview).not.toContainText('Synthetic Person')
  await page.getByRole('button', { name: 'Save new draft', exact: true }).click()
  await expect(page.getByText('Draft version 2 saved.', { exact: false })).toBeVisible()
  expect(state.writes[0].objectives).toEqual([{ objective: { id: objectiveID, version: 2 }, measure_ids: [measureID] }])
  expect(state.writes[0].markdown).toBe('Approved guidance text')
  expect(JSON.stringify(state.writes[0])).not.toContain('98765')
  await page.screenshot({ path: info.outputPath('objective-disclosure-authoring.png'), fullPage: true })
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
})

test('retained disclosure stays on its immutable version until explicitly removed and reselected', async ({ page }) => {
  const state = await contentFixture(page, true)
  const next = plannedDefinition('Later planning objective'); next.measures[0].unit = 'changed definition unit'
  state.advance(firstID, objectiveID, next)
  await state.open()
  await expect(page.getByText('This reference remains version 1', { exact: false })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Selected disclosure fields' })).toContainText('0 prototype')
  await page.getByRole('button', { name: 'Remove objective disclosure' }).click()
  await page.getByRole('button', { name: 'Select version 2 for disclosure' }).click()
  await page.getByLabel('Include measure: Working examples', { exact: true }).check()
  await expect(page.getByRole('region', { name: 'Selected disclosure fields' })).toContainText('changed definition unit')
  await page.getByRole('button', { name: 'Save new draft', exact: true }).click()
  await expect(page.getByText('Draft version 2 saved.', { exact: false })).toBeVisible()
  expect(state.writes[0].objectives![0].objective.version).toBe(2)
})

test('current planning denial clears private authoring previews without publishing or changing the content audience', async ({ page }) => {
  await page.clock.install()
  const state = await contentFixture(page, true)
  await state.open()
  await expect(page.getByText('Private onboarding objective', { exact: true })).toBeVisible()
  state.state.deny = true; await page.clock.fastForward(16_000)
  await expect(page.getByText('Current planning access changed. Private objective previews have been cleared.', { exact: true })).toBeVisible()
  await expect(page.getByText('Private onboarding objective', { exact: true })).toHaveCount(0)
  expect(state.writes).toHaveLength(0)
  expect(state.current.document.audience).toEqual({ confirmed: true, roles: [], teams: [] })
})

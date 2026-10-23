import { test, expect } from '@playwright/test'
import { fixtureEvent, firstID } from './fixtures'
import { mockObjectives, objectiveID, plannedDefinition } from './objective-fixtures'
import { defaultBranding } from '../src/branding'
import type { components } from '../src/api/schema.gen'

test('create, edit, compare, publish and clone an independent event from a starter', async ({ page }) => {
  const { state, advance } = await mockObjectives(page)
  const workspace = state.workspace
  const configuration: components['schemas']['ConfigurationBundle'] = { event: fixtureEvent().configuration, definitions: { forms: [], tasks: [], deliverables: [], role_briefs: [], rubrics: [], messages: [], branding: defaultBranding, objectives: [{ id: objectiveID, definition: plannedDefinition('Reusable private planning objective') }] } }
  const templates: components['schemas']['Template'][] = []
  const versions = new Map<number, components['schemas']['ConfigurationBundle']>()
  await page.route('**/api/v1/template-starters', route => route.fulfill({ json: { items: [{ key: 'all-team', name: 'All-team judging', configuration }, { key: 'qualification-final', name: 'Mentor qualification and final', configuration }] } }))
  await page.route('**/api/v1/templates**', async route => {
    const path = new URL(route.request().url()).pathname, method = route.request().method()
    if (path === '/api/v1/templates') {
      if (method === 'POST') { const body = route.request().postDataJSON(); const t = { ...body, revision: 1, latest_version: 1, version: 1, published_version: null }; templates.push(t); versions.set(1, structuredClone(body.configuration)); return route.fulfill({ json: t }) }
      return route.fulfill({ json: { items: templates, next_cursor: null } })
    }
    const t = templates[0]
    if (path.endsWith('/publish')) { t.published_version = route.request().postDataJSON().version; t.revision++; return route.fulfill({ json: t }) }
    if (path.endsWith('/clone')) { const body = route.request().postDataJSON(); expect(body.version).toBe(2); expect(body.acknowledge_competition_restriction).toBe(true); const cloned = { ...fixtureEvent(body.id, body.name), configuration: structuredClone(versions.get(2)!.event) }; workspace.events.push(cloned); const definition = structuredClone(versions.get(2)!.definitions.objectives![0].definition); definition.owner_id = null; definition.measures = definition.measures.map(m => ({ ...m, id: '45555555-5555-4555-8555-555555555555' })); advance(cloned.id, '46666666-6666-4666-8666-666666666666', definition); return route.fulfill({ json: cloned }) }
    if (method === 'PUT') { const body = route.request().postDataJSON(); t.configuration = body.configuration; t.name = body.name; t.revision++; t.latest_version++; t.version = t.latest_version; versions.set(t.version, structuredClone(t.configuration)) }
    const version = Number(new URL(route.request().url()).searchParams.get('version')) || t.version
    return route.fulfill({ json: { ...t, version, configuration: versions.get(version) } })
  })
  await page.goto('/')
  await page.getByRole('link', { name: 'Event templates' }).click()
  await page.getByLabel('Starter process').selectOption('qualification-final')
  await page.getByLabel('Template name').fill('Synthetic reusable hack')
  await page.getByRole('button', { name: 'Create template draft' }).click()
  await expect(page.getByRole('heading', { name: 'Synthetic reusable hack' })).toBeVisible()
  await page.getByRole('button', { name: 'Publish version 1', exact: true }).click()
  await expect(page.getByRole('status')).toContainText('Version 1 published')
  await page.getByText('Event defaults and agenda', { exact: true }).click()
  await page.getByLabel('Event description', { exact: true }).fill('A revised reusable process')
  await page.getByText('Objectives and success measures', { exact: true }).click()
  await expect(page.getByLabel('Objective owner (optional)')).toHaveCount(0)
  await page.getByLabel('Unit', { exact: true }).fill('reviewed example')
  await expect(page.getByLabel('Target (optional; blank means unknown)')).toHaveValue('0')
  await expect(page.getByText('Baseline unknown. This is not zero.', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Publish version 1', exact: true })).toBeDisabled()
  await expect(page.getByText('event / description:', { exact: false }).first()).toContainText('A revised reusable process')
  await page.getByRole('button', { name: 'Save new draft version' }).click()
  await expect(page.getByRole('status')).toContainText('Version 2 saved')
  expect(versions.get(1)?.event.description).toBe(configuration.event.description)
  expect(versions.get(1)?.definitions.objectives![0].definition.measures[0].unit).toBe('prototype')
  expect(versions.get(2)?.definitions.objectives![0].definition.measures[0].unit).toBe('reviewed example')
  await page.getByRole('button', { name: 'Publish version 2', exact: true }).click()
  await expect(page.getByRole('status')).toContainText('Version 2 published')
  await page.getByLabel('New event name').fill('Independent draft')
  await page.getByLabel('New event slug').fill('independent-draft')
  await page.getByLabel('Live start in event timezone').fill('2027-07-20T10:00')
  await page.getByRole('checkbox').last().check()
  await page.getByRole('button', { name: 'Create independent event draft' }).click()
  await expect(page.getByRole('heading', { name: 'Independent draft' })).toBeVisible()
  await page.getByRole('button', { name: 'Objectives', exact: true }).click()
  await page.getByRole('button', { name: 'Open objective', exact: true }).click()
  await expect(page.getByLabel('Unit', { exact: true })).toHaveValue('reviewed example')
  await expect(page.getByLabel('Objective owner (optional)')).toHaveValue('')
  await page.getByLabel('Unit', { exact: true }).fill('event-only unit')
  await page.getByLabel('Reason for this definition version').fill('Independent event planning')
  await page.getByRole('button', { name: 'Save definition version', exact: true }).click()
  await expect(page.getByRole('status', { name: 'Objective save state' })).toContainText('Definition version 2 saved')
  expect(versions.get(2)?.definitions.objectives![0].definition.measures[0].unit).toBe('reviewed example')
  expect(workspace.events.find(e => e.id === firstID)?.configuration.description).toBe('A shared place to build.')
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
})

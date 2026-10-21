import { test, expect } from '@playwright/test'
import { mockWorkspace, fixtureEvent, firstID } from './fixtures'
import { defaultBranding } from '../src/branding'
import type { components } from '../src/api/schema.gen'

test('create, edit, compare, publish and clone an independent event from a starter', async ({ page }) => {
  const workspace = await mockWorkspace(page)
  const configuration: components['schemas']['ConfigurationBundle'] = { event: fixtureEvent().configuration, definitions: { forms: [], tasks: [], deliverables: [], role_briefs: [], rubrics: [], messages: [], branding: defaultBranding } }
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
    if (path.endsWith('/clone')) { const body = route.request().postDataJSON(); expect(body.version).toBe(2); expect(body.acknowledge_competition_restriction).toBe(true); const cloned = { ...fixtureEvent(body.id, body.name), configuration: structuredClone(versions.get(2)!.event) }; workspace.events.push(cloned); return route.fulfill({ json: cloned }) }
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
  await expect(page.getByRole('button', { name: 'Publish version 1', exact: true })).toBeDisabled()
  await expect(page.getByText('event / description:', { exact: false }).first()).toContainText('A revised reusable process')
  await page.getByRole('button', { name: 'Save new draft version' }).click()
  await expect(page.getByRole('status')).toContainText('Version 2 saved')
  expect(versions.get(1)?.event.description).toBe(configuration.event.description)
  await page.getByRole('button', { name: 'Publish version 2', exact: true }).click()
  await expect(page.getByRole('status')).toContainText('Version 2 published')
  await page.getByLabel('New event name').fill('Independent draft')
  await page.getByLabel('New event slug').fill('independent-draft')
  await page.getByLabel('Live start in event timezone').fill('2027-07-20T10:00')
  await page.getByRole('checkbox').last().check()
  await page.getByRole('button', { name: 'Create independent event draft' }).click()
  await expect(page.getByRole('heading', { name: 'Independent draft' })).toBeVisible()
  expect(workspace.events.find(e => e.id === firstID)?.configuration.description).toBe('A shared place to build.')
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
})

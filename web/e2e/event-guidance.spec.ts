import { openEventSection } from './event-navigation-fixture'
import { test, expect, type Page, type Route } from '@playwright/test'
import { mockWorkspace, firstID } from './fixtures'
import type { components } from '../src/api/schema.gen'

type Item = components['schemas']['EventGuidanceItem']
const rulesID = '15c8ac1a-b80f-4c71-a603-5064a7d8eb20'
async function setup(page: Page) {
  const workspace = await mockWorkspace(page), event = workspace.events[0]
  event.stage = 'preparation'
  let hidden = false, acknowledged = false, guidanceDenied = 0, holdReads = false
  const heldReads: Route[] = []
  const writes: string[] = []
  const rules: Item = { id: rulesID, purpose: 'rules', title: 'Current event rules', version: 2, contacts: [], require_acknowledgement: true, acknowledged_at: null, previous_acknowledgement: { version: 1, at: '2027-05-31T10:00:00Z' } }
  const support: Item = { ...rules, id: '9778f63a-6000-4c63-8251-cc2bcc53005a', purpose: 'support', title: 'Get event support', contacts: [{ label: 'Support team', url: 'mailto:support@example.test' }], require_acknowledgement: false, previous_acknowledgement: null }
  const conduct: Item = { ...support, id: 'b6de7a67-0404-4140-8715-c6cdbaec7c6f', purpose: 'conduct', title: 'Report a conduct concern', contacts: [{ label: 'Published conduct route', url: 'https://example.test/conduct' }, { label: 'Injected active route', url: 'javascript:alert(1)' }] }
  const resource = (): components['schemas']['ContentPage'] => ({ id: rulesID, kind: 'page', version: 2, revision: 3, state: 'released', released_version: 2, released_at: '2027-06-01T09:00:00Z', attachment_details: [], can_manage: false, message: '', acknowledged_at: acknowledged ? '2027-06-01T11:00:00Z' : null, previous_acknowledgement: rules.previous_acknowledgement, document: { title: rules.title, purpose: 'rules', markdown: '# Corrected rules\n\nRead the updated competition rules.', require_acknowledgement: true, contacts: [], audience: { confirmed: true, roles: [], teams: [] }, attachments: [], scheduled_at: null } })
  await page.route(`**/events/${firstID}/guidance`, route => guidanceDenied ? route.fulfill({ status: guidanceDenied, json: { error: { code: 'forbidden', request_id: 'synthetic' } } }) : route.fulfill({ json: { items: hidden ? [] : [{ ...rules, acknowledged_at: resource().acknowledged_at }, support, conduct], warnings: [{ field: 'content', code: 'missing_conduct_contact', message: 'Publish a configured conduct contact for confirmed competitors.' }], can_manage: true } }))
  await page.route(`**/events/${firstID}/content/${rulesID}**`, route => {
    if (holdReads && route.request().method() === 'GET') { heldReads.push(route); return }
    if (hidden) return route.fulfill({ status: 403, json: { error: { code: 'forbidden', message: 'Current access changed', request_id: 'synthetic' } } })
    if (route.request().method() === 'POST') { writes.push(route.request().url()); expect(route.request().headers()['if-match']).toBe('"2"'); acknowledged = true }
    return route.fulfill({ json: resource() })
  })
  await page.route(`**/events/${firstID}/content?**`, route => route.fulfill({ json: { items: [], next_cursor: null } }))
  await page.route(`**/events/${firstID}/configuration-snapshot`, route => route.fulfill({ json: { definitions: { role_briefs: [] } } }))
  await page.route(`**/events/${firstID}/content-readiness`, route => route.fulfill({ json: { published_at: null, blockers: [] } }))
  await page.goto(`/events/${firstID}`)
  return { writes, heldReads, resource, revokeGuidance: (status: number) => { guidanceDenied = status; holdReads = true }, restoreGuidanceOnly: () => { guidanceDenied = 0; hidden = true }, hide: () => { hidden = true } }
}

test('Today opens corrected rules and records only the current personal version', async ({ page }, info) => {
  const { writes } = await setup(page)
  await openEventSection(page, 'Today')
  await expect(page.getByText('You acknowledged version 1. Read and acknowledge the updated version 2.')).toBeVisible()
  await page.getByRole('button', { name: 'Read Current event rules' }).focus()
  await page.keyboard.press('Enter')
  await expect(page.getByText('that receipt does not acknowledge version 2.', { exact: false })).toBeVisible()
  await page.getByRole('button', { name: 'I have read version 2' }).click(); await page.getByRole('button', { name: 'Confirm acknowledgement' }).click()
  await expect(page.getByRole('region', { name: 'Required acknowledgement' }).getByRole('status')).toContainText('You acknowledged version 2')
  expect(writes).toHaveLength(1)
  await page.screenshot({ path: info.outputPath('guidance-current-rules.png'), fullPage: true })
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
})

test('published support and conduct routes remain safe and no report is collected', async ({ page }, info) => {
  const { writes } = await setup(page)
  await openEventSection(page, 'Today')
  await expect(page.getByRole('link', { name: 'Support team' })).toHaveAttribute('href', 'mailto:support@example.test')
  await expect(page.getByRole('link', { name: 'Published conduct route' })).toHaveAttribute('rel', 'noopener noreferrer')
  await expect(page.getByRole('link', { name: 'Injected active route' })).toHaveCount(0)
  await expect(page.getByText('This app does not collect reports;', { exact: false })).toBeVisible()
  await expect(page.locator('textarea,input')).toHaveCount(0)
  await expect(page.getByText('Organiser preparation reminders.', { exact: false })).toHaveCount(0)
  expect(writes).toHaveLength(0)
  await page.screenshot({ path: info.outputPath('guidance-contacts.png'), fullPage: true })
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
})

test('resources recheck withdrawal and hide previously opened rules', async ({ page }) => {
  const state = await setup(page)
  await openEventSection(page, 'Resources')
  await page.getByRole('button', { name: 'Read Current event rules' }).click()
  await expect(page.getByText('Read the updated competition rules.')).toBeVisible()
  state.hide()
  await page.getByRole('button', { name: 'Refresh resources' }).click()
  await expect(page.getByText('You do not have access to this resource.')).toBeVisible()
  await expect(page.getByText('Read the updated competition rules.')).toHaveCount(0)
})


test('organiser readiness warns without a lifecycle gate and starter contacts remain unresolved drafts @admin', async ({ page }) => {
  await setup(page)
  await openEventSection(page, 'Content')
  await expect(page.getByRole('region', { name: 'Guidance readiness' })).toContainText('These do not block the event lifecycle.')
  await expect(page.getByRole('region', { name: 'Guidance readiness' })).toContainText('Publish a configured conduct contact')
  await page.getByLabel('Starting template').selectOption('conduct')
  await page.getByRole('button', { name: 'Start from template' }).click()
  await page.getByRole('navigation', { name: 'Content editor sections' }).getByRole('button', { name: 'Guidance purpose', exact: true }).click()
  await expect(page.getByLabel('Page purpose')).toHaveValue('conduct')
  await expect(page.getByLabel('Contact 1 URL')).toHaveValue('{{Approved contact URL or mailbox}}')
  await expect(page.getByText('Resolve each contact label and approved URL before approval.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Save new draft', exact: true })).toBeEnabled()
  await expect(page.getByRole('button', { name: /^Approve version/ })).toHaveCount(0)
})


test('keyboard guidance reader returns to its actual guidance action across widths', async ({ page }, info) => {
  await setup(page); await openEventSection(page, 'Resources')
  await page.getByRole('button', { name: 'Read Current event rules', exact: true }).focus(); await page.keyboard.press('Enter')
  await expect(page.getByRole('heading', { name: 'Current event rules', exact: true })).toBeFocused()
  await page.setViewportSize(info.project.name === 'mobile' ? { width: 1280, height: 800 } : { width: 390, height: 844 })
  await page.getByRole('button', { name: 'Back to resources', exact: true }).focus(); await page.keyboard.press('Enter')
  await expect(page.getByRole('button', { name: 'Read Current event rules', exact: true })).toBeFocused()
})


test('a removed guidance opener returns keyboard focus to the current named guidance heading', async ({ page }) => {
  await page.clock.install({ time: new Date('2027-06-14T09:00:00Z') })
  const state = await setup(page); await openEventSection(page, 'Today')
  await page.getByRole('button', { name: 'Read Current event rules', exact: true }).focus(); await page.keyboard.press('Enter')
  await expect(page.getByRole('heading', { name: 'Current event rules', exact: true })).toBeFocused()
  state.hide(); await page.clock.runFor(15_100)
  await expect(page.getByRole('alert')).toContainText('You do not have access')
  await page.getByRole('button', { name: 'Back to resources', exact: true }).focus(); await page.keyboard.press('Enter')
  await expect(page.getByRole('button', { name: 'Read Current event rules', exact: true })).toHaveCount(0)
  await expect(page.getByRole('heading', { name: 'Rules and contacts', exact: true })).toBeFocused()
})

// These are standalone Today/preparation consumers, not children of the library.
for (const section of ['Today', 'Deliverables', 'Content']) for (const status of [401, 403, 404]) for (const delay of [0, 16_000]) test(`standalone ${section} guidance ${status} scrubs its reader before partial restoration after ${delay}ms${section === 'Content' ? ' @admin' : ''}`, async ({ page }) => {
  await page.clock.install()
  const state = await setup(page); await openEventSection(page, section)
  await page.getByRole('button', { name: 'Read Current event rules', exact: true }).click()
  await expect(page.getByRole('article', { name: 'Released resource text' })).toContainText('Read the updated competition rules.')
  state.revokeGuidance(status); await page.clock.runFor(16_000)
  if (status === 401) {
    await expect(page.getByRole('heading', { name: 'Sign in to continue.', exact: true })).toBeVisible()
    await expect(page.getByRole('article', { name: 'Released resource text' })).toHaveCount(0)
    await expect(page.getByRole('button', { name: 'I have read version 2', exact: true })).toHaveCount(0)
    for (const held of state.heldReads) await held.fulfill({ json: state.resource() }).catch(() => undefined)
    await expect(page.getByRole('article', { name: 'Released resource text' })).toHaveCount(0)
    expect(state.writes).toHaveLength(0); return
  }
  await expect(page.getByText('Released guidance is available after your event access is confirmed.', { exact: true })).toBeVisible()
  await expect(page.getByRole('article', { name: 'Released resource text' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'I have read version 2', exact: true })).toHaveCount(0)
  state.restoreGuidanceOnly(); if (delay) await page.clock.runFor(delay)
  await page.getByRole('button', { name: 'Check guidance access', exact: true }).click()
  await expect(page.getByText('No rules or contact pages have been released to your current audience yet.', { exact: true })).toBeVisible()
  for (const held of state.heldReads) await held.fulfill({ json: state.resource() }).catch(() => undefined)
  await expect(page.getByRole('article', { name: 'Released resource text' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'I have read version 2', exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Read Current event rules', exact: true })).toHaveCount(0)
  expect(state.writes).toHaveLength(0)
})

test('standalone guidance denial redacts private acknowledgement intent without confirming or replaying it', async ({ page }) => {
  await page.clock.install(); const state = await setup(page); await openEventSection(page, 'Today')
  let attempts = 0
  await page.route(`**/content/${rulesID}/acknowledgements`, route => {
    attempts++; expect(route.request().headers()['if-match']).toBe('"2"')
    expect(route.request().headers()['x-csrf-token']).toBe('synthetic-csrf-token-32-characters')
    return route.fulfill({ status: 503, json: { error: { code: 'unavailable', request_id: 'synthetic' } } })
  })
  await page.getByRole('button', { name: 'Read Current event rules', exact: true }).click()
  await page.getByRole('button', { name: 'I have read version 2', exact: true }).click()
  await page.getByRole('button', { name: 'Confirm acknowledgement', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Outcome unknown', exact: true })).toBeVisible()
  state.revokeGuidance(403); await page.clock.runFor(16_000)
  const history = page.getByRole('complementary', { name: 'Unconfirmed command history', exact: true })
  await expect(history).toContainText('Resource acknowledgement · Expected revision 2 · Original outcome remains unconfirmed')
  await expect(history).not.toContainText('Current event rules')
  state.restoreGuidanceOnly(); await page.getByRole('button', { name: 'Check guidance access', exact: true }).click()
  await expect(page.getByText('No rules or contact pages have been released to your current audience yet.', { exact: true })).toBeVisible()
  for (const held of state.heldReads) await held.fulfill({ json: state.resource() }).catch(() => undefined)
  await expect(history).toContainText('Original outcome remains unconfirmed')
  await expect(page.getByRole('button', { name: 'Confirm acknowledgement', exact: true })).toHaveCount(0)
  expect(attempts).toBe(1)
})

import { openEventSection } from './event-navigation-fixture'
import { test, expect, type Page } from '@playwright/test'
import { mockWorkspace, firstID } from './fixtures'
import type { components } from '../src/api/schema.gen'

type Item = components['schemas']['EventGuidanceItem']
const rulesID = '15c8ac1a-b80f-4c71-a603-5064a7d8eb20'
async function setup(page: Page) {
  const workspace = await mockWorkspace(page), event = workspace.events[0]
  event.stage = 'preparation'
  let hidden = false, acknowledged = false
  const writes: string[] = []
  const rules: Item = { id: rulesID, purpose: 'rules', title: 'Current event rules', version: 2, contacts: [], require_acknowledgement: true, acknowledged_at: null, previous_acknowledgement: { version: 1, at: '2027-05-31T10:00:00Z' } }
  const support: Item = { ...rules, id: '9778f63a-6000-4c63-8251-cc2bcc53005a', purpose: 'support', title: 'Get event support', contacts: [{ label: 'Support team', url: 'mailto:support@example.test' }], require_acknowledgement: false, previous_acknowledgement: null }
  const conduct: Item = { ...support, id: 'b6de7a67-0404-4140-8715-c6cdbaec7c6f', purpose: 'conduct', title: 'Report a conduct concern', contacts: [{ label: 'Published conduct route', url: 'https://example.test/conduct' }, { label: 'Injected active route', url: 'javascript:alert(1)' }] }
  const resource = (): components['schemas']['ContentPage'] => ({ id: rulesID, kind: 'page', version: 2, revision: 3, state: 'released', released_version: 2, released_at: '2027-06-01T09:00:00Z', attachment_details: [], can_manage: false, message: '', acknowledged_at: acknowledged ? '2027-06-01T11:00:00Z' : null, previous_acknowledgement: rules.previous_acknowledgement, document: { title: rules.title, purpose: 'rules', markdown: '# Corrected rules\n\nRead the updated competition rules.', require_acknowledgement: true, contacts: [], audience: { confirmed: true, roles: [], teams: [] }, attachments: [], scheduled_at: null } })
  await page.route(`**/events/${firstID}/guidance`, route => route.fulfill({ json: { items: hidden ? [] : [{ ...rules, acknowledged_at: resource().acknowledged_at }, support, conduct], warnings: [{ field: 'content', code: 'missing_conduct_contact', message: 'Publish a configured conduct contact for confirmed competitors.' }], can_manage: true } }))
  await page.route(`**/events/${firstID}/content/${rulesID}**`, route => {
    if (hidden) return route.fulfill({ status: 403, json: { error: { code: 'forbidden', message: 'Current access changed', request_id: 'synthetic' } } })
    if (route.request().method() === 'POST') { writes.push(route.request().url()); expect(route.request().headers()['if-match']).toBe('"2"'); acknowledged = true }
    return route.fulfill({ json: resource() })
  })
  await page.route(`**/events/${firstID}/content?**`, route => route.fulfill({ json: { items: [], next_cursor: null } }))
  await page.route(`**/events/${firstID}/configuration-snapshot`, route => route.fulfill({ json: { definitions: { role_briefs: [] } } }))
  await page.route(`**/events/${firstID}/content-readiness`, route => route.fulfill({ json: { published_at: null, blockers: [] } }))
  await page.goto(`/events/${firstID}`)
  return { writes, hide: () => { hidden = true } }
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


test('organiser readiness warns without a lifecycle gate and starter contacts remain unresolved drafts', async ({ page }) => {
  await setup(page)
  await openEventSection(page, 'Content')
  await expect(page.getByRole('region', { name: 'Guidance readiness' })).toContainText('These do not block the event lifecycle.')
  await expect(page.getByRole('region', { name: 'Guidance readiness' })).toContainText('Publish a configured conduct contact')
  await page.getByLabel('Starting template').selectOption('conduct')
  await page.getByRole('button', { name: 'Start from template' }).click()
  await expect(page.getByLabel('Page purpose')).toHaveValue('conduct')
  await expect(page.getByLabel('Contact 1 URL')).toHaveValue('{{Approved contact URL or mailbox}}')
  await expect(page.getByText('Resolve each contact label and approved URL before approval.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Save new draft', exact: true })).toBeEnabled()
  await expect(page.getByRole('button', { name: /^Approve version/ })).toHaveCount(0)
})

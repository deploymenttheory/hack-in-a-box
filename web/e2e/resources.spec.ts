import { openEventSection, expectEventSection } from './event-navigation-fixture'
import { test, expect, type Page } from '@playwright/test'
import { mockWorkspace, firstID } from './fixtures'
import type { components } from '../src/api/schema.gen'

type Content = components['schemas']['ContentPage']
const guideID = '32ad4764-17ab-4b74-8227-0796b9aba384', challengeID = '3bdfdb01-6bb1-4838-bd06-83a5e583c8eb', judgeID = '90715f40-41ab-467c-8ecb-e3652831923f', teamID = '8881846e-f84c-433e-acbd-87e051a2d6ea', fileID = '2cd31e17-de34-4120-8d4d-6cf01eb64eb0'
function content(id: string, title: string): Content {
  return { id, kind: 'page', revision: 1, version: 1, state: 'released', released_version: 1, released_at: '2027-06-13T09:00:00Z', acknowledged_at: null, can_manage: false, message: '', attachment_details: [], document: { title, markdown: '# Read this guide\n\nSynthetic guidance for your team.', audience: { confirmed: true, roles: [], teams: [] }, attachments: [], scheduled_at: null, require_acknowledgement: true } }
}
async function setup(page: Page, judge = false) {
  const workspace = await mockWorkspace(page), event = workspace.events[0]
  event.can_edit = false; event.can_read_staff = false; event.can_manage_owner = false; event.can_transition = false; event.stage = 'preparation'
  const guide = content(guideID, 'Getting started'), challenge = { ...content(challengeID, 'Common challenge'), kind: 'challenge' as const }, judgeBrief = content(judgeID, 'Judge brief'), team = content(teamID, 'Aurora team resource')
  judgeBrief.document.audience = { confirmed: false, roles: ['judge'], teams: [] }
  guide.document.markdown += '\n\n[Useful link](https://example.test/guide)\n\n[Danger](javascript:alert%281%29)\n\n![Tracker](https://example.test/pixel.png)\n\n<script>window.attacked = true</script>'
  guide.document.attachments = [fileID]; guide.attachment_details = [{ id: fileID, filename: 'guide.txt', media_type: 'text/plain', size: 33, can_download: true }]
  const gets: string[] = [], acknowledgements: { id: string; version: string; csrf: string }[] = []
  let released = false, revoked = false, member = true, paginate = false
  await page.route(`**/events/${firstID}/content**`, async route => {
    const r = route.request(), url = new URL(r.url()), path = url.pathname
    const fail = (status: number, code: string) => route.fulfill({ status, json: { error: { code, message: 'Synthetic', request_id: 'synthetic' } } })
    if (r.method() === 'GET') gets.push(url.toString())
    if (revoked) return fail(403, 'forbidden')
    const visible = judge ? [judgeBrief] : [guide, ...(released ? [challenge] : []), ...(member ? [team] : [])]
    if (path.endsWith('/content')) {
      const q = url.searchParams.get('q')?.toLowerCase() ?? '', cursor = url.searchParams.get('cursor')
      const found = visible.filter(p => `${p.document.title} ${p.document.markdown}`.toLowerCase().includes(q))
      const rest = cursor ? found.slice(found.findIndex(p => p.id === cursor) + 1) : found
      return route.fulfill({ json: { items: paginate ? rest.slice(0, 1) : rest, next_cursor: paginate && rest.length > 1 ? rest[0].id : null } })
    }
    const id = path.split('/content/')[1]?.split('/')[0], selected = visible.find(p => p.id === id)
    if (!selected) return fail(404, 'not_found')
    if (r.method() === 'POST') {
      acknowledgements.push({ id: selected.id, version: r.headers()['if-match'], csrf: r.headers()['x-csrf-token'] })
      if (r.headers()['if-match'] !== `"${selected.version}"`) return fail(409, 'revision_conflict')
      selected.acknowledged_at ??= '2027-06-13T09:30:00Z'
    }
    return route.fulfill({ json: selected })
  })
  await page.goto(`/events/${firstID}`); await openEventSection(page, 'Resources')
  return { guide, challenge, event, gets, acknowledgements, release: () => { released = true; event.published_at = '2027-06-13T09:00:00Z' }, revoke: () => { revoked = true }, removeTeam: () => { member = false }, paginate: () => { paginate = true } }
}

test('reads safely, downloads protected attachments and explicitly acknowledges the displayed version', async ({ page }, info) => {
  const state = await setup(page)
  await page.getByRole('button', { name: 'Read Getting started' }).click()
  await expect(page.getByRole('heading', { name: 'Getting started', exact: true })).toBeFocused()
  await expect(page.getByRole('link', { name: 'Useful link' })).toHaveAttribute('rel', 'noopener noreferrer')
  await expect(page.getByRole('link', { name: 'Danger' })).toHaveCount(0)
  await expect(page.locator('.content-markdown script,.content-markdown iframe,.content-markdown img')).toHaveCount(0)
  await expect(page.getByRole('link', { name: 'guide.txt' })).toHaveAttribute('href', `/api/v1/events/${firstID}/files/${fileID}`)
  expect(state.acknowledgements).toHaveLength(0)
  await page.getByRole('button', { name: 'I have read version 1' }).click(); await page.getByRole('button', { name: 'Confirm acknowledgement' }).click()
  await expect(page.getByRole('region', { name: 'Required acknowledgement' }).getByRole('status')).toContainText('You acknowledged version 1')
  expect(state.acknowledgements).toEqual([{ id: guideID, version: '"1"', csrf: 'synthetic-csrf-token-32-characters' }])
  await page.screenshot({ path: info.outputPath('resource-reader.png'), fullPage: true })
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
  await page.getByRole('button', { name: 'Back to resources' }).click()
  await expect(page.getByText('Version 1 acknowledged')).toBeVisible()
  expect(state.gets.every(url => !url.includes('preview=true'))).toBe(true)
})

test('a material release requires reading the new version before acknowledging again', async ({ page }) => {
  const state = await setup(page)
  await page.getByRole('button', { name: 'Read Getting started' }).click()
  state.guide.version = 2; state.guide.revision = 2; state.guide.released_version = 2; state.guide.document.markdown = '# Updated guide\n\nRead the material change.'
  await page.getByRole('button', { name: 'I have read version 1' }).click(); await page.getByRole('button', { name: 'Confirm acknowledgement' }).click()
  await expect(page.getByRole('status').filter({ hasText: 'Current version needed' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'I have read version 2' })).toHaveCount(0)
  await page.getByRole('button', { name: 'Read current state' }).click()
  await expect(page.getByText('Read the material change.', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Use reviewed current state' }).click()
  await page.getByRole('button', { name: 'I have read version 2' }).click(); await page.getByRole('button', { name: 'Confirm acknowledgement' }).click()
  await expect(page.getByRole('region', { name: 'Required acknowledgement' }).getByRole('status')).toContainText('You acknowledged version 2')
  expect(state.acknowledgements.map(a => a.version)).toEqual(['"1"', '"2"'])
})

test('search and navigation reveal the common challenge only after publication', async ({ page }) => {
  const state = await setup(page)
  await expect(page.getByRole('button', { name: 'Read Common challenge' })).toHaveCount(0)
  await page.getByRole('searchbox', { name: 'Search resources', exact: true }).fill('Common challenge'); await page.getByRole('button', { name: 'Search', exact: true }).click()
  await expect(page.getByText('No released resources match your search.')).toBeVisible()
  state.release(); await page.getByRole('button', { name: 'Refresh resources' }).click()
  await page.getByRole('button', { name: 'Read Common challenge' }).click()
  await expect(page.getByText('Your team may start building now.', { exact: false })).toBeVisible()
  await expect(page.getByText('Formal submissions and mentor or judge review open at live start:', { exact: false })).toContainText('20 Jun 2027, 10:00')
})

test('role-specific libraries contain only returned authorised resources', async ({ page }) => {
  const state = await setup(page, true)
  await expect(page.getByRole('button', { name: 'Read Judge brief' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Read Getting started' })).toHaveCount(0)
  await expectEventSection(page,'Content',false)
  await page.getByRole('button', { name: 'Read Judge brief' }).click()
  await expect(page.getByRole('heading', { name: 'Judge brief' })).toBeVisible()
  expect(state.gets.every(url => !url.includes('preview=true'))).toBe(true)
})

test('pagination, search and refreshed team access update resource navigation', async ({ page }) => {
  const state = await setup(page); state.paginate()
  await page.getByRole('button', { name: 'Refresh resources' }).click()
  await page.getByRole('button', { name: 'More records' }).click()
  await expect(page.getByRole('button', { name: 'Read Aurora team resource' })).toBeVisible()
  await page.getByRole('searchbox', { name: 'Search resources', exact: true }).fill('Aurora'); await page.getByRole('button', { name: 'Search', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Read Getting started' })).toHaveCount(0)
  state.removeTeam(); await page.getByRole('button', { name: 'Refresh resources' }).click()
  await expect(page.getByText('No released resources match your search.')).toBeVisible()
})

test('revocation removes the reader and prevents a stale acknowledgement', async ({ page }) => {
  const state = await setup(page)
  await page.getByRole('button', { name: 'Read Getting started' }).click()
  state.revoke(); await page.getByRole('button', { name: 'I have read version 1' }).click(); await page.getByRole('button', { name: 'Confirm acknowledgement' }).click()
  await expect(page.getByRole('alert')).toContainText('You do not have access')
  await expect(page.getByRole('heading', { name: 'Getting started' })).toHaveCount(0)
  await expect(page.getByRole('link', { name: 'guide.txt' })).toHaveCount(0)
  expect(state.acknowledgements).toHaveLength(0)
})

test('an unknown acknowledgement keeps its original intent after a current read and never replays', async ({ page }) => {
  const state = await setup(page)
  let attempts = 0
  await page.route(`**/content/${guideID}/acknowledgements`, async route => {
    attempts++
    expect(route.request().headers()['if-match']).toBe('"1"')
    expect(route.request().headers()['x-csrf-token']).toBe('synthetic-csrf-token-32-characters')
    state.guide.acknowledged_at = '2027-06-13T09:30:00Z'
    await route.fulfill({ status: 503, json: { error: { code: 'unavailable', request_id: 'synthetic' } } })
  })
  await page.getByRole('button', { name: 'Read Getting started', exact: true }).click()
  await page.getByRole('button', { name: 'I have read version 1', exact: true }).click()
  expect(attempts).toBe(0)
  await page.getByRole('button', { name: 'Confirm acknowledgement', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Outcome unknown', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Back to resources', exact: true })).toBeDisabled()
  await page.getByRole('button', { name: 'Read current state', exact: true }).click()
  await expect(page.getByRole('region', { name: 'Required acknowledgement' })).toContainText('You acknowledged version 1')
  await expect(page.getByRole('region', { name: 'Earlier unconfirmed resource commands' })).toContainText('Acknowledge Getting started · Expected revision 1')
  await expect(page.getByText('Acknowledgement recorded for version 1.', { exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: 'Use reviewed current state', exact: true }).click()
  expect(attempts).toBe(1)
  await expect(page.getByRole('region', { name: 'Earlier unconfirmed resource commands' })).toContainText('Original outcome remains unconfirmed')
})

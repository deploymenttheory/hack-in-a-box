import { openEventSection } from './event-navigation-fixture'
import { test, expect, type Page } from '@playwright/test'
import { mockWorkspace, firstID } from './fixtures'
import type { components } from '../src/api/schema.gen'

type Content = components['schemas']['ContentPage']
const actorA = { id: 'b3135585-8ac0-4143-903e-1e03a8bb17a8', display_name: 'Team A competitor', platform_admin: false }
const actorB = { id: 'd34bc827-b1b0-42bd-ad09-462af83b7a18', display_name: 'Team B competitor', platform_admin: false }
const resourceID = '719d60be-7844-4f24-ae5b-055e35005e5a', fileID = '5ea320af-5711-407f-a4e7-471bc2bde0b9'
const canaryA = 'CONFIDENTIAL-TEAM-A-INSTRUCTIONS', canaryB = 'CONFIDENTIAL-TEAM-B-INSTRUCTIONS'
function resource(actor: typeof actorA): Content { return { id: resourceID, kind: 'brief', revision: 1, version: 1, state: 'released', document: { title: actor.id === actorA.id ? 'Team A brief' : 'Team B brief', markdown: actor.id === actorA.id ? canaryA : canaryB, audience: { confirmed: true, roles: [], teams: [] }, attachments: [fileID], scheduled_at: '2027-06-13T09:00:00Z', require_acknowledgement: true }, released_version: 1, released_at: '2027-06-13T09:00:00Z', acknowledged_at: null, can_manage: false, attachment_details: [{ id: fileID, filename: 'private-brief.txt', media_type: 'text/plain', size: 33, can_download: true }], message: '' } }
async function setup(page: Page) {
  const workspace = await mockWorkspace(page), event = workspace.events[0]
  event.can_edit = false; event.can_read_staff = false; event.can_manage_owner = false; event.can_transition = false; event.stage = 'preparation'
  let actor = actorA, released = true, withdrawn = false, authorised = true, hold = false, oldRead: Promise<void> | undefined, finish: (() => void) | undefined, entered: (() => void) | undefined
  const requests: string[] = [], readerArrived = new Promise<void>(resolve => { entered = resolve })
  const headers = { 'Cache-Control': 'no-store', 'X-Content-Type-Options': 'nosniff' }
  await page.route('**/api/v1/session', route => route.fulfill({ headers, json: { person: actor, csrf_token: 'synthetic-csrf-token-32-characters', can_create_events: false } }))
  await page.route(`**/events/${firstID}/content**`, async route => {
    const r = route.request(), url = new URL(r.url()), path = url.pathname; requests.push(r.url())
    const fail = (status: number, code: string) => route.fulfill({ status, headers, json: { error: { code, message: 'Synthetic denial', request_id: 'synthetic' } } })
    if (!authorised) return fail(403, 'forbidden')
    if (path.endsWith('/content')) { const item = resource(actor), q = url.searchParams.get('q')?.toLowerCase() ?? ''; return route.fulfill({ headers, json: { items: released && !withdrawn && `${item.document.title} ${item.document.markdown}`.toLowerCase().includes(q) ? [item] : [], next_cursor: null } }) }
    if (!released || withdrawn) return fail(404, 'not_found')
    const snapshot = resource(actor)
    if (hold) { hold = false; entered?.(); await oldRead }
    try { return await route.fulfill({ headers, json: snapshot }) } catch (error) { if (!page.isClosed() && !/closed|handled|canceled|cancelled/i.test(String(error))) throw error }
  })
  await page.route(`**/events/${firstID}/files/${fileID}`, route => route.fulfill(!authorised || !released || withdrawn ? { status: 403, headers, json: { error: { code: 'forbidden', message: 'Synthetic denial', request_id: 'synthetic' } } } : { headers: { ...headers, 'Content-Disposition': 'attachment; filename=private-brief.txt' }, contentType: 'text/plain', body: actor.id === actorA.id ? canaryA : canaryB }))
  await page.goto(`/events/${firstID}`); await openEventSection(page, 'Resources')
  return { requests, revoke: () => { authorised = false }, withdraw: () => { withdrawn = true }, embargo: () => { released = false }, release: () => { released = true }, switchAccount: () => { actor = actorB }, holdRead: () => { hold = true; oldRead = new Promise<void>(resolve => { finish = resolve }); return readerArrived }, finishRead: () => finish?.() }
}
async function copiedDownload(page: Page) { return page.evaluate(async url => { const r = await fetch(url); return { status: r.status, body: await r.text() } }, `/api/v1/events/${firstID}/files/${fileID}`) }

for (const change of ['withdrawal', 'revocation'] as const) {
  test(`${change} removes cached content on refresh and cannot resurrect it through navigation`, async ({ page }) => {
    const state = await setup(page)
    await page.getByRole('button', { name: 'Read Team A brief' }).click()
    await expect(page.getByText(canaryA, { exact: true })).toBeVisible()
    expect((await copiedDownload(page)).body).toBe(canaryA)
    if (change === 'withdrawal') state.withdraw(); else state.revoke()
    await page.getByRole('button', { name: 'Refresh resources' }).click()
    await expect(page.getByText(canaryA, { exact: true })).toHaveCount(0)
    await expect(page.getByRole('link', { name: 'private-brief.txt' })).toHaveCount(0)
    await page.getByRole('button', { name: 'Back to resources' }).click()
    await expect(page.getByRole('button', { name: 'Read Team A brief' })).toHaveCount(0)
    await openEventSection(page, 'Overview'); await openEventSection(page, 'Resources')
    await expect(page.getByRole('button', { name: 'Read Team A brief' })).toHaveCount(0)
    const denied = await copiedDownload(page); expect(denied.status).toBe(403); expect(denied.body).not.toContain(canaryA)
    const persisted = await page.evaluate(async () => ({ local: JSON.stringify(Object.entries(localStorage)), session: JSON.stringify(Object.entries(sessionStorage)), caches: await caches.keys() }))
    expect(persisted.local + persisted.session).not.toContain(canaryA); expect(persisted.caches).toEqual([])
    expect(state.requests.every(url => !url.includes('preview=true'))).toBe(true)
  })
}

test('an old response arriving after an account switch cannot populate the new account reader', async ({ page }) => {
  await page.clock.install()
  const state = await setup(page)
  const arrived = state.holdRead()
  await page.getByRole('button', { name: 'Read Team A brief' }).click(); await arrived
  state.switchAccount(); await page.clock.fastForward(61_000)
  await expect(page.getByRole('button', { name: actorB.display_name, exact: true })).toBeVisible()
  state.finishRead()
  await openEventSection(page, 'Resources')
  await page.getByRole('button', { name: 'Read Team B brief' }).click()
  await expect(page.getByText(canaryB, { exact: true })).toBeVisible()
  await expect(page.getByText(canaryA, { exact: true })).toHaveCount(0)
  await expect(page.getByRole('heading', { name: 'Team A brief', exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: 'Back to resources' }).click()
  await expect(page.getByRole('button', { name: 'Read Team A brief' })).toHaveCount(0)
})

test('an embargoed resource stays out of warm navigation and search until release', async ({ page }) => {
  const state = await setup(page); state.embargo()
  await page.getByRole('button', { name: 'Refresh resources' }).click()
  await expect(page.getByRole('button', { name: 'Read Team A brief' })).toHaveCount(0)
  await page.getByLabel('Search resources').fill('Team A'); await page.getByRole('button', { name: 'Search', exact: true }).click()
  await expect(page.getByText('No released resources match your search.')).toBeVisible()
  expect((await copiedDownload(page)).status).toBe(403)
  state.release(); await page.getByRole('button', { name: 'Refresh resources' }).click()
  await page.getByRole('button', { name: 'Read Team A brief' }).click()
  await expect(page.getByText(canaryA, { exact: true })).toBeVisible()
  expect(state.requests.every(url => !url.includes('preview=true'))).toBe(true)
})

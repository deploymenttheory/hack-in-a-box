import { expect, test, type Page } from '@playwright/test'
import { firstID, mockWorkspace, person } from './fixtures'
import type { components } from '../src/api/schema.gen'
const firstScope = '22222222-2222-4222-8222-222222222222', nextScope = '66666666-6666-4666-8666-666666666666'
type Session = components['schemas']['Session']
function initial(): Session { return { person, csrf_token: 'synthetic-csrf-token-32-characters', can_create_events: false, draft_scope: firstScope } }
async function prepare(page: Page, state: { session: Session | null; reads: number; writes: string[] }) {
  await mockWorkspace(page)
  await page.route('**/api/v1/session', route => { state.reads++; return state.session ? route.fulfill({ json: state.session }) : route.fulfill({ status: 401, json: { error: { code: 'unauthorized', message: 'Sign in again.', request_id: 'synthetic-session' } } }) })
  page.on('request', request => { if (['POST', 'PUT', 'DELETE'].includes(request.method()) && new URL(request.url()).pathname.startsWith('/api/')) state.writes.push(request.url()) })
}
async function putSyntheticLocalCopy(page: Page, scope = firstScope, expired = false) {
  return page.evaluate(({ personID, scope, eventID, expired }) => {
    const target = { personID, sessionScope: scope, eventID, teamID: '44444444-4444-4444-8444-444444444444', noteID: '55555555-5555-4555-8555-555555555555' }
    const name = 'hackbox:mentor-text:v1:' + Object.values(target).join(':')
    const createdAt = Date.now() - (expired ? 30 * 60 * 1000 : 0)
    sessionStorage.setItem(name, JSON.stringify({ version: 1, target, createdAt, expiresAt: createdAt + 30 * 60 * 1000, baseRevision: 1, noteVersion: 0, text: { category: 'technical', evidence: 'SYNTHETIC-PRIVATE-LOCAL-COPY', outcome: '' } }))
    return name
  }, { personID: person.id, scope, eventID: firstID, expired })
}

test('actual shell reports network loss and checks the server after reconnect without replaying writes', async ({ page, context }) => {
  const state = { session: initial(), reads: 0, writes: [] as string[] }
  await prepare(page, state)
  await page.goto('/')
  await expect(page.getByLabel('Connection status')).toContainText('Connected to server')
  const initialReads = state.reads
  await context.setOffline(true)
  await expect(page.getByLabel('Connection status')).toContainText('Offline')
  await expect(page.getByLabel('Connection status')).toContainText('judging require a connection')
  await context.setOffline(false)
  await expect.poll(() => state.reads).toBeGreaterThan(initialReads)
  await expect(page.getByLabel('Connection status')).toContainText('Connected to server')
  expect(state.writes).toEqual([])
})

test('late verified API response cannot clear the actual shell offline state', async ({ page, context }) => {
  const state = { session: initial(), reads: 0, writes: [] as string[] }
  await prepare(page, state)
  let release!: () => void
  const pending = new Promise<void>(resolve => { release = resolve })
  let started = false
  await page.route(/\/api\/v1\/events(?:\?.*)?$/, async route => {
    started = true
    await pending
    await route.fallback()
  })
  await page.goto('/')
  await expect(page.getByLabel('Connection status')).toContainText('Connected to server')
  await expect.poll(() => started).toBe(true)
  await context.setOffline(true)
  expect(await page.evaluate(() => navigator.onLine)).toBe(false)
  await expect(page.getByLabel('Connection status')).toContainText('Offline')
  await page.evaluate(() => window.addEventListener('workspace-response', () => {
    document.documentElement.dataset.lateRecoveryResponse = 'received'
  }, { once: true }))
  release()
  await expect.poll(() => page.evaluate(() => document.documentElement.dataset.lateRecoveryResponse)).toBe('received')
  await page.evaluate(() => new Promise<void>(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))))
  await expect(page.getByLabel('Connection status')).toContainText('Offline')
  expect(state.writes).toEqual([])
})

test('session-scoped local storage survives same-session refresh then expires or clears on session replacement', async ({ page }) => {
  const state = { session: initial(), reads: 0, writes: [] as string[] }
  await prepare(page, state)
  await page.goto('/'); await expect(page.getByRole('heading', { name: 'Your events' })).toBeVisible()
  const name = await putSyntheticLocalCopy(page)
  await page.evaluate(personID => sessionStorage.setItem(`hackbox:draft:${personID}:safe-event`, 'synthetic safe configuration'), person.id)
  await page.reload(); await expect(page.getByRole('heading', { name: 'Your events' })).toBeVisible()
  expect(await page.evaluate(name => sessionStorage.getItem(name), name)).toContain('SYNTHETIC-PRIVATE-LOCAL-COPY')
  state.session = { ...initial(), draft_scope: nextScope }
  await page.reload(); await expect(page.getByRole('heading', { name: 'Your events' })).toBeVisible()
  await expect.poll(() => page.evaluate(name => sessionStorage.getItem(name), name)).toBeNull()
  expect(await page.evaluate(personID => sessionStorage.getItem(`hackbox:draft:${personID}:safe-event`), person.id)).toBe('synthetic safe configuration')
  const expiredName = await putSyntheticLocalCopy(page, nextScope, true)
  await page.evaluate(() => window.dispatchEvent(new Event('focus')))
  await expect.poll(() => page.evaluate(name => sessionStorage.getItem(name), expiredName)).toBeNull()
  expect(state.writes).toEqual([])
})

test('sign-out clears private copies in another real tab without a session-expiry broadcast loop', async ({ page, context }) => {
  const state: { session: Session | null; reads: number; writes: string[] } = { session: initial(), reads: 0, writes: [] }
  const peer = await context.newPage()
  await prepare(page, state); await prepare(peer, state)
  await page.route('**/api/v1/logout', route => { expect(route.request().headers()['x-csrf-token']).toBe(initial().csrf_token); state.session = null; return route.fulfill({ status: 204 }) })
  await page.goto('/'); await peer.goto('/')
  await expect(page.getByRole('heading', { name: 'Your events' })).toBeVisible(); await expect(peer.getByRole('heading', { name: 'Your events' })).toBeVisible()
  const own = await putSyntheticLocalCopy(page), other = await putSyntheticLocalCopy(peer)
  await page.getByRole('button', { name: person.display_name, exact: true }).click()
  const navigated = page.waitForEvent('framenavigated', frame => frame === page.mainFrame())
  await page.getByRole('menuitem', { name: 'Sign out' }).click()
  await navigated; await page.waitForLoadState('domcontentloaded')
  await expect(page.getByRole('link', { name: /Sign in with your organisation/ })).toBeVisible()
  await expect.poll(() => page.evaluate(name => sessionStorage.getItem(name), own)).toBeNull()
  await expect.poll(() => peer.evaluate(name => sessionStorage.getItem(name), other)).toBeNull()
  await expect(peer.getByRole('heading', { name: 'Sign in to continue.' })).toBeVisible()
  const settled = state.reads
  await peer.waitForTimeout(150)
  expect(state.reads).toBeLessThanOrEqual(settled + 1)
  expect(state.writes).toHaveLength(1)
})

test('a delayed401 from the previous session cannot erase a newer session local copy', async ({ page, context }) => {
  const state = { session: initial(), reads: 0, writes: [] as string[] }
  await prepare(page, state)
  let oldRequest!: () => Promise<void>, eventReads = 0
  await page.route('**/api/v1/events*', async route => {
    if (new URL(route.request().url()).pathname !== '/api/v1/events' || ++eventReads !== 1) return route.fallback()
    await new Promise<void>(resolve => { oldRequest = async () => { await route.fulfill({ status: 401, json: { error: { code: 'unauthorized', message: 'Old session ended', request_id: 'synthetic-old-session' } } }); resolve() } })
  })
  await page.goto('/'); await expect(page.getByRole('heading', { name: 'Your events' })).toBeVisible()
  await expect.poll(() => eventReads).toBe(1)
  state.session = { ...initial(), draft_scope: nextScope }
  const peer = await context.newPage(); await prepare(peer, state); await peer.goto('/')
  await expect(peer.getByRole('heading', { name: 'Your events' })).toBeVisible()
  await expect.poll(() => eventReads).toBeGreaterThan(1)
  await expect(page.getByRole('link', { name: 'North Star Hack', exact: true })).toBeVisible()
  const currentCopy = await putSyntheticLocalCopy(page, nextScope)
  const finished = page.waitForEvent('requestfinished', async request => (await request.response())?.status() === 401)
  await oldRequest(); await finished
  await page.evaluate(() => new Promise<void>(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))))
  await expect(page.getByRole('heading', { name: 'Your events' })).toBeVisible()
  expect(await page.evaluate(name => sessionStorage.getItem(name), currentCopy)).toContain('SYNTHETIC-PRIVATE-LOCAL-COPY')
  expect(state.writes).toEqual([])
})

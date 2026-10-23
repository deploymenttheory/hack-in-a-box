import { openEventSection } from './event-navigation-fixture'
import { expect, type Page, type Route } from '@playwright/test'
import { mockWorkspace, firstID, person } from './fixtures'
import type { components } from '../src/api/schema.gen'

type Metadata = components['schemas']['CredentialMetadata']
export const northPerson = person.id, southPerson = '00000000-0000-4000-8000-000000000088'
const northTeam = '00000000-0000-4000-8000-000000000011', southTeam = '00000000-0000-4000-8000-000000000022'
const firstResource = '00000000-0000-4000-8000-000000000033', secondResource = '00000000-0000-4000-8000-000000000044'
export async function accessFixture(page: Page, account: 'north' | 'south' = 'north') {
  const workspace = await mockWorkspace(page), event = workspace.events[0]
  event.can_edit = false; event.can_manage_owner = false; event.can_read_staff = false; event.can_transition = false
  event.stage = 'preparation'; event.published_at = '2027-06-13T09:00:00Z'
  const state = { account, signedOut: false, member: true, assigned: true, status: 200, revision: 1, hold: false, delayed: undefined as Route | undefined, items: [] as Metadata[], reveals: [] as { id: string; revision: string | undefined }[], credentialReads: 0 }
  const currentPerson = () => state.account === 'north' ? northPerson : southPerson
  const currentTeam = () => state.account === 'north' ? northTeam : southTeam
  const name = () => state.account === 'north' ? 'North' : 'South'
  const metadata = (id: string, label: string): Metadata => ({ id, label, version: 1, active: true, team_ids: [], updated_at: '2027-06-14T09:00:00Z', resource_type: 'credential', available_at: null, expires_at: null, readiness: 'ready' })
  state.items = [metadata(firstResource, 'Team account'), metadata(secondResource, 'Room access code')]
  const secret = (id = firstResource) => `SYNTHETIC-${state.account.toUpperCase()}-VALUE-${state.items.find(item => item.id === id)?.version ?? 1}-${id === firstResource ? 'ACCOUNT' : 'ROOM'}`
  await page.route('**/api/v1/session', route => state.signedOut ? route.fulfill({ status: 401, json: { error: { code: 'unauthenticated', message: 'Synthetic expiry' } } }) : route.fulfill({ json: { person: { ...person, id: currentPerson(), display_name: `${name()} competitor`, platform_admin: false }, csrf_token: 'synthetic-csrf-token-32-characters', can_create_events: false } }))
  await page.route('**/api/v1/logout', route => { state.signedOut = true; return route.fulfill({ status: 204 }) })
  await page.route(`**/events/${firstID}/teams`, route => route.fulfill({ json: { revision: state.revision, can_manage: false, can_create: false, frozen: true, freeze_at: event.configuration.team_freeze, teams: state.member ? [{ id: currentTeam(), name: `${name()} team`, state: 'published', frozen: true, captain: currentPerson(), published_captain: currentPerson(), invitation_code: null, draft: [], published: [{ id: currentPerson(), name: `${name()} competitor` }], allocation_locked: true, capacity: null }] : [], people: [], requests: [] } }))
  await page.route(`**/events/${firstID}/credentials`, route => {
    state.credentialReads++
    return route.fulfill({ json: { revision: state.revision, can_manage: false, items: state.member && state.assigned ? state.items : [] } })
  })
  await page.route(`**/events/${firstID}/credentials/*/reveal`, async route => {
    const request = route.request(), id = new URL(request.url()).pathname.split('/').at(-2)!
    expect(request.method()).toBe('POST'); expect(request.headers()['x-csrf-token']).toBe('synthetic-csrf-token-32-characters')
    state.reveals.push({ id, revision: request.headers()['if-match'] })
    if (state.hold) { state.delayed = route; return }
    const status = state.status !== 200 ? state.status : !state.member || !state.assigned ? 403 : request.headers()['if-match'] !== `"${state.revision}"` ? 409 : 200
    if (status !== 200) return route.fulfill({ status, json: { error: { code: status === 409 ? 'revision_conflict' : status === 401 ? 'unauthenticated' : 'forbidden', message: 'Synthetic rejection' } } })
    return route.fulfill({ headers: { 'Cache-Control': 'no-store' }, json: { id, version: state.items.find(item => item.id === id)!.version, value: secret(id) } })
  })
  await page.goto(`/events/${firstID}`)
  await openEventSection(page, 'Team access')
  await expect(page.getByRole('heading', { name: `${name()} team`, exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Reveal Team account', exact: true })).toBeVisible()
  return { state, secret, firstResource, secondResource, event }
}

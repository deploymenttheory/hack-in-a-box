import { expect, type Page } from '@playwright/test'
import { firstID, mockWorkspace } from './fixtures'
import type { components } from '../src/api/schema.gen'

type Inventory = components['schemas']['CredentialPage']
type Command = components['schemas']['CredentialCommand']
export const resourceIDs = ['00000000-0000-4000-8000-000000000041', '00000000-0000-4000-8000-000000000042', '00000000-0000-4000-8000-000000000043']
export const inventoryTeamIDs = ['00000000-0000-4000-8000-000000000031', '00000000-0000-4000-8000-000000000032']
export function fixtureInventory(): Inventory {
  return { revision: 10, can_manage: true, items: [
    { id: resourceIDs[0], label: 'Cloud account', version: 1, resource_type: 'account', active: true, team_ids: [inventoryTeamIDs[0]], available_at: null, expires_at: null, readiness: 'ready', updated_at: '2026-10-22T09:00:00Z' },
    { id: resourceIDs[1], label: 'Build access code', version: 1, resource_type: 'access_code', active: true, team_ids: [], available_at: null, expires_at: null, readiness: 'unassigned', updated_at: '2026-10-22T09:00:00Z' },
    { id: resourceIDs[2], label: 'Expired licence', version: 1, resource_type: 'licence', active: true, team_ids: [inventoryTeamIDs[1]], available_at: null, expires_at: '2020-01-01T00:00:00Z', readiness: 'expired', updated_at: '2026-10-22T09:00:00Z' },
  ] }
}
export async function mockInventory(page: Page) {
  const workspace = await mockWorkspace(page); workspace.events[0].configuration.timezone = 'UTC'
  const state: { value: Inventory; commands: Omit<Command, 'value'>[]; values: string[]; denied: boolean; failure: boolean; stale: boolean } = { value: fixtureInventory(), commands: [], values: [], denied: false, failure: false, stale: false }
  await page.route(`**/events/${firstID}/teams`, route => {
    expect(route.request().method()).toBe('GET')
    return route.fulfill({ json: { revision: state.value.revision, can_manage: true, can_create: false, frozen: true, freeze_at: workspace.events[0].configuration.team_freeze, people: [], requests: [], teams: inventoryTeamIDs.map((id, index) => ({ id, name: ['North Stars', 'South Stars'][index], state: 'published', captain: null, published_captain: null, invitation_code: null, frozen: true, allocation_locked: true, capacity: null, draft: [], published: [] })) } })
  })
  await page.route(`**/events/${firstID}/credentials`, route => {
    const fail = (status: number, code: string) => route.fulfill({ status, headers: { 'Cache-Control': 'no-store' }, json: { error: { code, message: 'Synthetic inventory boundary', request_id: 'synthetic' } } })
    if (state.denied) return fail(403, 'forbidden')
    if (route.request().method() === 'POST') {
      const { value, ...command } = route.request().postDataJSON() as Command
      state.commands.push(command); if (value) state.values.push(value)
      if (state.stale) { state.stale = false; state.value.revision++; return fail(409, 'conflict') }
      if (state.failure) return fail(503, 'unavailable')
      expect(route.request().headers()['if-match']).toBe(`"${state.value.revision}"`)
      const next = structuredClone(state.value); next.revision++
      if (command.action === 'create') next.items.push({ id: '00000000-0000-4000-8000-000000000044', label: command.label!, resource_type: command.resource_type ?? 'credential', active: true, version: 1, team_ids: command.team_ids ?? [], available_at: command.available_at ?? null, expires_at: command.expires_at ?? null, readiness: 'unassigned', updated_at: '2026-10-22T10:00:00Z' })
      else next.items = next.items.map(r => r.id !== command.id ? r : command.action === 'assign' ? { ...r, team_ids: command.team_ids ?? [], readiness: command.team_ids?.length ? 'ready' : 'unassigned' } : command.action === 'revoke' ? { ...r, active: false, readiness: 'revoked' } : command.action === 'replace' ? { ...r, version: r.version + 1, label: command.label! } : command.action === 'metadata' ? { ...r, label: command.label!, resource_type: command.resource_type!, available_at: command.available_at ?? null, expires_at: command.expires_at ?? null } : r)
      state.value = next
    }
    return route.fulfill({ headers: { 'Cache-Control': 'no-store' }, json: state.value })
  })
  await page.route('**/credentials/*/reveal', route => { throw new Error(`Organiser inventory must never reveal stored values: ${route.request().method()}`) })
  return { ...workspace, state }
}

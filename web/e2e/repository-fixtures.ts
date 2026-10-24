import { expect, type Page } from '@playwright/test'
import { firstID, fixtureEvent, mockWorkspace, person } from './fixtures'
import type { components } from '../src/api/schema.gen'
type S = components['schemas']
export const repositoryTeam = '33333333-3333-4333-8333-333333333333', uncertainTeam = '55555555-5555-4555-8555-555555555555', unusedTeam = '66666666-6666-4666-8666-666666666666', repositoryMember = '77777777-7777-4777-8777-777777777777'
export async function repositoryFixture(page: Page, regular = false) {
  await mockWorkspace(page)
  const event = { ...fixtureEvent(), stage: 'preparation' as const, published_at: '2026-10-01T12:00:00Z', can_read_staff: !regular, can_edit: !regular }
  const member: S['RepositoryAccessMember'] = { person_id: person.id, github_id: 98765, link_revision: 2, permission: 'push', role: 'competitor', coverage_start: null, coverage_end: null }
  const rows: S['TeamRepository'][] = [{ team_id: repositoryTeam, name: 'synthetic-repository', organization: 'synthetic-org', github_repository_id: 123456, state: 'ready', diagnostic: '', unused: false }, { team_id: uncertainTeam, name: 'uncertain-repository', organization: 'synthetic-org', github_repository_id: null, state: 'ambiguous', diagnostic: 'reconciliation_required', unused: false }, { team_id: unusedTeam, name: 'disbanded-repository', organization: 'synthetic-org', github_repository_id: null, state: 'review', diagnostic: '', unused: true }]
  const app: S['GitHubAppStatus'] = { revision: 1, configured: true, installation_id: 45678, webhook_configured: true, configuration: { app_id: 42, app_slug: 'synthetic-app', organization_id: 99, organization: 'synthetic-org', template_repository: 'synthetic-template' }, diagnostics: { code: 'healthy', checks: [{ operation: 'manage_repository_invitations', required: ['administration:write'], allowed: true }], checked_at: '2026-10-24T12:00:00Z' } }
  const access: S['RepositoryAccessView'] = { revision: 1, team_id: repositoryTeam, plan: null, grants: [], execution: [], can_manage: !regular, grants_open: true, flags: [{ code: 'verified_account_required', person_id: repositoryMember, github_id: null }], observed_at: null }
  const own: S['OwnRepositoryInvitations'] = { team_id: repositoryTeam, repository_id: 123456, current_github_id: 98765, link_revision: 2, grants: [{ id: '88888888-8888-4888-8888-888888888888', member, app_managed: true, effect_uncertain: false, state: 'pending', invitation_id: 91234, diagnostic: 'awaiting_acceptance', observed_at: '2026-10-24T12:00:00Z' }], observed_at: '2026-10-24T12:00:00Z', diagnostic: '' }
  const state = { event, rows, app, access, own, denyAccess: false, holdAccess: undefined as Promise<void> | undefined, accessReads: 0, ownReads: 0, mode: 'normal' as 'normal' | 'lost-before' | 'lost-after', writes: [] as { path: string; body: Record<string, unknown>; revision: string }[], receipts: new Map<string, { actor_id: string; plan: S['RepositoryAccessPlan'] }>() }
  const sync = () => { app.revision = event.revision; access.revision = event.revision }
  await page.route(`**/api/v1/events/${firstID}`, route => { sync(); return route.fulfill({ json: event }) })
  await page.route(`**/api/v1/events/${firstID}/teams`, route => route.fulfill({ json: { revision: event.revision, can_manage: !regular, can_create: false, frozen: true, freeze_at: '2026-10-01T12:00:00Z', teams: rows.map((row, i) => ({ id: row.team_id, name: ['Synthetic team', 'Uncertain team', 'Disbanded team'][i], state: row.unused ? 'disbanded' : 'published', captain: person.id, published_captain: person.id, invitation_code: null, frozen: true, draft: [], published: [{ id: person.id, name: person.display_name }] })), people: [], requests: [] } }))
  await page.route(`**/api/v1/events/${firstID}/github-app**`, async route => {
    const path = new URL(route.request().url()).pathname
    if (route.request().method() === 'GET') { sync(); return route.fulfill({ json: app }) }
    const body = route.request().postDataJSON() ?? {}
    state.writes.push({ path, body, revision: route.request().headers()['if-match'] })
    expect(route.request().headers()['x-csrf-token']).toBeTruthy()
    if (state.mode === 'lost-before') return route.abort('failed')
    if (body.action === 'configure') { app.configuration = { app_id: body.app_id, app_slug: body.app_slug, organization_id: body.organization_id, organization: body.organization, template_repository: body.template_repository }; app.configured = true; app.installation_id = 0 }
    if (body.action === 'disconnect') { app.configured = false; app.installation_id = 0; delete app.configuration }
    event.revision++; sync()
    if (state.mode === 'lost-after') return route.abort('failed')
    if (path.endsWith('/installation-authorizations')) return route.fulfill({ json: { url: 'https://github.com/apps/synthetic-app/installations/new?state=synthetic-only' } })
    return route.fulfill({ json: app })
  })
  await page.route(`**/api/v1/events/${firstID}/repositories**`, async route => {
    const request = route.request(), url = new URL(request.url()), path = url.pathname
    const error = (status: number) => route.fulfill({ status, json: { error: { code: 'forbidden', message: 'Synthetic current denial', request_id: 'synthetic' } } })
    sync()
    if (request.method() === 'GET') {
      if (path.includes('/receipts/')) { const id = path.split('/').at(-1)!, action = url.searchParams.get('action')!, receipt = state.receipts.get(`${action}:${id}`); return route.fulfill({ json: { action, actor_id: receipt?.actor_id ?? null, plan: receipt?.plan ?? null } }) }
      if (path.endsWith('/access')) { state.accessReads++; if (state.holdAccess) await state.holdAccess; if (state.denyAccess) return error(403); return route.fulfill({ json: access }) }
      if (path.endsWith('/invitations')) { state.ownReads++; return route.fulfill({ json: own }) }
      return route.fulfill({ json: regular ? rows.slice(0, 1) : rows })
    }
    const body = request.postDataJSON(); state.writes.push({ path, body, revision: request.headers()['if-match'] })
    expect(request.headers()['x-csrf-token']).toBeTruthy()
    if (request.headers()['if-match'] !== `"${event.revision}"`) return route.fulfill({ status: 409, json: { error: { code: 'revision_conflict', message: 'Synthetic stale revision', request_id: 'synthetic' } } })
    if (state.mode === 'lost-before') return route.abort('failed')
    if (!path.endsWith('/access')) {
      if (body.action === 'reconcile') { expect(body.team_id).toBe(uncertainTeam); rows[1].state = 'ready'; rows[1].github_repository_id = 223344; rows[1].diagnostic = '' }
      event.revision++; sync(); if (state.mode === 'lost-after') return route.abort('failed'); return route.fulfill({ json: regular ? rows.slice(0, 1) : rows })
    }
    let receipt = access.plan
    if (body.action === 'review') {
      receipt = { id: body.plan_id, version: (access.plan?.version ?? 0) + 1, event_id: firstID, team_id: repositoryTeam, repository_id: 123456, fingerprint: 'synthetic-current-membership', reason: body.reason, approval_reason: '', members: [member], actions: [{ id: crypto.randomUUID(), kind: 'invite', member, grant_id: null, previous_state: '' }], flags: access.flags, created_at: '2026-10-24T12:00:00Z', approved_at: null, approved_by: null }
      access.plan = receipt
    }
    if (body.action === 'approve') { expect(body.plan_id).toBe(access.plan?.id); expect(body.plan_version).toBe(access.plan?.version); receipt = { ...access.plan!, approval_reason: body.reason, approved_at: '2026-10-24T12:01:00Z', approved_by: person.id }; access.plan = receipt; access.execution = receipt.actions.map(action => ({ action_id: action.id, state: 'planned', diagnostic: '', attempted: false, updated_at: '2026-10-24T12:01:00Z' })) }
    if (receipt && body.action !== 'refresh') state.receipts.set(`${body.action}:${receipt.id}`, { actor_id: person.id, plan: structuredClone(receipt) })
    event.revision++; sync(); if (state.mode === 'lost-after') return route.abort('failed'); return route.fulfill({ json: { receipt, current: access } })
  })
  return state
}

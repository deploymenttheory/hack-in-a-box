import { expect, type Page } from '@playwright/test'
import { firstID, secondID, mockWorkspace, person } from './fixtures'
import type { components } from '../src/api/schema.gen'
type Request = components['schemas']['HelpRequest']
type History = components['schemas']['HelpTransition']
type Help = components['schemas']['HelpPage']
export const teamID = '11111111-1111-4111-8111-111111111111', otherTeam = '22222222-2222-4222-8222-222222222222', requestID = '33333333-3333-4333-8333-333333333333', otherRequest = '44444444-4444-4444-8444-444444444444'
export const helper = { id: '55555555-5555-4555-8555-555555555555', name: 'Synthetic Helper', kind: 'helper' as const }, secondHelper = { id: '66666666-6666-4666-8666-666666666666', name: 'Second Helper', kind: 'helper' as const }, member = { id: person.id, name: person.display_name, kind: 'team' as const }, organiser = { id: '77777777-7777-4777-8777-777777777777', name: 'Synthetic Organiser', kind: 'organiser' as const }
export type Actor = { id: string; name: string; kind: 'team' | 'helper' | 'organiser' }
const now = '2027-06-20T09:00:00Z'
export function helpRequest(id = requestID, team = teamID, category: Request['category'] = 'setup'): Request { return { id, event_id: firstID, team_id: team, requester_id: person.id, revision: 1, category, description: 'PRIVATE HELP <script>window.badHelp=true</script>', location: 'PRIVATE LOCATION', meeting_url: '', status: 'waiting', assignee_id: null, waiting_cycle: 1, requested_at: now, waiting_since: now, first_claimed_at: null, claimed_at: null, resolved_at: null, updated_at: now } }
export function helpState() { return { records: [helpRequest(), helpRequest(otherRequest, otherTeam, 'logistics')], history: new Map<string, History[]>(), commands: [] as { actor: string; action: string; key: string; id: string }[], receipts: new Map<string, string>(), denied: false, expired: false, switched: false, open: true, failCreate: false, lostCreate: false, lostAction: false, targetEligible: true, allowOtherEvent: false, secondRecord: { ...helpRequest(), event_id: secondID, description: 'SECOND EVENT PROBLEM' } } }
export type State = ReturnType<typeof helpState>
export async function mockHelp(page: Page, state: State, actor: Actor = member) {
  await mockWorkspace(page)
  const name = (id: string | null) => [helper, secondHelper, member, organiser].find(p => p.id === id)?.name ?? ''
  const visible = (r: Request) => !state.denied && (actor.kind !== 'team' || r.team_id === teamID) && (r.category !== 'competition_output' || state.open)
  function projection(r: Request): Help {
    const assigned = actor.kind !== 'team' && r.assignee_id === actor.id, managed = actor.kind === 'organiser', release = r.status === 'claimed' && (assigned || managed)
    const history = state.history.get(r.id) ?? [{ revision: 1, action: 'create', status: 'waiting', actor_id: r.requester_id, assignee_id: null, at: now }]
    return { event_name: r.event_id === secondID ? 'Southern Lights Hack' : 'North Star Hack', timezone: 'Europe/London', request: r, history, assignee_name: name(r.assignee_id), can_claim: r.status === 'waiting' && actor.kind !== 'team', can_release: release, can_transfer: release, can_resolve: release, can_reopen: ['resolved', 'cancelled'].includes(r.status) && (managed || actor.kind === 'team'), can_cancel: ['waiting', 'claimed'].includes(r.status) && (managed || actor.kind === 'team'), transfer_targets: release ? [helper, secondHelper, organiser].filter(p => p.id !== r.assignee_id && (state.targetEligible || p.id !== secondHelper.id)).map(p => ({ person_id: p.id, name: p.name })) : [] }
  }
  await page.route('**/api/v1/session', route => state.expired ? route.fulfill({ status: 401, json: { error: { code: 'unauthorized', message: 'Expired', request_id: 'synthetic' } } }) : route.fulfill({ json: { person: { id: state.switched ? secondHelper.id : actor.id, display_name: state.switched ? 'Different account' : actor.name, platform_admin: actor.kind === 'organiser' }, draft_scope: state.switched ? '99999999-9999-4999-8999-999999999999' : '88888888-8888-4888-8888-888888888888', csrf_token: 'synthetic-csrf-token-32-characters', can_create_events: false } }))
  await page.route('**/api/v1/events/*/help**', async route => {
    const req = route.request(), path = new URL(req.url()).pathname, id = path.split('/help/')[1]?.split('/')[0]
    const error = (status: number, code: string) => route.fulfill({ status, json: { error: { code, message: 'Synthetic native help contract', request_id: 'synthetic' } } })
    if ((!path.includes(firstID) && !(state.allowOtherEvent && path.includes(secondID))) || state.denied) return error(404, 'not_found')
    if (!id) {
      const records = state.records.filter(visible)
      return route.fulfill({ json: { can_manage: actor.kind === 'organiser', can_request: actor.kind === 'team' && !state.switched, request_team: actor.kind === 'team' && !state.switched ? { id: teamID, name: 'Your published team' } : null, waiting: records.filter(r => r.status === 'waiting').length, claimed: records.filter(r => r.status === 'claimed').length, resolved: records.filter(r => r.status === 'resolved').length, cancelled: records.filter(r => r.status === 'cancelled').length, items: records.map(r => ({ id: r.id, team_id: r.team_id, category: r.category, status: r.status, assignee_id: r.assignee_id, assignee_name: name(r.assignee_id), requested_at: r.requested_at, updated_at: r.updated_at, waiting_seconds: 1200, response_seconds: null, completion_seconds: null, overdue: r.status === 'waiting' })) } })
    }
    const record = path.includes(secondID) ? state.secondRecord : state.records.find(r => r.id === id)
    if (req.method() === 'GET') return record && visible(record) ? route.fulfill({ json: projection(record) }) : error(404, 'not_found')
    const key = req.headers()['idempotency-key']; expect(key).toMatch(/^[a-f0-9-]{36}$/i)
    if (state.receipts.get(key) === id && record && visible(record)) return route.fulfill({ json: projection(record) })
    if (req.method() === 'PUT') {
      const input = req.postDataJSON() as components['schemas']['HelpInput']; state.commands.push({ actor: actor.id, action: 'create', key, id })
      if (state.failCreate) return route.abort('failed')
      if (!input.location.trim() && !input.meeting_url.trim()) return error(400, 'invalid')
      if (actor.kind !== 'team' || input.team_id !== teamID) return error(404, 'not_found')
      if (input.category === 'competition_output' && !state.open) return error(409, 'event_blocked')
      expect(req.headers()['if-match']).toBe('"0"')
      const created = { ...helpRequest(id), ...input }; state.records.push(created); state.receipts.set(key, id)
      if (state.lostCreate) return route.abort('failed')
      return route.fulfill({ json: projection(created) })
    }
    if (!record || !visible(record)) return error(404, 'not_found')
    const input = req.postDataJSON() as components['schemas']['HelpCommand']; state.commands.push({ actor: actor.id, action: input.action, key, id })
    if (req.headers()['if-match'] !== `"${record.revision}"`) return error(409, 'conflict')
    const allowed = projection(record), permission = { claim: allowed.can_claim, release: allowed.can_release, transfer: allowed.can_transfer, resolve: allowed.can_resolve, reopen: allowed.can_reopen, cancel: allowed.can_cancel }
    if (!permission[input.action]) return error(403, 'forbidden')
    if (input.action === 'transfer' && !allowed.transfer_targets.some(p => p.person_id === input.assignee_id)) return error(400, 'invalid')
    const initial = projection(record).history
    if (input.action === 'claim') { record.status = 'claimed'; record.assignee_id = actor.id; record.first_claimed_at ??= now; record.claimed_at = now }
    if (input.action === 'transfer') record.assignee_id = input.assignee_id
    if (input.action === 'resolve') { record.status = 'resolved'; record.resolved_at = now }
    if (input.action === 'cancel') { record.status = 'cancelled'; record.assignee_id = null }
    if (['reopen', 'release'].includes(input.action)) { record.status = 'waiting'; record.assignee_id = null; record.waiting_cycle++ }
    record.revision++; state.receipts.set(key, id)
    state.history.set(id, [...initial, { revision: record.revision, action: input.action, status: record.status, actor_id: actor.id, assignee_id: record.assignee_id, at: now }])
    if (state.lostAction) return route.abort('failed')
    return route.fulfill({ json: projection(record) })
  })
}

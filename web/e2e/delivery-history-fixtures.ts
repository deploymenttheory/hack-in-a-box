import type { Page } from '@playwright/test'
import type { components } from '../src/api/schema.gen'
import { firstID, mockWorkspace, person } from './fixtures'

export const historyPath = `/events/${firstID}/communications/deliveries`
export const deliveryIDs = { never: 'a1000000-0000-4000-8000-000000000001', accepted: 'a1000000-0000-4000-8000-000000000002', ambiguous: 'a1000000-0000-4000-8000-000000000003', retryable: 'a1000000-0000-4000-8000-000000000004', local: 'a1000000-0000-4000-8000-000000000005', omitted: 'a1000000-0000-4000-8000-000000000006', teams: 'a1000000-0000-4000-8000-000000000007' }
export const detailPath = (kind: keyof typeof deliveryIDs) => `${historyPath}/${deliveryIDs[kind]}`
type Item = components['schemas']['DeliveryHistoryItem']
type Detail = components['schemas']['DeliveryHistoryDetail']
type Command = components['schemas']['DeliveryReviewCommand']
export async function deliveryFixture(page: Page) {
  const base = await mockWorkspace(page), campaignID = 'b1000000-0000-4000-8000-000000000001'
  const state = { allowed: true, expired: false, archived: false, paused: false, drop: false, reject: false, transientReads: 0, pagedAttempts: false, actor: { ...person, platform_admin: false }, requests: [] as string[], posts: [] as { body: Command; revision?: string }[] }
  const rows: Item[] = Object.entries(deliveryIDs).map(([kind, id], i) => {
    const status = { never: 'never_attempted', accepted: 'provider_accepted', ambiguous: 'ambiguous', retryable: 'retryable', local: 'local_persisted', omitted: 'unavailable', teams: 'provider_accepted' }[kind] as Item['state']
    const attempts = ['never', 'local', 'omitted'].includes(kind) ? 0 : 1
    return { id, campaign_id: campaignID, campaign_version: 3, recipient_id: kind === 'teams' ? null : person.id, recipient_name: kind === 'teams' ? '' : `Synthetic ${kind} recipient`, channel: kind === 'teams' ? 'teams' : kind === 'local' ? 'in_app' : 'smtp', purpose: kind === 'omitted' ? 'omission' : kind === 'teams' ? 'broadcast' : 'campaign', selected_at: `2027-06-20T09:0${6 - i}:00Z`, state: status, reason: kind === 'omitted' ? 'address_unverified' : kind === 'local' ? 'recorded_in_app' : kind === 'ambiguous' ? 'acceptance_unknown' : kind === 'retryable' ? 'relay_temporary_rejection' : kind === 'never' ? 'unknown' : 'relay_accepted', attempted: attempts > 0, attempts, revision: ['local', 'omitted'].includes(kind) ? 0 : 1, operation_state: ['local', 'omitted'].includes(kind) ? '' : ['accepted', 'teams'].includes(kind) ? 'delivered' : kind === 'ambiguous' ? 'ambiguous' : 'pending', cancelled: false }
  })
  const commands = new Map<string, { command: Command; revision?: string; id: string }>(), reviews = new Map<string, Detail['reviews']>()
  const actions = (item: Item): Detail['actions'] => state.archived || item.cancelled || !item.revision ? [] : item.operation_state === 'pending' && ['provider_accepted', 'ambiguous'].includes(item.state) ? ['cancel_resend'] : item.state === 'ambiguous' ? state.paused ? ['resolve_no_resend'] : ['resolve_no_resend', 'resend_acknowledged'] : item.state === 'provider_accepted' ? state.paused ? [] : ['resend_acknowledged'] : item.state === 'retryable' ? state.paused ? ['cancel'] : ['retry', 'cancel'] : item.state === 'never_attempted' ? ['cancel'] : []
  const detail = (item: Item, before?: string | null): Detail => ({ item: structuredClone(item), archived: state.archived, paused: state.paused, actions: actions(item), attempts: item.attempted ? [{ id: 'c1000000-0000-4000-8000-000000000001', number: before ? 1 : state.pagedAttempts ? 2 : 1, started_at: '2027-06-20T09:20:00Z', state: item.state === 'in_progress' || item.state === 'never_attempted' || item.state === 'unavailable' || item.state === 'local_persisted' ? 'ambiguous' : item.state, reason: item.reason, recorded_at: item.state === 'ambiguous' ? null : '2027-06-20T09:21:00Z', http_status: item.channel === 'teams' ? 202 : null, retry_at: null }] : [], next_attempt_number: state.pagedAttempts && !before && item.attempted ? 2 : null, reviews: reviews.get(item.id) ?? [], next_review_id: null })
  await page.route('**/api/v1/session', route => state.expired ? route.fulfill({ status: 401, json: { error: { code: 'unauthenticated', message: 'Sign in.', request_id: 'synthetic' } } }) : route.fulfill({ json: { person: state.actor, draft_scope: 'c1000000-0000-4000-8000-000000000011', csrf_token: 'synthetic-csrf-token-32-characters', can_create_events: false } }))
  await page.route('**/api/v1/events/**', async route => {
    const request = route.request(), url = new URL(request.url()), path = url.pathname
    state.requests.push(`${request.method()} ${path}`)
    const error = (status: number) => route.fulfill({ status, json: { error: { code: status === 409 ? 'conflict' : status === 503 ? 'unavailable' : 'forbidden', message: 'Not currently available.', request_id: 'synthetic' } } })
    if (path.includes('/communications') && (!state.allowed || state.expired)) return error(state.expired ? 401 : 403)
    if (path.endsWith('/communications')) return route.fulfill({ json: { revision: 1, templates: [], audiences: [], variables: [], preview: null } })
    if (path.endsWith('/communications/campaigns')) return route.fulfill({ json: { revision: 1, timezone: 'Europe/London', can_manage: !state.archived, campaigns: [], providers: ['smtp', 'teams', 'in_app'].map(channel => ({ channel, config_revision: 'opaque-local-revision', available: channel !== 'teams', supports_self_test: channel !== 'teams', self_test_available: channel !== 'teams', reason: channel === 'teams' ? 'not_configured' : 'ready', self_test_reason: channel === 'teams' ? 'broadcast_only' : 'ready', setup_url: '' })) } })
    if (path.includes('/communications/deliveries')) {
      if (request.method() === 'GET' && state.transientReads > 0) { state.transientReads--; return error(503) }
      if (path.endsWith('/deliveries')) {
        const filtered = rows.filter(row => !url.searchParams.get('channel') || row.channel === url.searchParams.get('channel')).filter(row => !url.searchParams.get('campaign_id') || row.campaign_id === url.searchParams.get('campaign_id'))
        const offset = url.searchParams.has('before_id') ? filtered.findIndex(row => row.id === url.searchParams.get('before_id')) + 1 : 0, items = filtered.slice(offset, offset + 3), last = items.at(-1)
        return route.fulfill({ json: { items, next_cursor: offset + 3 < filtered.length && last ? { at: last.selected_at, id: last.id } : null, archived: state.archived, paused: state.paused } })
      }
      const row = rows.find(item => item.id === path.split('/').at(-1))
      if (!row || !path.startsWith(`/api/v1/events/${firstID}/`)) return error(404)
      if (request.method() === 'POST') {
        const body = request.postDataJSON() as Command, revision = request.headers()['if-match']; state.posts.push({ body, revision })
        if (state.archived) return error(403)
        const previous = commands.get(body.request_id)
        if (previous) return JSON.stringify(previous.command) === JSON.stringify(body) && previous.revision === revision && previous.id === row.id ? route.fulfill({ json: detail(row) }) : error(409)
        if (state.reject || revision !== `"${row.revision}"` || !actions(row).includes(body.action)) return error(409)
        commands.set(body.request_id, { command: body, revision, id: row.id }); reviews.set(row.id, [{ id: body.request_id, actor_id: state.actor.id, expected_revision: row.revision, action: body.action, reason: body.reason, created_at: '2027-06-20T10:00:00Z' }, ...(reviews.get(row.id) ?? [])]); row.revision++
        if (body.action === 'resend_acknowledged') row.operation_state = 'pending'
        if (body.action === 'cancel' || body.action === 'cancel_resend') { row.cancelled = true; row.operation_state = 'cancelled' }
        if (body.action === 'resolve_no_resend') row.cancelled = true
        if (state.drop) { state.drop = false; return route.abort('failed') }
        return route.fulfill({ json: detail(row) })
      }
      return route.fulfill({ json: detail(row, url.searchParams.get('before_attempt')) })
    }
    return route.fallback()
  })
  return { state, rows, commands, reviews, events: base.events }
}

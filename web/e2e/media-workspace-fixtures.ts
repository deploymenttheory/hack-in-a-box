import { expect, type Page } from '@playwright/test'
import { firstID, mockWorkspace, person } from './fixtures'
import type { components } from '../src/api/schema.gen'
type Item = components['schemas']['MediaItem']
export const mediaID = '22222222-2222-4222-8222-222222222222'
export const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jL1sAAAAASUVORK5CYII=', 'base64')
export const source = (): Item => ({ id: mediaID, version: 1, kind: 'photo', title: 'Reviewed team photograph', caption: 'A team building together', captured_at: null, state: 'released', consent_state: 'reviewed', recording_url: '', can_read_original: true, can_read_history: false, can_edit: false, can_review: false, can_publish: false, can_withdraw: false, people: [], challenge_related: false })
export function state() { return { role: 'attendee' as 'attendee' | 'photographer' | 'organiser', items: [source()], revision: 1, denied: false, consent: 'yes' as 'yes' | 'no' | 'unknown', choiceRevision: 1, archived: false, lostUpload: false, uploads: [] as { key: string; input: unknown }[], writes: [] as string[], receipts: new Map<string, Item>(), sessions: 0, deferSession: undefined as undefined | (() => Promise<void>), deferPreflight: undefined as undefined | (() => Promise<void>), preflights: 0, transient: false, transientAfterLostUpload: false } }
export async function mockMedia(page: Page, s: ReturnType<typeof state>) {
  await mockWorkspace(page)
  const error = { error: { code: 'forbidden', message: 'Current media authority lost', request_id: 'synthetic' } }
  await page.route('**/api/v1/session', async route => { s.sessions++; if (s.sessions > 1) await s.deferSession?.(); return route.fulfill({ json: { person: { ...person, platform_admin: false }, draft_scope: '33333333-3333-4333-8333-333333333333', csrf_token: 'synthetic-csrf-token-32-characters', can_create_events: false } }) })
  await page.route('**/api/v1/events/*/photo-consent', async route => {
    if (!route.request().url().includes(firstID)) return route.fulfill({ status: 403, json: error })
    if (route.request().method() === 'PUT') { s.consent = route.request().postDataJSON().choice; s.choiceRevision++; if (s.consent !== 'yes') s.items = s.items.map(item => ({ ...item, can_read_original: false })) }
    return route.fulfill({ json: { choice: s.consent, revision: s.choiceRevision, can_opt_in: !s.archived, can_revoke: true } })
  })
  await page.route('**/api/v1/events/*/media**', async route => {
    const request = route.request(), path = new URL(request.url()).pathname
    if (request.method() === 'GET' && new URL(request.url()).searchParams.get('limit') === '1') { s.preflights++; await s.deferPreflight?.() }
    const staff = s.role !== 'attendee'
    if (s.transient && request.method() === 'GET') return route.fulfill({ status: 503, json: { error: { code: 'unavailable', message: 'Temporary read failure', request_id: 'synthetic' } } })
    if (s.denied || !path.includes(firstID)) return route.fulfill({ status: 403, json: error })
    if (path.endsWith('/consent-guidance')) return staff ? route.fulfill({ json: { items: [{ person_id: person.id, display_name: 'Synthetic participant', choice: s.consent, eligible: true }], next_cursor: null } }) : route.fulfill({ status: 403, json: error })
    if (path.endsWith('/original') || path.endsWith('/thumbnail')) return s.consent === 'yes' ? route.fulfill({ contentType: 'application/octet-stream', body: png }) : route.fulfill({ status: 403, json: error })
    if (path.endsWith('/upload')) {
      const key = request.headers()['idempotency-key'], input = request.postDataJSON(); s.uploads.push({ key, input }); expect(request.headers()['x-csrf-token']).toBe('synthetic-csrf-token-32-characters')
      const existing = s.receipts.get(key); if (existing) return route.fulfill({ json: existing })
      const item = { ...source(), id: input.id, title: input.metadata.title, caption: input.metadata.caption, state: 'draft' as const, can_edit: true, can_read_original: s.consent === 'yes', people: input.metadata.people.map((id: string) => ({ id, name: 'Synthetic participant', consent: s.consent, eligible: true })) }; s.items.push(item); s.receipts.set(key, item); s.revision++
      if (s.lostUpload) { if (s.transientAfterLostUpload) s.transient = true; return route.abort('failed') }
      return route.fulfill({ json: item })
    }
    if (request.method() === 'PUT') {
      const input = request.postDataJSON(), id = path.split('/media/')[1], item = s.items.find(i => i.id === id)!; s.writes.push(input.action)
      if (input.action === 'review') { expect(input.review).toEqual({ identification_reviewed: true, no_identifiable_people: true, content_reviewed: true }); item.state = 'reviewed'; item.can_publish = true; item.consent_state = 'reviewed' }
      if (input.action === 'publish') item.state = 'released'
      item.version++; s.revision++; return route.fulfill({ json: item })
    }
    const items = s.role === 'attendee' ? s.items.filter(item => item.state === 'released' && s.consent === 'yes') : s.items
    const id = path.split('/media/')[1]
    if (id) { const item = items.find(i => i.id === id); return item ? route.fulfill({ json: item }) : route.fulfill({ status: 404, json: error }) }
    return route.fulfill({ json: { revision: s.revision, can_collect: staff && !s.archived, can_review: s.role === 'organiser' && !s.archived, items, next_cursor: null } })
  })
}

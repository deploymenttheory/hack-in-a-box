import type { Page } from '@playwright/test'
import type { components } from '../src/api/schema.gen'
import { firstID, person, mockWorkspace } from './fixtures'

type View = components['schemas']['ObjectiveView']
type Definition = components['schemas']['ObjectiveDefinition']
type Command = components['schemas']['ObjectiveCommand']
export const objectiveID = '41111111-1111-4111-8111-111111111111', measureID = '42222222-2222-4222-8222-222222222222', resourceID = '43333333-3333-4333-8333-333333333333'
export function plannedDefinition(description = 'Private onboarding objective'): Definition { return { description, owner_id: null, theme_sensitive: false, measures: [{ id: measureID, description: 'Working examples', baseline: null, target: '0', unit: 'prototype', method: 'Review the examples', evidence_source: 'A later explicit evidence review', review_horizon: 'One month after event' }] } }
export function objectiveVersion(id: string, version: number, definition: Definition, reason = 'Defined planning intent', retired = false): View { return { reference: { id, version }, definition: structuredClone(definition), retired, reason, actor_id: person.id, at: '2027-06-01T09:00:00Z', current_version: version, can_manage: true, planning_gaps: definition.owner_id ? [] : ['owner_missing', 'baseline_unknown'] } }
// Match the native generated DTO's object-key ordering to exercise exact-save
// comparisons across authoring objects and JSON HTTP representations.
function nativeOrder<T>(value: T): T { if (Array.isArray(value)) return value.map(nativeOrder) as T; if (value && typeof value === 'object') return Object.fromEntries(Object.entries(value).sort(([a], [b]) => a.localeCompare(b)).map(([key, item]) => [key, nativeOrder(item)])) as T; return value }
export async function mockObjectives(page: Page, initial = false) {
  const workspace = await mockWorkspace(page)
  const state = { workspace, records: new Map<string, View[]>(), requests: [] as { event: string; id: string; expected: string; command: Command }[], deny: false, readFail: false, mode: '' as '' | 'before' | 'after' | 'validation', planningReads: 0 }
  const key = (event: string, id: string) => event + '/' + id
  function advance(event: string, id: string, definition: Definition, reason = 'A newer explicit version', retired = false) { const rows = state.records.get(key(event, id)) ?? []; const next = objectiveVersion(id, rows.length + 1, definition, reason, retired); rows.push(next); state.records.set(key(event, id), rows); return next }
  if (initial) advance(firstID, objectiveID, plannedDefinition())
  await page.route(/\/api\/v1\/events\/[^/]+\/(?:objectives|objective-owners)(?:\/|\?|$)/, async route => {
    const req = route.request(), url = new URL(req.url()), segments = url.pathname.split('/'), event = segments[4], id = segments[6], versions = state.records.get(key(event, id)) ?? [], current = versions.at(-1)
    const fail = (status: number) => route.fulfill({ status, json: { error: { code: status === 403 ? 'forbidden' : status === 409 ? 'revision_conflict' : status === 404 ? 'not_found' : 'invalid', message: 'Synthetic current native policy', request_id: 'synthetic' } } })
    if (state.deny) return fail(403)
    if (req.method() === 'GET') {
      state.planningReads++
      if (state.readFail) return fail(500)
      if (segments[5] === 'objective-owners') return route.fulfill({ json: { items: [{ id: person.id, display_name: person.display_name }] } })
      if (!id) return route.fulfill({ json: { items: [...state.records].filter(([k]) => k.startsWith(event + '/')).map(([, rows]) => ({ ...rows.at(-1)!, can_manage: state.workspace.events.find(e => e.id === event)?.can_edit ?? false })), next_cursor: null } })
      if (!current) return fail(404)
      if (segments[7] === 'history') return route.fulfill({ json: { items: [...versions].reverse(), next_cursor: null } })
      const selected = url.searchParams.has('version') ? versions.find(v => v.reference.version === Number(url.searchParams.get('version'))) : current
      if (!selected) return fail(404)
      return route.fulfill({ json: nativeOrder({ ...selected, current_version: current.current_version, can_manage: state.workspace.events.find(e => e.id === event)?.can_edit ?? false }) })
    }
    const command = req.postDataJSON() as Command, expected = req.headers()['if-match']
    state.requests.push({ event, id, expected, command: structuredClone(command) })
    if (!req.headers()['x-csrf-token']) return fail(403)
    if (expected !== `"${current?.current_version ?? 0}"`) return fail(409)
    if (state.mode === 'validation') return fail(400)
    if (state.mode === 'before') { state.mode = ''; return route.abort() }
    const next = advance(event, id, command.definition ?? current!.definition, command.reason, command.action === 'retire')
    if (state.mode === 'after') { state.mode = ''; return route.abort() }
    return route.fulfill({ json: nativeOrder(next) })
  })
  return { state, advance, key }
}

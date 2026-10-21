import { defaultBranding } from '../src/branding'
import type { Page } from '@playwright/test'
import type { components } from '../src/api/schema.gen'
export const person = { id: '1ca7d59b-3160-48b7-99b7-aad2cb648d64', display_name: 'Synthetic Person', platform_admin: true }
export const firstID = '8b4917c1-f27a-40c8-838b-a7fd047c4313'
export const secondID = 'b9eb455e-e9c8-488f-b16a-7da3f0f25b30'
export function fixtureEvent(id = firstID, name = 'North Star Hack'): components['schemas']['Event'] {
  return { id, name, slug: name.toLowerCase().replaceAll(' ', '-'), owner_id: person.id, revision: 1, stage: 'draft', confirmed_attendees: 0, can_edit: true, can_manage_owner: true, can_read_staff: true, can_transition: true, blockers: [], challenge_publication: '2027-06-13T09:00:00Z', published_at: null,
    configuration: { description: 'A shared place to build.', timezone: 'Europe/London', venue: 'Synthetic venue', capacity: 200, minimum_attendance: 30, live_start: '2027-06-20T09:00:00Z', live_end: '2027-06-20T17:00:00Z', admission_cutoff: '2027-06-12T09:00:00Z', team_freeze: '2027-06-12T10:00:00Z', submission_cutoff: '2027-06-20T16:00:00Z', dst_choice: '', calendar: { working_days: [1, 2, 3, 4, 5], excluded_dates: [] }, schedule: [] } }
}
export async function mockWorkspace(page: Page) {
  const events = [fixtureEvent(), fixtureEvent(secondID, 'Southern Lights Hack')]
  const staff: components['schemas']['StaffMember'][] = [{ person_id: person.id, display_name: person.display_name, role: 'organiser', active: true, changed_at: '2026-10-21T12:00:00Z' }]
  await page.route('**/api/v1/**', async route => {
    const request = route.request(); const path = new URL(request.url()).pathname
    const error = (status: number, code: string, blockers?: components['schemas']['Blocker'][]) => route.fulfill({ status, json: { error: { code, message: 'Safe fixture', request_id: 'synthetic', blockers } } })
    if (path === '/api/v1/session') return route.fulfill({ json: { person, csrf_token: 'synthetic-csrf-token-32-characters', can_create_events: person.platform_admin } })
    if (path === '/api/v1/account/github') return route.fulfill({ json: { enabled: false, revision: 0, account: null, invitations_needing_review: 0 } })
    if (path === '/api/v1/time/resolve') { const body = request.postDataJSON(); return route.fulfill({ json: { instant: body.wall_time + 'Z' } }) }
    if (path === '/api/v1/event-preview') return route.fulfill({ json: { blockers: [], challenge_publication: '2027-06-13T10:00:00Z', schedule: [] } })
    if (path === '/api/v1/events') {
      if (request.method() === 'POST') { const body = request.postDataJSON(); const created = { ...fixtureEvent(body.id, body.name), slug: body.slug, configuration: body.configuration }; events.push(created); return route.fulfill({ json: created }) }
      return route.fulfill({ json: { items: events, next_cursor: null } })
    }
    const event = events.find(event => path.startsWith('/api/v1/events/' + event.id))
    if (!event) return error(404, 'not_found')
    if (path.endsWith('/branding')) return route.fulfill({ json: { revision: event.revision, branding: defaultBranding } })
    if (path.endsWith('/staff')) {
      if (request.method() === 'PUT') {
        const body = request.postDataJSON()
        if (!body.active && body.person_id === person.id) return error(409, 'last_organiser')
        const existing = staff.find(member => member.person_id === body.person_id && member.role === body.role)
        if (existing) existing.active = body.active
        else staff.push({ person_id: body.person_id, display_name: 'New Staff Member', role: body.role, active: body.active, changed_at: '2026-10-21T13:00:00Z' })
        event.revision++; return route.fulfill({ json: { revision: event.revision } })
      }
      return route.fulfill({ json: { items: staff } })
    }
    if (path.endsWith('/audit') || path.endsWith('/schedule') || path.endsWith('/decisions')) return route.fulfill({ json: { items: [], next_cursor: 0 } })
    if (path.endsWith('/transition')) return error(409, 'event_blocked', [{ field: 'minimum_attendance', code: 'attendance_shortfall', message: 'Confirmed competitors are below the configured minimum.' }])
    if (path.endsWith('/owner')) { event.owner_id = request.postDataJSON().person_id; event.revision++; return route.fulfill({ json: { revision: event.revision } }) }
    if (request.method() === 'PUT') { const body = request.postDataJSON(); event.name = body.name; event.slug = body.slug; event.configuration = body.configuration; event.revision++ }
    return route.fulfill({ json: event })
  })
  return { events, staff }
}

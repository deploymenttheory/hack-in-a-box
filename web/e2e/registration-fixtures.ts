import { firstID, mockWorkspace, person } from './fixtures'
import type { Page } from '@playwright/test'
import type { components } from '../src/api/schema.gen'
// Applicant journeys must not borrow the organiser's or platform admin's powers.
export async function mockApplicantWorkspace(page: Page) {
  const workspace = await mockWorkspace(page)
  for (const event of workspace.events) {
    event.owner_id = 'bf77d0e4-2ee2-4b94-af84-d6f2091c4644'
    event.can_edit = false
    event.can_manage_owner = false
    event.can_read_staff = false
    event.can_transition = false
  }
  workspace.staff.splice(0)
  await page.route('**/api/v1/session', route => route.fulfill({ json: {
    person: { ...person, platform_admin: false },
    csrf_token: 'synthetic-csrf-token-32-characters',
    can_create_events: false,
  } }))
  return workspace
}
export const registrationEvent: components['schemas']['RegistrationEvent'] = { id: firstID, name: 'Open Hack', description: 'Build together', venue: 'Synthetic venue', timezone: 'Europe/London', live_start: '2027-06-20T09:00:00Z', live_end: '2027-06-20T17:00:00Z', admission_cutoff: '2027-06-12T09:00:00Z', team_freeze: '2027-06-12T10:00:00Z', stage: 'registration', open: true, team_frozen: false }
export const questionID = '00000000-0000-4000-8000-000000000001'
export const attendeeForm: components['schemas']['PublishedRegistrationForm'] = { version: 1, definition: { id: '00000000-0000-4000-8000-000000000002', name: 'Attend as a competitor', audience: 'attendee', fields: [{ id: questionID, label: 'What would you like to learn?', type: 'text', required: true, help: '', choices: [] }] } }
export function fixtureRegistration(state = 'confirmed'): components['schemas']['Registration'] { return { id: '00000000-0000-4000-8000-000000000004', event_id: firstID, person_id: person.id, form_id: attendeeForm.definition.id, form_version: 1, audience: 'attendee', kind: 'attendee', state, sequence: 1, revision: 1, team_preference: 'solo', invitation_reference: '', fallback_to_attendee: false, offer: null, waitlist_position: state === 'waitlisted' ? 2 : null } as components['schemas']['Registration'] }

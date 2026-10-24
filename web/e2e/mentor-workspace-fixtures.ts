import { expect, type Page } from '@playwright/test'
import { firstID, mockWorkspace, person } from './fixtures'
import { fixtureMentors, teamIDs } from './mentor-fixtures'
import type { components } from '../src/api/schema.gen'
type Note = components['schemas']['MentorObservation']
type Command = components['schemas']['MentorObservationCommand']
const noteID = '55555555-5555-4555-8555-555555555555'
function saved(id = noteID, version = 1): Note { return { id, team_id: teamIDs[0], author_id: person.id, version, observed_at: '2027-06-20T09:05:00Z', changed_at: '2027-06-20T09:10:00Z', evidence: { category: 'Technical approach', evidence: 'SAVED PRIVATE EVIDENCE', outcome: 'Follow up integration', feedback_reference: 'feedback-ref' } } }
async function fixture(page: Page) {
  const workspace = await mockWorkspace(page), coverage = fixtureMentors()
  coverage.can_manage = false; coverage.profiles = [{ ...coverage.profiles[0], person_id: person.id }]
  coverage.published = teamIDs.map(team_id => ({ team_id, person_id: person.id, slot: 'primary', window: coverage.live }))
  const state = { revision: 10, open: true, denied: false, failWrite: false, acceptedButLost: false, rejectWrite: false, notes: [] as Note[], commands: [] as Command[], exactReads: 0, status: null as components['schemas']['MentorOperationalStatus'] | null, firstPageEmpty: false, readFailure: false }
  workspace.events[0].stage = 'live'
  await page.route(`**/events/${firstID}/mentor-coverage`, route => route.fulfill({ json: coverage }))
  await page.route(`**/events/${firstID}/teams/*/mentor-observations**`, async route => {
    const request = route.request(), url = new URL(request.url()), teamID = url.pathname.split('/teams/')[1].split('/')[0]
    const error = (status: number, code: string) => route.fulfill({ status, json: { error: { code, message: 'Synthetic native boundary', request_id: 'synthetic' } } })
    if (state.denied) return error(403, 'forbidden')
    if (state.readFailure && request.method() === 'GET') return route.abort('failed')
    if (url.pathname.endsWith('/history')) {
      const id = url.searchParams.get('observation_id'); state.exactReads++
      const note = state.notes.find(n => n.id === id && n.team_id === teamID)
      if (id && (!state.open || !note)) return error(state.open ? 404 : 403, state.open ? 'not_found' : 'forbidden')
      return route.fulfill({ json: { observations: note ? [note] : [], statuses: state.status ? [state.status] : [], next_observation_cursor: null, next_status_cursor: null } })
    }
    if (request.method() === 'POST') {
      const input = request.postDataJSON() as Command; state.commands.push(input)
      if (state.failWrite) return route.abort('failed')
      if (state.rejectWrite) return error(409, 'conflict')
      expect(request.headers()['if-match']).toBe(`"${state.revision}"`)
      if (input.action === 'observation') {
        if (new TextEncoder().encode(input.evidence?.feedback_reference ?? '').length > 256) return error(400, 'invalid')
        if (!state.open) return error(409, 'event_blocked')
        const old = state.notes.find(n => n.id === input.id)
        expect(input.version).toBe(old?.version ?? 0)
        state.notes = [...state.notes.filter(n => n.id !== input.id), { ...saved(input.id, input.version + 1), team_id: teamID, evidence: input.evidence! }]
      } else state.status = { version: (state.status?.version ?? 0) + 1, actor_id: person.id, state: input.state!, reason: input.reason!, next_action: input.next_action ?? '', changed_at: '2027-06-20T09:20:00Z' }
      state.revision++
      if (state.acceptedButLost) return route.abort('failed')
    }
    return route.fulfill({ json: { revision: state.revision, can_record: state.open, can_change_status: true, observations: state.open && !state.firstPageEmpty ? state.notes.filter(n => n.team_id === teamID) : [], status: state.status, next_cursor: state.firstPageEmpty ? '99999999-9999-4999-8999-999999999999' : null } })
  })
  return state
}
async function leaveTask(page: Page, operation: () => Promise<unknown>) { const accept = (dialog: import('@playwright/test').Dialog) => void dialog.accept(); page.on('dialog', accept); try { await operation() } finally { page.off('dialog', accept) } }
async function openNew(page: Page) { await leaveTask(page, () => page.goto(`/events/${firstID}/mentor/${teamIDs[0]}?note=${noteID}&fresh=true`)); await expect(page.getByLabel('Category (private)', { exact: true })).toBeVisible() }
async function typeNote(page: Page, evidence = 'UNSENT PRIVATE <script> literal evidence') { await page.getByLabel('Category (private)', { exact: true }).fill('Technical approach'); await page.getByLabel('Evidence (private)', { exact: true }).fill(evidence); await page.getByLabel('Outcome (private)', { exact: true }).fill('Follow up integration') }
async function restore(page: Page) { await page.getByRole('button', { name: 'Check for local text', exact: true }).click(); await page.getByRole('button', { name: 'Restore text to the editor', exact: true }).click() }
async function copies(page: Page) { return page.evaluate(() => Object.keys(sessionStorage).filter(k => k.startsWith('hackbox:mentor-text:')).length) }

async function saveObservation(page: Page) {
  const task = page.getByRole('region', { name: 'Private observation editor' })
  await page.getByRole('button', { name: 'Review private observation', exact: true }).click()
  const confirm = page.getByRole('button', { name: 'Save private observation', exact: true })
  await expect.poll(async () => await confirm.isVisible() || (await task.getAttribute('aria-busy') === 'false' && await page.getByRole('button', { name: 'Review private observation', exact: true }).count() > 0)).toBe(true)
  if (await confirm.isVisible()) await confirm.click()
}
async function saveStatus(page: Page) { await page.getByRole('button', { name: 'Review operational check-in', exact: true }).click(); await page.getByRole('button', { name: 'Save operational check-in', exact: true }).click() }


export { fixture, saved, noteID, openNew, typeNote, restore, copies, leaveTask, saveObservation, saveStatus }

import { rubricGuideConfiguration } from '../src/styleguide/rubric-fixture'
import { expect, type Page } from '@playwright/test'
import { firstID, mockWorkspace } from './fixtures'
import type { components } from '../src/api/schema.gen'
import { replaceSlot } from '../src/mentor-model'

type MentorPage = components['schemas']['MentorPage']
export const mentorIDs = ['00000000-0000-4000-8000-000000000021', '00000000-0000-4000-8000-000000000022', '00000000-0000-4000-8000-000000000023']
export const teamIDs = ['00000000-0000-4000-8000-000000000031', '00000000-0000-4000-8000-000000000032']
export function fixtureMentors(): MentorPage {
  const live = { start: '2027-06-20T09:00:00Z', end: '2027-06-20T17:00:00Z' }
  const assignments: MentorPage['draft'] = [
    { team_id: teamIDs[0], person_id: mentorIDs[0], slot: 'primary', window: live },
    { team_id: teamIDs[0], person_id: mentorIDs[1], slot: 'second', window: live },
    { team_id: teamIDs[1], person_id: mentorIDs[1], slot: 'primary', window: live },
    { team_id: teamIDs[1], person_id: mentorIDs[2], slot: 'second', window: live },
  ]
  return { revision: 10, can_manage: true, default_capacity: 2, live, profiles: mentorIDs.map((id, index) => ({ person_id: id, name: ['Morgan Design', 'Sam Systems', 'Rae Review'][index], active: true, skills: [index === 0 ? 'design' : 'systems'], availability: [{ ...live }], maximum_teams: null })), teams: teamIDs.map((id, index) => ({ id, name: ['North Stars', 'South Stars'][index], published: true, skills: ['design'] })), conflicts: [], draft: structuredClone(assignments), published: structuredClone(assignments), gaps: [], published_gaps: [] }
}
export async function mockMentors(page: Page) {
  const workspace = await mockWorkspace(page)
  workspace.events[0].configuration.timezone = 'UTC'; workspace.events[0].stage = 'live'
  await page.route(`**/events/${firstID}/rubric/configuration*`, route => route.fulfill({ json: rubricGuideConfiguration() }))
  await page.route(`**/events/${firstID}/scorecard-checkpoints*`, route => route.fulfill({ json: { revision: workspace.events[0].revision, items: [], next_cursor: null } }))
  const state: { value: MentorPage; commands: components['schemas']['MentorCommand'][]; denied: boolean; stale: boolean } = { value: fixtureMentors(), commands: [], denied: false, stale: false }
  await page.route(`**/events/${firstID}/teams`, route => {
    expect(route.request().method()).toBe('GET')
    const teams = teamIDs.map((id, index) => { const members = [0, 1, 2].map(n => ({ id: `00000000-0000-4000-8000-${(100 + index * 3 + n).toString().padStart(12, '0')}`, name: `Competitor ${index * 3 + n + 1}` })); return { id, name: ['North Stars', 'South Stars'][index], state: 'published', captain: members[0].id, published_captain: members[0].id, invitation_code: null, frozen: true, allocation_locked: true, capacity: null, draft: members, published: members } })
    return route.fulfill({ json: { revision: state.value.revision, can_manage: true, can_create: false, frozen: true, freeze_at: workspace.events[0].configuration.team_freeze, people: teams.flatMap(t => t.draft), requests: [], teams } })
  })
  await page.route(`**/events/${firstID}/mentor-coverage`, route => {
    const error = (status: number, code: string) => route.fulfill({ status, json: { error: { code, message: 'Synthetic mentor boundary', request_id: 'synthetic' } } })
    if (state.denied) return error(403, 'forbidden')
    if (route.request().method() === 'POST') {
      const command = route.request().postDataJSON() as components['schemas']['MentorCommand']
      state.commands.push(command)
      if (state.stale) { state.stale = false; state.value = { ...state.value, revision: state.value.revision + 1 }; return error(409, 'conflict') }
      expect(route.request().headers()['if-match']).toBe(`"${state.value.revision}"`)
      const next = structuredClone(state.value); next.revision++
      switch (command.action) {
        case 'assign': next.draft = replaceSlot(next.draft, command.team_id!, command.slot!, command.person_id ? { team_id: command.team_id!, person_id: command.person_id, slot: command.slot!, window: command.window! } : undefined); break
        case 'profile': next.profiles = next.profiles.map(p => p.person_id === command.person_id ? { ...p, skills: command.skills ?? [], availability: command.availability ?? [], maximum_teams: command.maximum_teams ?? null } : p); break
        case 'settings': next.default_capacity = command.default_capacity!; break
        case 'publish': next.published = structuredClone(next.draft); next.published_gaps = [...next.gaps]; break
        case 'team_skills': next.teams = next.teams.map(t => t.id === command.team_id ? { ...t, skills: command.skills ?? [] } : t); break
        case 'apply_suggestion': break
        case 'conflict': {
          if (command.enabled) next.conflicts.push({ mentor_id: command.person_id!, competitor_id: command.competitor_id!, kind: command.kind! })
          else next.conflicts = next.conflicts.filter(c => c.mentor_id !== command.person_id || c.competitor_id !== command.competitor_id || c.kind !== command.kind)
          break
        }
      }
      state.value = next
    }
    return route.fulfill({ json: state.value })
  })
  await page.route(`**/events/${firstID}/mentor-coverage/suggestions`, route => route.fulfill({ json: { revision: state.value.revision, assignments: state.value.draft, gaps: state.value.gaps } }))
  return { ...workspace, state }
}

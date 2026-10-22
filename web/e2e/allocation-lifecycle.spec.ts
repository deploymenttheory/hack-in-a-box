import { test, expect } from '@playwright/test'
import { firstID } from './fixtures'
import { mockMentors, mentorIDs, teamIDs } from './mentor-fixtures'
import type { components } from '../src/api/schema.gen'

test('withdrawal invalidates mentor publication while frozen competitors stay in place', async ({ page }) => {
  const { state, events } = await mockMentors(page)
  events[0].stage = 'preparation'
  const roster: components['schemas']['TeamPage'] = {
    revision: state.value.revision, can_manage: true, can_create: false, frozen: true,
    freeze_at: events[0].configuration.team_freeze, people: [], requests: [],
    teams: teamIDs.map((id, index) => {
      const members = [0, 1, 2].map(n => ({ id: `00000000-0000-4000-8000-${(100 + index * 3 + n).toString().padStart(12, '0')}`, name: `Competitor ${index * 3 + n + 1}` }))
      return { id, name: ['North Stars', 'South Stars'][index], state: 'published', captain: members[0].id, published_captain: members[0].id, invitation_code: null, frozen: true, allocation_locked: true, capacity: null, draft: [...members], published: [...members] }
    }),
  }
  roster.people = roster.teams.flatMap(t => t.published)
  const competitorWrites: string[] = []
  await page.route(`**/events/${firstID}/teams`, route => {
    if (route.request().method() !== 'GET') competitorWrites.push(route.request().method())
    return route.fulfill({ json: { ...roster, revision: state.value.revision } })
  })
  await page.goto(`/events/${firstID}`)
  await page.getByRole('button', { name: 'Mentors', exact: true }).click()
  await page.getByLabel('Mentor decision reason').fill('Replace mentor after competitor freeze')
  await page.getByRole('button', { name: 'Change mentor slot', exact: true }).click()
  await page.getByLabel('Team receiving coverage').selectOption(teamIDs[0])
  await page.getByLabel('Mentor for this slot').selectOption(mentorIDs[2])
  await page.getByRole('button', { name: 'Preview mentor change', exact: true }).click()
  await page.getByRole('button', { name: 'Save mentor draft change', exact: true }).click()
  await page.getByRole('button', { name: 'Review mentor publication', exact: true }).click()

  // Another participant withdraws after this organiser reviewed the roster.
  // PostgreSQL tests exercise the actual withdrawal transaction and lock race.
  const withdrawn = roster.teams[0].published[1].id
  roster.teams[0].published = roster.teams[0].published.filter(p => p.id !== withdrawn)
  roster.teams[0].draft = [...roster.teams[0].published]
  roster.people = roster.people.filter(p => p.id !== withdrawn)
  state.stale = true
  await page.getByRole('button', { name: 'Publish mentor coverage', exact: true }).click()
  await expect(page.getByText('The roster changed. Review the current mentor publication again.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Publish mentor coverage', exact: true })).toHaveCount(0)
  await expect(page.getByRole('region', { name: 'Published mentor roster' })).toContainText('Morgan Design')
  expect(state.commands.filter(c => c.action === 'publish')).toHaveLength(1)
  await page.getByRole('button', { name: 'Review mentor publication', exact: true }).click()
  await page.getByRole('button', { name: 'Publish mentor coverage', exact: true }).click()
  await expect(page.getByRole('region', { name: 'Published mentor roster' })).toContainText('Rae Review')
  await expect(page.getByRole('region', { name: 'Published mentor roster' })).not.toContainText('Morgan Design')

  await page.getByRole('button', { name: 'Teams', exact: true }).click()
  await expect(page.getByText('This depleted team may continue. Reassignment is unavailable.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Assign draft member', exact: true })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Review publication and freeze', exact: true })).toHaveCount(0)
  expect(competitorWrites).toEqual([])
})

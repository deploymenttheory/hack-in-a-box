import { openEventSection } from './event-navigation-fixture'
import { test, expect } from '@playwright/test'
import { firstID } from './fixtures'
import { mockMentors, mentorIDs, teamIDs } from './mentor-fixtures'

async function openMentor(page: import('@playwright/test').Page) { await page.getByLabel('Coverage view').selectOption('mentors'); await page.getByRole('button', { name: 'View Morgan Design', exact: true }).click(); await page.getByRole('button', { name: 'Edit Morgan Design', exact: true }).click() }
async function confirm(page: import('@playwright/test').Page) { await page.getByRole('button', { name: 'Confirm mentor change', exact: true }).click() }
async function open(page: import('@playwright/test').Page) { await page.goto(`/events/${firstID}`); await openEventSection(page, 'Mentors') }

test('mentor change previews workload, keeps the old roster until publication and preserves frozen competitors', async ({ page }, testInfo) => {
  const { state } = await mockMentors(page)
  await open(page)
  await expect(page.getByText(/Competitor freeze does not prevent mentor replacements/)).toBeVisible()
  await page.getByLabel('Mentor decision reason').fill('Reviewed replacement after competitor freeze')
  await page.getByRole('button', { name: 'Change mentor slot', exact: true }).click()
  await page.getByLabel('Team receiving coverage').selectOption(teamIDs[0]); await page.getByLabel('Mentor for this slot').selectOption(mentorIDs[2])
  await page.getByRole('button', { name: 'Preview mentor change', exact: true }).click()
  await expect(page.getByRole('table', { name: 'Proposed mentor workloads' }).or(page.getByRole('list', { name: 'Proposed mentor workloads' }))).toContainText('Rae Review')
  const save = page.getByRole('button', { name: 'Review mentor draft change', exact: true }); await save.focus(); await page.keyboard.press('Enter'); await confirm(page)
  await expect(page.getByRole('region', { name: 'Mentor publication preview' })).toHaveCount(0)
  await page.getByLabel('Coverage view').selectOption('published')
  await expect(page.getByRole('region', { name: 'Published mentor roster' })).toContainText('Morgan Design')
  expect(state.commands.map(c => c.action)).toEqual(['assign'])
  await page.getByRole('button', { name: 'Review mentor publication', exact: true }).click()
  await expect(page.getByRole('region', { name: 'Mentor publication preview' })).toContainText('Morgan Design')
  await expect(page.getByRole('region', { name: 'Mentor publication preview' })).toContainText('Rae Review')
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
  await page.screenshot({ path: testInfo.outputPath('mentor-publication-review.png'), fullPage: true })
  await page.getByRole('button', { name: 'Publish mentor coverage', exact: true }).click()
  await expect(page.getByRole('region', { name: 'Published mentor roster' })).not.toContainText('Morgan Design')
  await expect(page.getByText(/Server response received/)).toBeVisible()
  expect(state.commands.map(c => c.action)).toEqual(['assign', 'publish'])
})

test('mentor publication requires an explicit recorded decision for coverage gaps', async ({ page }) => {
  const { state } = await mockMentors(page)
  state.value.draft = state.value.draft.filter(a => a.team_id !== teamIDs[1] || a.slot !== 'primary')
  state.value.gaps = [{ code: 'missing_primary', hard: false, team_id: teamIDs[1], person_id: null, message: 'This team has no primary mentor.' }]
  await open(page)
  await expect(page.getByRole('table', { name: 'Draft mentor coverage' }).or(page.getByRole('list', { name: 'Draft mentor coverage' }))).toContainText('Uncovered')
  await expect(page.getByRole('button', { name: 'Review mentor publication', exact: true })).toBeDisabled()
  await page.getByLabel('Mentor decision reason').fill('Temporary gap approved while replacement is contacted')
  await page.getByRole('button', { name: 'Review mentor publication', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Publish mentor coverage', exact: true })).toBeDisabled()
  await page.getByLabel('Approve these coverage gaps with the recorded reason').check()
  await page.getByRole('button', { name: 'Publish mentor coverage', exact: true }).click()
  expect(state.commands[0]).toMatchObject({ action: 'publish', allow_gaps: true, reason: 'Temporary gap approved while replacement is contacted' })
  await page.getByLabel('Coverage view').selectOption('published')
  await expect(page.getByRole('region', { name: 'Published coverage gaps', exact: true })).toContainText('This team has no primary mentor.')
})

test('over-capacity previews and known conflicts block manual assignment', async ({ page }) => {
  const { state } = await mockMentors(page)
  state.value.profiles[0].maximum_teams = 1
  await open(page); await page.getByLabel('Mentor decision reason').fill('Inspect workload before changing')
  await page.getByRole('button', { name: 'Change mentor slot', exact: true }).click()
  await page.getByLabel('Team receiving coverage').selectOption(teamIDs[1]); await page.getByLabel('Mentor for this slot').selectOption(mentorIDs[0])
  await page.getByRole('button', { name: 'Preview mentor change', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('Over capacity')
  await expect(page.getByRole('button', { name: 'Review mentor draft change', exact: true })).toBeDisabled()
  await page.getByRole('button', { name: 'Cancel mentor edit', exact: true }).click()
  await page.getByRole('button', { name: /Declared conflicts/ }).click()
  await page.getByLabel('Conflicted mentor').selectOption(mentorIDs[0]); await page.getByLabel('Confirmed competitor in relationship').selectOption('00000000-0000-4000-8000-000000000103')
  await page.getByRole('button', { name: 'Record mentor conflict', exact: true }).click()
  await confirm(page)
  await expect(page.getByRole('table', { name: 'Declared mentor conflicts' }).or(page.getByRole('list', { name: 'Declared mentor conflicts' }))).toContainText('Competitor 4')
  await page.getByRole('button', { name: 'Back to roster', exact: true }).click()
  await page.getByRole('button', { name: 'Change mentor slot', exact: true }).click()
  await page.getByLabel('Team receiving coverage').selectOption(teamIDs[1]); await page.getByLabel('Mentor for this slot').selectOption(mentorIDs[0])
  await expect(page.getByRole('alert')).toContainText('A declared reporting or own-team conflict')
  await expect(page.getByRole('button', { name: 'Preview mentor change', exact: true })).toBeDisabled()
  expect(state.commands.map(c => c.action)).toEqual(['conflict'])
})

test('mentor profiles save declared local availability', async ({ page }) => {
  const { state } = await mockMentors(page)
  await open(page); await page.getByLabel('Mentor decision reason').fill('Updated declared availability')
  await openMentor(page)
  await page.getByLabel('Declared skills', { exact: true }).fill('design, testing')
  await page.getByLabel('Personal maximum assigned teams (optional)').fill('3')
  await page.getByLabel('Coverage end (UTC)', { exact: true }).fill('2027-06-20T13:00')
  await page.getByRole('button', { name: 'Review declared availability', exact: true }).click()
  await expect(page.getByRole('form', { name: 'Mentor profile editor' })).toHaveCount(0)
  await confirm(page)
  expect(state.commands[0]).toMatchObject({ action: 'profile', person_id: mentorIDs[0], skills: ['design', 'testing'], maximum_teams: 3, availability: [{ start: '2027-06-20T09:00:00Z', end: '2027-06-20T13:00:00Z' }] })

})

test('permitted published roster is read only and disappears when access is revoked', async ({ page }) => {
  const { state, events } = await mockMentors(page)
  events[0].can_edit = false; events[0].can_read_staff = false
  state.value = { ...state.value, can_manage: false, teams: [state.value.teams[0]], profiles: state.value.profiles.slice(0, 2), published: state.value.published.filter(a => a.team_id === teamIDs[0]), draft: [], conflicts: [], gaps: [], published_gaps: [] }
  await open(page)
  await expect(page.getByRole('region', { name: 'Published mentor roster' })).toContainText('Morgan Design')
  await expect(page.getByText('South Stars')).toHaveCount(0)
  await expect(page.getByLabel('Mentor decision reason')).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Change mentor slot', exact: true })).toHaveCount(0)
  state.denied = true
  await openEventSection(page, 'Overview'); await openEventSection(page, 'Mentors')
  await expect(page.getByRole('alert')).toContainText('Current access to this roster is unavailable')
  await expect(page.getByText('Morgan Design')).toHaveCount(0)
  expect(state.commands).toHaveLength(0)
})


test('stale mentor publication requires renewed review against the current roster', async ({ page }) => {
  const { state } = await mockMentors(page)
  await open(page); await page.getByLabel('Mentor decision reason').fill('Review current publication')
  await page.getByRole('button', { name: 'Review mentor publication', exact: true }).click()
  state.stale = true
  await page.getByRole('button', { name: 'Publish mentor coverage', exact: true }).click()
  await expect(page.getByText('The roster changed. Review the current mentor publication again.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Publish mentor coverage', exact: true })).toBeDisabled()
})

test('assignment previews explain partial coverage and block unavailable intervals', async ({ page }) => {
  const { state } = await mockMentors(page)
  state.value.profiles[2].availability[0].end = '2027-06-20T13:00:00Z'
  await open(page); await page.getByLabel('Mentor decision reason').fill('Review selected availability')
  await page.getByRole('button', { name: 'Change mentor slot', exact: true }).click()
  await page.getByLabel('Team receiving coverage').selectOption(teamIDs[0]); await page.getByLabel('Mentor for this slot').selectOption(mentorIDs[2])
  await page.getByRole('button', { name: 'Preview mentor change', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('Declared availability does not cover')
  await expect(page.getByRole('button', { name: 'Review mentor draft change', exact: true })).toBeDisabled()
  await page.getByLabel('Coverage end (UTC)', { exact: true }).fill('2027-06-20T13:00')
  await page.getByRole('button', { name: 'Preview mentor change', exact: true }).click()
  await expect(page.getByText('Partial primary coverage leaves a gap. Publication requires a recorded organiser exception.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Review mentor draft change', exact: true })).toBeEnabled()
  expect(state.commands).toHaveLength(0)
})


test('capacity and team skills are explicit edits and proposals do not publish automatically', async ({ page }) => {
  const { state } = await mockMentors(page)
  await open(page)
  await page.getByRole('button', { name: 'Edit event mentor capacity', exact: true }).click()
  await page.getByLabel('Mentor decision reason').fill('Review declared coverage needs')
  await page.getByLabel('Default maximum assigned teams per mentor').fill('3')
  await page.getByRole('button', { name: 'Review mentor capacity', exact: true }).click()
  await confirm(page)
  await expect(page.getByText(/Default maximum: 3 assigned teams per mentor/)).toBeVisible()
  await page.getByRole('button', { name: 'View North Stars', exact: true }).click()
  await page.getByRole('button', { name: 'Edit skills for North Stars', exact: true }).click()
  await page.getByLabel('Skills requested for team coverage').fill('security, systems')
  await page.getByRole('button', { name: 'Review team skill needs', exact: true }).click()
  await expect(page.getByRole('form', { name: 'Team skill editor' })).toHaveCount(0)
  await confirm(page)
  await page.getByRole('button', { name: 'Back to roster', exact: true }).click()
  await page.getByRole('button', { name: 'Suggest mentor coverage', exact: true }).click()
  await expect(page.getByRole('region', { name: 'Mentor proposal' })).toBeVisible()
  await page.getByRole('button', { name: 'Dismiss mentor proposal', exact: true }).click()
  await page.getByRole('button', { name: 'Review mentor publication', exact: true }).click()
  await page.getByRole('button', { name: 'Cancel mentor publication', exact: true }).click()
  expect(state.commands.map(c => c.action)).toEqual(['settings', 'team_skills'])
})

test('availability preserves an ambiguous local interval until a repeated-time choice is made', async ({ page }) => {
  const { state, events } = await mockMentors(page)
  events[0].configuration.timezone = 'Europe/London'
  await page.route('**/api/v1/time/resolve', route => {
    const body = route.request().postDataJSON()
    expect(body.timezone).toBe('Europe/London')
    if (body.wall_time === '2027-10-31T01:30:00' && !body.dst_choice) return route.fulfill({ status: 400, json: { error: { code: 'form_invalid', message: 'Choose occurrence', request_id: 'synthetic', blockers: [{ field: 'availability-0-start-dst', code: 'ambiguous', message: 'Choose the earlier or later occurrence.' }] } } })
    return route.fulfill({ json: { instant: body.wall_time === '2027-10-31T01:30:00' ? '2027-10-31T00:30:00Z' : '2027-10-31T02:30:00Z' } })
  })
  await open(page); await page.getByLabel('Mentor decision reason').fill('Declared availability through repeated local time')
  await openMentor(page)
  await page.getByLabel('Coverage start (Europe/London)', { exact: true }).fill('2027-10-31T01:30')
  await page.getByLabel('Coverage end (Europe/London)', { exact: true }).fill('2027-10-31T02:30')
  await page.getByRole('button', { name: 'Review declared availability', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('Choose the earlier or later occurrence.')
  await expect(page.getByLabel('Coverage start (Europe/London)', { exact: true })).toHaveValue('2027-10-31T01:30')
  expect(state.commands).toHaveLength(0)
  await page.getByLabel('Repeated start time', { exact: true }).selectOption('earlier')
  await page.getByRole('button', { name: 'Review declared availability', exact: true }).click()
  await expect(page.getByRole('form', { name: 'Mentor profile editor' })).toHaveCount(0)
  await confirm(page)
  expect(state.commands[0].availability).toEqual([{ start: '2027-10-31T00:30:00Z', end: '2027-10-31T02:30:00Z' }])
})

test('coverage timeout retains the reviewed intent after a read without replay or invented receipt', async ({ page }) => {
  const { state } = await mockMentors(page)
  let writes = 0
  await page.route(`**/events/${firstID}/mentor-coverage`, route => {
    if (route.request().method() === 'POST') { writes++; state.value = { ...state.value, revision: 11 }; return route.fulfill({ status: 408, json: { error: { code: 'request_timeout', request_id: 'synthetic-timeout' } } }) }
    return route.fulfill({ json: state.value })
  })
  await open(page); await page.getByLabel('Mentor decision reason').fill('Reviewed coverage before worker timeout')
  await page.getByRole('button', { name: 'Review mentor publication', exact: true }).click()
  await page.getByRole('button', { name: 'Publish mentor coverage', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Outcome unknown', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Read current roster', exact: true }).click()
  await expect(page.getByText('Current roster revision 11. Original outcome remains unconfirmed.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Publish mentor coverage', exact: true })).toBeDisabled()
  expect(writes).toBe(1)
})

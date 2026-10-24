import { test, expect, type Page } from '@playwright/test'
import { mockMentors, teamIDs } from './mentor-fixtures'
import { mockWorkspace, firstID, person } from './fixtures'
import { openEventSection } from './event-navigation-fixture'
import type { components } from '../src/api/schema.gen'

for (const task of ['profile', 'publication'] as const) test(`coverage downgrade removes the retained ${task} snapshot without event denial`, async ({ page }) => {
  const { state } = await mockMentors(page)
  await page.clock.install({ time: new Date('2026-10-23T12:00:00Z') })
  state.value.profiles[0].maximum_teams = 9
  state.value.profiles[0].availability = [{ start: '2027-06-20T10:37:00Z', end: '2027-06-20T15:43:00Z' }]
  state.value.draft[0].window = { ...state.value.profiles[0].availability[0] }
  await page.goto(`/events/${firstID}`); await openEventSection(page, 'Mentors')
  await page.getByLabel('Mentor decision reason').fill('Review private planning details')
  if (task === 'profile') {
    await page.getByLabel('Coverage view').selectOption('mentors')
    await page.getByRole('button', { name: 'View Morgan Design', exact: true }).click()
    await page.getByRole('button', { name: 'Edit Morgan Design', exact: true }).click()
    await expect(page.getByLabel('Personal maximum assigned teams (optional)')).toHaveValue('9')
  } else {
    await page.getByRole('button', { name: 'Review mentor publication', exact: true }).click()
    await expect(page.getByRole('region', { name: 'Mentor publication preview' })).toContainText('10:37')
  }
  state.value = { ...state.value, can_manage: false, teams: [state.value.teams[0]], profiles: state.value.profiles.slice(0, 2).map(p => ({ ...p, availability: [], maximum_teams: null })), published: state.value.published.filter(a => a.team_id === teamIDs[0]), draft: [], conflicts: [], gaps: [], published_gaps: [] }
  await page.clock.fastForward(31000)
  await expect(page.getByRole('form', { name: 'Mentor profile editor' })).toHaveCount(0)
  await expect(page.getByRole('region', { name: 'Mentor publication preview' })).toHaveCount(0)
  await expect(page.getByLabel('Personal maximum assigned teams (optional)')).toHaveCount(0)
  await expect(page.getByText(/10:37/)).toHaveCount(0)
  await expect(page.getByRole('region', { name: 'Published mentor roster' })).toContainText('Morgan Design')
  expect(state.commands).toHaveLength(0)
})

test('published coverage restores the exact initiating slot action across viewport changes', async ({ page }, info) => {
  const { state, events } = await mockMentors(page)
  events[0].can_edit = false
  state.value = { ...state.value, can_manage: false, draft: [], conflicts: [], gaps: [], published_gaps: [] }
  await page.goto(`/events/${firstID}`); await openEventSection(page, 'Mentors')
  const source = page.getByRole('button', { name: 'View coverage for North Stars', exact: true }).nth(1)
  await source.focus(); await page.keyboard.press('Enter')
  await expect(page.getByRole('heading', { name: 'Published coverage: North Stars', exact: true })).toBeFocused()
  await page.setViewportSize(info.project.name === 'desktop' ? { width: 390, height: 844 } : { width: 1280, height: 800 })
  await page.getByRole('button', { name: 'Back to roster', exact: true }).click()
  await expect(source).toBeFocused()
})

type TeamPage = components['schemas']['TeamPage']
async function setupTeams(page: Page) {
  await mockWorkspace(page)
  const members = [person, { id: '00000000-0000-4000-8000-000000000111', display_name: 'Synthetic teammate two' }, { id: '00000000-0000-4000-8000-000000000112', display_name: 'Synthetic teammate three' }].map(p => ({ id: p.id, name: p.display_name }))
  const team = { id: teamIDs[0], name: 'North Stars', state: 'published' as const, captain: person.id, published_captain: person.id, invitation_code: teamIDs[0], frozen: false, allocation_locked: false, capacity: 8, draft: members, published: members }
  let value: TeamPage = { revision: 2, can_manage: true, can_create: true, frozen: false, freeze_at: '2027-06-12T10:00:00Z', people: members, teams: [team], requests: [] }
  const writes: components['schemas']['TeamCommand'][] = []
  await page.route(`**/events/${firstID}/teams`, route => { if (route.request().method() === 'POST') { const input = route.request().postDataJSON(); writes.push(input); value = { ...value, revision: value.revision + 1, teams: value.teams.map(t => ({ ...t, allocation_locked: input.action === 'lock' ? true : t.allocation_locked })) } }; return route.fulfill({ json: value }) })
  await page.goto(`/events/${firstID}`); await openEventSection(page, 'Teams')
  await page.getByRole('button', { name: 'View North Stars', exact: true }).click()
  await page.getByLabel('Allocation decision reason').fill('Reviewed team controls')
  await page.getByRole('button', { name: 'Lock team', exact: true }).click()
  await page.getByRole('button', { name: 'Confirm team change', exact: true }).click()
  await expect(page.getByText(/Server response received/)).toBeVisible()
  return { writes }
}

test('nested capacity draft requires deliberate discard and retains its value when cancelled', async ({ page }) => {
  const { writes } = await setupTeams(page)
  await page.getByRole('button', { name: 'View North Stars', exact: true }).click()
  await page.getByLabel('Hard capacity for North Stars').fill('7')
  const dialogs: string[] = []
  page.once('dialog', dialog => { dialogs.push(dialog.message()); void dialog.dismiss() })
  await page.getByRole('button', { name: 'Back to roster', exact: true }).click()
  expect(dialogs).toHaveLength(1)
  await expect(page.getByLabel('Hard capacity for North Stars')).toHaveValue('7')
  page.once('dialog', dialog => void dialog.accept())
  await page.getByRole('button', { name: 'Back to roster', exact: true }).click()
  await page.getByRole('button', { name: 'View North Stars', exact: true }).click()
  await expect(page.getByLabel('Hard capacity for North Stars')).toHaveValue('8')
  expect(writes.map(w => w.action)).toEqual(['lock'])
})

test('allocation declarations are retained on cancelled discard and cleared only after accepted local discard', async ({ page }) => {
  const { writes } = await setupTeams(page)
  await page.getByRole('button', { name: 'Review allocation', exact: true }).click()
  await page.getByText('Declared allocation features', { exact: true }).click()
  await page.getByLabel('Competitor declarations', { exact: true }).selectOption(person.id)
  await page.getByLabel(`Declared skills for ${person.display_name}`, { exact: true }).fill('Synthetic declared collaboration skill')
  const dialogs: string[] = []
  page.once('dialog', dialog => { dialogs.push(dialog.message()); void dialog.dismiss() })
  await page.getByRole('button', { name: 'Back to roster', exact: true }).click()
  expect(dialogs).toHaveLength(1)
  await expect(page.getByLabel(`Declared skills for ${person.display_name}`, { exact: true })).toHaveValue('Synthetic declared collaboration skill')
  page.once('dialog', dialog => void dialog.accept())
  await page.getByRole('button', { name: 'Back to roster', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Review allocation', exact: true })).toBeFocused()
  await page.getByRole('button', { name: 'Review allocation', exact: true }).click()
  await page.getByText('Declared allocation features', { exact: true }).click()
  await page.getByLabel('Competitor declarations', { exact: true }).selectOption(person.id)
  await expect(page.getByLabel(`Declared skills for ${person.display_name}`, { exact: true })).toHaveValue('')
  expect(writes.map(w => w.action)).toEqual(['lock'])
})

test('mentor availability review cancellation preserves inputs and local discard restores the originating edit action', async ({ page }) => {
  const { state } = await mockMentors(page)
  await page.goto(`/events/${firstID}`); await openEventSection(page, 'Mentors')
  await page.getByLabel('Mentor decision reason').fill('Review revised declared availability')
  await page.getByLabel('Coverage view').selectOption('mentors')
  await page.getByRole('button', { name: 'View Morgan Design', exact: true }).click()
  const source = page.getByRole('button', { name: 'Edit Morgan Design', exact: true })
  await source.click()
  await page.getByLabel('Coverage end (UTC)', { exact: true }).fill('2027-06-20T13:00')
  const review = page.getByRole('button', { name: 'Review declared availability', exact: true })
  await review.focus(); await page.keyboard.press('Enter')
  await page.getByRole('button', { name: 'Cancel mentor change', exact: true }).click()
  await expect(review).toBeFocused()
  await expect(page.getByLabel('Coverage end (UTC)', { exact: true })).toHaveValue('2027-06-20T13:00')
  const dialogs: string[] = []
  page.once('dialog', dialog => { dialogs.push(dialog.message()); void dialog.dismiss() })
  await page.getByRole('button', { name: 'Back to roster', exact: true }).click()
  expect(dialogs).toHaveLength(1)
  await expect(page.getByLabel('Coverage end (UTC)', { exact: true })).toHaveValue('2027-06-20T13:00')
  page.once('dialog', dialog => void dialog.accept())
  await page.getByRole('button', { name: 'Back to roster', exact: true }).click()
  await expect(source).toBeFocused()
  await source.click()
  await expect(page.getByLabel('Coverage end (UTC)', { exact: true })).toHaveValue('2027-06-20T17:00')
  expect(state.commands).toHaveLength(0)
})

test('capacity review corrects without dropping the typed value and success restores its source action', async ({ page }) => {
  const { state } = await mockMentors(page)
  await page.goto(`/events/${firstID}`); await openEventSection(page, 'Mentors')
  await page.getByLabel('Mentor decision reason').fill('Review event capacity')
  const source = page.getByRole('button', { name: 'Edit event mentor capacity', exact: true })
  await source.click()
  await page.getByLabel('Default maximum assigned teams per mentor').fill('3')
  const review = page.getByRole('button', { name: 'Review mentor capacity', exact: true })
  await review.click(); await page.getByRole('button', { name: 'Cancel mentor change', exact: true }).click()
  await expect(review).toBeFocused()
  await expect(page.getByLabel('Default maximum assigned teams per mentor')).toHaveValue('3')
  await review.click(); await page.getByRole('button', { name: 'Confirm mentor change', exact: true }).click()
  await expect(source).toBeFocused()
  expect(state.commands.map(command => command.action)).toEqual(['settings'])
})

test('a held time resolution cannot prepare or send an old profile after projection downgrade', async ({ page }) => {
  const { state } = await mockMentors(page)
  await page.clock.install({ time: new Date('2026-10-23T12:00:00Z') })
  let release!: () => void
  const held = new Promise<void>(resolve => { release = resolve })
  await page.route('**/api/v1/time/resolve', async route => { const wall = route.request().postDataJSON().wall_time; await held; await route.fulfill({ json: { instant: wall + 'Z' } }).catch(() => undefined) })
  await page.goto(`/events/${firstID}`); await openEventSection(page, 'Mentors')
  await page.getByLabel('Mentor decision reason').fill('Review held declared availability')
  await page.getByLabel('Coverage view').selectOption('mentors')
  await page.getByRole('button', { name: 'View Morgan Design', exact: true }).click()
  await page.getByRole('button', { name: 'Edit Morgan Design', exact: true }).click()
  await page.getByLabel('Coverage end (UTC)', { exact: true }).fill('2027-06-20T13:00')
  await page.getByRole('button', { name: 'Review declared availability', exact: true }).click()
  state.value = { ...state.value, can_manage: false, draft: [], conflicts: [], gaps: [], published_gaps: [], profiles: state.value.profiles.map(p => ({ ...p, availability: [], maximum_teams: null })) }
  await page.clock.fastForward(31000)
  await expect(page.getByRole('form', { name: 'Mentor profile editor' })).toHaveCount(0)
  release()
  await expect(page.getByRole('button', { name: 'Confirm mentor change', exact: true })).toHaveCount(0)
  await expect(page.getByRole('region', { name: 'Published mentor roster' })).toBeVisible()
  expect(state.commands).toHaveLength(0)
})

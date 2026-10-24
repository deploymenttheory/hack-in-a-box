import { openEventSection, expectEventSection } from './event-navigation-fixture'
import { test, expect } from '@playwright/test'
import { mockMentors } from './mentor-fixtures'
import { firstID, mockWorkspace } from './fixtures'

test('participant build omits organiser navigation even for an organiser identity', async ({ page }) => {
  await mockWorkspace(page)
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Your events' })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Create event' })).toHaveCount(0)
  await expect(page.getByRole('link', { name: 'Event templates' })).toHaveCount(0)
  await expect(page.getByRole('navigation', { name: 'Main navigation' }).getByRole('link', { name: 'Find events', exact: true })).toBeVisible()
  await page.getByRole('link', { name: /North Star Hack/ }).click()
  await expect(page.getByRole('heading', { name: 'North Star Hack' })).toBeVisible()
  await expectEventSection(page,'Resources')
  await expectEventSection(page,'Deliverables')
  for (const name of ['Setup', 'Staff', 'Applicants', 'Teams', 'Content', 'Advance to registration']) {
    await expectEventSection(page,name,false)
  }
  for (const path of ['/events/new', '/templates']) {
    await page.goto(path)
    await expect(page.getByRole('heading', { name: 'Page not found' })).toBeVisible()
  }
  await page.goto(`/events/${firstID}`)
  await expect(page.getByRole('heading', { name: 'North Star Hack' })).toBeVisible()
})

test('public mentor roster stays read only even when the identity can administer coverage', async ({ page }) => {
  const { state } = await mockMentors(page)
  let teamRequests = 0
  page.on('request', request => { if (new URL(request.url()).pathname.endsWith('/teams')) teamRequests++ })
  await page.goto(`/events/${firstID}`)
  await openEventSection(page, 'Mentors')
  await expect(page.getByRole('region', { name: 'Published mentor roster' })).toContainText('Morgan Design')
  await expect(page.getByRole('table', { name: 'Draft mentor coverage' })).toHaveCount(0)
  await expect(page.getByRole('list', { name: 'Draft mentor coverage' })).toHaveCount(0)
  await expect(page.getByLabel('Mentor decision reason')).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Change mentor slot', exact: true })).toHaveCount(0)
  expect(teamRequests).toBe(0)
  expect(state.commands).toEqual([])
  state.denied = true
  await openEventSection(page, 'Overview')
  await openEventSection(page, 'Mentors')
  await expect(page.getByRole('region', { name: 'Published mentor roster' })).toHaveCount(0)
  await expect(page.getByRole('alert')).toBeVisible()
})

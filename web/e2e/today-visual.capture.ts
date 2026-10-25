import { test, expect } from '@playwright/test'
import { resolve } from 'node:path'
import { todayFixture, preparation } from './today-fixtures'
import { firstID } from './fixtures'
import { openEventSection } from './event-navigation-fixture'
import type { components } from '../src/api/schema.gen'
const scenarios: { id: string; roles: components['schemas']['StaffRole'][]; context: 'personal' | 'mentor' | 'judge' }[] = [{ id: 'competitor', roles: [], context: 'personal' }, { id: 'mentor', roles: ['mentor'], context: 'mentor' }, { id: 'judge', roles: ['organiser', 'judge'], context: 'judge' }]
for (const scenario of scenarios) test(`matched synthetic ${scenario.id} Today`, async ({ page }, info) => {
 await page.clock.install({ time: new Date('2027-06-14T12:00:00Z') })
 const f = await todayFixture(page, scenario.roles)
 f.state.serverNow = '2027-06-14T12:00:00Z'; f.state.published = true; f.state.draft = scenario.context === 'personal'; f.state.task = preparation(`Assigned ${scenario.id} preparation`, scenario.context === 'personal' ? 'attendee' : scenario.context)
 await page.goto(`/events/${firstID}`); await openEventSection(page, 'Today')
 if (info.project.metadata.stage === 'after') {
  if (scenario.context !== 'personal') await page.getByRole('combobox', { name: 'Today context', exact: true }).selectOption(scenario.context)
  await expect(page.getByRole('heading', { name: `Assigned ${scenario.id} preparation`, exact: true })).toBeVisible()
 } else await expect(page.getByRole('heading', { name: 'Today', exact: true })).toBeVisible()
 await expect(page.getByRole('button', { name: 'Read Released event rules', exact: true })).toBeVisible()
 expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
 const filename = `${info.project.metadata.portal}-${scenario.id}-${info.project.metadata.width}-${info.project.metadata.stage}.png`
 await page.screenshot({ path: resolve('..', 'docs/design/evidence/today', filename), fullPage: true, animations: 'disabled' })
 expect(f.calls.filter(call => call.method !== 'GET')).toEqual([])
})

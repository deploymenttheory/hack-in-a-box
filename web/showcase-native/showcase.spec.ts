import { expect, test } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'
import { actor, control, native } from './native'

test('actual reviewed two-event discovery preserves explicit awards, consent, audience, archive and cache boundaries', async ({ browser }, testInfo) => {
 const reader = await actor(browser), outsider = await actor(browser, 'outsider')
 const page = await reader.newPage(), organisation = await outsider.newPage()
 const winner = `/projects/${native.winner_project}`, restricted = `/projects/${native.restricted_project}`, cached = `/projects/${native.second_project}`
 try {
  await page.goto('/projects'); await expect(page.getByRole('heading', { name: 'Reviewed projects', exact: true })).toBeVisible()
  await expect(page.getByLabel('Source event')).toContainText('Synthetic autumn hack'); await expect(page.getByLabel('Source event')).toContainText('Synthetic spring hack')
  await expect(page.getByRole('button', { name: 'More records', exact: true })).toBeVisible(); await page.getByRole('button', { name: 'More records', exact: true }).click(); await expect(page.getByRole('navigation', { name: 'Showcase pages' })).toContainText('30 loaded')
  await page.getByLabel('Source event').selectOption(native.event_id); await page.getByLabel('Reviewed follow-up', { exact: true }).selectOption('proposed')
  await expect(page.getByRole('link', { name: 'Useful work from a non-winning team', exact: true })).toBeVisible(); await expect(page.getByRole('link', { name: 'Recover services with less effort', exact: true })).toHaveCount(0)
  await page.getByRole('link', { name: 'Useful work from a non-winning team', exact: true }).click(); await expect(page.getByRole('heading', { name: 'Useful work from a non-winning team', exact: true })).toBeVisible(); await expect(page.getByText('No published award shown', { exact: true })).toBeVisible()
  await page.getByRole('link', { name: 'Reviewed projects', exact: true }).click(); await expect(page.getByLabel('Source event')).toHaveValue(native.event_id); await expect(page.getByLabel('Reviewed follow-up', { exact: true })).toHaveValue('proposed')
  await page.getByLabel('Reviewed follow-up', { exact: true }).selectOption(''); await page.getByRole('searchbox').fill('PRIVATE-'); await page.getByRole('button', { name: 'Search', exact: true }).click(); await expect(page.getByText('No currently permitted projects match these filters.', { exact: false })).toBeVisible()
  await page.goto(`/projects/${native.draft_project}`); await expect(page.getByRole('heading', { name: 'Project unavailable', exact: true })).toBeVisible(); await expect(page.getByText('PRIVATE-UNPUBLISHED-PROJECT', { exact: true })).toHaveCount(0)
  await page.goto(winner); await expect(page.getByRole('heading', { name: 'Recover services with less effort', exact: true })).toBeVisible(); await expect(page.getByText('Reviewed recovery award', { exact: true })).toHaveCount(0); await expect(page.getByRole('link', { name: 'Reviewed recovery demonstration (current authorised media)', exact: true })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Approved private source repository', exact: true })).toHaveAttribute('rel', 'noopener noreferrer')
  await control('announce'); await page.getByRole('button', { name: 'Read current project', exact: true }).click(); await expect(page.getByText('Reviewed recovery award', { exact: true })).toBeVisible()
  await organisation.goto(winner); await expect(organisation.getByRole('heading', { name: 'Recover services with less effort', exact: true })).toBeVisible(); await expect(organisation.getByText('Reviewed recovery award', { exact: true })).toHaveCount(0); await expect(organisation.getByText('No published award shown', { exact: true })).toBeVisible(); await expect(organisation.getByRole('link', { name: 'Reviewed recovery demonstration (current authorised media)', exact: true })).toHaveCount(0)
  await control('correct'); await page.getByRole('button', { name: 'Read current project', exact: true }).click(); await expect(page.getByText('Corrected approved recovery approach.', { exact: true })).toBeVisible(); await expect(page.getByText('Follow-up in progress', { exact: true })).toBeVisible()
  await control('withdraw-nonwinner'); const hidden = await reader.request.get(`/api/v1/projects/${native.nonwinner_project}`); expect(hidden.status()).toBe(404)
  const evidence = await control<{ nonwinner_versions: number }>('evidence'); expect(evidence.nonwinner_versions).toBe(3)
  await control('archive'); await page.getByRole('button', { name: 'Read current project', exact: true }).click(); await expect(page.getByText('Archived · Read-only', { exact: true })).toBeVisible(); await expect(page.getByRole('button', { name: /save|publish|edit/i })).toHaveCount(0)
  for (const colorScheme of ['light', 'dark'] as const) {
   await page.emulateMedia({ colorScheme }); await expect(page.getByText('Reviewed recovery award', { exact: true })).toBeVisible()
   const audit = await new AxeBuilder({ page }).include('main').analyze(); expect(audit.violations.filter(v => v.impact === 'serious' || v.impact === 'critical')).toEqual([])
   expect(await page.locator('html').evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
   await page.screenshot({ path: testInfo.outputPath(`reviewed-project-${colorScheme}.png`), fullPage: true })
  }
  await page.emulateMedia({ media: 'print' }); await expect(page.getByText('Corrected approved recovery approach.', { exact: true })).not.toBeVisible(); await page.emulateMedia({ media: 'screen' })
  await control('decline-consent'); await page.getByRole('button', { name: 'Read current project', exact: true }).click(); await expect(page.getByRole('link', { name: 'Reviewed recovery demonstration (current authorised media)', exact: true })).toHaveCount(0)
  const revokedMedia = await reader.request.get(`/api/v1/events/${native.event_id}/media/${native.media_id}`); expect(revokedMedia.status()).toBe(404)
  await page.goto(restricted); await expect(page.getByRole('heading', { name: 'Event audience project', exact: true })).toBeVisible(); await control('withdraw-audience'); await page.getByRole('button', { name: 'Read current project', exact: true }).click(); await expect(page.getByRole('heading', { name: 'Project unavailable', exact: true })).toBeVisible()
  await page.goto(winner); await expect(page.getByRole('heading', { name: 'Recover services with less effort', exact: true })).toBeVisible(); await expect(page.getByText('Reviewed recovery award', { exact: true })).toHaveCount(0)
  // Retain an actual former native response, then withdraw its publication and
  // revisit under a fresh mount before delivering that stale response.
  let release!: () => void, entered!: () => void
  const gate = new Promise<void>(resolve => { release = resolve }), reached = new Promise<void>(resolve => { entered = resolve })
  await page.route(`**/api/v1/projects/${native.second_project}`, async route => { const response = await route.fetch(); entered(); await gate; await route.fulfill({ response }) }, { times: 1 })
  const pending = page.goto(cached); await reached; await control('withdraw-cache-project'); await page.goto('/projects'); await page.goto(cached); await expect(page.getByRole('heading', { name: 'Project unavailable', exact: true })).toBeVisible(); release(); await pending.catch(() => {}); await expect(page.getByText('Explore work across events', { exact: true })).toHaveCount(0)
  expect(await page.locator('body').innerText()).not.toContain('PRIVATE-')
 } finally { await Promise.all([reader.close(), outsider.close()]) }
})

import AxeBuilder from '@axe-core/playwright'
import { test, expect, type Route } from '@playwright/test'
import type { components } from '../src/api/schema.gen'
import { actor, native } from './native'
test('native assigned overall and category ties require explicit selection, all judges and reviewed replacements', async ({ browser }, testInfo) => {
 const first = await actor(browser, 'public', 'judge', 'ballots'), second = await actor(browser, 'admin', 'judge2', 'ballots'), owner = await actor(browser, 'admin', 'owner', 'ballots')
 const a = await first.newPage(), b = await second.newPage(), path = `/events/${native.ballot_event_id}/results/ballots`
 async function vote(page: typeof a, winner: string, note: string) {
  await page.getByLabel('Your selected winner', { exact: true }).selectOption(winner)
  for (const input of await page.getByLabel(/^Your observed evidence for /).all()) await input.fill(note)
  await page.getByLabel('Reason for your tie vote', { exact: true }).fill(note)
  const submit = page.getByRole('button', { name: 'Review tie vote', exact: true }); await submit.click()
  const review = page.getByRole('alertdialog', { name: 'Review your tie vote', exact: true })
  // Exercise the CI-observed current-read/cancel overlap deterministically. Hold
  // only the actual native200 response; neither data nor authority is fabricated.
  let release!: () => void, ready!: () => void, held = false
  const resume = new Promise<void>(resolve => { release = resolve }), started = new Promise<void>(resolve => { ready = resolve })
  const holdRead = async (route: Route) => {
   if (route.request().method() !== 'GET' || held) return route.fallback()
   held = true; const response = await route.fetch(); expect(response.status()).toBe(200); ready(); await resume; await route.fulfill({ response })
  }
  const readPath = '**/api/v1/events/' + native.ballot_event_id + '/results/ballots'
  await page.route(readPath, holdRead)
  await page.evaluate(() => window.dispatchEvent(new Event('focus'))); await started
  await review.getByRole('button', { name: 'Cancel', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Decision status', exact: true })).toBeFocused()
  release(); await expect(submit).toBeFocused(); await page.unroute(readPath, holdRead)
  await submit.click(); await review.getByRole('button', { name: 'Record reviewed tie vote', exact: true }).click()
 }
 try {
  await a.goto(path); await expect(a.getByRole('heading', { name: 'Review assigned ties' })).toBeVisible(); await expect(a.getByLabel(/^Your observed evidence for /)).toHaveCount(0); await a.getByRole('button', { name: 'Review Overall final', exact: true }).press('Enter'); await expect(a.getByRole('region', { name: 'Selected tie', exact: true }).getByRole('heading')).toBeFocused(); await expect(a.getByLabel('Your selected winner', { exact: true })).toHaveValue(''); await expect(a.getByLabel(/^Your observed evidence for /)).toHaveCount(2)
  const privatePath = `/api/v1/events/${native.ballot_event_id}/results/ballots`
  let sent = 0
  await a.route('**' + privatePath, async route => { if (route.request().method() !== 'POST') return route.continue(); sent++; const response = await route.fetch(); expect(response.status()).toBe(200); await route.abort('failed') })
  await vote(a, native.ballot_team_id, 'PRIVATE-FIRST-OWN-VOTE-NATIVE')
  await expect(a.getByText('The original outcome is unknown.', { exact: false })).toBeVisible(); await a.getByRole('button', { name: 'Read current tie ballots', exact: true }).click(); await expect(a.getByRole('complementary', { name: 'Original request history' })).toContainText('Original outcome unknown'); expect(sent).toBe(1)
  await a.getByRole('button', { name: 'Read original acknowledgement', exact: true }).click(); await expect(a.getByText('The exact original request was acknowledged.', { exact: false })).toBeVisible(); expect(sent).toBe(1)
  await b.goto(path); await expect(b.getByLabel('Your selected winner', { exact: true })).toHaveCount(0); await b.getByRole('button', { name: 'Review Overall final', exact: true }).click(); await expect(b.getByRole('region', { name: 'Selected tie', exact: true }).getByText('1/2 required ballots', { exact: false })).toBeVisible(); expect(await b.locator('body').innerText()).not.toContain('PRIVATE-FIRST-OWN-VOTE-NATIVE')
  await vote(b, native.second_team_id, 'PRIVATE-SECOND-SPLIT-VOTE-NATIVE'); await expect(b.getByRole('region', { name: 'Selected tie', exact: true }).getByText('recorded judge majority required', { exact: false })).toBeVisible(); await expect(b.getByRole('region', { name: 'Selected tie', exact: true }).getByText('Recorded winner:', { exact: false })).toHaveCount(0)
  await a.getByRole('button', { name: 'Read current tie ballots', exact: true }).click(); expect(await a.locator('body').innerText()).not.toContain('PRIVATE-SECOND-SPLIT-VOTE-NATIVE')
  await vote(b, native.ballot_team_id, 'Explicit reviewed native replacement after split'); await expect(b.getByRole('region', { name: 'Selected tie', exact: true }).getByText('Recorded winner:', { exact: false })).toBeVisible()
  // Discover the category and its label from the same real native run/brief.
  const page: components['schemas']['ResultBallotPage'] = await (await second.request.get(native.ballot_admin + privatePath)).json()
  const category = page.ties.find(item => item.state.tie.kind === 'category')!
  expect(category).toBeTruthy()
  const brief: components['schemas']['JudgeBrief'] = await (await second.request.get(native.ballot_admin + `/api/v1/events/${native.ballot_event_id}/judging/${page.judge_run_id}/teams/${native.ballot_team_id}`)).json()
  const categoryLabel = brief.criteria.categories.find(item => item.id === category.state.tie.category_id)!.label + ' category'
  await expect(b.getByRole('region', { name: 'Assigned tie tasks', exact: true }).getByRole('button')).toHaveCount(page.ties.length)
  await b.getByRole('button', { name: 'Review next assigned tie', exact: true }).click()
  await expect(b.getByRole('region', { name: 'Selected tie', exact: true }).getByRole('heading')).toHaveText(categoryLabel)
  await expect(b.getByRole('region', { name: 'Selected tie', exact: true }).getByRole('heading')).toBeFocused()
  await expect(b.getByLabel('Your selected winner', { exact: true })).toHaveValue('')
  await expect(b.getByLabel('Reason for your tie vote', { exact: true })).toHaveValue('')
  await a.getByRole('button', { name: 'Read current tie ballots', exact: true }).click()
  a.once('dialog', dialog => void dialog.accept())
  await a.getByRole('button', { name: 'Review ' + categoryLabel, exact: true }).click()
  await vote(a, native.ballot_team_id, 'PRIVATE-FIRST-CATEGORY-VOTE-NATIVE')
  await expect(a.getByText('The original outcome is unknown.', { exact: false })).toBeVisible()
  await a.getByRole('button', { name: 'Read current tie ballots', exact: true }).click()
  await expect(a.getByRole('complementary', { name: 'Original request history' })).toContainText('Original outcome unknown')
  expect(sent).toBe(2)
  await a.getByRole('button', { name: 'Read original acknowledgement', exact: true }).click()
  await expect(a.getByText('The exact original request was acknowledged.', { exact: false })).toBeVisible()
  expect(sent).toBe(2)
  await b.getByRole('button', { name: 'Read current tie ballots', exact: true }).click()
  await expect(b.getByRole('region', { name: 'Selected tie', exact: true })).toContainText('1/2 required ballots')
  expect(await b.locator('body').innerText()).not.toContain('PRIVATE-FIRST-CATEGORY-VOTE-NATIVE')
  await vote(b, native.second_team_id, 'PRIVATE-SECOND-CATEGORY-SPLIT-NATIVE')
  await expect(b.getByRole('region', { name: 'Selected tie', exact: true })).toContainText('recorded judge majority required')
  await expect(b.getByRole('region', { name: 'Selected tie', exact: true }).getByText('Recorded winner:', { exact: false })).toHaveCount(0)
  await a.getByRole('button', { name: 'Read current tie ballots', exact: true }).click()
  expect(await a.locator('body').innerText()).not.toContain('PRIVATE-SECOND-CATEGORY-SPLIT-NATIVE')
  await vote(b, native.ballot_team_id, 'Explicit reviewed category replacement after split')
  await expect(b.getByRole('region', { name: 'Selected tie', exact: true })).toContainText('Recorded winner:')
  const settled: components['schemas']['ResultBallotPage'] = await (await second.request.get(native.ballot_admin + privatePath)).json()
  expect(settled.ties.find(item => item.state.tie.id === category.state.tie.id)?.state.winner_id).toBe(native.ballot_team_id)
  expect(settled.ties.find(item => item.state.tie.kind === 'final')?.state.winner_id).toBe(native.ballot_team_id)
  await b.getByRole('button', { name: 'Review next assigned tie', exact: true }).click()
  await expect(b.getByRole('region', { name: 'Selected tie', exact: true }).getByRole('heading')).not.toHaveText(categoryLabel)
  await expect(b.getByRole('region', { name: 'Selected tie', exact: true }).getByRole('heading')).toBeFocused()
  await expect(b.getByLabel('Your selected winner', { exact: true })).toHaveValue('')
  await b.getByRole('button', { name: 'Review ' + categoryLabel, exact: true }).click()
  const denied = await owner.request.get(native.ballot_admin + privatePath); expect(denied.status()).toBe(403)
  for (const scheme of ['light', 'dark'] as const) { await b.emulateMedia({ colorScheme: scheme }); const audit = await new AxeBuilder({ page: b }).include('main').analyze(); expect(audit.violations.filter(v => v.impact === 'serious' || v.impact === 'critical')).toEqual([]); expect(await b.locator('html').evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false); await b.screenshot({ path: testInfo.outputPath(`category-ballots-${scheme}.png`), fullPage: true }) }
  await b.emulateMedia({ media: 'print' }); await expect(b.getByRole('heading', { name: 'Review assigned ties' })).not.toBeVisible(); await b.emulateMedia({ media: 'screen' }); await expect(b.getByRole('heading', { name: 'Review assigned ties' })).toBeVisible()
 } finally { await Promise.all([first.close(), second.close(), owner.close()]) }
})

import { test, expect } from '@playwright/test'
import { actor, native } from './native'
test('native assigned overall tie requires all judges, keeps own prose private and resolves only after an explicit revote', async ({ browser }) => {
 const first = await actor(browser, 'public', 'judge', 'ballots'), second = await actor(browser, 'admin', 'judge2', 'ballots'), owner = await actor(browser, 'admin', 'owner', 'ballots')
 const a = await first.newPage(), b = await second.newPage(), path = `/events/${native.ballot_event_id}/results/ballots`
 async function vote(page: typeof a, winner: string, note: string) {
  await page.getByLabel('Your selected winner', { exact: true }).selectOption(winner)
  for (const input of await page.getByLabel(/^Your observed evidence for /).all()) await input.fill(note)
  await page.getByLabel('Reason for your final tie vote', { exact: true }).fill(note)
  const submit = page.getByRole('button', { name: 'Review final tie vote', exact: true }); await submit.click()
  const review = page.getByRole('alertdialog', { name: 'Review your final tie vote', exact: true }); await review.getByRole('button', { name: 'Cancel', exact: true }).click(); await expect(submit).toBeFocused(); await submit.click(); await review.getByRole('button', { name: 'Record reviewed final tie vote', exact: true }).click()
 }
 try {
  await a.goto(path); await expect(a.getByRole('heading', { name: 'Review final ties' })).toBeVisible(); await expect(a.getByLabel(/^Your observed evidence for /)).toHaveCount(2)
  const privatePath = `/api/v1/events/${native.ballot_event_id}/results/ballots`
  let sent = 0
  await a.route('**' + privatePath, async route => { if (route.request().method() !== 'POST') return route.continue(); sent++; const response = await route.fetch(); expect(response.status()).toBe(200); await route.abort('failed') })
  await vote(a, native.ballot_team_id, 'PRIVATE-FIRST-OWN-VOTE-NATIVE')
  await expect(a.getByText('The original outcome is unknown.', { exact: false })).toBeVisible(); await a.getByRole('button', { name: 'Read current tie ballots', exact: true }).click(); await expect(a.getByRole('complementary', { name: 'Original request history' })).toContainText('Original outcome unknown'); expect(sent).toBe(1)
  await a.getByRole('button', { name: 'Read original acknowledgement', exact: true }).click(); await expect(a.getByText('The exact original request was acknowledged.', { exact: false })).toBeVisible(); expect(sent).toBe(1)
  await b.goto(path); await expect(b.getByText('1/2 required ballots', { exact: false })).toBeVisible(); expect(await b.locator('body').innerText()).not.toContain('PRIVATE-FIRST-OWN-VOTE-NATIVE')
  await vote(b, native.second_team_id, 'PRIVATE-SECOND-SPLIT-VOTE-NATIVE'); await expect(b.getByText('recorded judge majority required', { exact: false })).toBeVisible(); await expect(b.getByText('Recorded winner:', { exact: false })).toHaveCount(0)
  await a.getByRole('button', { name: 'Read current tie ballots', exact: true }).click(); expect(await a.locator('body').innerText()).not.toContain('PRIVATE-SECOND-SPLIT-VOTE-NATIVE')
  await vote(b, native.ballot_team_id, 'Explicit reviewed native replacement after split'); await expect(b.getByText('Recorded winner:', { exact: false })).toBeVisible()
  const denied = await owner.request.get(native.ballot_admin + privatePath); expect(denied.status()).toBe(403)
  await b.emulateMedia({ media: 'print' }); await expect(b.getByRole('heading', { name: 'Review final ties' })).not.toBeVisible(); await b.emulateMedia({ media: 'screen' }); await expect(b.getByRole('heading', { name: 'Review final ties' })).toBeVisible()
 } finally { await Promise.all([first.close(), second.close(), owner.close()]) }
})

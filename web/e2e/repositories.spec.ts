import { expect, test } from '@playwright/test'
import { firstID } from './fixtures'
import { repositoryFixture, repositoryTeam } from './repository-fixtures'
import { openEventSection } from './event-navigation-fixture'
const teamID = repositoryTeam
test('current repository collection opens an exact team inspector with independent private read and keyboard return', async ({ page }, info) => {
  const regular = info.project.metadata.portal === 'regular'
  const state = await repositoryFixture(page, regular)
  await page.goto(`/events/${firstID}`); await openEventSection(page, 'Repositories')
  await expect(page.getByRole('heading', { name: 'Repositories', exact: true })).toBeVisible()
  await expect(page.getByText('Repository confirmed', { exact: true })).toBeVisible()
  if (!regular) await expect(page.getByText('Unused · unscored', { exact: true })).toBeVisible()
  const opener = page.getByRole('button', { name: 'Inspect repository', exact: true }).first()
  await opener.focus(); await page.keyboard.press('Enter')
  const inspector = page.getByRole('region', { name: 'synthetic-org/synthetic-repository', exact: true })
  await expect(inspector).toContainText(teamID); await expect(inspector).toContainText('123456'); await expect(inspector.getByRole('link', { name: 'Open repository on GitHub', exact: true })).toHaveAttribute('href', 'https://github.com/synthetic-org/synthetic-repository'); await expect(inspector.getByRole('link', { name: 'Open repository on GitHub', exact: true })).toHaveAttribute('target', '_blank')
  if (!regular) { await expect(inspector).toContainText('verified account required'); expect(state.accessReads).toBe(1) }
  else { expect(state.accessReads).toBe(0); await expect(inspector).toContainText('Your repository invitation') }
  await inspector.getByRole('link', { name: 'Back to repositories', exact: true }).click(); await expect(opener).toBeFocused()
  expect(state.writes).toHaveLength(0); expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(page.viewportSize()!.width)
})


test('regular repository view shows only the current own identity and distinguishes pending from accepted or ambiguous invitations', async ({ page }, info) => {
  test.skip(info.project.metadata.portal !== 'regular', 'Owning regular-portal policy')
  const state = await repositoryFixture(page, true); state.exposeAllRows = true; state.event.can_read_staff = true; state.event.can_edit = true
  await page.goto(`/events/${firstID}`); await openEventSection(page, 'Repositories'); await expect(page.getByRole('button', { name: 'Inspect repository', exact: true })).toHaveCount(1)
  await expect(page.getByRole('button', { name: 'Open App setup', exact: true })).toHaveCount(0); await expect(page.getByRole('button', { name: 'Review published-team provisioning', exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: 'Inspect repository', exact: true }).click(); const own = page.getByRole('heading', { name: 'Your repository invitation', exact: true }).locator('..'); await expect(own).toContainText('pending'); await expect(own).toContainText('A pending invitation is not accepted access'); expect(state.accessReads).toBe(0)
  state.own.grants[0].state = 'accepted'; await own.getByRole('button', { name: 'Refresh your current invitation status', exact: true }).click(); await expect(own.getByText('accepted', { exact: true })).toBeVisible()
  state.own.grants[0].state = 'ambiguous'; state.own.grants[0].effect_uncertain = true; await own.getByRole('button', { name: 'Refresh your current invitation status', exact: true }).click(); await expect(own).toContainText('Provider effect unknown'); await expect(own.getByText('ambiguous', { exact: true })).toBeVisible(); state.own.current_github_id = 54321; state.own.link_revision = 3; await own.getByRole('button', { name: 'Refresh your current invitation status', exact: true }).click(); await expect(own).toContainText('earlier linked GitHub account'); await expect(own).toContainText('GitHub ID 98765'); expect(state.writes).toHaveLength(0)
})

test('regular parent denial and partial restoration require fresh own authority before any old invitation can return', async ({ page }, info) => {
  test.skip(info.project.metadata.portal !== 'regular', 'Owning regular-portal policy')
  const state = await repositoryFixture(page, true); await page.goto(`/events/${firstID}`); await openEventSection(page, 'Repositories'); await page.getByRole('button', { name: 'Inspect repository', exact: true }).click(); await expect(page.getByText('Invitation 91234', { exact: false })).toBeVisible()
  state.ownDenied = true; await page.getByRole('button', { name: 'Refresh current repository facts', exact: true }).click(); await expect(page.getByRole('button', { name: 'Check current repository access', exact: true })).toBeVisible(); await expect(page.getByText('Invitation 91234', { exact: false })).toHaveCount(0)
  let release!: () => void; state.holdOwn = new Promise<void>(resolve => { release = resolve }); state.ownDenied = false; state.own.grants = []
  await page.getByRole('button', { name: 'Check current repository access', exact: true }).click(); await expect(page.getByRole('button', { name: 'Inspect repository', exact: true })).toHaveCount(0); await expect(page.getByText('Invitation 91234', { exact: false })).toHaveCount(0)
  release(); await page.getByRole('button', { name: 'Inspect repository', exact: true }).click(); await expect(page.getByText('No current invitation is recorded.', { exact: false })).toBeVisible(); await expect(page.getByText('Invitation 91234', { exact: false })).toHaveCount(0); expect(state.writes).toHaveLength(0)
})

test('regular missing verified identity links to account verification without pretending access or allowing provisioning', async ({ page }, info) => {
  test.skip(info.project.metadata.portal !== 'regular', 'Owning regular-portal policy')
  const state = await repositoryFixture(page, true); state.own.current_github_id = null; state.own.grants = []; state.own.diagnostic = 'verified_account_required'
  await page.goto(`/events/${firstID}`); await openEventSection(page, 'Repositories'); await page.getByRole('button', { name: 'Inspect repository', exact: true }).click(); await expect(page.getByText('No verified GitHub identity linked', { exact: false })).toBeVisible(); await expect(page.getByRole('link', { name: 'Verify your own GitHub account', exact: true })).toHaveAttribute('href', '/account'); await expect(page.getByRole('button', { name: /approval|provisioning|reconciliation/ })).toHaveCount(0); expect(state.writes).toHaveLength(0)
})

test('hostile team prose stays inert and wraps while the static inspection action remains one visible line inside its record', async ({ page }, info) => {
  const state = await repositoryFixture(page, info.project.metadata.portal === 'regular'); state.teamNames[0] = 'Synthetic <script>window.repoLeak=true</script> ' + 'x'.repeat(75)
  await page.goto(`/events/${firstID}`); await openEventSection(page, 'Repositories'); await expect(page.getByText(state.teamNames[0], { exact: true })).toBeVisible()
  const action = page.getByRole('button', { name: 'Inspect repository', exact: true }).first()
  const geometry = await action.evaluate(button => {
    const walker = document.createTreeWalker(button, NodeFilter.SHOW_TEXT); let text: Node | null = walker.nextNode(); while (text && text.textContent?.trim() !== 'Inspect repository') text = walker.nextNode(); if (!text) throw new Error('Missing actual action label')
    const range = document.createRange(); range.selectNodeContents(text); const lines = [...range.getClientRects()].filter(rect => rect.width > 0), bounds = button.getBoundingClientRect(), record = (button.closest('td') ?? button.closest('li'))!.getBoundingClientRect()
    return { lines: lines.length, inside: bounds.left >= record.left - .5 && bounds.right <= record.right + .5 && lines.every(rect => rect.left >= bounds.left && rect.right <= bounds.right) }
  })
  expect(geometry.lines).toBe(1); expect(geometry.inside).toBe(true); expect(await page.evaluate(() => 'repoLeak' in window)).toBe(false); expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(page.viewportSize()!.width); expect(state.writes).toHaveLength(0)
})

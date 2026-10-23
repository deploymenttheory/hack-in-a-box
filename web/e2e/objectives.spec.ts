import { expect, test } from '@playwright/test'
import { firstID, person } from './fixtures'
import { mockObjectives, objectiveID, plannedDefinition } from './objective-fixtures'
const eventPath = `/events/${firstID}`
async function enter(page: import('@playwright/test').Page) { await page.goto(eventPath); await page.getByRole('button', { name: 'Objectives', exact: true }).click() }
async function create(page: import('@playwright/test').Page, description = 'PRIVATE OBJECTIVE UNSENT') { await page.getByRole('button', { name: 'Add objective', exact: true }).click(); await page.getByLabel('Objective description', { exact: true }).fill(description); await page.getByLabel('Reason for this definition version').fill('Explicit planning decision') }

test('keyboard/mobile optional planning saves explicit zero separately from unknown and shows retained history', async ({ page }, info) => {
  const { state } = await mockObjectives(page); await enter(page); await create(page, 'PRIVATE SAFE OBJECTIVE <script>window.objectiveLeak=true</script>')
  await page.getByRole('button', { name: 'Add success measure' }).click(); await page.getByLabel('Measure description').fill('Example count'); await page.getByLabel('Target (optional; blank means unknown)').fill('0'); await page.getByLabel('Unit', { exact: true }).fill('working example'); await page.getByLabel('Measurement method').fill('Review examples after the event'); await page.getByLabel('Where the review should obtain evidence').fill('An explicit later review'); await page.getByLabel('Review horizon').fill('One month after event')
  await expect(page.getByText('Baseline unknown. This is not zero.')).toBeVisible()
  await page.getByRole('button', { name: 'Save definition version', exact: true }).focus(); await page.keyboard.press('Enter')
  await expect(page.getByRole('status', { name: 'Objective save state' })).toContainText('Definition version 1 saved')
  expect(state.requests).toHaveLength(1); expect(state.requests[0].command.definition!.measures[0]).toMatchObject({ baseline: null, target: '0' }); expect(state.requests[0].command.definition!.owner_id).toBeNull()
  await page.getByRole('button', { name: 'Show definition history' }).click(); await expect(page.getByRole('region', { name: 'Objective definition history' })).toContainText('target 0 working example; baseline Unknown')
  expect(await page.evaluate(() => [...Object.values(sessionStorage), ...Object.values(localStorage)].join('').includes('PRIVATE SAFE OBJECTIVE'))).toBe(false)
  expect(await page.evaluate(() => 'objectiveLeak' in window)).toBe(false)
  await page.screenshot({ path: info.outputPath('objectives-private-planning.png'), fullPage: true }); expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(page.viewportSize()!.width)
})

test('stale unit/definition change keeps unsent edits and requires explicit current-base review before saving', async ({ page }) => {
  const { state, advance } = await mockObjectives(page, true); await enter(page); await page.getByRole('button', { name: 'Open objective', exact: true }).click()
  await page.getByLabel('Unit', { exact: true }).fill('reviewed demonstration'); await page.getByLabel('Reason for this definition version').fill('Clarify evidence unit')
  const latest = plannedDefinition('Updated server objective'); latest.measures[0].unit = 'server-selected unit'; advance(firstID, objectiveID, latest)
  await page.getByRole('button', { name: 'Save definition version', exact: true }).click(); await expect(page.getByRole('region', { name: 'Current definition comparison' })).toContainText('server-selected unit')
  await expect(page.getByLabel('Unit', { exact: true })).toHaveValue('reviewed demonstration'); expect(state.requests).toHaveLength(0)
  await page.getByRole('button', { name: 'Use version 2 as base and keep my edits' }).click(); expect(state.requests).toHaveLength(0)
  await page.getByRole('button', { name: 'Save definition version', exact: true }).click(); await expect(page.getByRole('status', { name: 'Objective save state' })).toContainText('Definition version 3 saved')
  expect(state.requests).toHaveLength(1); expect(state.requests[0].expected).toBe('"2"'); expect(state.requests[0].command.definition!.measures[0].unit).toBe('reviewed demonstration')
})

test('accepted response loss is reconciled against exact retained version without replay or object-order assumptions', async ({ page }) => {
  const { state } = await mockObjectives(page); await enter(page); await create(page)
  state.mode = 'after'; await page.getByRole('button', { name: 'Save definition version', exact: true }).click(); await expect(page.getByRole('status', { name: 'Objective save state' })).toContainText('unconfirmed')
  expect(state.requests).toHaveLength(1); await page.getByRole('button', { name: 'Check exact save outcome' }).click(); await expect(page.getByRole('status', { name: 'Objective save state' })).toContainText('Exact retained definition version 1 matches')
  expect(state.requests).toHaveLength(1); await expect(page.getByRole('button', { name: 'Retry the same request' })).toHaveCount(0)
})

test('uncommitted response loss offers only deliberate same-body same-base retry', async ({ page }) => {
  const { state } = await mockObjectives(page); await enter(page); await create(page)
  state.mode = 'before'; await page.getByRole('button', { name: 'Save definition version', exact: true }).click(); await expect(page.getByRole('status', { name: 'Objective save state' })).toContainText('unconfirmed')
  await page.getByRole('button', { name: 'Check exact save outcome' }).click(); await expect(page.getByRole('status', { name: 'Objective save state' })).toContainText('original base is still current')
  expect(state.requests).toHaveLength(1); await page.getByRole('button', { name: 'Retry the same request', exact: true }).click(); await expect(page.getByRole('status', { name: 'Objective save state' })).toContainText('Definition version 1 saved')
  expect(state.requests).toHaveLength(2); expect(state.requests[0]).toEqual(state.requests[1])
})

test('structured validation rejection preserves unsent private input and does not claim an uncertain save', async ({ page }) => {
  const { state } = await mockObjectives(page); await enter(page); await create(page)
  state.mode = 'validation'; await page.getByRole('button', { name: 'Save definition version', exact: true }).click(); await expect(page.getByRole('status', { name: 'Objective save state' })).toContainText('Validation rejected the request')
  await expect(page.getByLabel('Objective description', { exact: true })).toHaveValue('PRIVATE OBJECTIVE UNSENT'); await expect(page.getByRole('button', { name: 'Check exact save outcome' })).toHaveCount(0); expect(state.records.size).toBe(0)
})

test('transient current read failure retains same-page unsent edits and cannot write automatically', async ({ page }) => {
  const { state } = await mockObjectives(page, true); await enter(page); await page.getByRole('button', { name: 'Open objective', exact: true }).click(); await page.getByLabel('Objective description', { exact: true }).fill('PRIVATE INPUT THROUGH NETWORK FAILURE')
  state.readFail = true; await page.getByRole('button', { name: 'Refresh planning', exact: true }).click(); await expect(page.getByText('Current planning access could not be checked.', { exact: false })).toBeVisible(); await expect(page.getByLabel('Objective description', { exact: true })).toHaveValue('PRIVATE INPUT THROUGH NETWORK FAILURE'); await expect(page.getByRole('button', { name: 'Save definition version', exact: true })).toBeDisabled()
  state.readFail = false; await page.getByRole('button', { name: 'Check connection and current access' }).click(); await expect(page.getByLabel('Objective description', { exact: true })).toBeEnabled(); await expect(page.getByLabel('Objective description', { exact: true })).toHaveValue('PRIVATE INPUT THROUGH NETWORK FAILURE'); expect(state.requests).toHaveLength(0)
})

test('active current-role revocation clears private editor and caches without a focus change', async ({ page }) => {
  const { state } = await mockObjectives(page, true); await enter(page); await page.getByRole('button', { name: 'Open objective', exact: true }).click(); await page.getByLabel('Objective description', { exact: true }).fill('REVOKED PRIVATE EDITOR INPUT')
  state.deny = true; await expect(page.getByRole('heading', { name: 'Objectives unavailable' })).toBeVisible({ timeout: 20_000 }); await expect(page.getByLabel('Objective description', { exact: true })).toHaveCount(0); await expect(page.locator('body')).not.toContainText('REVOKED PRIVATE EDITOR INPUT'); expect(state.requests).toHaveLength(0)
})

test('archive is read-only planning and missing owners/baselines remain advisory', async ({ page }) => {
  const { state } = await mockObjectives(page, true); state.workspace.events[0].can_edit = false; state.workspace.events[0].stage = 'archived'; await enter(page); await page.getByRole('button', { name: 'Open objective', exact: true }).click()
  await expect(page.getByLabel('Objective description', { exact: true })).toBeDisabled(); await expect(page.getByText('No owner selected', { exact: true }).first()).toBeVisible(); await page.getByRole('button', { name: 'Show definition history' }).click(); await expect(page.getByRole('region', { name: 'Objective definition history' })).toContainText('prototype'); expect(state.requests).toHaveLength(0)
})

test('navigation warnings keep safe unsent planning on explicit cancel', async ({ page }) => {
  const { state } = await mockObjectives(page); await enter(page); await create(page)
  page.on('dialog', dialog => dialog.dismiss()); await page.getByRole('button', { name: 'Overview', exact: true }).click(); await expect(page.getByLabel('Objective description', { exact: true })).toHaveValue('PRIVATE OBJECTIVE UNSENT'); expect(state.requests).toHaveLength(0)
})

test('owner remains an optional current-organiser selection rather than a directory or lifecycle gate', async ({ page }) => {
  const { state } = await mockObjectives(page); await enter(page); await create(page); await page.getByLabel('Objective owner (optional)').selectOption(person.id); await page.getByRole('button', { name: 'Save definition version', exact: true }).click(); await expect(page.getByRole('status', { name: 'Objective save state' })).toContainText('Definition version 1 saved'); expect(state.requests[0].command.definition!.owner_id).toBe(person.id)
})

for (const phase of ['preflight', 'accepted', 'outcome'] as const) {
  for (const leave of ['event', 'session'] as const) {
    test(`in-flight ${phase} cannot continue private editor work after ${leave} departure`, async ({ page }) => {
      const { state, gates } = await mockObjectives(page, true)
      await enter(page); await page.getByRole('button', { name: 'Open objective', exact: true }).click()
      await page.getByLabel('Objective description', { exact: true }).fill('PRIVATE DEPARTED OBJECTIVE')
      await page.getByLabel('Reason for this definition version').fill('Explicit in-flight change')
      let release!: () => void, entered!: () => void
      const held = new Promise<void>(resolve => { release = resolve }), reached = new Promise<void>(resolve => { entered = resolve })
      const gate = async () => { entered(); await held }
      if (phase === 'preflight') gates.current = gate
      if (phase === 'accepted') gates.accepted = gate
      if (phase === 'outcome') state.mode = 'after'
      await page.getByRole('button', { name: 'Save definition version', exact: true }).click()
      if (phase === 'outcome') {
        await expect(page.getByRole('status', { name: 'Objective save state' })).toContainText('unconfirmed')
        gates.retained = gate
        await page.getByRole('button', { name: 'Check exact save outcome' }).click()
      }
      await reached
      page.on('dialog', dialog => dialog.accept())
      if (leave === 'event') {
        await page.getByRole('link', { name: '← Switch event' }).click()
        const destination = page.getByRole('link', { name: 'Southern Lights Hack', exact: true })
        await expect(destination).toHaveAttribute('href', `/events/${state.workspace.events[1].id}`)
        await destination.click()
        await expect(page.getByRole('heading', { name: 'Southern Lights Hack' })).toBeVisible()
      } else {
        await page.evaluate(() => window.dispatchEvent(new Event('session-expired')))
        await expect(page.getByLabel('Objective description', { exact: true })).toHaveCount(0)
      }
      release()
      // Allow the held response and its continuation to settle without routing
      // or issuing any new user command in the departed scope.
      await page.waitForTimeout(150)
      expect(state.requests).toHaveLength(phase === 'preflight' ? 0 : 1)
      await expect(page.getByLabel('Objective description', { exact: true })).toHaveCount(0)
      await expect(page.getByText('PRIVATE DEPARTED OBJECTIVE', { exact: true })).toHaveCount(0)
      expect(await page.evaluate(() => [...Object.values(localStorage), ...Object.values(sessionStorage)].some(value => value.includes('PRIVATE DEPARTED OBJECTIVE')))).toBe(false)
    })
  }
}

import { randomUUID } from 'node:crypto'
import { test, expect } from '@playwright/test'
import { actor, control, native } from '../live-rehearsal/native'
import { activate, scan, typeInto } from './checks'

test('native participant reaches rules, reviews and acknowledges the exact version using only the keyboard', async ({ browser }, info) => {
  await control('clock', { instant: '2027-03-29T09:00:00Z' })
  const context = await actor(browser, 'public', 'competitor'), page = await context.newPage()
  try {
    await page.goto(`/events/${native.event_id}`)
    await expect(page.getByRole('link', { name: 'Hack in a Box home', exact: true })).toBeVisible()
    await activate(page, page.getByRole('button', { name: 'Today', exact: true }))
    await expect(page.getByRole('region', { name: 'Your event Today', exact: true })).toBeVisible()
    await scan(page, info, 'participant-today')
    const opener = page.getByRole('button', { name: 'Read Rehearsal rules', exact: true })
    await activate(page, opener)
    await expect(page.getByRole('heading', { name: 'Rehearsal rules', exact: true })).toBeVisible()
    await scan(page, info, 'released-rules')
    await activate(page, page.getByRole('button', { name: 'I have read version 1', exact: true }))
    await scan(page, info, 'rules-acknowledgement-review')
    const response = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname.endsWith('/acknowledgements') && r.status() === 200)
    await activate(page, page.getByRole('button', { name: 'Confirm acknowledgement', exact: true }))
    expect((await (await response).json()).version).toBe(1)
    await activate(page, page.getByRole('button', { name: 'Back to resources', exact: true }))
    await expect(opener).toBeFocused()
  } finally { await context.close() }
})

test('native mentor reaches private fields and reviews unsent evidence through keyboard navigation', async ({ browser }, info) => {
  await control('clock', { instant: '2027-03-29T09:00:00Z' }); await control('mentor-role-restore')
  const context = await actor(browser, 'public', 'mentor'), page = await context.newPage()
  try {
    await page.goto(`/events/${native.event_id}/mentor/${native.team_id}?note=${randomUUID()}&fresh=true`)
    await expect(page.getByRole('heading', { name: 'New private observation', exact: true })).toBeVisible()
    await typeInto(page, page.getByLabel('Category (private)', { exact: true }), 'Synthetic accessibility evidence')
    await typeInto(page, page.getByLabel('Evidence (private)', { exact: true }), 'Synthetic keyboard review; unsent.')
    await typeInto(page, page.getByLabel('Outcome (private)', { exact: true }), 'Review only')
    await scan(page, info, 'native-mentor-editor')
    await activate(page, page.getByRole('button', { name: 'Review private observation', exact: true }))
    await expect(page.getByRole('heading', { name: 'Review private observation', exact: true })).toBeFocused()
    await scan(page, info, 'native-mentor-review')
    await activate(page, page.getByRole('button', { name: 'Correct details', exact: true }))
    await expect(page.getByLabel('Evidence (private)', { exact: true })).toHaveValue('Synthetic keyboard review; unsent.')
  } finally { await context.close() }
})

test('native administrator can reach MC review and correction without a pointer', async ({ browser }, info) => {
  await control('clock', { instant: '2027-03-29T09:00:00Z' }); await control('reset-room')
  const context = await actor(browser, 'admin', 'mc'), page = await context.newPage()
  try {
    await page.goto(`/events/${native.event_id}`)
    await expect(page.getByRole('link', { name: 'Hack in a Box · Admin home', exact: true })).toBeVisible()
    await activate(page, page.getByRole('button', { name: 'MC console', exact: true }))
    const opener = page.getByRole('button', { name: 'Hold countdown', exact: true })
    await scan(page, info, 'native-mc-controls')
    await activate(page, opener)
    await expect(page.getByText('Record a reason within 2000 UTF-8 bytes.', { exact: true })).toBeVisible()
    await scan(page, info, 'native-mc-required-reason')
    await typeInto(page, page.getByLabel('Reason for room changes', { exact: true }), 'Synthetic keyboard room review')
    await activate(page, opener)
    await expect(page.getByRole('button', { name: 'Confirm room change', exact: true })).toBeVisible()
    await scan(page, info, 'native-mc-review')
    await activate(page, page.getByRole('button', { name: 'Correct details', exact: true }))
    await expect(opener).toBeFocused()
  } finally { await context.close() }
})

test('native participant keyboard journey reaches help context and published official answers', async ({ browser }, info) => {
  await control('clock', { instant: '2027-03-29T09:00:00Z' })
  const context = await actor(browser, 'public', 'competitor'), page = await context.newPage()
  try {
    await page.goto(`/events/${native.event_id}`)
    await activate(page, page.getByRole('button', { name: 'Today', exact: true }))
    await activate(page, page.getByRole('button', { name: 'Open the current help request', exact: true }))
    await expect(page).toHaveURL(new RegExp(`/events/${native.event_id}/help/${native.help_id}$`))
    await expect(page.getByRole('region', { name: 'Private request context', exact: true })).toBeVisible()
    await scan(page, info, 'native-team-help-reader')
    await activate(page, page.getByRole('link', { name: 'Back to event', exact: true }))
    await activate(page, page.getByRole('button', { name: 'Today', exact: true }))
    await activate(page, page.getByRole('button', { name: 'Open official clarifications', exact: true }))
    await expect(page.getByRole('heading', { name: 'Questions and official clarifications', exact: true })).toBeVisible()
    await scan(page, info, 'native-common-answer-collection')
    await activate(page, page.getByRole('link', { name: 'Rehearsal official answer', exact: true }))
    await expect(page.getByRole('heading', { name: 'Rehearsal official answer', exact: true })).toBeVisible()
    expect(await page.locator('body').textContent()).not.toContain('PRIVATE-QUESTION-ORIGIN')
    await scan(page, info, 'native-common-answer-reader')
    await activate(page, page.getByRole('link', { name: 'Search common official answers', exact: true }))
    await expect(page.getByRole('searchbox', { name: 'Search published answers', exact: true })).toBeVisible()
  } finally { await context.close() }
})

test('native administrator creates an objective and success measure entirely by keyboard', async ({ browser }, info) => {
  await control('clock', { instant: '2027-03-29T09:00:00Z' })
  const context = await actor(browser, 'admin', 'owner'), page = await context.newPage()
  try {
    await page.goto(`/events/${native.event_id}`)
    await activate(page, page.getByRole('button', { name: 'Objectives', exact: true }))
    await activate(page, page.getByRole('button', { name: 'Add objective', exact: true }))
    await expect(page.getByRole('heading', { name: 'New objective', exact: true })).toBeVisible()
    await scan(page, info, 'native-objective-empty-editor')
    await typeInto(page, page.getByLabel('Objective description', { exact: true }), 'Synthetic keyboard objective')
    await activate(page, page.getByRole('button', { name: 'Add success measure', exact: true }))
    await typeInto(page, page.getByLabel('Measure description', { exact: true }), 'Synthetic measured follow-through')
    await typeInto(page, page.getByLabel('Target (optional; blank means unknown)', { exact: true }), '5')
    await typeInto(page, page.getByLabel('Unit', { exact: true }), 'teams')
    await typeInto(page, page.getByLabel('Measurement method', { exact: true }), 'Synthetic reviewed evidence')
    await typeInto(page, page.getByLabel('Where the review should obtain evidence', { exact: true }), 'Synthetic follow-up record')
    await typeInto(page, page.getByLabel('Review horizon', { exact: true }), 'After the synthetic event')
    await typeInto(page, page.getByLabel('Reason for this definition version', { exact: true }), 'Synthetic accessible planning check')
    await scan(page, info, 'native-objective-measure-editor')
    const accepted = page.waitForResponse(response => response.request().method() === 'PUT' && new URL(response.url()).pathname.includes('/objectives/') && response.status() === 200)
    await activate(page, page.getByRole('button', { name: 'Save definition version', exact: true }))
    const value = await (await accepted).json()
    expect(value.reference.version).toBe(1)
    expect(value.definition.description).toBe('Synthetic keyboard objective')
    expect(value.definition.measures[0].baseline).toBeNull()
    await expect(page.getByRole('status', { name: 'Objective save state', exact: true })).toContainText('No achieved results were recorded.')
    await scan(page, info, 'native-objective-saved-version')
    await activate(page, page.getByRole('button', { name: 'Back to objectives', exact: true }))
    await expect(page.getByRole('table', { name: 'Private objective definitions', exact: true })).toContainText('Synthetic keyboard objective')
  } finally { await context.close() }
})

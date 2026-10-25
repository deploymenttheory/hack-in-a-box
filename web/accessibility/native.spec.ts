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

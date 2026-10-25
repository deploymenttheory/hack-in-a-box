import { randomUUID } from 'node:crypto'
import { test, expect } from '@playwright/test'
import { actor, control, native } from './native'

test('mentor offline text is deliberately recovered only after native current authorization, then erased on role loss', async ({ browser }, info) => {
  await control('clock', { instant: '2027-03-29T09:00:00Z' })
  await control('mentor-role-restore')
  const context = await actor(browser, 'public', 'mentor'), page = await context.newPage()
  const note = randomUUID(), canary = `PRIVATE-MENTOR-${native.run_id}`
  const writes: string[] = []
  page.on('request', request => { if (request.method() === 'POST' && new URL(request.url()).pathname.startsWith('/api/')) writes.push(request.url()) })
  try {
    await page.goto(`/events/${native.event_id}/mentor/${native.team_id}?note=${note}&fresh=true`)
    await expect(page.getByRole('heading', { name: 'New private observation', exact: true })).toBeVisible()
    await context.setOffline(true)
    await expect(page.getByLabel('Connection status')).toContainText('Offline')
    await page.getByLabel('Category (private)', { exact: true }).fill('Technical rehearsal')
    await page.getByLabel('Evidence (private)', { exact: true }).fill(canary)
    await page.getByLabel('Outcome (private)', { exact: true }).fill('Synthetic unsent observation')
    await expect.poll(() => page.evaluate(value => Object.values(sessionStorage).some(raw => raw.includes(value)), canary)).toBe(true)
    expect(writes).toEqual([])
    await context.setOffline(false)
    await expect(page.getByLabel('Connection status')).toContainText('Connected to server')
    await page.reload()
    await expect(page.getByLabel('Evidence (private)', { exact: true })).toHaveValue('')
    await page.getByRole('button', { name: 'Check for local text', exact: true }).click()
    await expect(page.getByRole('region', { name: 'Recover unsent mentor text' })).toContainText('Current event access has been checked.')
    await page.getByRole('button', { name: 'Restore text to the editor', exact: true }).click()
    await expect(page.getByLabel('Evidence (private)', { exact: true })).toHaveValue(canary)
    expect(writes).toEqual([])
    await page.screenshot({ path: info.outputPath('native-current-authorized-mentor-recovery.png'), fullPage: true })
    await control('mentor-role-remove')
    const denied = page.waitForResponse(response => response.url().includes(`/events/${native.event_id}/`) && response.status() === 403)
    await page.evaluate(() => window.dispatchEvent(new Event('visibilitychange')))
    await denied
    await expect(page.getByLabel('Evidence (private)', { exact: true })).toHaveCount(0)
    expect(await page.evaluate(value => Object.values(sessionStorage).some(raw => raw.includes(value)), canary)).toBe(false)
    expect(await page.locator('body').textContent()).not.toContain(canary)
    expect(writes).toEqual([])
  } finally {
    await control('mentor-role-restore')
    await context.close()
  }
})

test('mentor draft stays scoped to its event and is erased by actual logout before another account enters', async ({ browser }) => {
  await control('clock', { instant: '2027-03-29T09:00:00Z' })
  await control('mentor-role-restore')
  const context = await actor(browser, 'public', 'mentor'), page = await context.newPage()
  page.on('dialog', dialog => { void dialog.accept() })
  const note = randomUUID(), canary = `PRIVATE-EVENT-ACCOUNT-${native.run_id}`
  const path = `/events/${native.event_id}/mentor/${native.team_id}?note=${note}&fresh=true`
  try {
    await page.goto(path)
    await page.getByLabel('Category (private)', { exact: true }).fill('Unsent event-specific note')
    await page.getByLabel('Evidence (private)', { exact: true }).fill(canary)
    await expect.poll(() => page.evaluate(value => Object.values(sessionStorage).some(raw => raw.includes(value)), canary)).toBe(true)
    const otherRead = page.waitForResponse(response => new URL(response.url()).pathname === `/api/v1/events/${native.other_event_id}` && response.status() === 403)
    await page.goto(`/events/${native.other_event_id}/mentor/${native.team_id}`)
    await otherRead
    await expect(page.getByLabel('Evidence (private)', { exact: true })).toHaveCount(0)
    expect(await page.locator('body').textContent()).not.toContain(canary)
    // Returning never restores a private editor automatically. The retained
    // event-scoped copy still requires a fresh native authorized read.
    await page.goto(path)
    await expect(page.getByLabel('Evidence (private)', { exact: true })).toHaveValue('')
    await page.getByRole('button', { name: 'Check for local text', exact: true }).click()
    await expect(page.getByRole('region', { name: 'Recover unsent mentor text' })).toContainText('Current event access has been checked.')
    await page.getByRole('button', { name: 'Restore text to the editor', exact: true }).click()
    await expect(page.getByLabel('Evidence (private)', { exact: true })).toHaveValue(canary)
    const logout = page.waitForResponse(response => new URL(response.url()).pathname === '/api/v1/logout' && response.status() === 204)
    await page.getByRole('button', { name: 'Synthetic person', exact: true }).click()
    await page.getByRole('menuitem', { name: 'Sign out', exact: true }).click()
    await logout
    await expect(page.getByRole('link', { name: 'Sign in with your organisation', exact: true })).toBeVisible()
    expect(await page.evaluate(value => Object.values(sessionStorage).some(raw => raw.includes(value)), canary)).toBe(false)
    // Synthetic session issuance uses native PostgreSQL-backed SCS, not a fake
    // session response or an authentication route added to the application.
    const next = await control<{ Name: string; Value: string }>('session', { actor: 'competitor', surface: 'public' })
    await context.addCookies([{ name: next.Name, value: next.Value, url: native.public, httpOnly: true, sameSite: 'Lax' }])
    await page.goto(`/events/${native.event_id}`)
    await expect(page.getByRole('navigation', { name: 'Event sections', exact: true })).toBeVisible()
    expect(await page.locator('body').textContent()).not.toContain(canary)
    expect(await page.evaluate(value => Object.values(sessionStorage).some(raw => raw.includes(value)), canary)).toBe(false)
  } finally { await context.close() }
})

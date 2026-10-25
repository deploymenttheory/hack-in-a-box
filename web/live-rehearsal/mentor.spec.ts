import { randomUUID } from 'node:crypto'
import { test, expect } from '@playwright/test'
import { actor, control, native } from './native'

test('mentor offline text is deliberately recovered only after native current authorization, then erased on role loss', async ({ browser }, info) => {
  await control('clock', { instant: '2027-03-29T09:00:00Z' })
  await control('mentor-role-restore')
  const context = await actor(browser, 'public', native.mentor_cookie), page = await context.newPage()
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

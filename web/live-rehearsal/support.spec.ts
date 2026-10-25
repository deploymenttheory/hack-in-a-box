import { test, expect } from '@playwright/test'
import { actor, control, native } from './native'

test('native help and corrected common answers preserve current account/event grants and never disclose the private source question', async ({ browser }, info) => {
  await control('clock', { instant: '2027-03-29T09:00:00Z' })
  const context = await actor(browser, 'public', 'competitor'), page = await context.newPage()
  const answerPath = `/api/v1/events/${native.event_id}/clarifications/${native.answer_id}`
  try {
    await page.goto(`/events/${native.event_id}/help/${native.help_id}`)
    await expect(page.getByRole('heading', { name: 'Team help request', exact: true })).toBeVisible()
    await expect(page.getByRole('region', { name: 'Private request context', exact: true })).toContainText('PRIVATE-HELP-DESCRIPTION')
    await page.goto(`/events/${native.event_id}/clarifications/${native.answer_id}`)
    await expect(page.getByRole('heading', { name: 'Rehearsal official answer', exact: true })).toBeVisible()
    await expect(page.getByText('Initial approved common answer.', { exact: true })).toBeVisible()
    expect(await page.locator('body').textContent()).not.toContain('PRIVATE-QUESTION-ORIGIN')
    await control('correct-clarification')
    const corrected = page.waitForResponse(response => new URL(response.url()).pathname === answerPath && response.status() === 200)
    await page.evaluate(() => window.dispatchEvent(new Event('visibilitychange')))
    const response = await corrected, document = await response.json()
    expect(document.answer.version).toBe(2)
    expect(document.answer.source_current).toBe(true)
    expect(JSON.stringify(document)).not.toContain(native.private_question_id)
    expect(JSON.stringify(document)).not.toContain('PRIVATE-QUESTION-ORIGIN')
    await expect(page.getByText('Corrected approved common answer applies to every current confirmed competitor.', { exact: true })).toBeVisible()
    await page.screenshot({ path: info.outputPath('native-current-corrected-common-answer.png'), fullPage: true })
    // A mentor grant alone does not confer confirmed-participant access to the
    // common clarification. Use the exact native 404 policy, not a UI role guess.
    const mentor = await control<{ Name: string; Value: string }>('session', { actor: 'mentor', surface: 'public' })
    await context.clearCookies()
    await context.addCookies([{ name: mentor.Name, value: mentor.Value, url: native.public, httpOnly: true, sameSite: 'Lax' }])
    const denied = page.waitForResponse(response => new URL(response.url()).pathname === answerPath && response.status() === 404)
    await page.evaluate(() => window.dispatchEvent(new Event('visibilitychange')))
    await denied
    await expect(page.getByRole('heading', { name: 'Rehearsal official answer', exact: true })).toHaveCount(0)
    expect(await page.locator('body').textContent()).not.toContain('Corrected approved common answer')
    const otherRead = page.waitForResponse(response => new URL(response.url()).pathname === `/api/v1/events/${native.other_event_id}/clarifications/${native.answer_id}` && response.status() === 404)
    await page.goto(`/events/${native.other_event_id}/clarifications/${native.answer_id}`)
    await otherRead
    await expect(page.getByRole('heading', { name: 'Rehearsal official answer', exact: true })).toHaveCount(0)
    expect(await page.locator('body').textContent()).not.toContain('Corrected approved common answer')
  } finally { await context.close() }
})

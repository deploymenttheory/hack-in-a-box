import { expect, test } from '@playwright/test'
import { surveyFixture, managePath, surveyPath, privateText, questionID, textID } from './survey-workspace-fixtures'

for (const cause of ['parent denial', 'session end', 'print'] as const) test(`held restricted CSV bytes cannot download after ${cause}`, async ({ page }) => {
  const f = await surveyFixture(page, true)
  await page.addInitScript(() => {
    const stats = { urls: 0, clicks: 0 }; Object.assign(window, { surveyExportStats: stats })
    URL.createObjectURL = () => { stats.urls++; return 'blob:synthetic-held-csv' }
    const original = HTMLAnchorElement.prototype.click
    HTMLAnchorElement.prototype.click = function () { if (this.download) { stats.clicks++; return }; original.call(this) }
  })
  let release!: () => void, started = false, settled = false
  const held = new Promise<void>(resolve => { release = resolve })
  await page.route('**/responses/export.csv', async route => { started = true; await held; await route.fulfill({ headers: { 'Content-Type': 'text/csv' }, body: privateText }).catch(() => {}); settled = true })
  await page.goto(managePath); await page.getByRole('button', { name: 'Restricted responses', exact: true }).click()
  await page.getByRole('button', { name: 'Review restricted CSV export' }).click(); await page.getByRole('button', { name: 'Download restricted responses CSV' }).click()
  await expect.poll(() => started).toBe(true)
  if (cause === 'parent denial') {
    // Hold the child list refresh: the management dashboard, above the child,
    // must fence the download even while the child's own access is unresolved.
    await page.route('**/surveys/*/responses', async route => { await held; await route.fallback().catch(() => {}) })
    f.state.staff = false; await page.evaluate(() => window.dispatchEvent(new Event('focus')))
    await expect(page.getByText('You do not have access to this resource.')).toBeVisible()
  } else await page.evaluate(event => window.dispatchEvent(new Event(event)), cause === 'session end' ? 'session-ending' : 'beforeprint')
  release(); await expect.poll(() => settled).toBe(true)
  await expect.poll(() => page.evaluate(() => (window as unknown as { surveyExportStats: { urls: number; clicks: number } }).surveyExportStats)).toEqual({ urls: 0, clicks: 0 })
  await expect(page.getByText(privateText, { exact: true })).toHaveCount(0)
  expect(await page.evaluate(() => JSON.stringify(localStorage) + JSON.stringify(sessionStorage))).not.toContain(privateText)
})

for (const status of [401, 403, 404]) test(`private retention/access ${status} clears own response and cannot restore a held older read`, async ({ page }) => {
  const f = await surveyFixture(page); f.state.response = { revision: 1, form_version: 1, answers: { [questionID]: 'yes', [textID]: privateText }, submitted_at: '2027-06-22T09:00:00Z' }
  await page.goto(surveyPath); await expect(page.getByText(privateText, { exact: true })).toBeVisible()
  let release!: () => void, reads = 0
  const held = new Promise<void>(resolve => { release = resolve }), original = { id: f.configuration.id, version: 1, form: f.configuration.definition.form, opens_at: f.configuration.definition.opens_at, closes_at: f.configuration.definition.closes_at, timezone: 'UTC', response_visibility: 'Private native response', measures: [], can_respond: true, response: f.state.response }
  await page.route(`**${surveyPath}`, async route => { reads++; if (reads === 1) { await held; await route.fulfill({ json: original }).catch(() => {}) } else await route.fulfill({ status, json: { error: { code: status === 401 ? 'unauthorized' : status === 404 ? 'not_found' : 'forbidden', message: 'Native private access unavailable.', request_id: 'synthetic' } } }) })
  await page.getByRole('button', { name: 'Refresh survey', exact: true }).click(); await expect.poll(() => reads).toBe(1)
  await page.getByRole('button', { name: 'Refresh survey', exact: true }).click(); await expect(page.getByLabel('Private explanation (optional)')).toHaveCount(0)
  release(); await expect(page.getByText(privateText, { exact: true })).toHaveCount(0); expect(f.puts).toHaveLength(0)
})

test('cohort editing keeps invalid input editable and never captures an invalid native definition', async ({ page }) => {
  const f = await surveyFixture(page, true); f.configuration.published_at = null; f.configuration.approved_at = null
  await page.goto(managePath); await page.getByRole('navigation', { name: 'Survey sections', exact: true }).getByRole('button', { name: 'Questions', exact: true }).click()
  await page.getByRole('button', { name: 'Measures and privacy', exact: true }).click()
  const cohort = page.getByLabel('Minimum report cohort (at least 5)')
  for (const value of ['', '4', '5.5', '101']) {
    await cohort.fill(value); await expect(cohort).toHaveAttribute('aria-invalid', 'true'); await expect(cohort).toHaveAccessibleDescription('Enter a whole-number report cohort from 5 to 100 people.')
    await expect(page.getByRole('button', { name: 'Review draft changes', exact: true })).toBeDisabled(); expect(f.configurationWrites).toHaveLength(0)
  }
  await cohort.fill('7'); await expect(cohort).not.toHaveAttribute('aria-invalid', 'true')
  await page.getByRole('button', { name: 'Review draft changes', exact: true }).click(); await expect(page.getByRole('alertdialog')).toContainText('7')
  expect(f.configurationWrites).toHaveLength(0); await page.getByRole('button', { name: 'Cancel', exact: true }).click(); await expect(cohort).toHaveValue('7')
})

test('lost report approval stays unknown when a current read already shows approved flags', async ({ page }) => {
  const f = await surveyFixture(page, true); await page.goto(managePath); await page.getByRole('button', { name: 'Reviewed report', exact: true }).click()
  await page.getByLabel('Redacted summary for wider release').fill('Reviewed synthetic common themes.')
  await page.getByLabel('I reviewed and redacted this wording', { exact: false }).check(); await page.getByRole('button', { name: 'Review frozen cohort and summary', exact: true }).click(); await page.getByRole('button', { name: 'Freeze cohort and save reviewed summary', exact: true }).click()
  await expect(page.getByRole('status', { name: 'Survey report status' })).toContainText('not yet published')
  f.state.dropReport = 1
  await page.getByRole('button', { name: 'Review exact summary approval', exact: true }).click(); await page.getByRole('button', { name: 'Approve exact reviewed summary', exact: true }).click()
  await expect(page.getByRole('status', { name: 'Survey report status' })).toContainText('outcome is unknown'); const original = structuredClone(f.reportWrites.at(-1))
  await page.getByRole('button', { name: 'Read current report facts', exact: true }).click()
  await expect(page.getByRole('status', { name: 'Survey report status' })).toContainText('do not confirm the original approve')
  await expect(page.getByRole('button', { name: 'Review report publication', exact: true })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Retry the same report text command', exact: true })).toHaveCount(0)
  expect(f.state.review?.approved_at).not.toBeNull(); expect(f.reportWrites.map(command => command.action)).toEqual(['review_text', 'approve']); expect(f.reportWrites.at(-1)).toEqual(original)
  const dialog = page.waitForEvent('dialog'), adopting = page.getByRole('button', { name: 'Load current report review', exact: true }).click(); await (await dialog).accept(); await adopting
  await expect(page.getByText('Earlier command outcomes remain unconfirmed.', { exact: false })).toBeVisible(); await expect(page.getByRole('button', { name: 'Review report publication', exact: true })).toBeEnabled()
  expect(f.reportWrites).toHaveLength(2)
})

for (const change of ['close', 'question version'] as const) test(`held original response cannot repopulate the disposed editor after ${change}`, async ({ page }) => {
  const f = await surveyFixture(page)
  let writeCount = 0
  let release!: () => void, attempted: { body: Record<string, unknown>; revision: string; csrf: string } | undefined, settled = false
  const held = new Promise<void>(resolve => { release = resolve })
  await page.route('**/surveys/*/response', async route => {
    writeCount++; const request = route.request(); attempted = { body: request.postDataJSON(), revision: request.headers()['if-match'], csrf: request.headers()['x-csrf-token'] }
    await held; await route.fulfill({ json: { revision: 1, form_version: 1, answers: attempted.body.answers, submitted_at: '2027-06-22T09:00:00Z' } }).catch(() => {}); settled = true
  })
  await page.goto(surveyPath); await page.getByLabel('Did you learn something? (required)').selectOption('yes'); await page.getByLabel('Private explanation (optional)').fill(privateText)
  await page.getByRole('button', { name: 'Review response', exact: true }).click(); await page.getByRole('button', { name: 'Save response', exact: true }).click(); await expect.poll(() => !!attempted).toBe(true)
  const captured = structuredClone(attempted)
  if (change === 'close') f.state.open = false
  else { f.configuration.version = 2; f.configuration.definition.form.fields[1].label = 'Current version explanation' }
  await page.evaluate(() => window.dispatchEvent(new Event('focus')))
  if (change === 'close') await expect(page.getByText('Response editing is currently closed or unavailable.')).toBeVisible()
  else await expect(page.getByLabel('Current version explanation (optional)')).toHaveCount(0) // Current version has an unanswered conditional question.
  await expect(page.getByLabel('Private explanation (optional)')).toHaveCount(0)
  release(); await expect.poll(() => settled).toBe(true)
  await expect(page.getByText(privateText, { exact: true })).toHaveCount(0)
  await expect(page.getByRole('heading', { name: 'Your saved response: revision 1', exact: false })).toHaveCount(0)
  expect(writeCount).toBe(1); expect(attempted).toEqual(captured); expect(captured?.revision).toBe('"0"'); expect(captured?.csrf).toBe('synthetic-csrf-token-32-characters'); expect(captured?.body.command_id).toMatch(/^[a-f0-9-]{36}$/)
  expect(f.puts).toHaveLength(0); expect(await page.evaluate(() => JSON.stringify(localStorage) + JSON.stringify(sessionStorage))).not.toContain(privateText)
})

test('native response field rejection restores invalid-field focus and preserves the exact editable answer', async ({ page }) => {
  const f = await surveyFixture(page); let attempts = 0
  await page.route('**/surveys/*/response', route => { attempts++; return route.fulfill({ status: 422, json: { error: { code: 'invalid', message: 'Review the answer.', request_id: 'synthetic-field', blockers: [{ field: textID, code: 'invalid', message: 'Explain the learning in your own words.' }] } } }) })
  await page.goto(surveyPath); await page.getByLabel('Did you learn something? (required)').selectOption('yes'); const answer = page.getByLabel('Private explanation (optional)'); await answer.fill(privateText)
  await page.getByRole('button', { name: 'Review response', exact: true }).click(); expect(attempts).toBe(0)
  await page.getByRole('button', { name: 'Save response', exact: true }).click()
  await expect(answer).toHaveValue(privateText); await expect(answer).toBeEnabled(); await expect(answer).toHaveAttribute('aria-invalid', 'true'); await expect(answer).toBeFocused()
  await expect(answer).toHaveAccessibleDescription('Only you and current organisers can read this response. Explain the learning in your own words.')
  expect(attempts).toBe(1); expect(f.puts).toHaveLength(0); expect(await page.evaluate(() => JSON.stringify(localStorage) + JSON.stringify(sessionStorage))).not.toContain(privateText)
})

test('native stale response revision preserves local draft until a deliberate current-base choice', async ({ page }) => {
  const f = await surveyFixture(page); let attempts = 0
  await page.route('**/surveys/*/response', route => { attempts++; f.state.response = { revision: 2, form_version: 1, answers: { [questionID]: 'yes', [textID]: 'A separately saved current answer.' }, submitted_at: '2027-06-22T10:00:00Z' }; return route.fulfill({ status: 409, json: { error: { code: 'conflict', request_id: 'synthetic-stale' } } }) })
  await page.goto(surveyPath); await page.getByLabel('Did you learn something? (required)').selectOption('yes'); const answer = page.getByLabel('Private explanation (optional)'); await answer.fill(privateText)
  await page.getByRole('button', { name: 'Review response', exact: true }).click(); await page.getByRole('button', { name: 'Save response', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Your saved response: revision 2', exact: false })).toBeVisible(); await expect(answer).toHaveValue(privateText)
  await expect(page.getByRole('button', { name: 'Review response', exact: true })).toBeDisabled(); await expect(page.getByRole('button', { name: 'Retry the same survey response', exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: 'Check saved response', exact: true }).click()
  const dialog = page.waitForEvent('dialog'), adopt = page.getByRole('button', { name: 'Load current saved response', exact: true }).click(); await (await dialog).accept(); await adopt
  await expect(answer).toHaveValue('A separately saved current answer.'); expect(attempts).toBe(1); expect(f.puts).toHaveLength(0)
})

test('print excludes the identifiable response owner while the separately published report remains readable', async ({ page }) => {
  const f = await surveyFixture(page); f.state.response = { revision: 1, form_version: 1, answers: { [questionID]: 'yes', [textID]: privateText }, submitted_at: '2027-06-22T09:00:00Z' }
  await page.goto(surveyPath); await expect(page.getByText(privateText, { exact: true })).toBeVisible()
  await page.emulateMedia({ media: 'print' }); await expect(page.getByText(privateText, { exact: true })).not.toBeVisible(); await expect(page.getByRole('button', { name: 'Review response', exact: true })).not.toBeVisible()
  await page.goto(surveyPath + '/results'); await expect(page.getByText('Explicitly reviewed common themes.', { exact: true })).toBeVisible(); await expect(page.getByText(privateText, { exact: true })).toHaveCount(0)
  expect(f.puts).toHaveLength(0)
})

import { openEventSection } from './event-navigation-fixture'
import { test, expect } from '@playwright/test'
import { mockWorkspace, firstID } from './fixtures'
import type { components } from '../src/api/schema.gen'

test('form editing validates branches, previews answers and separates save from publication', async ({ page }) => {
  await mockWorkspace(page)
  const parent = '00000000-0000-4000-8000-000000000001', child = '00000000-0000-4000-8000-000000000002'
  let form: components['schemas']['RegistrationForm'] = { revision: 0, version: 0, published_version: null, definition: { id: '00000000-0000-4000-8000-000000000003', name: 'Attendee form', audience: 'attendee', fields: [
    { id: parent, label: 'Have a team?', type: 'single', required: true, help: '', choices: ['yes', 'no'] },
    { id: child, label: 'Team preference', type: 'text', required: true, help: '', choices: [], visible_when: { field_id: parent, equals: 'yes' } },
  ] } }
  await page.route(`**/events/${firstID}/registration/forms**`, async route => {
    const path = new URL(route.request().url()).pathname
    if (path.endsWith('/preview')) { const body = route.request().postDataJSON(); const visible = body.answers[parent] === 'yes' ? [parent, child] : [parent]; const issues = visible.includes(child) && !body.answers[child] ? [{ field: child, code: 'required', message: 'Answer this required question.' }] : []; return route.fulfill({ json: { answers: body.answers, visible, issues } }) }
    if (path.endsWith('/publish')) { expect(route.request().postDataJSON().acknowledge_response_versions).toBe(true); form = { ...form, revision: form.revision + 1, published_version: form.version }; return route.fulfill({ json: form }) }
    if (route.request().method() === 'PUT') { form = { ...form, definition: route.request().postDataJSON(), revision: form.revision + 1, version: form.version + 1 }; return route.fulfill({ json: form }) }
    return route.fulfill({ json: { items: [form] } })
  })
  await page.goto(`/events/${firstID}`)
  await openEventSection(page, 'Forms')
  await page.getByRole('button', { name: 'Attendee form · Draft' }).click()
  await page.getByRole('button', { name: 'Edit Team preference' }).click()
  await expect(page.getByRole('heading', { name: 'Editing Team preference' })).toBeFocused()
  await page.getByRole('button', { name: 'Back to questions' }).click(); await expect(page.getByRole('button', { name: 'Edit Team preference' })).toBeFocused()
  await page.getByRole('button', { name: 'Edit Team preference' }).click()
  await page.getByLabel('Matching answer').fill('impossible')
  await expect(page.getByRole('alert')).toContainText('possible answer')
  await page.getByLabel('Matching answer').fill('yes')
  await page.getByLabel('Have a team? (required)').selectOption('yes')
  await page.getByRole('button', { name: 'Validate preview answers' }).click()
  await expect(page.getByLabel('Team preference (required)')).toHaveAttribute('aria-invalid', 'true')
  await expect(page.getByLabel('Team preference (required)')).toHaveAccessibleDescription('Answer this required question.')
  await expect(page.getByLabel('Team preference (required)')).toBeFocused()
  await page.getByLabel('Have a team? (required)').selectOption('no')
  await expect(page.getByLabel('Team preference (required)')).toHaveCount(0)
  await page.getByRole('button', { name: 'Save form draft' }).click()
  await expect(page.getByRole('status').filter({ hasText: /saved|published/ })).toContainText('Draft version 1 saved')
  await expect(page.getByRole('button', { name: 'Publish form version 1' })).toBeDisabled()
  await page.getByLabel('I acknowledge the effect on existing and new responses.').check()
  await page.getByRole('button', { name: 'Publish form version 1' }).click()
  await expect(page.getByRole('status').filter({ hasText: /saved|published/ })).toContainText('published for new responses')
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
})

test('unknown form save retains its draft and exact version context while read-only reconciliation cannot publish', async ({ page }) => {
  await mockWorkspace(page)
  let writes = 0, reads = 0
  const form: components['schemas']['RegistrationForm'] = { revision: 1, version: 1, published_version: null, definition: { id: '00000000-0000-4000-8000-000000000003', name: 'Synthetic draft', audience: 'attendee', fields: [] } }
  await page.route(`**/events/${firstID}/registration/forms**`, route => {
    if (route.request().method() === 'PUT') { writes++; expect(route.request().headers()['if-match']).toBe('"1"'); expect(route.request().postDataJSON().name).toBe('Retained private draft'); return route.fulfill({ status: 503, json: { error: { code: 'unavailable', request_id: 'synthetic' } } }) }
    reads++; return route.fulfill({ json: { items: [{ ...form, revision: reads > 1 ? 2 : 1 }] } })
  })
  await page.goto(`/events/${firstID}`); await page.getByRole('button', { name: 'Forms', exact: true }).click(); await page.getByRole('button', { name: 'Synthetic draft · Draft' }).click()
  await page.getByLabel('Form name', { exact: true }).fill('Retained private draft'); await page.getByRole('button', { name: 'Save form draft' }).click()
  await expect(page.getByRole('status').filter({ hasText: 'Outcome unknown' })).toContainText('Expected revision 1')
  await expect(page.getByRole('button', { name: 'Back to forms' })).toBeDisabled(); await expect(page.getByRole('button', { name: 'New registration form' })).toBeDisabled()
  await page.getByRole('button', { name: 'Check current status' }).click()
  await expect(page.getByLabel('Form name', { exact: true })).toHaveValue('Retained private draft')
  await expect(page.getByLabel('Form name', { exact: true })).toBeDisabled(); await expect(page.getByRole('button', { name: /Publish form version/ })).toBeDisabled()
  expect(writes).toBe(1); expect(reads).toBeGreaterThan(1)
  await expect(page.getByRole('status').filter({ hasText: 'Outcome unknown' })).toContainText('Expected revision 1')
})

test('an explicit loss of form access clears the mounted private draft', async ({ page }) => {
  await mockWorkspace(page)
  let denied = false
  const form = { revision: 1, version: 1, published_version: null, definition: { id: '00000000-0000-4000-8000-000000000003', name: 'Restricted draft', audience: 'attendee', fields: [] } }
  await page.route(`**/events/${firstID}/registration/forms**`, route => {
    if (route.request().method() === 'PUT') { denied = true; return route.fulfill({ status: 403, json: { error: { code: 'forbidden', request_id: 'synthetic' } } }) }
    if (denied) return route.fulfill({ status: 403, json: { error: { code: 'forbidden', request_id: 'synthetic' } } })
    return route.fulfill({ json: { items: [form] } })
  })
  await page.goto(`/events/${firstID}`); await page.getByRole('button', { name: 'Forms', exact: true }).click(); await page.getByRole('button', { name: 'Restricted draft · Draft' }).click()
  await page.getByLabel('Form name', { exact: true }).fill('Private unsent wording'); await page.getByRole('button', { name: 'Save form draft' }).click()
  await expect(page.getByRole('alert')).toContainText('do not have access')
  await expect(page.getByLabel('Form name', { exact: true })).toHaveCount(0)
  await expect(page.getByText('Private unsent wording')).toHaveCount(0)
})

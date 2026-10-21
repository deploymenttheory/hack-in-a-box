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
  await page.getByRole('button', { name: 'Forms', exact: true }).click()
  await page.getByRole('button', { name: 'Attendee form · Draft' }).click()
  await page.getByLabel('Matching answer').fill('impossible')
  await expect(page.getByRole('alert')).toContainText('possible answer')
  await page.getByLabel('Matching answer').fill('yes')
  await page.getByLabel('Have a team? (required)').selectOption('yes')
  await page.getByRole('button', { name: 'Validate preview answers' }).click()
  await expect(page.getByRole('alert')).toContainText('required question')
  await page.getByLabel('Have a team? (required)').selectOption('no')
  await expect(page.getByLabel('Team preference (required)')).toHaveCount(0)
  await page.getByRole('button', { name: 'Save form draft' }).click()
  await expect(page.getByRole('status')).toContainText('Draft version 1 saved')
  await expect(page.getByRole('button', { name: 'Publish form version 1' })).toBeDisabled()
  await page.getByLabel('I acknowledge the effect on existing and new responses.').check()
  await page.getByRole('button', { name: 'Publish form version 1' }).click()
  await expect(page.getByRole('status')).toContainText('published for new responses')
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
})

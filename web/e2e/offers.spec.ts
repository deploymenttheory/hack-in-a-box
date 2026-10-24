import { test, expect } from '@playwright/test'
import { firstID } from './fixtures'
import { registrationEvent, fixtureRegistration, attendeeForm, questionID, mockApplicantWorkspace } from './registration-fixtures'
import { defaultBranding } from '../src/branding'

for (const expired of [false, true]) test(`offer response shows ${expired ? 'expiry' : 'acceptance and withdrawal'}`, async ({ page }) => {
  await mockApplicantWorkspace(page)
  let registration = { ...fixtureRegistration('offered'), offer: { id: '00000000-0000-4000-8000-000000000006', expires_at: '2027-06-12T09:00:00Z', state: 'pending' } }
  let responses = 0
  await page.route('**/api/v1/registration/events/**', route => route.fulfill({ json: { event: registrationEvent, branding: defaultBranding, forms: [], registrations: [registration] } }))
  await page.route(`**/registrations/${registration.id}/respond`, route => {
    responses++; const action = route.request().postDataJSON().action
    registration = { ...registration, state: expired ? 'declined' : action === 'withdraw' ? 'withdrawn' : 'confirmed', revision: registration.revision + 1, offer: { ...registration.offer, state: expired ? 'expired' : 'accepted' } }
    return route.fulfill({ json: registration })
  })
  await page.goto(`/register/${firstID}`)
  await expect(page.getByText(/Respond by/)).toContainText('Europe/London')
  await page.getByRole('button', { name: 'Accept place' }).click(); await page.getByRole('button', { name: 'Confirm acceptance' }).click()
  await expect(page.getByRole('button', { name: 'Accept place' })).toHaveCount(0)
  if (expired) await expect(page.getByText('This offer has expired. Your latest status is shown above.')).toBeVisible()
  else {
    await expect(page.getByText('Your competitor place is confirmed.')).toBeVisible()
    await page.getByRole('button', { name: 'Withdraw application' }).click(); await page.getByRole('button', { name: 'Confirm withdrawal' }).click()
    await expect(page.getByText('This application has been withdrawn.')).toBeVisible()
  }
  expect(responses).toBe(expired ? 1 : 2)
})

test('applicant progresses from waitlist to offer, corrects answers and withdraws', async ({ page }) => {
  await mockApplicantWorkspace(page)
  let registration: ReturnType<typeof fixtureRegistration> | undefined
  let answer = 'Teamwork'
  await page.route('**/api/v1/registration/events/**', route => route.fulfill({ json: { event: registrationEvent, branding: defaultBranding, forms: [attendeeForm], registrations: registration ? [registration] : [] } }))
  await page.route(`**/events/${firstID}/registrations**`, route => {
    const path = new URL(route.request().url()).pathname
    if (path.endsWith('/respond')) { registration = { ...registration!, state: route.request().postDataJSON().action === 'withdraw' ? 'withdrawn' : 'confirmed', revision: registration!.revision + 1, offer: { ...registration!.offer!, state: 'accepted' } }; return route.fulfill({ json: registration }) }
    if (path.endsWith('/response')) {
      if (route.request().method() === 'PUT') { answer = route.request().postDataJSON()[questionID]; registration = { ...registration!, revision: registration!.revision + 1 } }
      return route.fulfill({ json: { definition: attendeeForm.definition, revision: registration!.revision, answers: { [questionID]: answer } } })
    }
    registration = { ...fixtureRegistration('waitlisted'), id: route.request().postDataJSON().id }
    return route.fulfill({ json: registration })
  })
  await page.goto(`/register/${firstID}`)
  await page.getByLabel('What would you like to learn? (required)').fill(answer)
  if (await page.getByRole('button', { name: 'Review application', exact: true }).isVisible()) await page.getByRole('button', { name: 'Review application', exact: true }).click(); await page.getByRole('button', { name: 'Submit application' }).click()
  await expect(page.getByText('Waitlist position: 2')).toBeVisible()
  registration = { ...registration!, state: 'offered', waitlist_position: null, revision: 2, offer: { id: '00000000-0000-4000-8000-000000000007', expires_at: '2027-06-12T09:00:00Z', state: 'pending' } }
  await page.reload(); await page.getByRole('button', { name: 'Accept place' }).click(); await page.getByRole('button', { name: 'Confirm acceptance' }).click()
  await expect(page.getByText('Your competitor place is confirmed.')).toBeVisible()
  await page.getByRole('button', { name: 'Review or correct answers' }).click()
  await page.getByLabel('What would you like to learn? (required)').fill('Accessible collaboration')
  await page.getByLabel('What would you like to learn? (required)').press('Enter'); expect(answer).toBe('Teamwork')
  await page.getByRole('button', { name: 'Review corrected answers', exact: true }).click()
  await page.getByRole('button', { name: 'Save corrected answers' }).click()
  await expect.poll(() => answer).toBe('Accessible collaboration')
  await expect(page.getByText(/Corrected answers saved at server revision/)).toBeVisible()
  await page.getByRole('button', { name: 'Withdraw application' }).click(); await page.getByRole('button', { name: 'Confirm withdrawal' }).click()
  await expect(page.getByText('This application has been withdrawn.')).toBeVisible()
})

test('post-freeze withdrawal explains the smaller team without promising reassignment', async ({ page }) => {
  await mockApplicantWorkspace(page)
  let registration = fixtureRegistration('confirmed')
  await page.route('**/api/v1/registration/events/**', route => route.fulfill({ json: { event: { ...registrationEvent, team_frozen: true }, branding: defaultBranding, forms: [], registrations: [registration] } }))
  await page.route(`**/registrations/${registration.id}/respond`, route => {
    expect(route.request().postDataJSON().action).toBe('withdraw')
    registration = { ...registration, state: 'withdrawn', revision: registration.revision + 1 }
    return route.fulfill({ json: registration })
  })
  await page.goto(`/register/${firstID}`)
  await expect(page.getByText('Teams are frozen.', { exact: false })).toContainText('The remaining team may continue or withdraw.')
  await page.getByRole('button', { name: 'Withdraw application' }).click(); await expect(page.getByRole('alertdialog')).toContainText('Reassignment and backfill are unavailable after team freeze.'); await page.getByRole('button', { name: 'Confirm withdrawal' }).click()
  await expect(page.getByText('This application has been withdrawn.')).toBeVisible()
})

test('a lost offer response retains the original intent and current state checks never replay acceptance', async ({ page }) => {
  await mockApplicantWorkspace(page)
  let registration = { ...fixtureRegistration('offered'), offer: { id: '00000000-0000-4000-8000-000000000008', expires_at: '2027-06-12T09:00:00Z', state: 'pending' } }, writes = 0
  await page.route('**/api/v1/registration/events/**', route => route.fulfill({ json: { event: registrationEvent, branding: defaultBranding, forms: [], registrations: [registration] } }))
  await page.route(`**/registrations/${registration.id}/respond`, route => {
    writes++; expect(route.request().headers()['if-match']).toBe('"1"'); expect(route.request().headers()['x-csrf-token']).toBe('synthetic-csrf-token-32-characters'); expect(route.request().postDataJSON()).toEqual({ action: 'accept' })
    registration = { ...registration, revision: 2, state: 'confirmed', offer: { ...registration.offer, state: 'accepted' } }
    return route.fulfill({ status: 503, json: { error: { code: 'unavailable', request_id: 'synthetic' } } })
  })
  await page.goto(`/register/${firstID}`)
  await page.getByRole('button', { name: 'Accept place' }).click()
  await page.getByRole('button', { name: 'Confirm acceptance' }).click()
  const dialog = page.getByRole('alertdialog')
  await expect(dialog).toContainText('Outcome unknown')
  await expect(dialog).toContainText('Expected revision 1')
  await expect(dialog.getByRole('button', { name: 'Confirm acceptance' })).toBeDisabled()
  await dialog.getByRole('button', { name: 'Check current status' }).click()
  await expect(page.getByText('Your competitor place is confirmed.')).toBeVisible()
  await expect(page.getByRole('status').filter({ hasText: 'Outcome unknown' })).toContainText('does not prove the outcome of this exact request')
  await expect(page.getByRole('button', { name: 'Withdraw application' })).toBeDisabled()
  await page.getByRole('button', { name: 'Check current status' }).click()
  expect(writes).toBe(1)
  await expect(page.getByText(/server returned application revision/)).toHaveCount(0)
  await expect(page.getByRole('complementary', { name: 'Unconfirmed command history' })).toContainText('Expected revision 1')
  page.once('dialog', dialog => { expect(dialog.message()).toContain('outcomes still unconfirmed'); void dialog.dismiss() })
  await page.getByRole('link', { name: 'Find events', exact: true }).click(); await expect(page).toHaveURL(new RegExp('/register/' + firstID + '$'))
  await expect(page.getByRole('button', { name: 'Withdraw application' })).toBeDisabled(); expect(writes).toBe(1)
  page.once('dialog', dialog => { expect(dialog.message()).toContain('outcomes still unconfirmed'); void dialog.accept() })
  await page.getByRole('link', { name: 'Find events', exact: true }).click(); await expect(page).toHaveURL(/\/register$/)
  await expect(page.getByRole('complementary', { name: 'Unconfirmed command history' })).toHaveCount(0); expect(writes).toBe(1)
})

test('a stale acceptance reads the latest revision before a separately reviewed request', async ({ page }) => {
  await mockApplicantWorkspace(page)
  let registration = { ...fixtureRegistration('offered'), offer: { id: '00000000-0000-4000-8000-000000000009', expires_at: '2027-06-12T09:00:00Z', state: 'pending' } }, writes = 0
  await page.route('**/api/v1/registration/events/**', route => route.fulfill({ json: { event: registrationEvent, branding: defaultBranding, forms: [], registrations: [registration] } }))
  await page.route(`**/registrations/${registration.id}/respond`, route => {
    writes++
    if (writes === 1) { expect(route.request().headers()['if-match']).toBe('"1"'); registration = { ...registration, revision: 2 }; return route.fulfill({ status: 409, json: { error: { code: 'revision_conflict', request_id: 'synthetic' } } }) }
    expect(route.request().headers()['if-match']).toBe('"2"'); registration = { ...registration, revision: 3, state: 'confirmed', offer: { ...registration.offer, state: 'accepted' } }; return route.fulfill({ json: registration })
  })
  await page.goto(`/register/${firstID}`)
  await page.getByRole('button', { name: 'Accept place' }).click(); await page.getByRole('button', { name: 'Confirm acceptance' }).click()
  await expect(page.getByRole('alertdialog')).toContainText('Current version needed')
  await page.getByRole('alertdialog').getByRole('button', { name: 'Check current status' }).click()
  expect(writes).toBe(1)
  await page.getByRole('button', { name: 'Accept place' }).click(); await expect(page.getByRole('alertdialog')).toContainText('2'); await page.getByRole('button', { name: 'Confirm acceptance' }).click()
  await expect(page.getByText('Your competitor place is confirmed.')).toBeVisible(); expect(writes).toBe(2)
})

test('pending and unknown corrections fence decline as well as every other admission action', async ({ page }) => {
  await mockApplicantWorkspace(page)
  const registration = { ...fixtureRegistration('offered'), offer: { id: '00000000-0000-4000-8000-000000000010', expires_at: '2027-06-12T09:00:00Z', state: 'pending' } }
  let release!: () => void, corrections = 0, responses = 0
  const gate = new Promise<void>(resolve => { release = resolve })
  await page.route('**/api/v1/registration/events/**', route => route.fulfill({ json: { event: registrationEvent, branding: defaultBranding, forms: [], registrations: [registration] } }))
  await page.route(`**/registrations/${registration.id}/respond`, route => { responses++; return route.fulfill({ json: registration }) })
  await page.route(`**/registrations/${registration.id}/response`, async route => {
    if (route.request().method() === 'PUT') { corrections++; expect(route.request().headers()['if-match']).toBe('"1"'); expect(route.request().postDataJSON()).toEqual({ [questionID]: 'Retained correction' }); await gate; return route.fulfill({ status: 503, json: { error: { code: 'unavailable', request_id: 'synthetic' } } }) }
    return route.fulfill({ json: { definition: attendeeForm.definition, revision: 1, answers: { [questionID]: 'Original answer' } } })
  })
  await page.goto(`/register/${firstID}`); await page.getByRole('button', { name: 'Review or correct answers' }).click(); await page.getByLabel('What would you like to learn? (required)').fill('Retained correction'); await page.getByRole('button', { name: 'Review corrected answers', exact: true }).click(); await page.getByRole('button', { name: 'Save corrected answers' }).click()
  try {
    await expect.poll(() => corrections).toBe(1)
    for (const name of ['Accept place', 'Decline offer', 'Withdraw application', 'Close answers']) await expect(page.getByRole('button', { name, exact: true })).toBeDisabled()
    expect(responses).toBe(0)
  } finally { release() }
  await expect(page.getByRole('status').filter({ hasText: 'Outcome unknown' })).toBeVisible()
  for (const name of ['Accept place', 'Decline offer', 'Withdraw application', 'Close answers']) await expect(page.getByRole('button', { name, exact: true })).toBeDisabled()
  await page.getByRole('button', { name: 'Check current status' }).click(); expect(corrections).toBe(1); expect(responses).toBe(0)
  await expect(page.getByRole('button', { name: 'Decline offer', exact: true })).toBeDisabled()
})

test('editable corrected answers and their review require deliberate discard before close or withdrawal', async ({ page }) => {
  await mockApplicantWorkspace(page)
  let registration = fixtureRegistration('confirmed'), responses = 0
  await page.route('**/api/v1/registration/events/**', route => route.fulfill({ json: { event: registrationEvent, branding: defaultBranding, forms: [], registrations: [registration] } }))
  await page.route(`**/registrations/${registration.id}/response`, route => route.fulfill({ json: { definition: attendeeForm.definition, revision: 1, answers: { [questionID]: 'Original answer' } } }))
  await page.route(`**/registrations/${registration.id}/respond`, route => { responses++; expect(route.request().postDataJSON()).toEqual({ action: 'withdraw' }); registration = { ...registration, state: 'withdrawn', revision: 2 }; return route.fulfill({ json: registration }) })
  await page.goto(`/register/${firstID}`); await page.getByRole('button', { name: 'Review or correct answers' }).click(); await page.getByLabel('What would you like to learn? (required)').fill('Private unsaved correction')
  await expect(page.getByRole('button', { name: 'Close answers' })).toBeEnabled()
  page.once('dialog', dialog => { expect(dialog.message()).toContain('unsaved corrected answers'); void dialog.dismiss() }); await page.getByRole('button', { name: 'Close answers' }).click()
  await expect(page.getByLabel('What would you like to learn? (required)')).toHaveValue('Private unsaved correction')
  await page.getByRole('button', { name: 'Review corrected answers', exact: true }).click()
  page.once('dialog', dialog => void dialog.dismiss()); await page.getByRole('button', { name: 'Close answers' }).click(); await expect(page.getByRole('button', { name: 'Save corrected answers' })).toBeVisible()
  page.once('dialog', dialog => void dialog.dismiss()); await page.getByRole('button', { name: 'Withdraw application' }).click(); await expect(page.getByRole('alertdialog')).toHaveCount(0); expect(responses).toBe(0)
  await page.getByRole('button', { name: 'Correct details' }).click(); await expect(page.getByLabel('What would you like to learn? (required)')).toBeEnabled()
  page.once('dialog', dialog => void dialog.accept()); await page.getByRole('button', { name: 'Close answers' }).click(); await expect(page.getByLabel('What would you like to learn? (required)')).toHaveCount(0)
  await page.getByRole('button', { name: 'Review or correct answers' }).click(); await expect(page.getByLabel('What would you like to learn? (required)')).toHaveValue('Original answer'); await page.getByLabel('What would you like to learn? (required)').fill('Another unsaved correction')
  page.once('dialog', dialog => void dialog.accept()); await page.getByRole('button', { name: 'Withdraw application' }).click(); await expect(page.getByRole('alertdialog')).toBeVisible(); await expect(page.getByLabel('What would you like to learn? (required)')).toHaveCount(0); await page.getByRole('button', { name: 'Confirm withdrawal' }).click()
  await expect(page.getByText('This application has been withdrawn.')).toBeVisible(); expect(responses).toBe(1)
})

for (const denial of ['write', 'read'] as const) test(`explicit correction ${denial} denial unmounts private answers and review`, async ({ page }) => {
  await mockApplicantWorkspace(page)
  const registration = fixtureRegistration('confirmed'); let writes = 0
  await page.route('**/api/v1/registration/events/**', route => route.fulfill({ json: { event: registrationEvent, branding: defaultBranding, forms: [], registrations: [registration] } }))
  await page.route(`**/registrations/${registration.id}/response`, route => {
    if (route.request().method() === 'PUT') { writes++; return route.fulfill({ status: denial === 'write' ? 403 : 503, json: { error: { code: denial === 'write' ? 'forbidden' : 'unavailable', request_id: 'synthetic' } } }) }
    if (writes) return route.fulfill({ status: 403, json: { error: { code: 'forbidden', request_id: 'synthetic' } } })
    return route.fulfill({ json: { definition: attendeeForm.definition, revision: 1, answers: { [questionID]: 'Original private answer' } } })
  })
  await page.goto(`/register/${firstID}`); await page.getByRole('button', { name: 'Review or correct answers' }).click(); await page.getByLabel('What would you like to learn? (required)').fill('Private denied correction'); await page.getByRole('button', { name: 'Review corrected answers', exact: true }).click(); await page.getByRole('button', { name: 'Save corrected answers' }).click()
  if (denial === 'read') { await expect(page.getByRole('status').filter({ hasText: 'Outcome unknown' })).toBeVisible(); await page.getByRole('button', { name: 'Check current status' }).click() }
  await expect(page.getByRole('alert')).toContainText('do not have access'); await expect(page.getByLabel('What would you like to learn? (required)')).toHaveCount(0); await expect(page.getByText('Private denied correction', { exact: true })).toHaveCount(0); await expect(page.getByRole('button', { name: 'Save corrected answers' })).toHaveCount(0); expect(writes).toBe(1)
})

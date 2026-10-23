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

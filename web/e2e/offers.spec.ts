import { test, expect } from '@playwright/test'
import { mockWorkspace, firstID } from './fixtures'
import { registrationEvent, fixtureRegistration, attendeeForm, questionID } from './registration-fixtures'
import { defaultBranding } from '../src/branding'

for (const expired of [false, true]) test(`offer response shows ${expired ? 'expiry' : 'acceptance and withdrawal'}`, async ({ page }) => {
  await mockWorkspace(page)
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
  await page.getByRole('button', { name: 'Accept place' }).click()
  await expect(page.getByRole('button', { name: 'Accept place' })).toHaveCount(0)
  if (expired) await expect(page.getByText('This offer has expired. Your latest status is shown above.')).toBeVisible()
  else {
    await expect(page.getByText('Your competitor place is confirmed.')).toBeVisible()
    page.once('dialog', dialog => { expect(dialog.message()).toContain('below three'); void dialog.accept() })
    await page.getByRole('button', { name: 'Withdraw application' }).click()
    await expect(page.getByText('This application has been withdrawn.')).toBeVisible()
  }
  expect(responses).toBe(expired ? 1 : 2)
})

test('applicant progresses from waitlist to offer, corrects answers and withdraws', async ({ page }) => {
  await mockWorkspace(page)
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
  await page.getByRole('button', { name: 'Submit application' }).click()
  await expect(page.getByText('Waitlist position: 2')).toBeVisible()
  registration = { ...registration!, state: 'offered', waitlist_position: null, revision: 2, offer: { id: '00000000-0000-4000-8000-000000000007', expires_at: '2027-06-12T09:00:00Z', state: 'pending' } }
  await page.reload(); await page.getByRole('button', { name: 'Accept place' }).click()
  await expect(page.getByText('Your competitor place is confirmed.')).toBeVisible()
  await page.getByRole('button', { name: 'Review or correct answers' }).click()
  await page.getByLabel('What would you like to learn? (required)').fill('Accessible collaboration')
  await page.getByRole('button', { name: 'Save corrected answers' }).click()
  await expect.poll(() => answer).toBe('Accessible collaboration')
  await expect(page.getByRole('button', { name: 'Save corrected answers' })).toBeEnabled()
  page.once('dialog', dialog => void dialog.accept())
  await page.getByRole('button', { name: 'Withdraw application' }).click()
  await expect(page.getByText('This application has been withdrawn.')).toBeVisible()
})

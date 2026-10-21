import { test, expect } from '@playwright/test'
import { mockWorkspace, firstID } from './fixtures'
import { registrationEvent, fixtureRegistration } from './registration-fixtures'
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

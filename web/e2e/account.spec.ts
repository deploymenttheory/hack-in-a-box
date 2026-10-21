import { test, expect } from '@playwright/test'
import { mockWorkspace } from './fixtures'
test.beforeEach(async ({ page }) => { await mockWorkspace(page) })
const person = { id: '1ca7d59b-3160-48b7-99b7-aad2cb648d64', display_name: 'Synthetic Person', platform_admin: false }
const eventPath = '/events/8b4917c1-f27a-40c8-838b-a7fd047c4313'
test('sign-in and expired sessions return to the current event', async ({ page }) => {
  await page.route('**/api/v1/session', route => route.fulfill({ status: 401, json: { error: { code: 'unauthenticated', message: 'Sign in', request_id: 'test' } } }))
  await page.goto(eventPath)
  await expect(page.getByRole('link', { name: /Sign in with your organisation/ })).toHaveAttribute('href', `/auth/login?return_to=${encodeURIComponent(eventPath)}`)
})
test('logout clears private browser drafts and account data', async ({ page }) => {
  let loggedOut = false
  await page.route('**/api/v1/session', route => route.fulfill(loggedOut ? { status: 401, json: { error: { code: 'unauthenticated', message: 'Sign in', request_id: 'test' } } } : { json: { person, csrf_token: 'synthetic-csrf-token-32-characters' } }))
  await page.route('**/api/v1/logout', route => { loggedOut = true; return route.fulfill({ status: 204 }) })
  await page.goto('/')
  await expect(page.getByRole('button', { name: person.display_name })).toBeVisible()
  await page.evaluate((id) => { sessionStorage.setItem(`hackbox:draft:${id}:event`, 'private'); localStorage.setItem(`hackbox:draft:${id}:old`, 'private') }, person.id)
  await page.getByRole('button', { name: person.display_name }).click()
  await page.getByRole('menuitem', { name: 'Sign out' }).click()
  await expect(page.getByRole('link', { name: /Sign in with your organisation/ })).toBeVisible()
  expect(await page.evaluate(() => Object.keys(sessionStorage).concat(Object.keys(localStorage)).filter(k => k.startsWith('hackbox:draft:')))).toEqual([])
  await expect(page.getByText(person.display_name)).toHaveCount(0)
})
test('switching accounts removes another account’s saved drafts', async ({ page }) => {
  await page.route('**/api/v1/session', route => route.fulfill({ json: { person, csrf_token: 'synthetic-csrf-token-32-characters' } }))
  await page.addInitScript((id) => { sessionStorage.setItem('hackbox:draft:another:event', 'private'); sessionStorage.setItem(`hackbox:draft:${id}:event`, 'safe configuration') }, person.id)
  await page.goto('/')
  await expect(page.getByRole('button', { name: person.display_name })).toBeVisible()
  expect(await page.evaluate(() => sessionStorage.getItem('hackbox:draft:another:event'))).toBeNull()
  expect(await page.evaluate((id) => sessionStorage.getItem(`hackbox:draft:${id}:event`), person.id)).toBe('safe configuration')
})

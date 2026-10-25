import { expect, test } from '@playwright/test'
import { firstID, fixtureEvent, mockWorkspace, person } from './fixtures'

test('shell collection, permitted destination search and current event context are keyboard usable', async ({ page }, info) => {
  await mockWorkspace(page)
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Your events', exact: true })).toBeVisible()
  await expect(page.getByRole('navigation', { name: 'Main navigation' }).getByRole('link', { name: 'Workspace', exact: true })).toHaveAttribute('aria-current', 'page')
  const target = page.getByRole('button', { name: 'Find a destination' })
  const box = (await target.boundingBox())!, minimum = await page.evaluate(() => matchMedia('(pointer: coarse)').matches ? 44 : 36)
  expect(box.width).toBeGreaterThanOrEqual(minimum); expect(box.height).toBeGreaterThanOrEqual(minimum)
  const inbox = page.getByRole('navigation', { name: 'Main navigation' }).getByRole('link', { name: /^Inbox/ })
  const inboxBox = (await inbox.boundingBox())!
  expect(inboxBox.width).toBeGreaterThanOrEqual(minimum); expect(inboxBox.height).toBeGreaterThanOrEqual(minimum)
  await expect(inbox.locator('svg')).toHaveCount(1)
  await target.focus(); await page.keyboard.press('Enter')
  const search = page.getByRole('searchbox', { name: 'Search permitted destinations' })
  await search.fill('no such destination'); await expect(page.getByText('No matching destinations. Try another name.')).toBeVisible()
  await search.fill('Account'); await expect(page.getByRole('link', { name: 'Account settings', exact: true })).toBeVisible()
  await page.keyboard.press('Escape')
  await page.getByRole('link', { name: 'North Star Hack', exact: true }).click()
  await expect(page.locator('.fleet-shell-context')).toContainText('North Star Hack')
  await expect(page.locator('.fleet-shell-context')).toContainText('Event editor access')
  await expect(page.getByRole('button', { name: 'Switch event · North Star Hack' })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.getByRole('button', { name: 'Switch event · North Star Hack' }).click()
  await page.getByRole('dialog', { name: 'Switch event', exact: true }).getByRole('link', { name: 'Southern Lights Hack' }).click()
  await expect(page.locator('.fleet-shell-context')).toContainText('Southern Lights Hack')
  await page.goto('/')
  await page.screenshot({ path: info.outputPath('portal-directory.png'), fullPage: true })
})

test('host does not invent event authority and revoked current reads withdraw contextual destinations', async ({ page }) => {
  await mockWorkspace(page)
  await page.route('**/api/v1/session', route => route.fulfill({ json: { person: { ...person, platform_admin: false }, csrf_token: 'synthetic-csrf-token-32-characters', draft_scope: '22222222-2222-4222-8222-222222222222', can_create_events: false } }))
  let revoked = false, reads = 0
  let release!: () => void
  const restoration = new Promise<void>(resolve => { release = resolve })
  let holdRestoration = false
  const event = { ...fixtureEvent(), can_read_staff: false, can_read_review_coverage: false, can_edit: false, can_present: false, can_transition: false, can_manage_owner: false }
  await page.route(`**/api/v1/events/${firstID}`, async route => { reads++; if (revoked) return route.fulfill({ status: 403, json: { error: { code: 'forbidden', message: 'Synthetic', request_id: 'synthetic' } } }); if (holdRestoration) await restoration; return route.fulfill({ json: event }) })
  await page.goto(`/events/${firstID}`)
  await expect(page.locator('.fleet-shell-context')).toContainText('Event access')
  await page.getByRole('button', { name: 'Find a destination' }).click()
  const search = page.getByRole('searchbox', { name: 'Search permitted destinations' })
  await search.fill('Support and assessment'); await expect(page.getByText('No matching destinations. Try another name.')).toBeVisible()
  await search.fill('Create event'); await expect(page.getByText('No matching destinations. Try another name.')).toBeVisible()
  await page.keyboard.press('Escape')
  revoked = true
  await page.evaluate(() => window.dispatchEvent(new Event('visibilitychange')))
  await expect(page.locator('.fleet-shell-context')).toContainText('Event access unavailable')
  await expect(page.getByRole('navigation', { name: 'Event sections', exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Switch event · North Star Hack', exact: true })).toHaveCount(0)
  await expect(page.getByRole('heading', { name: 'North Star Hack', exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: 'Find a destination' }).click()
  await search.fill('North Star'); await expect(page.getByText('No matching destinations. Try another name.')).toBeVisible()
  await search.fill('Surveys and feedback'); await expect(page.getByText('No matching destinations. Try another name.')).toBeVisible()
  await page.keyboard.press('Escape')
  revoked = false; holdRestoration = true
  const before = reads
  await page.getByRole('button', { name: 'Try again', exact: true }).click()
  await expect.poll(() => reads).toBeGreaterThan(before)
  await expect(page.locator('.fleet-shell-context')).toContainText('Event access unavailable')
  await expect(page.getByRole('navigation', { name: 'Event sections', exact: true })).toHaveCount(0)
  release()
  await expect(page.locator('.fleet-shell-context')).toContainText('North Star Hack')
  await expect(page.getByRole('navigation', { name: 'Event sections', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Find a destination' }).click()
  await search.fill('North Star')
  await expect(page.getByRole('link', { name: 'North Star Hack workspace', exact: true })).toBeVisible()
})

test('bookmarked unauthenticated destination keeps a safe same-origin login return and expiry recovery', async ({ page }) => {
  await mockWorkspace(page)
  await page.route('**/api/v1/session', route => route.fulfill({ status: 401, json: { error: { code: 'unauthorized', message: 'Synthetic', request_id: 'synthetic' } } }))
  await page.goto(`/events/${firstID}`)
  const signin = page.getByRole('link', { name: 'Sign in with your organisation' })
  await expect(signin).toHaveAttribute('href', `/auth/login?return_to=${encodeURIComponent(`/events/${firstID}`)}`)
  await expect(page.getByRole('navigation', { name: 'Main navigation' })).toHaveCount(0)
  await page.evaluate(() => window.dispatchEvent(new Event('session-expired')))
  await expect(page.getByRole('heading', { name: 'Sign in to continue.' })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})

test('directory exposes loading, authoritative empty and definite retry states without inventing records', async ({ page }) => {
  await mockWorkspace(page)
  let release: () => void = () => {}, failed = false
  const pending = new Promise<void>(resolve => { release = resolve })
  await page.route('**/api/v1/events?**', async route => {
    await pending
    if (failed) return route.fulfill({ status: 503, json: { error: { code: 'unavailable', message: 'Synthetic', request_id: 'synthetic' } } })
    return route.fulfill({ json: { items: [], next_cursor: null } })
  })
  await page.goto('/'); await expect(page.getByText('Loading records…')).toBeVisible()
  release(); await expect(page.getByRole('heading', { name: 'Nothing here yet' })).toBeVisible()
  await expect(page.getByRole('link', { name: 'North Star Hack', exact: true })).toHaveCount(0)
  failed = true; await page.reload(); await expect(page.getByRole('alert')).toContainText('Your events could not be loaded.')
  failed = false; await page.getByRole('button', { name: 'Try again', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Nothing here yet' })).toBeVisible()
})

test('uncertain logout has one explicit CSRF request and never replays on recovery', async ({ page }) => {
  await mockWorkspace(page)
  let attempts = 0
  await page.route('**/api/v1/logout', route => {
    attempts++; expect(route.request().headers()['x-csrf-token']).toBe('synthetic-csrf-token-32-characters')
    return route.abort('failed')
  })
  await page.goto('/')
  await page.getByRole('button', { name: 'Synthetic Person', exact: true }).click()
  await page.getByRole('menuitem', { name: 'Sign out', exact: true }).click()
  await expect(page.getByRole('alert')).toBeVisible()
  await page.evaluate(() => { window.dispatchEvent(new Event('online')); window.dispatchEvent(new Event('visibilitychange')) })
  expect(attempts).toBe(1)
})

test('GitHub identity review retains exact revision and definite failure without provider navigation', async ({ page }, info) => {
  await mockWorkspace(page)
  let revision = 4, attempts = 0
  await page.route('**/api/v1/account/github', route => {
    if (route.request().method() === 'DELETE') {
      attempts++; expect(route.request().headers()['if-match']).toBe('"4"')
      expect(route.request().headers()['x-csrf-token']).toBe('synthetic-csrf-token-32-characters')
      expect(route.request().postDataJSON()).toEqual({ acknowledge_invitation_impact: true })
      revision = 5
      return route.fulfill({ status: 409, json: { error: { code: 'revision_conflict', message: 'Synthetic', request_id: 'synthetic' } } })
    }
    return route.fulfill({ json: { enabled: true, revision, account: { login: 'synthetic-person', github_id: 456, verified_at: '2026-10-23T12:00:00Z' }, invitations_needing_review: 2 } })
  })
  await page.goto('/account')
  await expect(page.getByRole('button', { name: 'Unlink GitHub account' })).toBeDisabled()
  await page.getByRole('checkbox', { name: 'I understand that changing or unlinking', exact: false }).check()
  await page.getByRole('button', { name: 'Unlink GitHub account' }).click()
  await expect(page.getByRole('alert')).toContainText('This has changed. Refresh and try again.')
  await expect(page.getByText('@synthetic-person', { exact: true })).toBeVisible()
  await page.evaluate(() => window.dispatchEvent(new Event('visibilitychange')))
  expect(attempts).toBe(1)
  expect(await page.evaluate(() => JSON.stringify(localStorage) + JSON.stringify(sessionStorage))).not.toContain('synthetic-person')
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.screenshot({ path: info.outputPath('portal-account.png'), fullPage: true })
})


test('event switcher rechecks its paginated choices instead of advertising a removed cached event', async ({ page }) => {
  await mockWorkspace(page)
  await page.goto('/'); await expect(page.getByRole('link', { name: 'Southern Lights Hack', exact: true })).toBeVisible()
  await page.getByRole('link', { name: 'North Star Hack', exact: true }).click()
  await expect(page.locator('.fleet-shell-context')).toContainText('North Star Hack')
  let reads = 0
  await page.route('**/api/v1/events?**', route => { reads++; return route.fulfill({ json: { items: [fixtureEvent()], next_cursor: null } }) })
  await page.getByRole('button', { name: 'Switch event · North Star Hack' }).click()
  const choices = page.getByRole('dialog', { name: 'Switch event', exact: true })
  await expect(choices.getByRole('link', { name: 'North Star Hack', exact: true })).toBeVisible()
  expect(reads).toBe(1)
  await expect(choices.getByRole('link', { name: 'Southern Lights Hack', exact: true })).toHaveCount(0)
})

test('mentor-only review authority is discoverable and revocation withdraws it without reading private review records', async ({ page }) => {
  await mockWorkspace(page)
  let allowed = true, privateReads = 0
  const event = { ...fixtureEvent(), can_read_staff: false, can_edit: false, can_present: false, can_manage_owner: false, can_transition: false }
  await page.route(`**/api/v1/events/${firstID}`, route => route.fulfill({ json: { ...event, can_read_review_coverage: allowed } }))
  page.on('request', request => { if (new URL(request.url()).pathname.endsWith('/review-coverage')) privateReads++ })
  await page.goto(`/events/${firstID}`)
  await expect(page.locator('.fleet-shell-context')).toContainText('North Star Hack')
  await page.getByRole('button', { name: 'Find a destination' }).click()
  await page.getByRole('searchbox', { name: 'Search permitted destinations' }).fill('Support and assessment')
  const destination = page.getByRole('link', { name: 'Support and assessment', exact: true })
  await expect(destination).toBeVisible(); await expect(destination).toHaveAttribute('href', `/events/${firstID}/review`)
  expect(privateReads).toBe(0)
  allowed = false
  await page.evaluate(() => window.dispatchEvent(new Event('visibilitychange')))
  await expect(destination).toHaveCount(0)
  await expect(page.getByText('No matching destinations. Try another name.')).toBeVisible()
  await expect(page.locator('.fleet-shell-context')).toContainText('North Star Hack')
  expect(privateReads).toBe(0)
})

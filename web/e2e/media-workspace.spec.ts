import { expect, test } from '@playwright/test'
import { firstID, secondID } from './fixtures'
import { mockMedia, state, png, source } from './media-workspace-fixtures'
const gallery = `/events/${firstID}/media`
async function reviewDraft(page:import('@playwright/test').Page){await page.getByRole('button',{name:'Review private media draft',exact:true}).click();await page.getByRole('button',{name:'Confirm private media draft',exact:true}).click()}
test('confirmed gallery uses protected images, clears on denial, and never shows photographer guidance', async ({ page }) => {
  const s = state(); await mockMedia(page, s); await page.goto(gallery)
  await expect(page.getByRole('heading', { name: 'Reviewed gallery' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Submit media for organiser review' })).toHaveCount(0)
  await page.getByRole('button', { name: 'View protected photo' }).click(); await expect(page.locator('.media-photo img')).toBeVisible()
  s.denied = true; await page.evaluate(() => window.dispatchEvent(new Event('visibilitychange')))
  await expect(page.getByText('Current media access could not be verified. Images, local forms and private review details have been cleared.')).toBeVisible(); await expect(page.locator('.media-photo img')).toHaveCount(0)
  await expect(page.getByText('Synthetic participant')).toHaveCount(0)
})
test('archived self revocation remains usable and removes future gallery/download controls', async ({ page }) => {
  const s = state(); s.archived = true; await mockMedia(page, s); await page.goto(gallery)
  await expect(page.getByRole('button', { name: 'Allow identifiable event media' })).toBeDisabled()
  await page.getByRole('button', { name: 'Do not allow identifiable media' }).click();await page.getByRole('button',{name:'Confirm photo consent choice',exact:true}).click()
  await expect(page.getByText('No media is available in this view.')).toBeVisible(); await expect(page.getByRole('button', { name: 'Download original' })).toHaveCount(0)
  expect(s.consent).toBe('no')
})
test('photographer sees minimal consent guidance and retries a lost upload with identical intent', async ({ page }) => {
  const s = state(); s.role = 'photographer'; s.items = []; s.lostUpload = true; await mockMedia(page, s); await page.goto(gallery)
  await page.getByRole('button',{name:'Add media',exact:true}).click(); await page.getByLabel('Media title', { exact: true }).fill('Venue photograph')
  await page.getByLabel('Synthetic participant · yes').check()
  await page.getByLabel('Photograph (PNG or JPEG, up to 8 MiB)').setInputFiles({ name: 'venue.png', mimeType: 'image/png', buffer: png })
  await page.getByLabel('Submission reason').fill('Organiser sharing review')
  await reviewDraft(page)
  await expect(page.getByText(/The response was lost/)).toBeVisible(); expect(s.uploads).toHaveLength(1)
  s.lostUpload = false; await page.getByRole('button', { name: 'Retry identical media intent' }).click(); await expect(page.getByText(/Original intent accepted at media version/)).toBeVisible()
  expect(s.uploads).toHaveLength(2); expect(s.uploads[1]).toEqual(s.uploads[0]); expect(s.items).toHaveLength(1)
  await expect(page.getByText(/dietary|accessibility|contact_email/i)).toHaveCount(0)
  const viewport = page.viewportSize()!; expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(viewport.width)
})
test('organiser explicitly reviews untagged content before separate release and private controls do not print', async ({ page }) => {
  const s = state(); s.role = 'organiser'; s.items = [{ ...source(), state: 'draft', can_edit: true, can_review: true, can_publish: false, can_withdraw: true, consent_state: 'not_reviewed' }]; await mockMedia(page, s); await page.goto(gallery)
  await page.getByRole('button', { name: 'Open private media review' }).click()
  await page.getByLabel('Reason for this action').fill('Checked venue image')
  await page.emulateMedia({ media: 'print' }); await expect(page.locator('[data-media-record]')).toBeHidden(); await page.emulateMedia({ media: 'screen' })
  await expect(page.getByRole('button', { name: 'Review organiser approval' })).toBeDisabled()
  await page.getByLabel('I checked the media and identified every recognisable person.').check(); await page.getByLabel('No identifiable people are present.').check(); await page.getByLabel('I reviewed the content, caption, recording destination and challenge classification.').check()
  await page.getByRole('button', { name: 'Review organiser approval' }).click(); await page.getByRole('button',{name:'Confirm organiser approval',exact:true}).click(); page.once('dialog',dialog=>void dialog.accept());await page.getByRole('button', { name: 'Load current media metadata' }).click()
  await page.getByLabel('Reason for this action').fill('Explicit gallery release'); await page.getByRole('button', { name: 'Review gallery release' }).click(); await page.getByRole('button',{name:'Confirm explicit gallery release',exact:true}).click(); await expect.poll(() => s.writes).toEqual(['review', 'publish'])
  page.once('dialog',dialog=>void dialog.accept());await page.getByRole('button', { name: 'Load current media metadata' }).click(); await page.emulateMedia({ media: 'print' }); await expect(page.locator('.media-private').last()).toBeHidden(); await expect(page.locator('.media-consent')).toBeHidden()
})
test('an unsent deferred preflight cannot submit after definite authority denial', async ({ page }) => {
  const s = state(); s.role = 'photographer'; s.items = []
  let resume!: () => void; const hold = new Promise<void>(resolve => { resume = resolve }); await mockMedia(page, s); await page.goto(gallery)
  await page.getByRole('button',{name:'Add media',exact:true}).click(); await page.getByLabel('Media type').selectOption('recording'); await page.getByLabel('Media title', { exact: true }).fill('Private recording draft'); await page.getByLabel('Recording HTTPS link').fill('https://recordings.example.test/review'); await page.getByLabel('Submission reason').fill('Review recording')
  s.deferSession = () => hold; await reviewDraft(page); await expect(page.getByText('Checking current authority for this exact reviewed intent…')).toBeVisible()
  s.denied = true; await page.evaluate(() => window.dispatchEvent(new Event('visibilitychange'))); await expect(page.getByText(/Current media access could not be verified/)).toBeVisible(); resume(); await page.waitForTimeout(100)
  expect(s.writes).toHaveLength(0); await expect(page.locator('input[value="Private recording draft"]')).toHaveCount(0)
})
test('event navigation clears protected image and private unsent metadata', async ({ page }) => {
  const s = state(); s.role = 'photographer'; await mockMedia(page, s); await page.goto(gallery)
  await page.getByRole('button',{name:'Add media',exact:true}).click(); await page.getByLabel('Media title', { exact: true }).fill('UNSENT PRIVATE CAPTION')
  await page.getByRole('button', { name: 'View protected photo' }).click(); await expect(page.locator('.media-photo img')).toBeVisible()
  await page.goto(`/events/${secondID}/media`); await expect(page.locator('.media-photo img')).toHaveCount(0); await expect(page.locator('input[value="UNSENT PRIVATE CAPTION"]')).toHaveCount(0)
})

test('two submits during deferred local file reading retain only one upload intent', async ({ page }) => {
  await page.addInitScript(() => {
    const original = File.prototype.arrayBuffer
    const readers: (() => void)[] = []
    Object.assign(window, { mediaFileReaders: readers })
    File.prototype.arrayBuffer = function () { return new Promise(resolve => { readers.push(() => { void original.call(this).then(resolve) }) }) }
  })
  const s = state(); s.role = 'photographer'; s.items = []; await mockMedia(page, s); await page.goto(gallery)
  await page.getByRole('button',{name:'Add media',exact:true}).click(); await page.getByLabel('Media title', { exact: true }).fill('One deliberate photo')
  await page.getByLabel('Photograph (PNG or JPEG, up to 8 MiB)').setInputFiles({ name: 'venue.png', mimeType: 'image/png', buffer: png })
  await page.getByLabel('Submission reason').fill('One collection intent')
  await page.getByRole('button',{name:'Review private media draft',exact:true}).click()
  await page.getByRole('region',{name:'Media collection task'}).locator('form').dispatchEvent('submit')
  await page.evaluate(() => { const readers = (window as unknown as { mediaFileReaders: (() => void)[] }).mediaFileReaders; readers.forEach(read => read()) })
  await page.getByRole('button',{name:'Confirm private media draft',exact:true}).click()
  await expect(page.getByText(/Original intent accepted at media version/)).toBeVisible()
  expect(s.uploads).toHaveLength(1)
})

test('own consent remains reachable after gallery denial without granting gallery access', async ({ page }) => {
  const s = state(); s.denied = true; s.archived = true; await mockMedia(page, s); await page.goto(gallery)
  await page.getByRole('link', { name: 'Manage your own photo consent' }).click()
  await expect(page.getByRole('heading', { name: 'Your event photo consent', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Do not allow identifiable media' }).click();await page.getByRole('button',{name:'Confirm photo consent choice',exact:true}).click()
  await expect(page.getByText('Current choice:', { exact: false })).toContainText('no')
  await expect(page.getByRole('button', { name: 'View protected photo' })).toHaveCount(0); expect(s.denied).toBe(true)
})

test('fresh200 preflight with lost collection capability stops an unsent upload', async ({ page }) => {
  const s = state(); s.role = 'photographer'; s.items = []; let resume!: () => void; const hold = new Promise<void>(resolve => { resume = resolve }); await mockMedia(page, s); await page.goto(gallery)
  await page.getByRole('button',{name:'Add media',exact:true}).click(); await page.getByLabel('Media title', { exact: true }).fill('Unsent after role loss'); await page.getByLabel('Submission reason').fill('Must not send stale authority'); await page.getByLabel('Photograph (PNG or JPEG, up to 8 MiB)').setInputFiles({ name: 'venue.png', mimeType: 'image/png', buffer: png })
  s.deferPreflight = () => hold; await reviewDraft(page); await expect.poll(() => s.preflights).toBe(1)
  s.role = 'attendee'; resume(); await expect(page.getByRole('heading', { name: 'Reviewed gallery' })).toBeVisible()
  expect(s.uploads).toHaveLength(0); await expect(page.getByRole('heading', { name: 'Submit media for organiser review' })).toHaveCount(0)
})

test('lost upload survives transient refresh failure and reconnect with its identical memory-only intent', async ({ page }) => {
  const s = state(); s.role = 'photographer'; s.items = []; s.lostUpload = true; s.transientAfterLostUpload = true; await mockMedia(page, s); await page.goto(gallery)
  await page.getByRole('button',{name:'Add media',exact:true}).click(); await page.getByLabel('Media title', { exact: true }).fill('Retained uncertain upload'); await page.getByLabel('Submission reason').fill('Exact command must survive failed reads'); await page.getByLabel('Photograph (PNG or JPEG, up to 8 MiB)').setInputFiles({ name: 'venue.png', mimeType: 'image/png', buffer: png })
  await reviewDraft(page); await expect(page.getByText(/Private drafts and uncertain intents remain only in this page/)).toBeVisible()
  expect(s.uploads).toHaveLength(1); await expect(page.getByRole('button', { name: 'Retry identical media intent' })).toBeHidden()
  s.transient = false; s.lostUpload = false; await page.getByRole('button', { name: 'Try again' }).click()
  await expect(page.getByRole('button', { name: 'Retry identical media intent' })).toBeVisible();await page.getByRole('button',{name:'Correct details',exact:true}).click(); await expect(page.getByLabel('Media title', { exact: true })).toHaveValue('Retained uncertain upload')
  await page.getByRole('button', { name: 'Retry identical media intent' }).click(); await expect(page.getByText(/Original intent accepted at media version/)).toBeVisible()
  expect(s.uploads).toHaveLength(2); expect(s.uploads[1]).toEqual(s.uploads[0]); expect(s.items).toHaveLength(1)
  const stored = await page.evaluate(() => JSON.stringify([Object.entries(localStorage), Object.entries(sessionStorage)])); expect(stored).not.toContain('Retained uncertain upload'); expect(stored).not.toContain('Exact command must survive failed reads')
})

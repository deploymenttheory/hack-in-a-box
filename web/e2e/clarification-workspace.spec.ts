import { openEventSection } from './event-navigation-fixture'
import { expect, test } from '@playwright/test'
import { firstID, secondID } from './fixtures'
import { clarificationFixture, feedPath, officialTitle, privateBody, questionPath, teamID } from './clarification-workspace-fixtures'

async function sendQuestion(page: import('@playwright/test').Page) { await page.getByRole('button', { name: 'Review private question', exact: true }).click(); await page.getByRole('button', { name: 'Send private question', exact: true }).click() }
async function answerDecision(page: import('@playwright/test').Page, action: 'save' | 'approve' | 'publish') {
 const choose = action === 'save' ? 'Review draft version' : action === 'approve' ? 'Review saved version approval' : 'Review approved version publication'
 const confirm = action === 'save' ? 'Save a new draft version' : action === 'approve' ? 'Approve saved version' : 'Publish approved version'
 await page.getByRole('button', { name: choose, exact: true }).click(); await page.getByRole('button', { name: confirm, exact: true }).click()
}

for (const unknown of [false, true]) {
  test(`staying on the private tab retains ${unknown ? 'the exact uncertain command for explicit retry' : 'unsent question text'}`, async ({ page }) => {
    const fixture = await clarificationFixture(page)
    fixture.state.dropAsk = unknown
    await page.goto(feedPath)
    await page.getByRole('button', { name: "Your team's private questions", exact: true }).click()
    await page.getByLabel('Your question', { exact: true }).fill('PRIVATE-TAB-STAY-CANARY')
    if (unknown) {
      await sendQuestion(page)
      await expect(page.getByText('The outcome is unknown.', { exact: false })).toBeVisible()
    }
    const dialog = page.waitForEvent('dialog')
    const switchTab = page.getByRole('button', { name: 'Official answers', exact: true }).click()
    const warning = await dialog
    expect(warning.message()).toContain('retained retry command')
    await warning.dismiss()
    await switchTab
    await expect(page.getByRole('button', { name: "Your team's private questions", exact: true })).toHaveAttribute('aria-pressed', 'true')
    await expect(page.getByLabel('Your question', { exact: true })).toHaveValue('PRIVATE-TAB-STAY-CANARY')
    expect(fixture.state.asks).toHaveLength(unknown ? 1 : 0)
    if (unknown) {
      await page.getByRole('button', { name: 'Retry the same private question', exact: true }).click()
      await expect(page.getByText('Saved as a private team question.', { exact: false })).toBeVisible()
      expect(fixture.state.asks).toHaveLength(2)
      expect(fixture.state.asks[0]).toEqual(fixture.state.asks[1])
      expect(fixture.commands.size).toBe(1)
    }
  })

  test(`explicitly leaving the private tab acknowledges clearing ${unknown ? 'an uncertain command' : 'unsent text'} without replay`, async ({ page }) => {
    const fixture = await clarificationFixture(page)
    fixture.state.dropAsk = unknown
    await page.goto(feedPath)
    await page.getByRole('button', { name: "Your team's private questions", exact: true }).click()
    await page.getByLabel('Your question', { exact: true }).fill('PRIVATE-TAB-LEAVE-CANARY')
    if (unknown) {
      await sendQuestion(page)
      await expect(page.getByText('The outcome is unknown.', { exact: false })).toBeVisible()
    }
    const dialog = page.waitForEvent('dialog')
    const switchTab = page.getByRole('button', { name: 'Official answers', exact: true }).click()
    const warning = await dialog
    expect(warning.message()).toContain('will be cleared')
    await warning.accept()
    await switchTab
    await expect(page.getByRole('heading', { name: 'Official common answers' })).toBeVisible()
    await page.getByRole('button', { name: "Your team's private questions", exact: true }).click()
    await expect(page.getByLabel('Your question', { exact: true })).toHaveValue('')
    await expect(page.getByRole('button', { name: 'Retry the same private question', exact: true })).toHaveCount(0)
    expect(fixture.state.asks).toHaveLength(unknown ? 1 : 0)
    expect(await page.evaluate(() => JSON.stringify(localStorage) + JSON.stringify(sessionStorage))).not.toContain('PRIVATE-TAB-LEAVE-CANARY')
  })
}

test('resource link opens common search, public rubric and canonical answer without private question text', async ({ page }) => {
  const fixture = await clarificationFixture(page)
  await page.goto(`/events/${firstID}`)
  await openEventSection(page, 'Resources')
  await page.getByRole('link', { name: 'Questions and official clarifications', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Official common answers' })).toBeVisible()
  await expect(page.getByText(privateBody)).toHaveCount(0)
  await page.getByText('Read the current published rubric', { exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Common published expectations · version 1' })).toBeVisible()
  await page.getByRole('searchbox', { name: 'Search published answers', exact: true }).fill(privateBody)
  await page.getByRole('button', { name: 'Search', exact: true }).click()
  await expect(page.getByText('No published answers match this search.')).toBeVisible()
  await expect(page.getByRole('link', { name: officialTitle })).toHaveCount(0)
  await page.getByRole('search', { name: 'Search published answers', exact: true }).getByRole('button', { name: 'Clear', exact: true }).click()
  const answer = page.getByRole('link', { name: officialTitle, exact: true })
  await answer.focus(); await page.keyboard.press('Enter')
  await expect(page.getByRole('heading', { name: officialTitle, exact: true })).toBeVisible()
  await page.getByRole('link', { name: 'Read the current owning challenge' }).click()
  await expect(page.getByRole('heading', { name: 'Actual common challenge', exact: true })).toBeVisible()
  expect(fixture.state.requests.some(path => path.includes('/rubric/configuration'))).toBe(false)
})

test('private upload and unknown ask use the same explicit command retry, then show authoritative saved question', async ({ page }) => {
  const fixture = await clarificationFixture(page)
  fixture.state.dropAsk = true
  await page.goto(feedPath)
  await page.getByRole('button', { name: "Your team's private questions", exact: true }).click()
  await page.getByLabel('Your question', { exact: true }).fill('PRIVATE-UNSENT-ASK-CANARY')
  await page.getByLabel('Choose a private attachment').setInputFiles({ name: 'private-evidence.txt', mimeType: 'text/plain', buffer: Buffer.from('Synthetic private evidence') })
  await page.getByRole('button', { name: 'Upload private attachment', exact: true }).click()
  await expect(page.getByText('Uploaded to your team.', { exact: false })).toBeVisible()
  expect(fixture.state.uploads[0].team_id).toBe(teamID)
  await sendQuestion(page)
  await expect(page.getByText('The outcome is unknown.', { exact: false })).toBeVisible()
  await expect(page.getByLabel('Your question', { exact: true })).toBeDisabled()
  expect(fixture.state.asks).toHaveLength(1)
  await page.getByRole('button', { name: 'Retry the same private question', exact: true }).click()
  await expect(page.getByText('Saved as a private team question.', { exact: false })).toBeVisible()
  expect(fixture.state.asks).toHaveLength(2)
  expect(fixture.state.asks[0]).toEqual(fixture.state.asks[1])
  expect(fixture.commands.size).toBe(1)
  await page.getByRole('link', { name: 'Read the saved private question' }).click()
  await expect(page.getByText('PRIVATE-UNSENT-ASK-CANARY', { exact: true })).toBeVisible()
  const download = page.waitForEvent('download')
  await page.getByRole('link', { name: 'Download private attachment 1', exact: true }).click()
  await expect((await download).suggestedFilename()).toBe('private-evidence.txt')
  expect(await page.evaluate(() => JSON.stringify(localStorage) + JSON.stringify(sessionStorage))).not.toContain('PRIVATE-UNSENT-ASK-CANARY')
})

test('other team has common answers but cannot open a copied private question or staff review', async ({ page }) => {
  const fixture = await clarificationFixture(page, 'other')
  await page.goto(feedPath)
  await expect(page.getByRole('link', { name: officialTitle, exact: true })).toBeVisible()
  await page.goto(questionPath)
  await expect(page.getByRole('alert')).toBeVisible()
  await expect(page.getByText(privateBody)).toHaveCount(0)
  await expect(page.getByLabel('Common official answer text')).toHaveCount(0)
  expect(fixture.state.requests.some(path => path.endsWith('/review'))).toBe(false)
})

test('pre-publication native denial withholds all answer titles, search and private form even on copied routes', async ({ page }) => {
  const fixture = await clarificationFixture(page)
  fixture.state.allowed = false
  await page.goto(feedPath)
  await expect(page.getByRole('alert')).toBeVisible()
  await expect(page.getByRole('link', { name: officialTitle, exact: true })).toHaveCount(0)
  await expect(page.getByRole('searchbox', { name: 'Search published answers', exact: true })).toHaveCount(0)
  await expect(page.getByLabel('Your question', { exact: true })).toHaveCount(0)
  expect(fixture.state.requests.some(path => path.endsWith('/clarifications'))).toBe(false)
  await page.goto(questionPath)
  await expect(page.getByText(privateBody)).toHaveCount(0)
})

test('staff explicitly reconciles unknown save before separate approval, publication and correction history', async ({ page }) => {
  const fixture = await clarificationFixture(page, 'staff')
  fixture.state.dropSave = true
  await page.goto(questionPath)
  await expect(page.getByText(privateBody, { exact: true })).toBeVisible()
  await expect(page.getByLabel('Common official answer text')).toHaveValue('')
  await expect(page.getByText('Current confirmed attendees: 2 people', { exact: false })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Download private attachment 1', exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: 'Mark triaged', exact: true }).focus()
  await page.keyboard.press('Enter')
  await page.getByRole('button', { name: 'Confirm reviewed change', exact: true }).click()
  await expect(page.getByText('Private challenge question · triaged', { exact: true })).toBeVisible()
  await page.getByLabel('Common answer title').fill('Reviewed answer for every team')
  await page.getByLabel('Common official answer text').fill('SHARED-OFFICIAL-ANSWER-ONE')
  await answerDecision(page, 'save')
  await expect(page.getByText('Outcome unknown — writes paused', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Review approved version publication', includeHidden: true })).toBeDisabled()
  expect(fixture.state.saves).toHaveLength(1)
  await page.getByRole('button', { name: 'Check the exact server outcome' }).click()
  await expect(page.getByText('The server confirms matching version 1 and the exact original transition:', { exact: false })).toBeVisible()
  expect(fixture.state.saves).toHaveLength(1)
  await answerDecision(page, 'approve')
  await expect(page.getByText('Version 1 approved.', { exact: false })).toBeVisible()
  expect(fixture.state.actions).toEqual(['approve'])
  await answerDecision(page, 'publish')
  await expect(page.getByText('Official version 1 published.', { exact: false })).toBeVisible()
  await page.getByLabel('Common official answer text').fill('SHARED-OFFICIAL-CORRECTION-TWO')
  await answerDecision(page, 'save')
  await expect(page.getByText('Saved draft version 2.', { exact: false })).toBeVisible()
  await answerDecision(page, 'approve')
  await expect(page.getByText('Version 2 approved.', { exact: false })).toBeVisible()
  await answerDecision(page, 'publish')
  await expect(page.getByText('Official version 2 published.', { exact: false })).toBeVisible()
  expect(fixture.state.actions).toEqual(['approve', 'publish', 'approve', 'publish'])
  await page.getByRole('link', { name: 'Read the official published answer' }).click()
  await expect(page.getByText('SHARED-OFFICIAL-CORRECTION-TWO', { exact: true })).toBeVisible()
  await page.getByText('Published correction history', { exact: true }).click()
  await expect(page.getByText('SHARED-OFFICIAL-ANSWER-ONE', { exact: true })).toBeVisible()
  await expect(page.getByText(privateBody, { exact: true })).toHaveCount(0)
  expect(fixture.state.requests.some(path => path.includes('/rubric/configuration'))).toBe(false)
})

test('current source change while save outcome is unknown retains exact saved version but blocks stale approval', async ({ page }) => {
  const fixture = await clarificationFixture(page, 'staff')
  fixture.state.dropSave = true
  await page.goto(questionPath)
  await page.getByLabel('Common answer title').fill('Source-bound reviewed draft')
  await page.getByLabel('Common official answer text').fill('PRIVATE-SOURCE-CHANGE-UNSENT')
  await answerDecision(page, 'save')
  await expect(page.getByText('Outcome unknown — writes paused', { exact: true })).toBeVisible()
  fixture.state.source.challenge_version = 2; fixture.state.source.rubric.version = 2; fixture.state.source.rubric.digest = 'b'.repeat(64)
  await page.getByRole('button', { name: 'Check the exact server outcome' }).click()
  await expect(page.getByText('This saved draft applies to an earlier source.', { exact: false })).toBeVisible()
  await expect(page.getByLabel('Common official answer text')).toHaveValue('PRIVATE-SOURCE-CHANGE-UNSENT')
  await expect(page.getByRole('button', { name: 'Review saved version approval', includeHidden: true })).toBeDisabled()
  await page.getByLabel('Type of answer').selectOption('rubric_change')
  await expect(page.getByText('A material-change notice requires its owning version release', { exact: false })).toBeVisible()
  fixture.state.rejectSave = true
  await answerDecision(page, 'save')
  await expect(page.getByRole('alert')).toBeVisible()
  await expect(page.getByLabel('Common official answer text')).toHaveValue('PRIVATE-SOURCE-CHANGE-UNSENT')
  expect(fixture.state.actions).toEqual([])
})

test('archive while outcome is unknown preserves private text for reconciliation and freezes further decisions', async ({ page }) => {
  const fixture = await clarificationFixture(page, 'staff')
  fixture.state.dropSave = true
  await page.clock.install()
  await page.goto(questionPath)
  await page.getByLabel('Common answer title').fill('Archived pending review')
  await page.getByLabel('Common official answer text').fill('PRIVATE-ARCHIVE-OUTCOME')
  await answerDecision(page, 'save')
  await expect(page.getByText('Outcome unknown — writes paused', { exact: true })).toBeVisible()
  fixture.state.archived = true
  await page.clock.fastForward(11_000)
  await expect(page.getByText('This event is read-only.', { exact: false })).toBeVisible()
  await expect(page.getByLabel('Common official answer text')).toHaveValue('PRIVATE-ARCHIVE-OUTCOME')
  await page.getByRole('button', { name: 'Check the exact server outcome' }).click()
  await expect(page.getByText('The server confirms matching version 1 and the exact original transition:', { exact: false })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Review saved version approval', includeHidden: true })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Review draft version', includeHidden: true })).toBeDisabled()
  expect(fixture.state.saves).toHaveLength(1)
})

test('staff review loss clears dirty private editor, original question and all action links', async ({ page }) => {
  const fixture = await clarificationFixture(page, 'staff')
  await page.clock.install()
  await page.goto(questionPath)
  await page.getByLabel('Common official answer text').fill('PRIVATE-REVOKED-EDITOR-CANARY')
  fixture.state.reviewAllowed = false
  await page.clock.fastForward(11_000)
  await expect(page.getByRole('alert')).toBeVisible()
  await expect(page.getByLabel('Common official answer text')).toHaveCount(0)
  await expect(page.getByText(privateBody, { exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Review saved version approval', includeHidden: true })).toHaveCount(0)
  expect(await page.evaluate(() => JSON.stringify(localStorage) + JSON.stringify(sessionStorage))).not.toContain('PRIVATE-REVOKED-EDITOR-CANARY')
})

test('current denial clears dirty ask and private caches; UTF-8 validation and event navigation keep honest unsent state', async ({ page }) => {
  const fixture = await clarificationFixture(page)
  await page.clock.install()
  await page.goto(feedPath)
  await page.getByRole('button', { name: "Your team's private questions", exact: true }).click()
  await page.getByLabel('Your question', { exact: true }).fill('🎉'.repeat(2100))
  await expect(page.getByRole('button', { name: 'Review private question', exact: true, includeHidden: true })).toBeDisabled()
  await page.getByLabel('Your question', { exact: true }).fill('PRIVATE-NAVIGATION-UNSENT')
  page.once('dialog', dialog => dialog.accept())
  await page.getByRole('link', { name: 'Back to event', exact: false }).click()
  await page.getByRole('link', { name: 'Switch event', exact: false }).click()
  await page.getByRole('link', { name: 'Second synthetic event', exact: false }).click()
  await openEventSection(page, 'Resources')
  await page.getByRole('link', { name: 'Questions and official clarifications', exact: true }).click()
  await expect(page).toHaveURL(`/events/${secondID}/clarifications`)
  await page.getByRole('button', { name: "Your team's private questions", exact: true }).click()
  await expect(page.getByLabel('Your question', { exact: true })).toHaveValue('')
  await page.getByLabel('Your question', { exact: true }).fill('PRIVATE-REVOKED-ASK')
  fixture.state.allowed = false
  await page.clock.fastForward(11_000)
  await expect(page.getByRole('alert')).toBeVisible()
  await expect(page.getByLabel('Your question', { exact: true })).toHaveCount(0)
  expect(fixture.state.asks).toHaveLength(0)
  expect(await page.evaluate(() => JSON.stringify(localStorage) + JSON.stringify(sessionStorage))).not.toMatch(/PRIVATE-(?:NAVIGATION|REVOKED)/)
})

test('account and expired-session changes clear private unsent forms without automatic writes', async ({ page }) => {
  const fixture = await clarificationFixture(page)
  await page.goto(feedPath)
  await page.getByRole('button', { name: "Your team's private questions", exact: true }).click()
  await page.getByLabel('Your question', { exact: true }).fill('PRIVATE-FIRST-ACCOUNT-DRAFT')
  fixture.state.actor = { ...fixture.state.actor, id: 'f6bc1607-c8b8-418c-9248-b9c9d937a013', display_name: 'Different synthetic account' }
  await page.evaluate(() => window.dispatchEvent(new Event('visibilitychange')))
  await expect(page.getByRole('heading', { name: 'Official common answers' })).toBeVisible()
  await expect(page.getByLabel('Your question', { exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: "Your team's private questions", exact: true }).click()
  await expect(page.getByLabel('Your question', { exact: true })).toHaveValue('')
  await page.getByLabel('Your question', { exact: true }).fill('PRIVATE-EXPIRED-SESSION-DRAFT')
  fixture.state.expired = true
  await page.evaluate(() => window.dispatchEvent(new Event('visibilitychange')))
  await expect(page.getByRole('link', { name: /Sign in with your organisation/ })).toBeVisible()
  await expect(page.getByLabel('Your question', { exact: true })).toHaveCount(0)
  expect(fixture.state.asks).toHaveLength(0)
  expect(await page.evaluate(() => JSON.stringify(localStorage) + JSON.stringify(sessionStorage))).not.toMatch(/PRIVATE-(?:FIRST|EXPIRED)/)
})

test('a confirmed draft write stays confirmed when its follow-up read fails and is never replayed', async ({ page }) => {
  const fixture = await clarificationFixture(page, 'staff')
  fixture.state.failReadAfterSave = true
  await page.goto(questionPath)
  await page.getByLabel('Common answer title').fill('Confirmed server draft')
  await page.getByLabel('Common official answer text').fill('PRIVATE-CONFIRMED-WRITE')
  await answerDecision(page, 'save')
  await expect(page.getByText('The write was confirmed, but the follow-up read failed.', { exact: false })).toBeVisible()
  await expect(page.getByText('Outcome unknown — writes paused', { exact: true })).toHaveCount(0)
  await expect(page.getByText('No server draft is currently recorded.', { exact: true })).toHaveCount(0)
  expect(fixture.state.saves).toHaveLength(1)
  await page.getByRole('button', { name: 'Refresh current review', exact: true }).click()
  await expect(page.getByText('Current review refreshed.', { exact: false })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Review saved version approval', includeHidden: true })).toBeEnabled()
  expect(fixture.state.saves).toHaveLength(1)
})

test('a competing server version preserves dirty text until explicit comparison and a new revision-bound save', async ({ page }) => {
  const fixture = await clarificationFixture(page, 'staff')
  await page.clock.install()
  await page.goto(questionPath)
  await page.getByLabel('Common answer title').fill('Shared draft')
  await page.getByLabel('Common official answer text').fill('Initial saved draft')
  await answerDecision(page, 'save')
  await expect(page.getByText('Saved draft version 1.', { exact: false })).toBeVisible()
  await page.getByLabel('Common official answer text').fill('PRIVATE-KEPT-UNSENT-COMPARISON')
  fixture.state.draft = { ...fixture.state.draft!, revision: 2, version: 2, document: { ...fixture.state.draft!.document, body: 'Other staff authoritative draft' } }
  await page.clock.fastForward(11_000)
  await expect(page.getByText('Other staff authoritative draft', { exact: true })).toBeVisible()
  await expect(page.getByLabel('Common official answer text')).toHaveValue('PRIVATE-KEPT-UNSENT-COMPARISON')
  await expect(page.getByRole('button', { name: 'Review draft version', includeHidden: true })).toBeDisabled()
  page.once('dialog', dialog => void dialog.accept()); await page.getByRole('button', { name: 'Keep my text against the current server version', exact: true }).click()
  await answerDecision(page, 'save')
  await expect(page.getByText('Saved draft version 3.', { exact: false })).toBeVisible()
  expect(fixture.state.saves[1].revision).toBe('"2"')
  expect(fixture.state.actions).toEqual([])
})

test('staff private attachment bytes open only at the native live gate and private content is hidden in print', async ({ page }) => {
  const fixture = await clarificationFixture(page, 'staff')
  await page.clock.install()
  await page.goto(questionPath)
  await expect(page.getByText('Staff byte access opens at live start', { exact: false })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Download private attachment 1', exact: true })).toHaveCount(0)
  expect(fixture.state.requests.some(path => path.includes('/attachments/'))).toBe(false)
  fixture.state.staffAttachments = true
  await page.clock.fastForward(11_000)
  const download = page.waitForEvent('download')
  await page.getByRole('link', { name: 'Download private attachment 1', exact: true }).click()
  expect((await download).suggestedFilename()).toBe('private-evidence.txt')
  await page.emulateMedia({ media: 'print' })
  await expect(page.getByText(privateBody, { exact: true })).not.toBeVisible()
})

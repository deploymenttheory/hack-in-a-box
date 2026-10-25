import { expect, test } from '@playwright/test'

test('nested preview does not move a pointer target before release', async ({ page }) => {
  await page.goto('/')
  await page.getByRole('link', { name: 'Task patterns', exact: true }).click()
  const frame = page.frameLocator('iframe[title="Production task pattern examples"]')
  await frame.getByText('Synthetic scenario controls', { exact: true }).click()
  await frame.getByLabel('Synthetic transport', { exact: true }).selectOption('delayed')
  const target = frame.getByRole('link', { name: 'Project Atlas', exact: true })
  await target.scrollIntoViewIfNeeded()
  const before = (await target.boundingBox())!
  await page.mouse.move(before.x + before.width / 2, before.y + before.height / 2)
  await page.mouse.down()
  await page.evaluate(() => new Promise<void>(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))))
  const during = await target.boundingBox()
  await test.info().attach('pointer geometry', { body: JSON.stringify({ before, during }), contentType: 'application/json' })
  await page.mouse.up()
  expect(during).toEqual(before)
  await expect(frame.getByLabel('Decision note', { exact: true })).toBeVisible()
})

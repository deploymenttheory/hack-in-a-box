import { expect, test } from '@playwright/test'
import { surveyFixture, surveyPath, managePath } from './survey-workspace-fixtures'

// Matched synthetic evidence against the pre-refactor commit and current build.
// Run explicitly; ordinary behavioral suites own all native policy assertions.
for (const surface of ['response', 'authoring'] as const) test(`matched survey ${surface} evidence`, async ({ page }, info) => {
  const f = await surveyFixture(page, surface === 'authoring')
  if (surface === 'authoring') { f.configuration.approved_at = null; f.configuration.published_at = null }
  await page.goto(surface === 'authoring' ? managePath : surveyPath)
  if (surface === 'authoring') {
    await page.getByRole('navigation', { name: 'Survey sections', exact: true }).getByRole('button', { name: 'Questions', exact: true }).click()
    await expect(page.getByLabel('Form name', { exact: true })).toHaveValue('Post-event learning')
  } else {
    await page.getByLabel('Did you learn something? (required)').selectOption('yes')
    await page.getByLabel('Private explanation (optional)').fill('Synthetic example: clearer handover steps.')
  }
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.screenshot({ path: process.env.SURVEY_EVIDENCE_DIR ? `${process.env.SURVEY_EVIDENCE_DIR}/${surface}-${info.project.name}.png` : info.outputPath(`${surface}.png`), fullPage: true, animations: 'disabled' })
})

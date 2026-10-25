import AxeBuilder from '@axe-core/playwright'
import { expect, type Locator, type Page, type TestInfo } from '@playwright/test'

// Navigation genuinely uses keyboard input. Never force focus/click the target:
// an unreachable or trapped control must fail rather than bypassing the defect.
export async function tabTo(page: Page, target: Locator, maximum = 140) {
  await expect(target).toBeVisible()
  await expect(target).toBeEnabled()
  for (let count = 0; count < maximum; count++) {
    if (await target.evaluate(element => element === element.ownerDocument.activeElement)) return
    await page.keyboard.press('Tab')
  }
  throw new Error(`Control was not keyboard reachable after ${maximum} Tab presses`)
}
export async function activate(page: Page, target: Locator) {
  await tabTo(page, target)
  await page.keyboard.press('Enter')
}
export async function typeInto(page: Page, target: Locator, text: string) {
  await tabTo(page, target)
  await page.keyboard.type(text)
}
export async function scan(page: Page, info: TestInfo, state: string) {
  const result = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']).analyze()
  // No disabled rules or exception baselines. Incomplete results require manual
  // review; passing automation does not establish WCAG conformance.
  const evidence = {
    state, browser: page.context().browser()?.version(), engine: info.project.name,
    viewport: page.viewportSize(), testEngine: result.testEngine,
    passes: result.passes.map(rule => rule.id),
    violations: result.violations.map(rule => ({ id: rule.id, impact: rule.impact, help: rule.help, targets: rule.nodes.map(node => node.target) })),
    incomplete: result.incomplete.map(rule => ({ id: rule.id, targets: rule.nodes.map(node => node.target) })),
  }
  await info.attach(`${state}-automated-scan.json`, { body: JSON.stringify(evidence, null, 2), contentType: 'application/json' })
  expect(evidence.violations, `Automatically detectable failures in ${state}`).toEqual([])
}

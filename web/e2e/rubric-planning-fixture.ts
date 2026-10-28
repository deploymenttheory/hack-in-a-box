import type { Page } from '@playwright/test'
import type { components } from '../src/api/schema.gen'
import { rubricGuideConfiguration } from '../src/styleguide/rubric-fixture'
import { firstID, fixtureEvent, mockWorkspace } from './fixtures'

export async function rubricPlanningFixture(page: Page) {
  await mockWorkspace(page)
  const state = { configuration: rubricGuideConfiguration(), event: fixtureEvent(), posts: [] as { body: components['schemas']['RubricCommand']; revision: string | undefined }[], reads: 0, deny: 0, lose: false, held: undefined as Promise<void> | undefined, started: undefined as (() => void) | undefined }
  await page.route(`**/api/v1/events/${firstID}`, route => route.fulfill({ json: state.event }))
  await page.route(`**/api/v1/events/${firstID}/rubric/configuration*`, async route => {
    const request = route.request()
    if (request.method() === 'GET') {
      state.reads++; state.started?.(); if (state.held) await state.held
      if (state.deny) return route.fulfill({ status: state.deny, json: { error: { code: 'forbidden', message: 'Synthetic current rubric read denied.' } } })
      return route.fulfill({ json: state.configuration })
    }
    const body = request.postDataJSON() as components['schemas']['RubricCommand'], revision = request.headers()['if-match']
    state.posts.push({ body, revision })
    if (revision !== `"${state.configuration.revision}"`) return route.fulfill({ status: 409, json: { error: { code: 'revision_conflict', message: 'Read the current rubric.' } } })
    if (body.action === 'save') state.configuration = { ...state.configuration, revision: state.configuration.revision + 1, reference: { id: '00000000-0000-4000-8000-000000000099', version: state.configuration.reference.version + 1, digest: 'a'.repeat(64) }, definition: body.definition!, approved: false }
    else if (body.action === 'approve') state.configuration = { ...state.configuration, revision: state.configuration.revision + 1, approved: true }
    else throw new Error('Unexpected rubric test command')
    if (state.lose) { state.lose = false; return route.abort('connectionfailed') }
    return route.fulfill({ json: state.configuration })
  })
  return state
}

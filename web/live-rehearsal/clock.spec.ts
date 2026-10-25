import { test, expect } from '@playwright/test'
import { actor, control, native } from './native'
import { openEventSection } from '../e2e/event-navigation-fixture'

test('production team dashboard follows native release/live/cutoff clocks while lifecycle lags and assignments stay frozen', async ({ browser }, info) => {
  const context = await actor(browser, 'public', 'competitor'), page = await context.newPage()
  await control('stage-lag')
  const base = `/api/v1/events/${native.event_id}`
  try {
    const phases = [
      { name: 'preparation', instant: '2027-03-21T09:00:00Z', draft: false, submit: false, outcome: 'open' },
      { name: 'released build week', instant: '2027-03-22T10:00:00Z', draft: true, submit: false, outcome: 'open' },
      { name: 'live opening', instant: '2027-03-29T09:00:00Z', draft: true, submit: true, outcome: 'open' },
      { name: 'exact final cutoff', instant: '2027-03-29T16:00:00Z', draft: false, submit: false, outcome: 'not_submitted' },
    ]
    for (const phase of phases) {
      await test.step(phase.name, async () => {
        await control('clock', { instant: phase.instant })
        const event = await context.request.get(base)
        expect(event.status()).toBe(200)
        expect((await event.json()).stage).toBe('preparation')
        const submission = await context.request.get(`${base}/teams/${native.team_id}/submission`)
        expect(submission.status()).toBe(200)
        const current = await submission.json()
        expect(current.can_draft).toBe(phase.draft)
        expect(current.can_submit).toBe(phase.submit)
        expect(current.outcome).toBe(phase.outcome)
        if (phase.outcome === 'not_submitted') expect(current.frozen_at).not.toBeNull()
        const teams = await context.request.get(`${base}/teams`)
        expect(teams.status()).toBe(200)
        const allocation = await teams.json()
        expect(allocation.frozen).toBe(true)
        // can_create is native actor eligibility; frozen is the separate gate.
        // Prove the actual write is refused rather than reinterpreting the flag.
        const session = await (await context.request.get('/api/v1/session')).json()
        const mutation = await context.request.post(`${base}/teams`, { headers: { Origin: native.public, 'X-CSRF-Token': session.csrf_token, 'If-Match': `"${allocation.revision}"` }, data: { action: 'create', name: 'Must never be created after freeze', reason: 'Synthetic frozen gate probe' } })
        expect(mutation.ok()).toBe(false)
        expect((await mutation.json()).error.code).toBe('teams_frozen')
        expect(allocation.teams.map((team: { id: string }) => team.id)).toEqual([native.team_id])
        await page.goto(`/events/${native.event_id}`)
        await openEventSection(page, 'Deliverables')
        const workspace = page.getByRole('region', { name: 'Team submission workspace', exact: true })
        await expect(workspace.getByRole('heading', { name: 'Rehearsal team', exact: true })).toBeVisible()
        if (phase.name === 'released build week') await expect(workspace).toContainText('Formal submissions and review open at live start.')
        if (phase.outcome === 'not_submitted') {
          await expect(workspace).toContainText('No formal submission was recorded by the cutoff.')
          await expect(workspace.getByRole('button', { name: 'Save private draft', exact: true })).toBeDisabled()
          await expect(workspace.getByRole('button', { name: 'Submit formal version', exact: true })).toBeDisabled()
          await page.screenshot({ path: info.outputPath('native-cutoff-before-lifecycle-transition.png'), fullPage: true })
        }
      })
    }
  } finally {
    await context.close()
    await control('stage-live')
    await control('clock', { instant: '2027-03-29T09:00:00Z' })
  }
})

import { test, expect } from '@playwright/test'
import { actor, control, native } from './native'
import { openEventSection } from '../e2e/event-navigation-fixture'

test('isolated Today and team dashboard follow monotonic preparation release live and cutoff while lifecycle lags', async ({ browser }, info) => {
  const context = await actor(browser, 'public', 'competitor', 'phase'), page = await context.newPage()
  const mentor = await actor(browser, 'public', 'mentor', 'phase')
  const fixture = native.phase
  const formalWrites: string[] = []
  page.on('request', request => { if (request.method() === 'POST' && new URL(request.url()).pathname.endsWith('/submission')) formalWrites.push(request.url()) })
  const base = `/api/v1/events/${fixture.event_id}`
  try {
    const phases = [
      { name: 'preparation', instant: '2027-03-22T09:30:00Z', draft: false, submit: false, outcome: 'open' },
      { name: 'released build week', instant: '2027-03-22T10:00:00Z', draft: true, submit: false, outcome: 'open' },
      { name: 'live opening', instant: '2027-03-29T09:00:00Z', draft: true, submit: true, outcome: 'open' },
      { name: 'exact final cutoff', instant: '2027-03-29T16:00:00Z', draft: false, submit: false, outcome: 'not_submitted' },
    ]
    for (const phase of phases) {
      await test.step(phase.name, async () => {
        await control('clock', { fixture: 'phase', instant: phase.instant })
        if (phase.name === 'released build week') await control('release-phase-challenge', { fixture: 'phase' })
        const event = await context.request.get(base)
        expect(event.status()).toBe(200)
        const currentEvent = await event.json()
        expect(currentEvent.stage).toBe('preparation')
        if (phase.name === 'preparation') expect(currentEvent.published_at).toBeNull()
        else expect(currentEvent.published_at).toBe('2027-03-22T10:00:00Z')
        const submission = await context.request.get(`${base}/teams/${fixture.team_id}/submission`)
        expect(submission.status()).toBe(200)
        const current = await submission.json()
        expect(current.can_draft).toBe(phase.draft)
        expect(current.can_submit).toBe(phase.submit)
        expect(current.outcome).toBe(phase.outcome)
        const observation = await mentor.request.get(`${base}/teams/${fixture.team_id}/mentor-observations`)
        expect(observation.status()).toBe(200)
        const review = await observation.json()
        expect(review.can_record).toBe(phase.name === 'live opening' || phase.name === 'exact final cutoff')
        expect(review.can_change_status).toBe(true)
        if (phase.outcome === 'not_submitted') expect(current.frozen_at).not.toBeNull()
        const teams = await context.request.get(`${base}/teams`)
        expect(teams.status()).toBe(200)
        const allocation = await teams.json()
        expect(allocation.frozen).toBe(true)
        // can_create is native actor eligibility; frozen is the separate gate.
        // Prove the actual write is refused rather than reinterpreting the flag.
        const session = await (await context.request.get('/api/v1/session')).json()
        const mutation = await context.request.post(`${base}/teams`, { headers: { Origin: fixture.public, 'X-CSRF-Token': session.csrf_token, 'If-Match': `"${allocation.revision}"` }, data: { action: 'create', name: 'Must never be created after freeze', reason: 'Synthetic frozen gate probe' } })
        expect(mutation.ok()).toBe(false)
        expect((await mutation.json()).error.code).toBe('teams_frozen')
        expect(allocation.teams.map((team: { id: string }) => team.id)).toEqual([fixture.team_id])
        await page.goto(`/events/${fixture.event_id}`)
        await openEventSection(page, 'Today')
        const lifecycle = page.getByRole('region', { name: 'Separate event lifecycle states', exact: true })
        await expect(lifecycle).toContainText('event stage: preparation')
        await expect(lifecycle).toContainText(phase.name === 'preparation' ? 'Common release not recorded' : 'Actual common release recorded')
        const formal = lifecycle.getByRole('row').filter({ has: page.getByRole('rowheader', { name: 'Live formal submissions', exact: true }) })
        await expect(formal).toContainText(phase.submit ? 'formal submission open for your team' : 'formal submission not open for your team')
        await expect(page.getByRole('region', { name: 'Separate personal and team readiness', exact: true })).toContainText(`Submission outcome: ${phase.outcome}.`)
        if (phase.name === 'preparation' || phase.name === 'live opening') await page.screenshot({ path: info.outputPath(`native-isolated-today-${phase.name.replaceAll(' ', '-')}.png`), fullPage: true })
        // Follow the actual next action, rather than only asserting API flags.
        await page.getByRole('button', { name: 'Open team drafts and formal deliverables', exact: true }).click()
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
    expect(formalWrites).toEqual([])
  } finally {
    await context.close()
    await mentor.close()

  }
})

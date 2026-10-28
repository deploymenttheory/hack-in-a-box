import type { JudgingSetup } from '../judging-setup-model'
import { publicChallengeRubric } from '../challenge-rubric-model'
import { rubricGuideConfiguration } from './rubric-fixture'

export function judgingSetupGuideSource(withRound = false): JudgingSetup {
  const configuration = rubricGuideConfiguration()
  configuration.reference = { id: '11111111-1111-4111-8111-111111111111', version: 1, digest: 'a'.repeat(64) }
  configuration.revision = 2; configuration.approved = true
  const run = { id: '22222222-2222-4222-8222-222222222222', reference: configuration.reference, purpose: 'judge_final' as const, started_at: null, previous_id: null }
  return {
    event: { id: '33333333-3333-4333-8333-333333333333', name: 'Synthetic final judging', slug: 'synthetic-judging', owner_id: null, revision: 7, stage: 'preparation', configuration: { description: 'Synthetic local guide only.', timezone: 'Europe/London', venue: '', capacity: 60, minimum_attendance: 30, live_start: '2027-06-20T09:00:00Z', live_end: '2027-06-20T17:00:00Z', admission_cutoff: '2027-06-12T09:00:00Z', team_freeze: '2027-06-12T09:00:00Z', submission_cutoff: '2027-06-20T16:00:00Z', dst_choice: '', calendar: { working_days: [1, 2, 3, 4, 5], excluded_dates: [] }, schedule: [] }, challenge_publication: '2027-06-13T09:00:00Z', published_at: '2027-06-13T09:00:00Z', confirmed_attendees: 30, blockers: [], can_present: false, can_edit: true, can_manage_owner: false, can_read_staff: true, can_transition: true },
    configuration: { revision: configuration.revision, reference: configuration.reference, runs: withRound ? [run] : [] },
    approved: publicChallengeRubric(configuration), criteria: withRound ? publicChallengeRubric(configuration) : undefined,
    teams: { revision: 7, can_manage: true, can_create: false, frozen: true, freeze_at: '2027-06-12T09:00:00Z', teams: ['Aurora', 'Beacon'].map((name, index) => ({ id: `44444444-4444-4444-8444-44444444444${index}`, name, state: 'published', frozen: true, captain: null, published_captain: null, invitation_code: null, draft: [], published: [] })), people: [], requests: [] },
    staff: { items: ['Morgan', 'Sam', 'Alex'].map((display_name, index) => ({ person_id: `55555555-5555-4555-8555-55555555555${index}`, display_name, role: 'judge', active: true, changed_at: '2027-06-01T09:00:00Z' })) },
    run: withRound ? run : undefined,
    round: withRound ? { run_id: run.id, revision: 0, deliberation: false, required_judges: 3, assignments: [] } : undefined,
  }
}

import { judgingSetupGuideSource } from './judging-setup-fixture'
import type { MentorCheckpointPlanning } from '../mentor-checkpoint-planning-model'
export function mentorCheckpointGuideSource(withRun = true): MentorCheckpointPlanning {
  const guide = judgingSetupGuideSource(true)
  guide.event.configuration.timezone = 'UTC'
  guide.approved!.definition.mode = 'mentor_qualification'; guide.approved!.definition.finalist_count = 2; guide.approved!.definition.reserve_count = 1
  return { event: guide.event, configuration: { ...guide.configuration, runs: withRun ? [{ ...guide.run!, purpose: 'checkpoint' }] : [] }, approved: guide.approved, checkpoints: { revision: guide.event.revision, items: [], next_cursor: null } }
}

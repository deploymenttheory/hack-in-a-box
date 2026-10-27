import type { RubricConfiguration, RubricDefinition } from '../rubric-planning-fields'

export function rubricGuideDefinition(): RubricDefinition {
  return {
    name: 'Synthetic common rubric', mode: 'all_teams', required_judge_count: 3, finalist_count: 0, reserve_count: 0,
    categories: ['Demo', 'Technical approach', 'UX', 'Feature fit', 'Innovation'].map((label, index) => ({ id: `00000000-0000-4000-8000-00000000000${index + 1}`, label, min_score: 1, max_score: 10, weight: 1, guidance: `Use demonstrated ${label.toLowerCase()} evidence.`, anchors: [{ score: 1, description: 'Material evidence missing.' }, { score: 5, description: 'Working path with limitations.' }, { score: 10, description: 'Coherent demonstrated outcome.' }] })),
    prizes: [], side_quests: [], private_judge_guidance: 'Synthetic private guidance. Never use this fixture as native authority.',
  }
}
export function rubricGuideConfiguration(): RubricConfiguration {
  return { revision: 0, reference: { id: '00000000-0000-0000-0000-000000000000', version: 0, digest: '' }, definition: rubricGuideDefinition(), approved: false, published_version: null, runs: [] }
}

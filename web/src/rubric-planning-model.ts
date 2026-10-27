import type { RubricDefinition, RubricConfiguration } from './rubric-planning-fields'

// Property order in a JSON response cannot change the meaning of a reviewed
// definition. Category/anchor/award ordering remains part of the authored draft.
export function rubricDefinitionKey(value: RubricDefinition) {
  return JSON.stringify([
    value.name, value.mode, value.required_judge_count, value.finalist_count, value.reserve_count,
    value.categories.map(category => [category.id, category.label, category.min_score, category.max_score, category.weight, category.guidance, category.anchors.map(anchor => [anchor.score, anchor.description])]),
    value.prizes.map(prize => [prize.id, prize.name, prize.description, prize.category_id]),
    value.side_quests.map(quest => [quest.id, quest.title, quest.evidence_guidance]), value.private_judge_guidance,
  ])
}

export function rubricReferenceKey(value: RubricConfiguration['reference']) {
  return JSON.stringify([value.id, value.version, value.digest])
}

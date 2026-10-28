import type { components } from './api/schema.gen'
import { rubricReferenceKey } from './rubric-planning-model'

type Schema = components['schemas']
export type AssessmentPurpose = 'checkpoint' | 'mentor_final' | 'judge_final'
export type AssessmentConfiguration = Pick<Schema['RubricConfiguration'], 'revision' | 'reference' | 'runs'>
const runKey = (run: Schema['RubricRun']) => JSON.stringify([run.id, rubricReferenceKey(run.reference), run.purpose, run.started_at, run.previous_id])

// The original response must retain every earlier run and add exactly the
// reviewed source/purpose. A later current read cannot settle an unknown write.
export function createdAssessmentRun(before: AssessmentConfiguration, after: Schema['RubricConfiguration'], chosen: Schema['RubricReference'], purpose: AssessmentPurpose) {
  const added = after.runs.filter(run => !before.runs.some(old => old.id === run.id))
  return after.revision === before.revision + 1 && rubricReferenceKey(after.reference) === rubricReferenceKey(before.reference)
    && after.runs.length === before.runs.length + 1 && before.runs.every(old => after.runs.some(run => runKey(run) === runKey(old)))
    && added.length === 1 && /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(added[0].id)
    && added[0].purpose === purpose && added[0].started_at === null && added[0].previous_id === null
    && rubricReferenceKey(added[0].reference) === rubricReferenceKey(chosen) ? added[0] : undefined
}

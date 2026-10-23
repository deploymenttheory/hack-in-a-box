import { canonical, type SurveyConfiguration } from './survey-context'

// Question identity is stable when a reviewed definition is deliberately reused.
// Include prior conditional context and the exact measure reference; a matching
// label or a coincidentally equal numeric version can never imply equivalence.
export function equivalentQuestion(a: SurveyConfiguration, b: SurveyConfiguration, id: string, measureA?: unknown, measureB?: unknown): boolean {
  function signature(value: SurveyConfiguration) {
    const form = value.definition.form, index = form.fields.findIndex(field => field.id === id)
    if (index < 0) return undefined
    return { audience: form.audience, fields: form.fields.slice(0, index + 1).map(field => ({ id: field.id, label: field.label, type: field.type, required: field.required, help: field.help, section: field.section ?? '', choices: field.choices ?? [], visible_when: field.visible_when ?? null })), measure: value.definition.measures.find(m => m.question_id === id) ?? null }
  }
  const first = signature(a), second = signature(b)
  if (!first || !second || canonical(first) !== canonical(second)) return false
  if (first.measure !== null) return measureA !== undefined && measureB !== undefined && canonical(measureA) === canonical(measureB)
  return true
}

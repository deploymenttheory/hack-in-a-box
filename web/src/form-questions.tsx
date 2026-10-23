import { Input, Select, Label } from './components/ui/fields'
import { z } from 'zod'
import type { components } from './api/schema.gen'

export type FormDefinition = components['schemas']['TemplateForm']
export type Answers = Record<string, unknown>
export function formDefinitionErrors(d: FormDefinition) {
  const errors: string[] = [], previous = new Map<string, FormDefinition['fields'][number]>()
  if (!d.name.trim()) errors.push('Enter a form name.')
  for (const field of d.fields) {
    if (!field.label.trim()) errors.push('Every question needs a label.')
    if (['single', 'multiple'].includes(field.type) && (!field.choices.length || field.choices.some(c => !c.trim()) || new Set(field.choices).size !== field.choices.length)) errors.push(`${field.label}: provide distinct, non-empty choices.`)
    if (field.visible_when) {
      const parent = previous.get(field.visible_when.field_id), value = field.visible_when.equals
      if (!parent || (['single', 'multiple'].includes(parent.type) && !parent.choices.includes(value)) || (parent.type === 'checkbox' && !['true', 'false'].includes(value)) || (parent.type === 'rating' && !/^(?:[1-9]|10)$/.test(value))) errors.push(`${field.label}: the visibility rule must match a possible answer to an earlier question.`)
    }
    previous.set(field.id, field)
  }
  return errors
}
export const formEditorSchema = z.object({ definition: z.custom<FormDefinition>().superRefine((d, ctx) => { for (const message of formDefinitionErrors(d)) ctx.addIssue({ code: 'custom', message }) }) })
export function visibleFields(d: FormDefinition, answers: Answers) {
  const visible = new Set<string>()
  return d.fields.filter(field => {
    const rule = field.visible_when
    if (rule) {
      if (!visible.has(rule.field_id)) return false
      const answer = answers[rule.field_id]
      if (Array.isArray(answer) ? !answer.includes(rule.equals) : answer === undefined || String(answer) !== rule.equals) return false
    }
    visible.add(field.id); return true
  })
}
export function FormQuestions({ definition, answers, change, issues = [], showOptional = false }: { definition: FormDefinition; answers: Answers; change: (answers: Answers) => void; issues?: components['schemas']['Blocker'][]; showOptional?: boolean }) {
  return <>{visibleFields(definition, answers).map((field, index, fields) => {
    const value = answers[field.id], id = `answer-${field.id}`, set = (v: unknown) => change({ ...answers, [field.id]: v }), errors = issues.filter(i => i.field === field.id)
    return <div key={field.id}>{field.section && fields[index-1]?.section !== field.section && <h3>{field.section}</h3>}<Label htmlFor={id}>{field.label}{field.required ? ' (required)' : showOptional ? ' (optional)' : ''}</Label>{field.help && <p id={`${id}-help`} className="muted">{field.help}</p>}
      {field.type === 'text' ? <Input aria-invalid={errors.length > 0 || undefined} id={id} value={typeof value === 'string' ? value : ''} onChange={e => set(e.target.value)} aria-describedby={`${id}-help ${id}-errors`} /> : field.type === 'single' ? <Select aria-invalid={errors.length > 0 || undefined} id={id} value={typeof value === 'string' ? value : ''} onChange={e => set(e.target.value)}><option value="">Choose an option</option>{field.choices.map(c => <option key={c}>{c}</option>)}</Select> : field.type === 'multiple' ? <fieldset id={id}><legend>{field.label}</legend>{field.choices.map(c => <Label className="check" key={c}><Input type="checkbox" checked={Array.isArray(value) && value.includes(c)} onChange={e => set(e.target.checked ? [...(Array.isArray(value) ? value : []), c] : (Array.isArray(value) ? value : []).filter(x => x !== c))} />{c}</Label>)}</fieldset> : field.type === 'checkbox' ? <Select aria-invalid={errors.length > 0 || undefined} id={id} value={value === undefined ? '' : String(value)} onChange={e => set(e.target.value === '' ? undefined : e.target.value === 'true')}><option value="">Choose yes or no</option><option value="true">Yes</option><option value="false">No</option></Select> : <Select aria-invalid={errors.length > 0 || undefined} id={id} value={typeof value === 'number' ? value : ''} onChange={e => set(e.target.value ? Number(e.target.value) : undefined)}><option value="">Choose a rating</option>{Array.from({ length: 10 }, (_, i) => <option key={i + 1} value={i + 1}>{i + 1}</option>)}</Select>}
      <div id={`${id}-errors`}>{errors.map(e => <p className="field-error" role="alert" key={e.code}>{e.message}</p>)}</div>
    </div>
  })}</>
}

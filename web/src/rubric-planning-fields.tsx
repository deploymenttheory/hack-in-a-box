import { useId, useRef, type ReactNode } from 'react'
import type { components } from './api/schema.gen'
import { Button } from './components/ui/button'
import { Field, Input, Select, Textarea } from './components/ui/fields'
import { Icon } from './components/ui/icon'
import { PlanningEditorSections, PlanningStep } from './planning-layout'

export type RubricDefinition = components['schemas']['PrivateRubricDefinition']
export type RubricConfiguration = components['schemas']['RubricConfiguration']
type Category = RubricDefinition['categories'][number]

export function revealRubricInvalidField(field: HTMLElement) {
  const details = field.closest('details')
  if (details) details.open = true
  requestAnimationFrame(() => field.focus())
}
export function validateRubricPlanningForm(form: HTMLFormElement | null) {
  if (!form?.reportValidity()) return false
  const invalid = form.querySelector<HTMLElement>('[aria-invalid="true"]')
  if (!invalid) return true
  invalid.dispatchEvent(new Event('invalid', { bubbles: false, cancelable: true }))
  revealRubricInvalidField(invalid)
  return false
}

function NumberField({ label, value, min, max, change, problem }: { label: string; value: number; min: number; max: number; problem?: string; change: (value: number) => void }) {
  const invalid = !Number.isInteger(value) || value < min || value > max
  return <Field label={label} error={problem ?? (invalid ? `Enter a whole number from ${min} to ${max}.` : undefined)}><Input type="number" required min={Number.isFinite(min) ? min : 0} max={Number.isFinite(max) ? max : 1000} step={1} value={Number.isNaN(value) ? '' : value} onChange={event => change(event.target.valueAsNumber)} /></Field>
}
function Words({ label, value, change, required = false, max = 10000 }: { label: string; value: string; change: (value: string) => void; required?: boolean; max?: number }) {
  return <Field label={label} error={required && !value.trim() ? 'Enter a value.' : undefined}>{max <= 160 ? <Input required={required} maxLength={max} value={value} onChange={event => change(event.target.value)} /> : <Textarea required={required} maxLength={max} value={value} onChange={event => change(event.target.value)} />}</Field>
}
function Part({ title, children }: { title: string; children: ReactNode }) {
  return <section className="grid gap-4 border-t border-border py-5"><h3>{title}</h3>{children}</section>
}

// The guide and the portal render these same controls. IDs remain stable when
// labels, order, bounds or guidance change; only an explicit Add creates an ID.
export function RubricPlanningFields({ value, change, disabled = false }: { value: RubricDefinition; change: (value: RubricDefinition) => void; disabled?: boolean }) {
  const categoryGroup = useId(), categoryAdd = useRef<HTMLButtonElement>(null)
  const updateCategory = (category: Category) => change({ ...value, categories: value.categories.map(old => old.id === category.id ? category : old) })
  return <fieldset disabled={disabled} className="min-w-0 border-0 p-0">
    <legend className="fleet-visually-hidden">Rubric definition</legend>
    <PlanningEditorSections label="Rubric definition sections">
    <PlanningStep title="Model"><Part title="Assessment model">
      <Words label="Rubric name" value={value.name} change={name => change({ ...value, name })} required max={160} />
      <Field label="Judging mode" description="Changing mode creates a new draft; it does not reinterpret an existing assessment run."><Select value={value.mode} onChange={event => { const mode = event.target.value as RubricDefinition['mode']; change({ ...value, mode, finalist_count: mode === 'mentor_qualification' ? value.finalist_count || 3 : 0, reserve_count: mode === 'mentor_qualification' ? value.reserve_count || 1 : 0 }) }}><option value="all_teams">All eligible teams</option><option value="mentor_qualification">Mentor qualification, then finalists</option></Select></Field>
      <div className="grid gap-4 sm:grid-cols-3"><NumberField label="Required judges" value={value.required_judge_count} min={1} max={50} change={required_judge_count => change({ ...value, required_judge_count })} />{value.mode === 'mentor_qualification' && <><NumberField label="Finalist count" value={value.finalist_count} min={1} max={100} change={finalist_count => change({ ...value, finalist_count })} /><NumberField label="Reserve count" value={value.reserve_count} min={0} max={100} change={reserve_count => change({ ...value, reserve_count })} /></>}</div>
    </Part></PlanningStep>
    <PlanningStep title="Categories"><Part title="Scoring categories">
      <p>Each category has its own bounds, weight and evidence guidance. Edit its descriptive anchors before reviewing the draft.</p>
      <div>{value.categories.map((category, index) => <details className="border-b border-border py-3" key={category.id} name={categoryGroup}><summary className="flex cursor-pointer items-center gap-3"><Icon name="list" /><strong>{category.label || 'Unnamed category'}</strong><span>{category.min_score}–{category.max_score} · Weight {category.weight}</span></summary><div className="grid gap-4 py-4">
        <Words label={`Category ${index + 1} name`} value={category.label} required max={160} change={label => updateCategory({ ...category, label })} />
        <div className="grid gap-4 sm:grid-cols-3"><NumberField label={`Minimum score for category ${index + 1}`} value={category.min_score} min={0} max={999} change={min_score => updateCategory({ ...category, min_score })} /><NumberField label={`Maximum score for category ${index + 1}`} value={category.max_score} min={category.min_score + 1} max={1000} change={max_score => updateCategory({ ...category, max_score })} /><NumberField label={`Weight for category ${index + 1}`} value={category.weight} min={1} max={1000} change={weight => updateCategory({ ...category, weight })} /></div>
        <Words label={`Public evidence guidance for category ${index + 1}`} value={category.guidance} change={guidance => updateCategory({ ...category, guidance })} />
        <h4>Descriptive score anchors</h4>
        {category.anchors.map((anchor, position) => <div className="grid gap-3 border-l border-border pl-4" key={position}><NumberField label={`Anchor ${position + 1} score for category ${index + 1}`} problem={category.anchors.some((other, i) => i !== position && other.score === anchor.score) ? 'Anchor scores must be unique within this category.' : undefined} value={anchor.score} min={category.min_score} max={category.max_score} change={score => updateCategory({ ...category, anchors: category.anchors.map((old, i) => i === position ? { ...old, score } : old) })} /><Words label={`Anchor ${position + 1} description for category ${index + 1}`} value={anchor.description} required max={2000} change={description => updateCategory({ ...category, anchors: category.anchors.map((old, i) => i === position ? { ...old, description } : old) })} /><Button type="button" icon="remove" variant="ghost" onClick={() => updateCategory({ ...category, anchors: category.anchors.filter((_, i) => i !== position) })}>Remove anchor {position + 1} from category {index + 1}</Button></div>)}
        {new Set(category.anchors.map(anchor => anchor.score)).size !== category.anchors.length && <p role="alert">Anchor scores in {category.label} must be unique.</p>}
        <div className="fleet-pattern-actions"><Button type="button" icon="add" variant="outline" disabled={category.anchors.length >= 30} onClick={() => { if (!Number.isInteger(category.min_score) || !Number.isInteger(category.max_score) || category.min_score < 0 || category.max_score > 1000 || category.max_score <= category.min_score) return; let score = category.min_score; while (score <= category.max_score && category.anchors.some(anchor => anchor.score === score)) score++; if (score <= category.max_score) updateCategory({ ...category, anchors: [...category.anchors, { score, description: '' }] }) }}>Add anchor to category {index + 1}</Button><Button type="button" icon="remove" variant="ghost" disabled={value.categories.length <= 1 || value.prizes.some(prize => prize.category_id === category.id)} onClick={() => { change({ ...value, categories: value.categories.filter(item => item.id !== category.id) }); requestAnimationFrame(() => categoryAdd.current?.focus()) }}>Remove category {index + 1}</Button></div>
        {value.prizes.some(prize => prize.category_id === category.id) && <p>Reassign this category’s prizes before removing it.</p>}
      </div></details>)}</div>
      <Button ref={categoryAdd} type="button" icon="add" variant="outline" disabled={value.categories.length >= 20} onClick={() => change({ ...value, categories: [...value.categories, { id: crypto.randomUUID(), label: 'New category', min_score: 1, max_score: 10, weight: 1, guidance: '', anchors: [] }] })}>Add scoring category</Button>
    </Part></PlanningStep>
    <PlanningStep title="Awards"><Part title="Declared awards">
      <p>Awards describe recognition. They do not execute payments or add score bonuses.</p>
      {value.prizes.map((prize, index) => <div className="grid gap-4 border-b border-border pb-4" key={prize.id}><Words label={`Award ${index + 1} name`} value={prize.name} required max={160} change={name => change({ ...value, prizes: value.prizes.map(old => old.id === prize.id ? { ...old, name } : old) })} /><Words label={`Award ${index + 1} description`} value={prize.description} change={description => change({ ...value, prizes: value.prizes.map(old => old.id === prize.id ? { ...old, description } : old) })} /><Field label={`Award ${index + 1} category`}><Select value={prize.category_id ?? ''} onChange={event => change({ ...value, prizes: value.prizes.map(old => old.id === prize.id ? { ...old, category_id: event.target.value || null } : old) })}><option value="">Overall award</option>{value.categories.map(category => <option key={category.id} value={category.id}>{category.label}</option>)}</Select></Field><Button type="button" icon="remove" variant="ghost" onClick={() => change({ ...value, prizes: value.prizes.filter(old => old.id !== prize.id) })}>Remove award {index + 1}</Button></div>)}
      <Button type="button" icon="add" variant="outline" disabled={value.prizes.length >= 50} onClick={() => change({ ...value, prizes: [...value.prizes, { id: crypto.randomUUID(), name: 'New award', description: '', category_id: null }] })}>Add declared award</Button>
    </Part></PlanningStep>
    <PlanningStep title="Side quests"><Part title="Optional evidence side quests">
      <p>Side quests provide evidence only. There are no automatic bonus points.</p>
      {value.side_quests.map((quest, index) => <div className="grid gap-4 border-b border-border pb-4" key={quest.id}><Words label={`Side quest ${index + 1} title`} value={quest.title} required max={160} change={title => change({ ...value, side_quests: value.side_quests.map(old => old.id === quest.id ? { ...old, title } : old) })} /><Words label={`Side quest ${index + 1} evidence guidance`} value={quest.evidence_guidance} change={evidence_guidance => change({ ...value, side_quests: value.side_quests.map(old => old.id === quest.id ? { ...old, evidence_guidance } : old) })} /><Button type="button" icon="remove" variant="ghost" onClick={() => change({ ...value, side_quests: value.side_quests.filter(old => old.id !== quest.id) })}>Remove side quest {index + 1}</Button></div>)}
      <Button type="button" icon="add" variant="outline" disabled={value.side_quests.length >= 50} onClick={() => change({ ...value, side_quests: [...value.side_quests, { id: crypto.randomUUID(), title: 'New side quest', evidence_guidance: '' }] })}>Add evidence side quest</Button>
    </Part></PlanningStep>
    <PlanningStep title="Private guidance"><Part title="Private judge guidance"><p>This guidance belongs to the private rubric snapshot. It is excluded from the common attendee projection.</p><Words label="Private judge guidance" value={value.private_judge_guidance} change={private_judge_guidance => change({ ...value, private_judge_guidance })} /></Part></PlanningStep>
    </PlanningEditorSections>
  </fieldset>
}

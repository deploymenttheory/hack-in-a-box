import { Input, Textarea, Select, Label } from './components/ui/fields'
import { useId } from 'react'
import type { components } from './api/schema.gen'
import { Button } from './components/ui/button'

export type ObjectiveDefinition = components['schemas']['ObjectiveDefinition']
export type ObjectiveConfiguration = components['schemas']['ObjectiveConfiguration']
export type ObjectiveOwner = components['schemas']['ObjectiveOwner']
const decimal = /^-?(?:0|[1-9][0-9]{0,17})(?:\.[0-9]{1,12})?$/
export function objectiveOwnerLabel(id: string | null, owners: ObjectiveOwner[]): string { if (!id) return 'No owner selected'; const found = owners.find(o => o.id === id); if (!found) return 'Previously selected owner · currently unavailable'; return owners.filter(o => o.display_name === found.display_name).length > 1 ? `${found.display_name} · ${id.slice(0, 8)}` : found.display_name }
export const emptyObjective = (): ObjectiveDefinition => ({ description: '', owner_id: null, theme_sensitive: false, measures: [] })
export const emptyMeasure = (): ObjectiveDefinition['measures'][number] => ({ id: crypto.randomUUID(), description: '', baseline: null, target: null, unit: '', method: '', evidence_source: '', review_horizon: '' })

export function objectiveProblems(d: ObjectiveDefinition): string[] {
  const problems: string[] = []
  if (!d.description.trim()) problems.push('Enter an objective description.')
  d.measures.forEach((m, index) => {
    const prefix = `Measure ${index + 1}`
    if (!m.description.trim()) problems.push(`${prefix}: enter a description.`)
    if (m.target !== null && !decimal.test(m.target)) problems.push(`${prefix}: enter a target as a decimal number, or leave it unknown.`)
    if (m.baseline !== null && !decimal.test(m.baseline.value)) problems.push(`${prefix}: enter a baseline as a decimal number, or leave the baseline unknown.`)
  })
  return problems
}

function Text({ label, value, change, multiline = false, maxLength, decimalInput = false }: { label: string; value: string; change: (value: string) => void; multiline?: boolean; maxLength?: number; decimalInput?: boolean }) {
  const id = useId()
  return <div><Label htmlFor={id}>{label}</Label>{multiline ? <Textarea id={id} value={value} maxLength={maxLength} onChange={e => change(e.target.value)} /> : <Input id={id} value={value} maxLength={maxLength} inputMode={decimalInput ? 'decimal' : undefined} onChange={e => change(e.target.value)} />}</div>
}

export function ObjectiveFields({ value: d, change, owners }: { value: ObjectiveDefinition; change: (value: ObjectiveDefinition) => void; owners?: ObjectiveOwner[] }) {
  const ownerID = useId()
  return <div className="objective-fields">
    <Text label="Objective description" value={d.description} change={description => change({ ...d, description })} multiline maxLength={10000} />
    {owners ? <div><Label htmlFor={ownerID}>Objective owner (optional)</Label><Select id={ownerID} value={d.owner_id ?? ''} onChange={e => change({ ...d, owner_id: e.target.value || null })}><option value="">No owner selected</option>{d.owner_id && !owners.some(o => o.id === d.owner_id) && <option value={d.owner_id}>Previously selected owner · currently unavailable</option>}{owners.map(o => <option key={o.id} value={o.id}>{objectiveOwnerLabel(o.id, owners)}</option>)}</Select></div> : <p>Choose a current organiser owner in each event. Person owners are cleared when configuration is cloned.</p>}
    <Label className="check"><Input type="checkbox" checked={d.theme_sensitive} onChange={e => change({ ...d, theme_sensitive: e.target.checked })} />This objective reveals the hackathon theme</Label>
    <p>Planning text stays organiser-only. Disclosure requires selecting exact objective and measure versions in approved native content. Theme-sensitive objectives cannot be disclosed before the common challenge release.</p>
    <section aria-label="Success measures"><h3>Success measures</h3><p>A target is a planning goal. Baselines are labelled estimates or self-reports, and an unknown value stays unknown. These fields do not record demonstrated achieved results.</p>
      {d.measures.map((m, index) => {
        const update = (next: typeof m) => change({ ...d, measures: d.measures.map(old => old.id === m.id ? next : old) })
        return <fieldset key={m.id}><legend>Measure {index + 1}</legend>
          <Text label="Measure description" value={m.description} change={description => update({ ...m, description })} multiline maxLength={4000} />
          <div className="form-grid"><Text label="Target (optional; blank means unknown)" value={m.target ?? ''} change={target => update({ ...m, target: target === '' ? null : target })} decimalInput /><Text label="Unit" value={m.unit} change={unit => update({ ...m, unit })} maxLength={160} /></div>
          <Label className="check"><Input type="checkbox" checked={m.baseline !== null} onChange={e => update({ ...m, baseline: e.target.checked ? { value: '', basis: 'estimate' } : null })} />Record a planning baseline</Label>
          {m.baseline ? <div className="form-grid"><Text label="Baseline value" value={m.baseline.value} change={value => update({ ...m, baseline: { ...m.baseline!, value } })} decimalInput /><Label>Baseline basis<Select value={m.baseline.basis} onChange={e => update({ ...m, baseline: { ...m.baseline!, basis: e.target.value as 'estimate' | 'self_report' } })}><option value="estimate">Estimate</option><option value="self_report">Self-report</option></Select></Label></div> : <p className="muted">Baseline unknown. This is not zero.</p>}
          <Text label="Measurement method" value={m.method} change={method => update({ ...m, method })} multiline maxLength={4000} />
          <Text label="Where the review should obtain evidence" value={m.evidence_source} change={evidence_source => update({ ...m, evidence_source })} multiline maxLength={4000} />
          <Text label="Review horizon" value={m.review_horizon} change={review_horizon => update({ ...m, review_horizon })} maxLength={1000} />
          <p className="muted">Evidence instructions are not a verified result or an automatic review schedule.</p>
          <Button type="button" variant="outline" onClick={() => change({ ...d, measures: d.measures.filter(old => old.id !== m.id) })}>Remove measure {index + 1}</Button>
        </fieldset>
      })}
      <Button type="button" variant="outline" disabled={d.measures.length >= 50} onClick={() => change({ ...d, measures: [...d.measures, emptyMeasure()] })}>Add success measure</Button>
    </section>
  </div>
}

export function ObjectiveTemplateFields({ value, change }: { value: ObjectiveConfiguration[]; change: (next: ObjectiveConfiguration[]) => void }) {
  return <section aria-label="Template objective definitions"><p>Optional planning configuration is copied by value. Achieved results, approvals, disclosures and follow-up history are never copied.</p>{value.map((item, index) => <fieldset key={item.id}><legend>Objective {index + 1}</legend><ObjectiveFields value={item.definition} change={definition => change(value.map(old => old.id === item.id ? { ...old, definition: { ...definition, owner_id: null } } : old))} /><Button type="button" variant="outline" onClick={() => change(value.filter(old => old.id !== item.id))}>Remove objective {index + 1}</Button>{objectiveProblems(item.definition).map(message => <p key={message} className="notice">{message}</p>)}</fieldset>)}<Button type="button" variant="outline" disabled={value.length >= 100} onClick={() => change([...value, { id: crypto.randomUUID(), definition: emptyObjective() }])}>Add objective</Button></section>
}

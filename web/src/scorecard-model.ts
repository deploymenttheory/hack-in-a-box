import type { components } from './api/schema.gen'
export type Card = components['schemas']['MentorScorecard']
export type Checkpoint = components['schemas']['ScorecardCheckpoint']
export type Score = components['schemas']['ScorecardCategory']
export type Evidence = components['schemas']['ScorecardEvidence']
export type Command = components['schemas']['ScorecardCommand']
export type Entry = { categoryID: string; pending: boolean; value: string; note: string; references: Evidence[] }
export type Issue = { categoryID: string; message: string }
export function entries(checkpoint: Checkpoint, card?: Card): Entry[] {
  return checkpoint.criteria.categories.map(category => {
    const saved = card?.scores.find(s => s.category_id === category.id)
    const firstNote = saved?.evidence.findIndex(e => e.kind === 'note') ?? -1
    return { categoryID: category.id, pending: saved?.pending ?? true, value: saved?.value == null ? '' : String(saved.value), note: firstNote < 0 ? '' : saved!.evidence[firstNote].note, references: saved?.evidence.filter((_, i) => i !== firstNote) ?? [] }
  })
}
export function validate(checkpoint: Checkpoint, values: Entry[], submit: boolean): Issue[] {
  const issues: Issue[] = []
  for (const category of checkpoint.criteria.categories) {
    const entry = values.find(e => e.categoryID === category.id)
    if (!entry) { issues.push({ categoryID: category.id, message: 'Choose Pending/unobserved or explicitly enter a score.' }); continue }
    if (!entry.pending && (entry.value.trim() === '' || !Number.isInteger(Number(entry.value)) || Number(entry.value) < category.min_score || Number(entry.value) > category.max_score)) issues.push({ categoryID: category.id, message: `Enter a whole score from ${category.min_score} to ${category.max_score}, or mark this category Pending/unobserved.` })
    if (new TextEncoder().encode(entry.note).byteLength > 10000) issues.push({ categoryID: category.id, message: 'Evidence note exceeds the native 10,000-byte limit; shorten it without losing your entered text.' })
    if (entry.references.length + (entry.note.trim() ? 1 : 0) > 50) issues.push({ categoryID: category.id, message: 'Use at most 50 evidence references for this category.' })
    if (submit && !entry.pending && !entry.note.trim() && !entry.references.length) issues.push({ categoryID: category.id, message: 'Add evidence before submitting this scored category. Your entered score and notes remain unsent.' })
  }
  return issues
}
export function scores(values: Entry[]): Score[] {
  return values.map(e => ({ category_id: e.categoryID, pending: e.pending, value: e.pending ? null : Number(e.value), evidence: [...(e.note.trim() ? [{ kind: 'note' as const, id: null, version: null, note: e.note }] : []), ...e.references] }))
}
export function canonicalScores(values: Score[]) {
  return JSON.stringify([...values].sort((a, b) => a.category_id.localeCompare(b.category_id)).map(s => ({ category_id: s.category_id, pending: s.pending, value: s.value, evidence: s.evidence.map(e => ({ kind: e.kind, id: e.id ?? null, version: e.version ?? null, note: e.note })) })))
}
export type Intent = { revision: number; command: Command; authorID: string; teamID: string; checkpoint: Checkpoint; expectedVersion: number; frozenRevision: number | null }
export function matchesReceipt(card: Card, intent: Intent) {
  return card.revision === intent.revision + 1 && card.author_id === intent.authorID && card.team_id === intent.teamID && card.checkpoint_id === intent.command.checkpoint_id && card.checkpoint_version === intent.command.checkpoint_version && card.reference.version === intent.expectedVersion && card.run_id === intent.checkpoint.definition.run_id && card.rubric.id === intent.checkpoint.rubric.id && card.rubric.version === intent.checkpoint.rubric.version && card.rubric.digest === intent.checkpoint.rubric.digest && card.purpose === intent.checkpoint.purpose && card.submission_revision === intent.frozenRevision && card.state === (intent.command.action === 'submit' ? 'submitted' : 'draft') && canonicalScores(card.scores) === canonicalScores(intent.command.scores)
}

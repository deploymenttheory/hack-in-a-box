import type { components } from './api/schema.gen'

export type SubmissionPage = components['schemas']['SubmissionPage']
export type Evidence = components['schemas']['SubmissionEvidence']
export type Definition = components['schemas']['SubmissionDefinition']
export type Team = components['schemas']['HackTeam']
export type Draft = components['schemas']['SubmissionDraft']
export const words = (value: string) => value.replaceAll('_', ' ')
export function referenceURL(raw: string) {
  try { const url = new URL(raw); return ['http:', 'https:'].includes(url.protocol) && !url.username && !url.password ? url.href : undefined } catch { return undefined }
}
export function evidenceProblems(items: Evidence[]) {
  return items.flatMap(e => !e.label.trim() ? ['Give every evidence item a label.'] : e.kind === 'file' ? e.file_id ? [] : ['Upload the selected evidence file.'] : referenceURL(e.url) ? [] : ['Use an HTTP or HTTPS reference without embedded credentials.'])
}
export function missingRequirements(definitions: Definition[], items: Evidence[]) {
  return definitions.filter(d => d.required && !items.some(e => e.deliverable_id === d.id)).map(d => d.title)
}

export function sameEvidence(a: Evidence[], b: Evidence[]) {
  const ordered = (items: Evidence[]) => [...items].sort((x, y) => x.deliverable_id.localeCompare(y.deliverable_id))
  return JSON.stringify(ordered(a)) === JSON.stringify(ordered(b))
}

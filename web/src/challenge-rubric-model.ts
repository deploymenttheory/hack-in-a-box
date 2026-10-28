import { api, APIError, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import { rubricReferenceKey } from './rubric-planning-model'

type Configuration = components['schemas']['RubricConfiguration']
export type RubricReference = components['schemas']['RubricReference']
export type ChallengeRubricSnapshot = { reference: RubricReference; definition: components['schemas']['PublicRubricDefinition'] }
export type ChallengeRubricRead = { approved?: ChallengeRubricSnapshot; selected?: ChallengeRubricSnapshot }

// Only the named public fields leave this API boundary. In particular, a private
// configuration object must never be spread into a document or attendee preview.
export function publicChallengeRubric(value: Configuration): ChallengeRubricSnapshot {
  const d = value.definition
  return { reference: { id: value.reference.id, version: value.reference.version, digest: value.reference.digest }, definition: {
    name: d.name, mode: d.mode, required_judge_count: d.required_judge_count,
    finalist_count: d.finalist_count, reserve_count: d.reserve_count,
    categories: d.categories.map(c => ({ id: c.id, label: c.label, min_score: c.min_score, max_score: c.max_score, weight: c.weight, guidance: c.guidance, anchors: c.anchors.map(a => ({ score: a.score, description: a.description })) })),
    prizes: d.prizes.map(p => ({ id: p.id, name: p.name, description: p.description, category_id: p.category_id })),
    side_quests: d.side_quests.map(q => ({ id: q.id, title: q.title, evidence_guidance: q.evidence_guidance })),
  } }
}
export function sameRubric(a?: RubricReference, b?: RubricReference) {
  return a && b ? rubricReferenceKey(a) === rubricReferenceKey(b) : !a && !b
}

export async function readChallengeRubric(eventID: string, selected: RubricReference | undefined, signal: AbortSignal, assertCurrent: () => void): Promise<ChallengeRubricRead> {
  async function configuration(version?: number) {
    const value = unwrap(await api.GET('/api/v1/events/{event_id}/rubric/configuration', { signal, cache: 'no-store', params: { path: { event_id: eventID }, query: { version } } }))
    assertCurrent(); return value
  }
  const latest = await configuration()
  let approved = latest.approved && latest.reference.version > 0 ? latest : undefined
  // Latest may be an unapproved draft while the previous version is still the
  // approved snapshot. Follow server cursors rather than guessing its version.
  if (!approved && latest.reference.version > 0) {
    let cursor: number | undefined
    const seen = new Set<number>()
    for (;;) {
      const history = unwrap(await api.GET('/api/v1/events/{event_id}/rubric/history', { signal, cache: 'no-store', params: { path: { event_id: eventID }, query: { limit: 100, cursor } } }))
      assertCurrent()
      const approval = history.items.find(change => change.action === 'approved')
      if (approval) {
        const candidate = await configuration(approval.version)
        if (!candidate.approved) throw new APIError(409, 'rubric_changed')
        approved = candidate; break
      }
      if (history.next_cursor === null || !history.items.length) break
      if (seen.has(history.next_cursor)) throw new Error('Rubric history did not advance')
      seen.add(history.next_cursor); cursor = history.next_cursor
    }
  }
  let retained: Configuration | undefined
  if (selected) {
    retained = sameRubric(selected, latest.reference) ? latest : approved && sameRubric(selected, approved.reference) ? approved : await configuration(selected.version)
    if (!sameRubric(selected, retained.reference)) throw new APIError(409, 'rubric_changed')
  }
  return { approved: approved ? publicChallengeRubric(approved) : undefined, selected: retained ? publicChallengeRubric(retained) : undefined }
}

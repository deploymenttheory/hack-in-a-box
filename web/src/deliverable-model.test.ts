import { describe, expect, it } from 'vitest'
import { sameEvidence, type Evidence } from './deliverable-model'

const evidence: Evidence = { deliverable_id: '11111111-1111-4111-8111-111111111111', kind: 'repository', label: 'Synthetic repository', url: 'https://example.test/repository', file_id: null }
const native = { deliverable_id: evidence.deliverable_id, file_id: evidence.file_id, kind: evidence.kind, label: evidence.label, url: evidence.url }

describe('reviewed evidence equivalence', () => {
  it('accepts generated native DTO property order and reordered evidence without mutating inputs', () => {
    const second = { ...evidence, deliverable_id: '22222222-2222-4222-8222-222222222222' }
    const sent = [second, evidence], receipt = [native, second]
    expect(sameEvidence(sent, receipt)).toBe(true)
    expect(sent[0]).toBe(second)
    expect(Object.keys(native)).toEqual(['deliverable_id', 'file_id', 'kind', 'label', 'url'])
  })
  it.each([
    ['deliverable', { deliverable_id: '33333333-3333-4333-8333-333333333333' }],
    ['kind', { kind: 'url' as const }],
    ['label', { label: 'Changed label' }],
    ['URL', { url: 'https://example.test/different' }],
    ['file identity', { file_id: '44444444-4444-4444-8444-444444444444' }],
  ])('rejects changed %s', (_name, change) => {
    expect(sameEvidence([evidence], [{ ...native, ...change }])).toBe(false)
  })
  it('rejects missing or additional evidence', () => {
    expect(sameEvidence([evidence], [])).toBe(false)
    expect(sameEvidence([evidence], [native, native])).toBe(false)
  })
})

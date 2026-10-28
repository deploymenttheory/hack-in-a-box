import { afterEach, expect, test, vi } from 'vitest'
import { api } from './api/client'
import { publicChallengeRubric, readChallengeRubric } from './challenge-rubric-model'
import { rubricGuideConfiguration } from './styleguide/rubric-fixture'
import type { components } from './api/schema.gen'

afterEach(() => vi.restoreAllMocks())
function configuration(version: number, approved = true) {
  const value = rubricGuideConfiguration()
  return { ...value, reference: { ...value.reference, version, digest: String(version).repeat(64) }, approved }
}
const result = (data: unknown) => ({ data, response: new Response(null, { status: 200 }) })

test('public challenge projection excludes private guidance and unexpected nested response fields', () => {
  const privateValue = configuration(1)
  privateValue.definition.private_judge_guidance = 'PRIVATE JUDGE ONLY'
  Object.assign(privateValue.definition.categories[0], { private_assessment: 'PRIVATE CATEGORY' })
  const snapshot = publicChallengeRubric(privateValue)
  expect(JSON.stringify(snapshot)).not.toContain('PRIVATE')
  expect(snapshot.definition.categories[0].guidance).toEqual(privateValue.definition.categories[0].guidance)
  expect(snapshot.reference).toEqual(privateValue.reference)
  snapshot.definition.categories[0].anchors[0].description = 'Local preview change'
  expect(privateValue.definition.categories[0].anchors[0].description).not.toEqual('Local preview change')
})

test('unapproved latest draft follows native history cursors to the still-approved older snapshot', async () => {
  const draft = configuration(4, false), approved = configuration(2), retained = configuration(1, false)
  const cursors: (number | undefined)[] = []
  vi.spyOn(api, 'GET').mockImplementation((async (path: string, options: { params?: { query?: { version?: number; cursor?: number } } }) => {
    const q = options.params?.query
    if (path.endsWith('/configuration')) return result(q?.version === 2 ? approved : q?.version === 1 ? retained : draft)
    cursors.push(q?.cursor)
    return result(q?.cursor === 7 ? { items: [{ action: 'approved', version: 2 }], next_cursor: null } : { items: [{ action: 'saved', version: 4 }], next_cursor: 7 })
  }) as typeof api.GET)
  const data = await readChallengeRubric('event', retained.reference, new AbortController().signal, () => {})
  expect(cursors).toEqual([undefined, 7])
  expect(data.approved?.reference).toEqual(approved.reference)
  expect(data.selected?.reference).toEqual(retained.reference)
})

test('unpersisted defaults provide no approved choice and do not request absent history', async () => {
  const read = vi.spyOn(api, 'GET').mockResolvedValue(result(configuration(0, false)) as never)
  expect(await readChallengeRubric('event', undefined, new AbortController().signal, () => {})).toEqual({ approved: undefined, selected: undefined })
  expect(read).toHaveBeenCalledTimes(1)
})

test('approval changed during history read is rejected instead of selecting a stale snapshot', async () => {
  vi.spyOn(api, 'GET').mockImplementation((async (path: string) => result(path.endsWith('/history') ? { items: [{ action: 'approved', version: 2 }], next_cursor: null } : configuration(3, false))) as typeof api.GET)
  await expect(readChallengeRubric('event', undefined, new AbortController().signal, () => {})).rejects.toMatchObject({ code: 'rubric_changed' })
})

test('an exact retained tuple cannot be substituted with a different digest', async () => {
  const c = configuration(1)
  vi.spyOn(api, 'GET').mockResolvedValue(result(c) as never)
  await expect(readChallengeRubric('event', { ...c.reference, digest: 'different' }, new AbortController().signal, () => {})).rejects.toMatchObject({ code: 'rubric_changed' })
})

test('lost private lifetime after history read prevents a late historical configuration request', async () => {
  const calls: string[] = []
  let active = true
  vi.spyOn(api, 'GET').mockImplementation((async (path: string) => {
    calls.push(path)
    if (path.endsWith('/history')) { active = false; return result({ items: [{ action: 'approved', version: 1 }], next_cursor: null }) }
    return result(configuration(2, false))
  }) as typeof api.GET)
  await expect(readChallengeRubric('event', undefined, new AbortController().signal, () => { if (!active) throw new DOMException('Ended', 'AbortError') })).rejects.toMatchObject({ name: 'AbortError' })
  expect(calls).toHaveLength(2)
})

test('repeated nonempty history cursor fails without looping or guessing an approval', async () => {
  const history: components['schemas']['RubricHistory'] = { items: [{ revision: 7, version: 3, action: 'saved', actor_id: null, reason: '', at: '2026-01-01T00:00:00Z' }], next_cursor: 7 }
  const read = vi.spyOn(api, 'GET').mockImplementation((async (path: string) => result(path.endsWith('/history') ? history : configuration(3, false))) as typeof api.GET)
  await expect(readChallengeRubric('event', undefined, new AbortController().signal, () => {})).rejects.toThrow('Rubric history did not advance')
  expect(read).toHaveBeenCalledTimes(3)
})

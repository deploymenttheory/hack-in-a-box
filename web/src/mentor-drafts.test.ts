import { beforeEach, expect, test, vi } from 'vitest'
import { MentorDraftStore, invalidateMentorDrafts, mentorDraftLifetime, retainMentorDraftScope, type DraftTarget, type CurrentDraftTarget } from './mentor-drafts'

const target: DraftTarget = { personID: '11111111-1111-4111-8111-111111111111', sessionScope: '22222222-2222-4222-8222-222222222222', eventID: '33333333-3333-4333-8333-333333333333', teamID: '44444444-4444-4444-8444-444444444444', noteID: '55555555-5555-4555-8555-555555555555' }
const text = { category: 'technical', evidence: 'Synthetic unsent private evidence', outcome: 'Synthetic outcome' }
const current: CurrentDraftTarget = { eventID: target.eventID, teamID: target.teamID, noteID: target.noteID, revision: 8, noteVersion: 0, canWrite: true, reviewOpen: true }
beforeEach(() => { sessionStorage.clear(); invalidateMentorDrafts(); retainMentorDraftScope(target.personID, target.sessionScope) })

test('refresh preserves only allowlisted text and never reveals it before authoritative access', async () => {
  const store = new MentorDraftStore()
  const poisoned = { ...text, password: 'CREDENTIAL-CANARY', score: 5, csrf: 'CSRF-CANARY' }
  expect(store.save(target, 8, 0, poisoned).status).toBe('local')
  expect(JSON.stringify(sessionStorage)).not.toMatch(/CREDENTIAL-CANARY|CSRF-CANARY|password|score/)
  let resolve!: (value: CurrentDraftTarget) => void
  const read = vi.fn(() => new Promise<CurrentDraftTarget>(r => { resolve = r }))
  const restored = new MentorDraftStore().review(target, read)
  expect(read).toHaveBeenCalledOnce()
  resolve(current)
  expect(await restored).toMatchObject({ status: 'ready', text, changed: false })
})

test('every submission preparation re-fetches authority and requires explicit current revision review', async () => {
  const store = new MentorDraftStore()
  store.save(target, 8, 0, text)
  const read = vi.fn(async () => ({ ...current, revision: 9, noteVersion: 1 }))
  expect(await store.prepareSubmission(target, read)).toMatchObject({ status: 'review_required' })
  // Background query updates or more typing cannot quietly rebase the draft.
  store.save(target, 9, 1, { ...text, outcome: 'Still my unsent text' })
  expect(await store.prepareSubmission(target, read)).toMatchObject({ status: 'review_required' })
  expect(await store.prepareSubmission(target, read, { revision: 9, noteVersion: 1 })).toMatchObject({ status: 'ready', revision: 9, text: { ...text, outcome: 'Still my unsent text' } })
  read.mockResolvedValue({ ...current, revision: 10, noteVersion: 2 })
  expect(await store.prepareSubmission(target, read, { revision: 9, noteVersion: 1 })).toMatchObject({ status: 'review_required' })
  expect(read).toHaveBeenCalledTimes(4)
  // The controller never invokes a write or labels a preparation as saved.
  store.confirmedSaved(target)
  expect(await store.review(target, read)).toEqual({ status: 'missing' })
})

test('connection failure retains a local copy with unconfirmed server state; revoked or pre-live sources erase it', async () => {
  const store = new MentorDraftStore()
  store.save(target, 8, 0, text)
  expect(await store.review(target, async () => { throw new TypeError('network unavailable') })).toEqual({ status: 'unavailable' })
  expect(await store.review(target, async () => current)).toMatchObject({ status: 'ready', text })
  expect(await store.review(target, async () => ({ ...current, reviewOpen: false }))).toEqual({ status: 'blocked' })
  expect(await store.review(target, async () => current)).toEqual({ status: 'missing' })
  store.save(target, 8, 0, text)
  expect(await store.review(target, async () => { throw { status: 403 } })).toEqual({ status: 'blocked' })
  expect(sessionStorage.length).toBe(0)
  store.save(target, 8, 0, text)
  expect(await store.review(target, async () => { throw { status: 409 } })).toEqual({ status: 'blocked' })
  expect(sessionStorage.length).toBe(0)
})

test('absolute30-minute expiry is not extended by typing or repeated writes after expiry', async () => {
  let now = 1_000
  const store = new MentorDraftStore(sessionStorage, () => now)
  store.save(target, 8, 0, text)
  now += mentorDraftLifetime - 1
  expect(store.save(target, 8, 0, text)).toEqual({ status: 'local', expiresAt: 1_000 + mentorDraftLifetime })
  now++
  expect(store.save(target, 8, 0, text)).toEqual({ status: 'expired' })
  expect(store.save(target, 8, 0, text)).toEqual({ status: 'expired' })
  expect(sessionStorage.length).toBe(0)
})

test('logout/session replacement cancels in-flight recovery and cannot write old-scope private text', async () => {
  const store = new MentorDraftStore()
  store.save(target, 8, 0, text)
  let resolve!: (value: CurrentDraftTarget) => void
  const pending = store.review(target, () => new Promise(r => { resolve = r }))
  sessionStorage.setItem('hackbox:draft:safe-event-setup', 'safe')
  invalidateMentorDrafts()
  resolve(current)
  expect(await pending).toEqual({ status: 'invalidated' })
  expect(store.save(target, 8, 0, text).status).toBe('unavailable')
  expect(sessionStorage.getItem('hackbox:draft:safe-event-setup')).toBe('safe')
  retainMentorDraftScope(target.personID, '66666666-6666-4666-8666-666666666666')
  expect(await store.review(target, async () => current)).toEqual({ status: 'unavailable' })
})

test('wrong team target and malformed/oversized storage never disclose private text', async () => {
  const store = new MentorDraftStore()
  store.save(target, 8, 0, text)
  expect(await store.review(target, async () => ({ ...current, teamID: target.eventID }))).toEqual({ status: 'blocked' })
  expect(store.save(target, 8, 0, { ...text, evidence: 'x'.repeat(30_001) }).status).toBe('unavailable')
  expect(sessionStorage.length).toBe(0)
  store.save(target, 8, 0, text)
  const name = sessionStorage.key(0)!
  sessionStorage.setItem(name, '{malformed')
  expect(await store.review(target, async () => current)).toEqual({ status: 'unavailable' })
  expect(sessionStorage.length).toBe(0)
})

test('expired draft text is purged on lifecycle maintenance without allowing silent TTL renewal', async () => {
  const { purgeExpiredMentorDrafts } = await import('./mentor-drafts')
  let now = 1000
  const store = new MentorDraftStore(sessionStorage, () => now)
  store.save(target, 8, 0, text)
  now += mentorDraftLifetime
  purgeExpiredMentorDrafts(sessionStorage, now)
  expect(sessionStorage.length).toBe(0)
  expect(store.save(target, 8, 0, text)).toEqual({ status: 'expired' })
  store.discard(target)
  expect(store.save(target, 8, 0, { ...text, evidence: 'Explicit new local copy' }).status).toBe('local')
})

test('typing during an authority refresh invalidates its older text instead of overwriting newer work', async () => {
  const store = new MentorDraftStore()
  store.save(target, 8, 0, text)
  let resolve!: (value: CurrentDraftTarget) => void
  const pending = store.review(target, () => new Promise(r => { resolve = r }))
  store.save(target, 8, 0, { ...text, evidence: 'Newer unsent words' })
  resolve(current)
  expect(await pending).toEqual({ status: 'invalidated' })
  expect(await store.review(target, async () => current)).toMatchObject({ status: 'ready', text: { evidence: 'Newer unsent words' } })
})

test('storage denial never reports a successful local recovery copy', () => {
  const denied: Storage = { length: 0, clear() {}, key() { return null }, getItem() { return null }, removeItem() {}, setItem() { throw new DOMException('Synthetic storage denial', 'QuotaExceededError') } }
  const store = new MentorDraftStore(denied)
  expect(store.save(target, 8, 0, text)).toEqual({ status: 'unavailable' })
})

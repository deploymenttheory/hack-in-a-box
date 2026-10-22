// Private local recovery contains only allowlisted unsent mentor text. It is
// neither an offline API cache nor an outbox and never performs a server write.
export const mentorDraftLifetime = 30 * 60 * 1000
const prefix = 'hackbox:mentor-text:v1:'
const maximumDrafts = 16
const expiredKeys = new Set<string>()
let generation = 0
let sessionGeneration = 0
export function mentorSessionGeneration() { return sessionGeneration }
let activeScope: string | undefined
let activeSessionScope: string | undefined
export function currentMentorDraftScope() { return activeSessionScope }
// Native owner denial is resource scoped; it must not erase another team's
// independent unsent work. No text is read or returned during invalidation.
export function discardMentorTeamDrafts(personID: string, sessionScope: string, eventID: string, teamID: string, storage = browserStorage()) {
  if (![personID, sessionScope, eventID, teamID].every(id => uuid.test(id)) || !storage) return
  const match = `${prefix}${personID}:${sessionScope}:${eventID}:${teamID}:`
  try { for (let i = storage.length - 1; i >= 0; i--) { const name = storage.key(i); if (name?.startsWith(match)) { storage.removeItem(name); generation++ } } } catch { /* A failed store remains unavailable, never authorised. */ }
}
export type DraftTarget = { personID: string; sessionScope: string; eventID: string; teamID: string; noteID: string }
export type MentorText = { category: string; evidence: string; outcome: string }
export type CurrentDraftTarget = { eventID: string; teamID: string; noteID: string; revision: number; noteVersion: number; canWrite: boolean; reviewOpen: boolean }
type Envelope = { version: 1; target: DraftTarget; createdAt: number; expiresAt: number; baseRevision: number; noteVersion: number; text: MentorText }
export type DraftResult = { status: 'missing' | 'expired' | 'unavailable' | 'blocked' | 'invalidated' }
export type DraftReview = { status: 'ready'; text: MentorText; changed: boolean; baseRevision: number; noteVersion: number; current: CurrentDraftTarget; expiresAt: number }
export type DraftSubmission = DraftResult | { status: 'review_required'; current: CurrentDraftTarget } | { status: 'ready'; text: MentorText; revision: number; noteVersion: number }
const uuid = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/i
function validTarget(value: DraftTarget) { return Object.values(value).length === 5 && Object.values(value).every(v => typeof v === 'string' && uuid.test(v)) }
function key(target: DraftTarget) { return prefix + [target.personID, target.sessionScope, target.eventID, target.teamID, target.noteID].join(':') }
function copyText(value: MentorText): MentorText {
  // Project explicitly rather than serialising a form/command with private
  // credentials, attachments or scoring actions accidentally attached to it.
  const result = { category: value.category, evidence: value.evidence, outcome: value.outcome }
  const lengths = [128, 30_000, 30_000]
  if (Object.values(result).some((v, i) => typeof v !== 'string' || new TextEncoder().encode(v).byteLength > lengths[i] || v.includes('\0'))) throw new Error('invalid_mentor_text')
  return result
}
function browserStorage() { try { return window.sessionStorage } catch { return undefined } }
export function invalidateMentorDrafts(storage = browserStorage()) {
  generation++
  expiredKeys.clear()
  sessionGeneration++
  activeScope = undefined
  activeSessionScope = undefined
  if (storage) {
    try { for (let i = storage.length - 1; i >= 0; i--) { const name = storage.key(i); if (name?.startsWith(prefix)) storage.removeItem(name) } } catch { /* No false storage-success claim. */ }
  }
  // The owning mentor editor must erase already-restored in-memory text too.
  window.dispatchEvent(new Event('mentor-draft-invalidated'))
}
export function retainMentorDraftScope(personID: string, sessionScope: string, storage = browserStorage()) {
  const keep = `${prefix}${personID}:${sessionScope}:`
  if (!uuid.test(personID) || !uuid.test(sessionScope)) { invalidateMentorDrafts(storage); return }
  if (activeScope !== keep) { generation++; sessionGeneration++; activeScope = keep; activeSessionScope = sessionScope; window.dispatchEvent(new Event('mentor-draft-invalidated')); window.dispatchEvent(new CustomEvent('mentor-draft-scope', { detail: sessionScope })) }
  if (!storage) return
  try { for (let i = storage.length - 1; i >= 0; i--) { const name = storage.key(i); if (name?.startsWith(prefix) && !name.startsWith(keep)) { storage.removeItem(name); generation++ } } } catch { /* Recovery is unavailable when storage cannot be read. */ }
}

export function purgeExpiredMentorDrafts(storage = browserStorage(), now = Date.now()) {
  if (!storage) return
  try {
    for (let i = storage.length - 1; i >= 0; i--) {
      const name = storage.key(i)
      if (!name?.startsWith(prefix)) continue
      try {
        const raw = storage.getItem(name)
        if (!raw || raw.length > 80_000) throw new Error('invalid_draft')
        const metadata = JSON.parse(raw) as Envelope
        if (!Number.isFinite(metadata.createdAt) || metadata.createdAt > now || metadata.expiresAt !== metadata.createdAt + mentorDraftLifetime) throw new Error('invalid_draft')
        if (metadata.expiresAt <= now) { expiredKeys.add(name); storage.removeItem(name) }
      } catch { storage.removeItem(name) }
    }
  } catch { /* Reads always check the absolute expiry too. */ }
}

export class MentorDraftStore {
  constructor(private storage = browserStorage(), private clock = () => Date.now()) {}
  private remove(target: DraftTarget) { try { this.storage?.removeItem(key(target)) } catch { /* Private state stays undisclosed. */ } }
  discard(target: DraftTarget) { generation++; expiredKeys.delete(key(target)); this.remove(target) }
  confirmedSaved(target: DraftTarget) { this.discard(target) }
  private load(target: DraftTarget): Envelope | DraftResult {
    if (!this.storage || !validTarget(target) || activeScope !== `${prefix}${target.personID}:${target.sessionScope}:`) return { status: 'unavailable' }
    if (expiredKeys.has(key(target))) return { status: 'expired' }
    try {
      const raw = this.storage.getItem(key(target))
      if (!raw) return { status: 'missing' }
      if (raw.length > 80_000) throw new Error('oversized_draft')
      const value = JSON.parse(raw) as Envelope
      if (value.version !== 1 || !validTarget(value.target) || key(value.target) !== key(target) || !Number.isSafeInteger(value.baseRevision) || value.baseRevision < 0 || !Number.isSafeInteger(value.noteVersion) || value.noteVersion < 0 || !Number.isFinite(value.createdAt) || value.createdAt > this.clock() || value.expiresAt !== value.createdAt + mentorDraftLifetime) throw new Error('invalid_draft')
      if (this.clock() >= value.expiresAt) { expiredKeys.add(key(target)); this.remove(target); return { status: 'expired' } }
      return { ...value, target: { ...target }, text: copyText(value.text) }
    } catch { this.remove(target); return { status: 'unavailable' } }
  }
  save(target: DraftTarget, baseRevision: number, noteVersion: number, text: MentorText): { status: 'local'; expiresAt: number } | DraftResult {
    if (!this.storage || !validTarget(target) || activeScope !== `${prefix}${target.personID}:${target.sessionScope}:` || !Number.isSafeInteger(baseRevision) || baseRevision < 0 || !Number.isSafeInteger(noteVersion) || noteVersion < 0) return { status: 'unavailable' }
    if (expiredKeys.has(key(target))) return { status: 'expired' }
    try {
      const current = this.load(target)
      if ('status' in current && current.status === 'expired') return current
      const createdAt = 'createdAt' in current ? current.createdAt : this.clock()
      const envelope: Envelope = { version: 1, target: { personID: target.personID, sessionScope: target.sessionScope, eventID: target.eventID, teamID: target.teamID, noteID: target.noteID }, createdAt, expiresAt: createdAt + mentorDraftLifetime, baseRevision: 'baseRevision' in current ? current.baseRevision : baseRevision, noteVersion: 'noteVersion' in current ? current.noteVersion : noteVersion, text: copyText(text) }
      purgeExpiredMentorDrafts(this.storage, this.clock())
      let count = 0
      for (let i = 0; i < this.storage.length; i++) { if (this.storage.key(i)?.startsWith(prefix)) count++ }
      if (count >= maximumDrafts && !this.storage.getItem(key(target))) return { status: 'unavailable' }
      this.storage.setItem(key(target), JSON.stringify(envelope))
      generation++
      return { status: 'local', expiresAt: envelope.expiresAt }
    } catch { return { status: 'unavailable' } }
  }
  async review(target: DraftTarget, readCurrent: () => Promise<CurrentDraftTarget>): Promise<DraftReview | DraftResult> {
    const draft = this.load(target), started = generation
    if ('status' in draft) return draft
    try {
      const current = await readCurrent()
      if (started !== generation || !this.storage?.getItem(key(target))) return { status: 'invalidated' }
      if (this.clock() >= draft.expiresAt) { expiredKeys.add(key(target)); this.remove(target); return { status: 'expired' } }
      if (current.eventID !== target.eventID || current.teamID !== target.teamID || current.noteID !== target.noteID || !current.canWrite || !current.reviewOpen || !Number.isSafeInteger(current.revision) || current.revision < 0 || !Number.isSafeInteger(current.noteVersion) || current.noteVersion < 0) { this.remove(target); return { status: 'blocked' } }
      return { status: 'ready', text: copyText(draft.text), changed: current.revision !== draft.baseRevision || current.noteVersion !== draft.noteVersion, baseRevision: draft.baseRevision, noteVersion: draft.noteVersion, current, expiresAt: draft.expiresAt }
    } catch (error) {
      if (started !== generation) return { status: 'invalidated' }
      const status = (error as { status?: number } | null)?.status
      if (status === 401 || status === 403 || status === 404 || status === 409) { this.remove(target); return { status: 'blocked' } }
      return { status: 'unavailable' }
    }
  }
  async prepareSubmission(target: DraftTarget, readCurrent: () => Promise<CurrentDraftTarget>, explicitlyReviewed?: { revision: number; noteVersion: number }): Promise<DraftSubmission> {
    const review = await this.review(target, readCurrent)
    if (review.status !== 'ready') return review
    if (review.changed && (explicitlyReviewed?.revision !== review.current.revision || explicitlyReviewed.noteVersion !== review.current.noteVersion)) return { status: 'review_required', current: review.current }
    return { status: 'ready', text: review.text, revision: review.current.revision, noteVersion: review.current.noteVersion }
  }
}

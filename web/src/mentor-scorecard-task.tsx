import { useEffect, useRef, useState } from 'react'
import { useInfiniteQuery } from '@tanstack/react-query'
import { api, APIError, privateKey, unwrap } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { useCurrentTime } from './use-current-time'
import { currentCard, reconcileCard } from './scorecard-api'
import { canonicalScores, entries, matchesReceipt, scores, validate, type Card, type Checkpoint, type Entry, type Intent, type Issue } from './scorecard-model'
import { EvidencePicker, EvidencePreview } from './scorecard-evidence'

import { Field, Checkbox, Input, Label, Textarea } from './components/ui/fields'
import { MentorTaskReview, MentorUnknown, useMentorTaskNavigation } from './mentor-task'
import { usePrivateRequestScope } from './private-request-scope'
import { usePlanningUnknownCommands, usePlanningDepartureBarrier } from './planning-operations'
export type ScorecardRisk = { dirty: boolean; pending: boolean; unknown: boolean }
export function CardEditor({ eventID, teamID, checkpoint, initial, refresh, onRisk }: { eventID: string; teamID: string; checkpoint: Checkpoint; initial: { revision: number; card: Card | null }; refresh: () => Promise<unknown>; onRisk: (risk: ScorecardRisk) => void }) {
  const identity = useIdentity(), [values, setValues] = useState(() => entries(checkpoint, initial.card ?? undefined)), [issues, setIssues] = useState<Issue[]>([]), [notice, setNotice] = useState(initial.card ? `Loaded your saved ${initial.card.state} version ${initial.card.reference.version}.` : 'All categories are Pending/unobserved. No server save exists yet.'), [busy, setBusy] = useState(false), [failure, setFailure] = useState<unknown>(), [uncertain, setUncertain] = useState<Intent>(), [review, setReview] = useState<Awaited<ReturnType<typeof currentCard>>>(), [receipt, setReceipt] = useState<Card | null>(initial.card)
  const owner = `${identity.person.id}:${identity.draft_scope}:${eventID}:${teamID}:score:${checkpoint.definition.id}:${checkpoint.version}:${checkpoint.definition.run_id}`
  const scope = usePrivateRequestScope(owner), history = usePlanningUnknownCommands(owner)
  const [prepared, setPrepared] = useState<Intent>(), [baseline, setBaseline] = useState(() => canonicalScores(scores(entries(checkpoint, initial.card ?? undefined))))
  const unknown = !!uncertain || history.unconfirmed, dirty = canonicalScores(scores(values)) !== baseline
  const focusRoot = useRef<HTMLElement>(null), navigation = useMentorTaskNavigation({ root: focusRoot, dirty: dirty || !!prepared, pending: busy, unknown }), writing = useRef(false), terminal = useRef(false)
  usePlanningDepartureBarrier(busy)
  useEffect(() => { onRisk({ dirty: dirty || !!prepared, pending: busy, unknown }); return () => onRisk({ dirty: false, pending: false, unknown: false }) }, [dirty, prepared, busy, unknown, onRisk])
  const clock = useCurrentTime()
  const base = useRef({ revision: initial.revision, version: initial.card?.reference.version ?? 0 }), generation = useRef(0)
  useEffect(() => () => { generation.current++ }, [])
  const inWindow = clock >= Date.parse(checkpoint.opens_at) && clock < Date.parse(checkpoint.closes_at)
  function change(index: number, next: Entry) { setValues(values.map((e, i) => i === index ? next : e)); setIssues([]); setNotice('Unsent private changes in page memory only. No new server save confirmed.') }
  function confirmed(card: Card) { terminal.current = false; if (uncertain && matchesReceipt(card, uncertain)) history.clear(); setPrepared(undefined); setBaseline(canonicalScores(card.scores)); navigation.restore(); setUncertain(undefined); setReview(undefined); setReceipt(card); base.current = { revision: card.revision, version: card.reference.version }; setNotice(`Confirmed your exact ${card.state} version ${card.reference.version} is saved on the server. ${card.scores.filter(s => s.pending).length} categories remain pending; this is not moderation or sign-off.`); void refresh() }
  async function send(intent: Intent) {
    if (writing.current || terminal.current || unknown) return
    const lifetime = scope.capture(); if (!lifetime.current()) return
    writing.current = true
    const ticket = generation.current; let attempted = false; setBusy(true); setFailure(undefined)
    try {
      const current = await currentCard(eventID, teamID, checkpoint, identity.person.id, lifetime.signal); lifetime.assert()
      if (current.revision !== intent.revision || (current.card?.reference.version ?? 0) !== intent.command.version || current.frozenRevision !== intent.frozenRevision) { setPrepared(undefined); setReview(current); setNotice('The reviewed source changed. Compare current versions and review again; no write was sent.'); return }
      attempted = true
      const card = unwrap(await api.POST('/api/v1/events/{event_id}/teams/{team_id}/scorecards', { params: { path: { event_id: eventID, team_id: teamID }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${intent.revision}"` } }, body: intent.command }))
      if (ticket !== generation.current || !lifetime.current()) return
      if (!matchesReceipt(card, intent)) throw new Error('The response did not confirm the exact scorecard version and content.')
      confirmed(card)
    } catch (error) {
      if (ticket !== generation.current || !lifetime.current()) return
      setFailure(error)
      if (error instanceof APIError && [400, 401, 403, 404, 409].includes(error.status)) {
        if ([401, 403, 404].includes(error.status)) { scope.invalidate(); terminal.current = true; setPrepared(undefined); setValues([]); setUncertain(undefined); setReview(undefined); setReceipt(null); setNotice('Current access was denied. Private scores and evidence have been cleared.'); void refresh() }
        else if (error.status === 400) { setPrepared(undefined); setUncertain(undefined); setNotice('Validation rejected this input; no server save was made. Your entered scores and notes remain unsent in memory.') }
        else { setPrepared(undefined); setReview(undefined); setNotice('The server rejected this version. Check the exact saved version before reviewing a newer base; no write will be replayed automatically.') }
      } else if (!attempted) { setPrepared(undefined); setNotice('Current sources could not be verified. No write was sent; your input remains unsent.') } else { terminal.current = true; setUncertain(intent); history.record({ operation: 'Private scorecard', revision: intent.revision, command: { intentID: `${intent.command.checkpoint_id}:${intent.expectedVersion}`, expectedVersion: intent.expectedVersion } }); setNotice('Save outcome is unconfirmed. Your original version-bound intent is retained in page memory. Check the exact saved version; do not assume a save or retry blindly.') }
    } finally { writing.current = false; if (lifetime.current()) setBusy(false) }
  }
  async function save(submit: boolean) {
    if (writing.current || terminal.current || unknown || busy || !inWindow) return
    navigation.capture()
    const lifetime = scope.capture(); if (!lifetime.current()) return
    const invalid = validate(checkpoint, values, submit); setIssues(invalid)
    if (invalid.length) { setNotice('Resolve the highlighted categories. Your entered notes remain unsent.'); document.getElementById(`score-${invalid[0].categoryID}`)?.focus(); return }
    const ticket = generation.current; setBusy(true); setFailure(undefined)
    try {
      const current = await currentCard(eventID, teamID, checkpoint, identity.person.id)
      if (ticket !== generation.current || !lifetime.current()) return
      if (current.revision !== base.current.revision || (current.card?.reference.version ?? 0) !== base.current.version) { setReview(current); setNotice('The server changed. Review the current saved card below, then explicitly choose it as the base before saving your unsent work.'); return }
      const intent: Intent = { revision: current.revision, command: { action: submit ? 'submit' : 'save', checkpoint_id: checkpoint.definition.id, checkpoint_version: checkpoint.version, version: base.current.version, scores: scores(values) }, authorID: identity.person.id, teamID, checkpoint, expectedVersion: base.current.version + 1, frozenRevision: current.frozenRevision }
      lifetime.assert(); setPrepared(structuredClone(intent))
    } catch (error) { if (ticket === generation.current && lifetime.current()) { setFailure(error); if (error instanceof APIError && [401, 403, 404].includes(error.status)) { scope.invalidate(); terminal.current = true; setPrepared(undefined); setValues([]); setReceipt(null); setNotice('Current authority was denied; private editor cleared.'); void refresh() } else setNotice('Current server authority/output could not be confirmed. No write was sent; your input remains unsent.') } } finally { setBusy(false) }
  }
  async function check() {
    if (!uncertain || writing.current || busy) { if (!uncertain) await refresh(); return }
    const lifetime = scope.capture(); if (!lifetime.current()) return
    const ticket = generation.current; setBusy(true); setFailure(undefined)
    try { const exact = await reconcileCard(eventID, uncertain); if (ticket !== generation.current || !lifetime.current()) return; if (exact) confirmed(exact); else { const current = await currentCard(eventID, teamID, checkpoint, identity.person.id); if (ticket !== generation.current || !lifetime.current()) return; setReview(current); setNotice('The original intent is not confirmed saved. Review current server state; no request was replayed. Explicitly release the old intent before a new save.') } } catch (error) { if (ticket === generation.current && lifetime.current()) { setFailure(error); if (error instanceof APIError && [401, 403, 404].includes(error.status)) { scope.invalidate(); terminal.current = true; setPrepared(undefined); setReview(undefined); setValues([]); setUncertain(undefined); setReceipt(null); void refresh() } } } finally { setBusy(false) }
  }
  const pending = values.filter(e => e.pending).length
  return <section ref={focusRoot} tabIndex={-1} className="py-6" aria-label="Your private scorecard" aria-busy={busy}><h2>{checkpoint.definition.name}</h2><p>{checkpoint.purpose === 'checkpoint' ? 'PROVISIONAL live checkpoint. This is not a final score or a running total.' : 'FINAL qualification uses the native post-cutoff frozen formal submission only. Live checkpoint scores are never copied or averaged here.'}</p><p>{pending} of {checkpoint.criteria.categories.length} categories pending. Pending/unobserved is not zero or poor performance.</p><p>{inWindow ? 'Configured checkpoint window is open; the server rechecks current gates and authority on every save.' : 'Outside the configured checkpoint window. Saving is unavailable; unsent work remains in this page only.'}</p>{receipt?.submission_revision != null && <p>Saved against frozen submission revision {receipt.submission_revision}.</p>}
    <div hidden={!!prepared || unknown} inert={!!prepared || unknown}><form className="grid gap-6" onSubmit={e => { e.preventDefault(); void save(false) }}><fieldset className="grid gap-6" disabled={busy || unknown || !!prepared || !values.length}>{values.map((entry, i) => { const category = checkpoint.criteria.categories.find(c => c.id === entry.categoryID)!; const errors = issues.filter(e => e.categoryID === category.id); return <fieldset className="grid gap-4 py-6 border-t border-border" key={category.id}><legend>{category.label}</legend><p>{category.guidance}</p><p>Score range {category.min_score}–{category.max_score} · Weight {category.weight}</p><details><summary>Rubric score anchors</summary><ul>{category.anchors.map(a => <li key={a.score}>{a.score}: {a.description}</li>)}</ul></details>
      <Label className="flex items-center gap-2"><Checkbox checked={entry.pending} onChange={e => change(i, { ...entry, pending: e.target.checked })} />Pending/unobserved — no numeric score</Label>
      <Field id={`score-${category.id}`} label={`${category.label} score (explicit zero is allowed when within range)`}><Input type="number" min={category.min_score} max={category.max_score} step="1" disabled={entry.pending} value={entry.pending ? '' : entry.value} aria-invalid={!!errors.length} aria-describedby={errors.length ? `errors-${category.id}` : undefined} onChange={e => change(i, { ...entry, value: e.target.value })} /></Field>
      <Field id={`evidence-${category.id}`} label={`Private evidence note for ${category.label}`}><Textarea rows={4} value={entry.note} onChange={e => change(i, { ...entry, note: e.target.value })} /></Field>{!!errors.length && <ul id={`errors-${category.id}`} role="alert">{errors.map(e => <li key={e.message}>{e.message}</li>)}</ul>}
      <EvidencePicker eventID={eventID} teamID={teamID} add={ref => { if (!entry.references.some(r => r.kind === ref.kind && r.id === ref.id && r.version === ref.version)) change(i, { ...entry, references: [...entry.references, ref] }) }} />{entry.references.map((ref, ri) => <div className="py-3 border-t border-border" key={ri}><EvidencePreview eventID={eventID} teamID={teamID} reference={ref} /><Button type="button" icon="remove" variant="ghost" onClick={() => change(i, { ...entry, references: entry.references.filter((_, n) => n !== ri) })}>Remove evidence reference {ri + 1}</Button></div>)}</fieldset> })}<div className="fleet-pattern-actions"><Button type="submit" icon="next" disabled={!inWindow}>Save private draft</Button><Button type="button" icon="next" disabled={!inWindow} onClick={() => void save(true)}>{pending ? `Submit independent card with ${pending} pending` : 'Submit independent card'}</Button></div></fieldset></form></div>
    {prepared && !unknown && <MentorTaskReview title="Review independent scorecard" pending={busy} disabled={!inWindow} confirmLabel={prepared.command.action === 'submit' ? 'Confirm independent submission' : 'Confirm private draft'} correct={() => { setPrepared(undefined); navigation.restore() }} confirm={() => void send(prepared)}><p>Team {teamID} · Author {identity.person.display_name}</p><p>Expected event revision {prepared.revision} · Card version {prepared.command.version} → {prepared.expectedVersion}</p><p>Checkpoint version {checkpoint.version} · Rubric version {checkpoint.rubric.version} · Native run {checkpoint.definition.run_id}</p>{prepared.frozenRevision != null && <p>Frozen submission revision {prepared.frozenRevision}</p>}<ul>{prepared.command.scores.map(score => <li className="py-4 border-t border-border" key={score.category_id}><h4>{checkpoint.criteria.categories.find(c => c.id === score.category_id)?.label}</h4><p>{score.pending ? 'Pending/unobserved — no score' : `Explicit score ${score.value}`}</p>{score.evidence.map((e, i) => <EvidencePreview key={i} eventID={eventID} teamID={teamID} reference={e} />)}</li>)}</ul><p>This records your independent {prepared.command.action === 'submit' ? 'submitted assessment' : 'private draft'}. It does not moderate, sign off or publish a result.</p></MentorTaskReview>}
    {unknown && <MentorUnknown intent={`${checkpoint.definition.id}:${uncertain?.expectedVersion ?? 'retained'}`} pending={busy} read={() => void check()}>The original version-bound assessment is unresolved. Only an exact immutable version matching its full source and captured content proves it. No automatic retry or version rebasing is available.</MentorUnknown>}
    <p role="status" aria-label="Scorecard save state">{notice}</p>{!!failure && <Failure error={failure} />}{uncertain && <Button icon="refresh" variant="outline" disabled={busy} onClick={() => void check()}>Check exact saved version</Button>}
    {review && <section className="py-6 border-t border-border" aria-label="Review current server card"><h3>Current saved server card</h3>{review.card ? <SavedCard card={review.card} eventID={eventID} teamID={teamID} /> : <p>No current own card for this checkpoint version.</p>}<Button icon="refresh" variant="outline" disabled={busy} onClick={() => { if (unknown && !window.confirm('Keep the original outcome unresolved and deliberately choose a new current base? This does not prove or replay the old write.')) return; base.current = { revision: review.revision, version: review.card?.reference.version ?? 0 }; setReview(undefined); setPrepared(undefined); setUncertain(undefined); terminal.current = false; history.selectNewBase(); setNotice('Explicitly reviewed current server base. Your input remains unsent; choose Save or Submit to create a new intent.') }}>Use reviewed current version as base (keep unsent input)</Button></section>}{receipt && <CardHistory eventID={eventID} teamID={teamID} card={receipt} />}
  </section>
}

function SavedCard({ card, eventID, teamID }: { card: Card; eventID: string; teamID: string }) { return <div><p>Your {card.state} version {card.reference.version} · {card.changed_at}</p><ul>{card.scores.map(s => <li key={s.category_id}>{card.criteria.categories.find(c => c.id === s.category_id)?.label ?? s.category_id}: {s.pending ? 'Pending/unobserved (no score)' : s.value}{s.evidence.map((e, i) => <EvidencePreview key={i} eventID={eventID} teamID={teamID} reference={e} />)}</li>)}</ul></div> }
function CardHistory({ eventID, teamID, card }: { eventID: string; teamID: string; card: Card }) {
  const identity = useIdentity(), [open, setOpen] = useState(false)
  const query = useInfiniteQuery({ queryKey: [...privateKey(identity.person.id, eventID, 'scorecard-history'), teamID, card.reference.id, card.reference.version], enabled: open, initialPageParam: undefined as number | undefined, queryFn: async ({ pageParam }) => { const page = unwrap(await api.GET('/api/v1/events/{event_id}/teams/{team_id}/scorecards/{card_id}/history', { params: { path: { event_id: eventID, team_id: teamID, card_id: card.reference.id }, query: { cursor: pageParam, limit: 50 } } })); if (page.items.some(c => c.author_id !== identity.person.id || c.team_id !== teamID)) throw new APIError(403, 'unexpected_private_author'); return page }, getNextPageParam: p => p.next_cursor ?? undefined, refetchInterval: 15_000 })
  return <section className="py-4 border-t border-border"><Button icon="list" variant="outline" onClick={() => setOpen(!open)}>{open ? 'Hide' : 'Show'} your saved card history</Button>{open && (query.isPending ? <Loading /> : query.error ? <Failure error={query.error} /> : <>{query.data.pages.flatMap(p => p.items).map(c => <SavedCard key={c.reference.version} card={c} eventID={eventID} teamID={teamID} />)}{query.hasNextPage && <Button variant="outline" onClick={() => void query.fetchNextPage()}>Earlier card versions</Button>}</>)}</section>
}

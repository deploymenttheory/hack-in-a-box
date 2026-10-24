import { useCallback, useEffect, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, APIError, privateKey, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Field, Input, Textarea } from './components/ui/fields'
import { Failure, Loading } from './components/states'
import { MentorDraftRecovery } from './mentor-draft-recovery'
import { MentorDraftStore, type DraftReview, type MentorText, type CurrentDraftTarget } from './mentor-drafts'
import { currentObservation, exactObservation, type Observation, type ObservationPage } from './mentor-observation-api'
import { scrubMentorPrivateQueries } from './mentor-private-lifecycle'
import { usePrivateRequestScope } from './private-request-scope'
import { usePlanningUnknownCommands, usePlanningDepartureBarrier } from './planning-operations'
import { MentorTaskReview, MentorUnknown, useMentorTaskNavigation } from './mentor-task'
export function ObservationEditor({ event, teamID, noteID, isNew, page, refresh }: { event: EventRecord; teamID: string; noteID: string; isNew: boolean; page: ObservationPage; refresh: () => Promise<unknown> }) {
  const identity = useIdentity(), cache = useQueryClient()
  const owner = `${identity.person.id}:${identity.draft_scope}:${event.id}:${teamID}:note:${noteID}`, history = usePlanningUnknownCommands(owner)
  const scope = usePrivateRequestScope(`${owner}:${page.can_record}`), focusRoot = useRef<HTMLElement>(null)
  type Captured = { revision: number; version: number; text: MentorText; reference: string }
  const [preparedIntent, setPreparedIntent] = useState<Captured>(), [unconfirmed, setUnconfirmed] = useState<Captured>()
  const sending = useRef(false), terminal = useRef(false)

  const [store] = useState(() => new MentorDraftStore())
  const target = { personID: identity.person.id, sessionScope: identity.draft_scope, eventID: event.id, teamID, noteID }
  const [text, setText] = useState<MentorText>({ category: '', evidence: '', outcome: '' }), [reference, setReference] = useState(''), [notice, setNotice] = useState('Unsent text. No server save confirmed.'), [failure, setFailure] = useState<unknown>(), [busy, setBusy] = useState(false), [reviewed, setReviewed] = useState<CurrentDraftTarget>(), [server, setServer] = useState<Observation | null>(), [confirmed, setConfirmed] = useState(!isNew)
  const unknown = !!unconfirmed || (!busy && history.unconfirmed)
  const navigation = useMentorTaskNavigation({ root: focusRoot, dirty: !!preparedIntent || !!reference || !!text.evidence || !!text.outcome || !!text.category, pending: busy, unknown })
  usePlanningDepartureBarrier(busy)
  const generation = useRef(0), base = useRef({ revision: page.revision, version: 0 })
  useEffect(() => () => { generation.current++ }, [])
  const clear = useCallback(() => { generation.current++; setText({ category: '', evidence: '', outcome: '' }); setReference(''); setReviewed(undefined); setPreparedIntent(undefined); setUnconfirmed(undefined); setServer(undefined); setNotice('Private editor cleared. Check current access before restoring local text.') }, [])
  const readCurrent = async () => { try { const current = await currentObservation(target, !confirmed); if (!current.canWrite) { store.discard(target); clear() }; return current } catch (error) { if (error instanceof APIError && [401, 403, 404, 409].includes(error.status)) { store.discard(target); clear() }; throw error } }
  const exact = useQuery({ queryKey: [...privateKey(identity.person.id, event.id, 'mentor-exact-note'), teamID, noteID], gcTime: 0, refetchOnMount: 'always', queryFn: async () => { const ticket = scope.capture(); try { const note = await exactObservation(event.id, teamID, noteID, isNew); ticket.assert(); if (note && note.author_id !== identity.person.id) { store.discard(target); clear(); scope.invalidate() }; return note } catch (error) { if (error instanceof APIError && [401, 403, 404, 409].includes(error.status)) { store.discard(target); clear() }; throw error } }, refetchOnWindowFocus: 'always', refetchInterval: 15_000 })
  useEffect(() => { if (exact.error instanceof APIError && [401, 403, 404].includes(exact.error.status)) scrubMentorPrivateQueries(cache, identity.person.id, event.id, teamID, 'observations', [...privateKey(identity.person.id, event.id, 'mentor-exact-note'), teamID, noteID]) }, [exact.error, cache, identity.person.id, event.id, teamID, noteID])
  function change(next: MentorText) { setText(next); setReviewed(undefined); const local = store.save(target, base.current.revision, base.current.version, next); setNotice(local.status === 'local' ? 'Unsent local copy only. Expires 30 minutes after first typing; not saved on the server.' : local.status === 'expired' ? 'Local copy expired. Discard it before starting a fresh copy.' : 'Local recovery unavailable. This text may be lost on refresh; no server save confirmed.') }
  function recover(review: DraftReview) { base.current = { revision: review.baseRevision, version: review.noteVersion }; setText(review.text); setReviewed(undefined); setNotice('Restored unsent text. Review current server state before saving. Feedback reference is not stored in local recovery.') }
  async function compare() {
    const lifetimeTicket = scope.capture(); const ticket = generation.current; setBusy(true); setFailure(undefined)
    try { const current = await readCurrent(); const note = current.canWrite ? await exactObservation(event.id, teamID, noteID, !confirmed) : null; if (ticket !== generation.current || !lifetimeTicket.current()) return; if (!current.canWrite) { store.discard(target); clear(); return }; setServer(note); setReviewed(current); setNotice('Current server version loaded. Compare it with your text, then deliberately choose Save private observation.') } catch (error) { if (ticket === generation.current && lifetimeTicket.current()) { setFailure(error); if (error instanceof APIError && [401, 403, 404, 409].includes(error.status)) { store.discard(target); clear() } } } finally { setBusy(false) }
  }
  async function prepare() {
    if (sending.current || terminal.current || unknown || !page.can_record) return
    navigation.capture(); const ticket = scope.capture(); setBusy(true)
    try { const prepared = await store.prepareSubmission(target, readCurrent, reviewed); ticket.assert(); if (prepared.status === 'review_required') { setReviewed(undefined); setNotice('The server changed. Use Compare current server version before deciding whether to save your text.'); return }; if (prepared.status !== 'ready') { setNotice(`Cannot save: recovery ${prepared.status}. No server save confirmed.`); return }; setPreparedIntent({ revision: prepared.revision, version: prepared.noteVersion, text: structuredClone(prepared.text), reference }) } catch (error) { if (ticket.current()) setFailure(error) } finally { if (ticket.current()) setBusy(false) }
  }
  async function save() {
    if (!preparedIntent || sending.current || terminal.current || unknown || !page.can_record) return
    const captured = preparedIntent, lifetime = scope.capture(); if (!lifetime.current()) return
    sending.current = true

    const lifetimeTicket = scope.capture(); const ticket = generation.current; let attempted = false; setBusy(true); setFailure(undefined)
    try {
      const prepared = await store.prepareSubmission(target, readCurrent, reviewed); lifetime.assert()
      if (ticket !== generation.current || !lifetimeTicket.current()) return
      if (prepared.status === 'review_required') { setReviewed(undefined); setNotice('The server changed. Use Compare current server version before deciding whether to save your text.'); return }
      if (prepared.status !== 'ready') { if (prepared.status === 'blocked') clear(); setNotice(`Cannot save: recovery ${prepared.status}. No server save confirmed.`); return }
      if (prepared.revision !== captured.revision || prepared.noteVersion !== captured.version) { setPreparedIntent(undefined); setNotice('The reviewed source changed. Compare and review again; no write was sent.'); return }
      attempted = true
      history.record({ operation: 'Private observation', revision: captured.revision, command: { intentID: noteID, noteVersion: captured.version + 1 } })
      const result = unwrap(await api.POST('/api/v1/events/{event_id}/teams/{team_id}/mentor-observations', { params: { path: { event_id: event.id, team_id: teamID }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${prepared.revision}"` } }, body: { action: 'observation', id: noteID, version: prepared.noteVersion, evidence: { ...captured.text, feedback_reference: captured.reference } } }))
      if (ticket !== generation.current || !lifetimeTicket.current()) return
      lifetime.assert(); setPreparedIntent(undefined); history.resolve(noteID); navigation.restore(); store.confirmedSaved(target); setConfirmed(true); setReviewed(undefined); setNotice('Saved on the server. No unsent local copy remains.'); base.current = { revision: result.revision, version: prepared.noteVersion + 1 }; await refresh(); await cache.invalidateQueries({ queryKey: ['private', identity.person.id, event.id] })
    } catch (error) { if (ticket !== generation.current || !lifetimeTicket.current()) return; setFailure(error); if (error instanceof APIError && [401, 403, 404, 409].includes(error.status)) { history.resolve(noteID); if (error.status !== 409 || error.code === 'event_blocked') store.discard(target); clear(); setNotice('The server rejected this write. No save confirmed. Check current access and explicitly restore any remaining local copy.'); void refresh() } else if (error instanceof APIError && error.status === 400) { history.resolve(noteID); setPreparedIntent(undefined); setNotice('Validation rejected this input. Your unsent local text remains; no server save was made. Correct the input before saving again.'); navigation.restore() } else if (!attempted) { setPreparedIntent(undefined); setNotice('Current observation source could not be verified. No write was sent; your input remains unsent.') } else { terminal.current = true; setUnconfirmed(captured); setNotice('Server save outcome is unconfirmed. No write can be replayed; check the original immutable note version.') } } finally { sending.current = false; if (lifetime.current()) setBusy(false) }
  }
  async function reconcile() {
    const lifetimeTicket = scope.capture(); const ticket = generation.current; setBusy(true); setFailure(undefined)
    try { const current = await readCurrent(); if (!current.canWrite) { store.discard(target); clear(); return }; const note = unconfirmed ? unwrap(await api.GET('/api/v1/events/{event_id}/teams/{team_id}/mentor-observations/history', { params: { path: { event_id: event.id, team_id: teamID }, query: { observation_id: noteID, observation_cursor: unconfirmed.version + 2, limit: 1 } } })).observations[0] : await exactObservation(event.id, teamID, noteID, !confirmed); if (ticket !== generation.current || !lifetimeTicket.current()) return; if (note && (!unconfirmed || (note.id === noteID && note.team_id === teamID && note.author_id === identity.person.id && note.version === unconfirmed.version + 1)) && note.evidence.category === (unconfirmed?.text.category ?? text.category) && note.evidence.evidence === (unconfirmed?.text.evidence ?? text.evidence) && note.evidence.outcome === (unconfirmed?.text.outcome ?? text.outcome) && note.evidence.feedback_reference === (unconfirmed?.reference ?? reference)) { history.resolve(noteID); terminal.current = false; setUnconfirmed(undefined); setPreparedIntent(undefined); store.confirmedSaved(target); setConfirmed(true); base.current = { revision: current.revision, version: note.version }; setNotice('Confirmed this exact text is saved on the server. Local copy removed.'); await refresh() } else { setServer(note); setReviewed(undefined); setNotice('This text is not confirmed saved. Compare the current server version before choosing to save. No write was replayed.') } } catch (error) { if (ticket === generation.current && lifetimeTicket.current()) { setFailure(error); if (error instanceof APIError && [401, 403, 404, 409].includes(error.status)) { store.discard(target); clear() } } } finally { setBusy(false) }
  }
  if (exact.isPending || !exact.isFetchedAfterMount) return <Loading />
  if (exact.error || !page.can_record) return <section><p role="alert">Private observation entry is unavailable. Formal review requires the scheduled live start, common publication and current write authority.</p>{exact.error && <Failure error={exact.error} />}</section>
  if (exact.data && exact.data.author_id !== identity.person.id) return <p role="alert">Only the original author can edit this private observation.</p>
  return <section ref={focusRoot} tabIndex={-1} className="py-6 border-t border-border" aria-label="Private observation editor" aria-busy={busy}><h3>{confirmed ? 'Edit private observation' : 'New private observation'}</h3><p>Private evidence and outcome. Nothing in this task publishes team feedback or changes a score.</p>
    {!preparedIntent && !unknown && <><div className="fleet-pattern-actions">{exact.data && <Button icon="refresh" variant="outline" disabled={busy} onClick={() => { if ((text.evidence || reference) && !window.confirm('Replace private unsent text with the saved note?')) return; store.discard(target); base.current = { revision: page.revision, version: exact.data!.version }; setText({ category: exact.data!.evidence.category, evidence: exact.data!.evidence.evidence, outcome: exact.data!.evidence.outcome }); setReference(exact.data!.evidence.feedback_reference ?? ''); setConfirmed(true); setReviewed(undefined); setNotice('Loaded saved server text. Edit to create an unsent local copy.') }}>Load saved server text (replace editor)</Button>}</div>
    <MentorDraftRecovery store={store} target={target} readCurrent={readCurrent} onRecover={recover} onInvalidate={clear} />
    <form className="grid gap-4 max-w-3xl py-4" onSubmit={e => { e.preventDefault(); void prepare() }}><fieldset className="fleet-collection-fieldset grid gap-4" disabled={busy}>
      <Field id="note-category" label="Category (private)"><Input maxLength={128} value={text.category} onChange={e => change({ ...text, category: e.target.value })} /></Field>
      <Field id="note-evidence" label="Evidence (private)"><Textarea rows={6} maxLength={30000} value={text.evidence} onChange={e => change({ ...text, evidence: e.target.value })} /></Field>
      <Field id="note-outcome" label="Outcome (private)"><Textarea maxLength={30000} value={text.outcome} onChange={e => change({ ...text, outcome: e.target.value })} /></Field>
      <Field id="note-reference" label="Feedback reference (private, not locally recovered)"><Input maxLength={256} value={reference} onChange={e => { setReference(e.target.value); change(text) }} /></Field>
      <div className="fleet-pattern-actions"><Button type="submit" icon="next" disabled={!text.category.trim() || !text.evidence.trim()}>Review private observation</Button><Button type="button" icon="refresh" variant="outline" onClick={() => void compare()}>Compare current server version</Button></div>
    </fieldset></form></>}
    {preparedIntent && !unknown && <MentorTaskReview title="Review private observation" pending={busy} confirmLabel="Save private observation" correct={() => { setPreparedIntent(undefined); navigation.restore() }} confirm={() => void save()}><p>Team {teamID} · Note {noteID} · expected event revision {preparedIntent.revision}, note version {preparedIntent.version}</p><p>{preparedIntent.text.category}</p><p className="whitespace-pre-wrap">{preparedIntent.text.evidence}</p><p className="whitespace-pre-wrap">Outcome: {preparedIntent.text.outcome || 'Not recorded'}</p><p>Feedback reference: {preparedIntent.reference || 'None'}</p><p>The original author and current review window are checked again before sending.</p></MentorTaskReview>}
    {unknown && <MentorUnknown intent={noteID} pending={busy} read={() => void reconcile()}>The original private observation remains unconfirmed. Only its exact author, note version and captured content can establish the original result. No write will be replayed.</MentorUnknown>}
    <Button icon="refresh" variant="outline" disabled={busy} onClick={() => void reconcile()}>Check whether this note was saved</Button>
    <p role="status" aria-label="Observation save state">{notice}</p>{!!failure && <Failure error={failure} />}{reviewed && <p>Reviewed event revision {reviewed.revision}; note version {reviewed.noteVersion}. A newer change requires another comparison.</p>}
    {server !== undefined && <section className="py-4 border-t border-border" aria-label="Current saved server version"><h4>Current saved server version</h4>{server ? <><p>{server.evidence.category} · Version {server.version}</p><p className="whitespace-pre-wrap">{server.evidence.evidence}</p><p className="whitespace-pre-wrap">{server.evidence.outcome}</p><p>Feedback reference: {server.evidence.feedback_reference || 'None'}</p></> : <p>No saved observation with this identity.</p>}</section>}
  </section>
}

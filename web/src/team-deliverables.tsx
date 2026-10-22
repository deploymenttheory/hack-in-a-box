import { useEffect, useState } from 'react'
import { useBlocker } from '@tanstack/react-router'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, APIError, privateKey, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { localDate } from './event-time'
import { PersonalPreparation } from './personal-preparation'
import { DeliverableEditor } from './deliverable-editor'
import { EvidenceList, OrganiserHelp, SubmissionHistory } from './deliverable-evidence'
import { words, type SubmissionPage, type Team } from './deliverable-model'

export function TeamDeliverables({ event, onDirty }: { event: EventRecord; onDirty: (dirty: boolean) => void }) {
  const identity = useIdentity(), [selected, setSelected] = useState(''), [personalDirty, setPersonalDirty] = useState(false), [teamDirty, setTeamDirty] = useState(false)
  useEffect(() => { onDirty(personalDirty || teamDirty); return () => onDirty(false) }, [personalDirty, teamDirty, onDirty])
  const teams = useQuery({ queryKey: privateKey(identity.person.id, event.id, 'teams'), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/teams', { params: { path: { event_id: event.id } } })), refetchOnWindowFocus: 'always', refetchInterval: 15_000 })
  const manageable = teams.data?.can_manage ?? false
  const active = teams.data?.teams.filter(t => t.state === 'published' && (manageable || t.published.some(p => p.id === identity.person.id))) ?? []
  const team = active.find(t => t.id === selected) ?? active[0]
  return <section className="team-deliverables"><p className="eyebrow">Preparation and competition evidence</p><h2>Deliverables</h2><PersonalPreparation event={event} onDirty={setPersonalDirty} />
    <section className="section-heading"><h3>Team deliverables</h3>{teams.isPending ? <Loading /> : teams.error ? teams.error instanceof APIError && teams.error.status === 403 ? <p>Team deliverables are available to confirmed competitors and organisers.</p> : <Failure error={teams.error} retry={() => void teams.refetch()} /> : <>{active.length > 1 && <><label htmlFor="deliverable-team">Team</label><select id="deliverable-team" value={team?.id ?? ''} onChange={e => { if (!teamDirty || window.confirm('Switch teams with unsaved evidence?')) setSelected(e.target.value) }}>{active.map(t => <option key={t.id} value={t.id}>{t.name} · {t.id.slice(0, 8)}</option>)}</select></>}{team ? <TeamSubmission key={team.id} event={event} team={team} organiser={manageable} onDirty={setTeamDirty} /> : <p className="empty-state">Your published team deliverables will appear here after allocation. Draft assignments cannot submit competition evidence.</p>}{teams.data.teams.filter(t => t.state === 'disbanded').map(t => <p className="notice" key={t.id}>{t.name} is disbanded. Its entry is withdrawn, unused and unscored; there is no submission to complete.</p>)}</>}</section>
  </section>
}
function TeamSubmission({ event, team, organiser, onDirty }: { event: EventRecord; team: Team; organiser: boolean; onDirty: (dirty: boolean) => void }) {
  const identity = useIdentity(), [editorDirty, setEditorDirty] = useState(false), [reviewDirty, setReviewDirty] = useState(false)
  useEffect(() => { onDirty(editorDirty || reviewDirty); return () => onDirty(false) }, [editorDirty, reviewDirty, onDirty])
  const page = useQuery({ queryKey: privateKey(identity.person.id, event.id, `submission-${team.id}`), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/teams/{team_id}/submission', { params: { path: { event_id: event.id, team_id: team.id } } })), refetchOnWindowFocus: 'always', refetchInterval: 15_000 })
  const draft = useQuery({ queryKey: privateKey(identity.person.id, event.id, `submission-draft-${team.id}`), enabled: page.data?.can_read_draft === true, queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/teams/{team_id}/submission/draft', { params: { path: { event_id: event.id, team_id: team.id } } })), refetchOnWindowFocus: 'always' })
  if (page.isPending) return <Loading />
  if (page.error) return <Failure error={page.error} retry={() => void page.refetch()} />
  const p = page.data, captain = team.published.find(person => person.id === team.published_captain)
  const editable = p.can_read_draft || p.can_submit
  return <section className="team-submission"><h3>{team.name}</h3><p>Owner: {team.name}{captain ? ` · Captain ${captain.name}` : ' · Organiser must appoint a captain'}</p><dl className="dates"><div><dt>Formal submissions and review open</dt><dd>{localDate(p.live_start, event.configuration.timezone)}</dd></div><div><dt>Shared submission cutoff</dt><dd>{localDate(p.cutoff, event.configuration.timezone)}</dd></div><div><dt>Recorded outcome</dt><dd>{words(p.outcome)}{p.frozen_at && ` · Frozen ${localDate(p.frozen_at, event.configuration.timezone)}`}</dd></div></dl>
    {!p.can_submit && p.outcome === 'open' && <p className="notice">{p.can_draft ? 'You may prepare and save a private draft. Formal submissions and review open at live start.' : 'Formal submission is unavailable. The challenge must be published and the live submission window must be open.'}</p>}{p.outcome !== 'open' && <p className="notice">{p.outcome === 'frozen' ? 'Final evidence is frozen at the cutoff.' : p.outcome === 'not_submitted' ? 'No formal submission was recorded by the cutoff. A private draft does not count as a submission.' : 'This entry is withdrawn and will not be scored.'} Ask an organiser about any exception.</p>}
    <section className="panel" aria-label="Current formal submission"><h4>Current formal evidence</h4>{p.final ? <><p>Formal version {p.final.revision} · Submitted {localDate(p.final.submitted_at, event.configuration.timezone)}{p.final.late ? ' · Late amendment' : ''} · Completion review: {words(p.final.review_status)}</p><EvidenceList eventID={event.id} items={p.final.evidence} /></> : <p>No formal submission accepted.</p>}</section>
    {editable ? p.can_read_draft && draft.isPending ? <Loading /> : p.can_read_draft && draft.error ? <Failure error={draft.error} retry={() => void draft.refetch()} /> : <DeliverableEditor event={event} team={team} page={p} initialDraft={p.can_read_draft ? draft.data : undefined} organiser={organiser} onDirty={setEditorDirty} /> : !organiser && !p.can_read_draft ? <section className="panel"><h4>Competition requirements</h4><p>Competition requirements will appear after the common challenge is published. Your personal preparation remains available above.</p></section> : <section className="panel"><h4>Required and optional work</h4><ul>{p.definitions.map(d => <li key={d.id}><strong>{d.title}</strong> · {d.required ? 'Required' : 'Optional'} · Due {localDate(d.due_at, event.configuration.timezone)}<p>{d.description}</p></li>)}</ul></section>}
    <OrganiserHelp page={p} team={team} event={event} />{p.final && <SubmissionHistory event={event} teamID={team.id} />}{p.can_review && <CompletionReview key={p.final?.revision} event={event} teamID={team.id} page={p} onDirty={setReviewDirty} />}
  </section>
}
function CompletionReview({ event, teamID, page, onDirty }: { event: EventRecord; teamID: string; page: SubmissionPage; onDirty: (dirty: boolean) => void }) {
  const identity = useIdentity(), cache = useQueryClient(), [status, setStatus] = useState<'pending' | 'accepted' | 'changes_requested'>('changes_requested'), [reason, setReason] = useState(''), [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>(), [saved, setSaved] = useState(false)
  const [revision, setRevision] = useState(page.revision)
  useEffect(() => { onDirty(!!reason); return () => onDirty(false) }, [reason, onDirty])
  useBlocker({ shouldBlockFn: () => !!reason && !window.confirm('Leave this unsaved completion feedback?'), enableBeforeUnload: !!reason })
  async function save() {
    setPending(true); setFailure(undefined)
    try {
      const result = unwrap(await api.POST('/api/v1/events/{event_id}/teams/{team_id}/submission/review', { params: { path: { event_id: event.id, team_id: teamID }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${revision}"` } }, body: { submission_revision: page.final!.revision, status, reason } }))
      setRevision(result.revision); setReason(''); setSaved(true)
      cache.setQueryData(privateKey(identity.person.id, event.id, `submission-${teamID}`), result)
      await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, event.id, `submission-history-${teamID}`) })
    } catch (error) { setFailure(error) } finally { setPending(false) }
  }
  return <section className="panel section-heading"><h3>Organiser completion review</h3><p>Review formal version {page.final?.revision}. This records completion feedback.</p><label htmlFor="completion-status">Completion decision</label><select id="completion-status" value={status} onChange={e => setStatus(e.target.value as typeof status)}><option value="changes_requested">Changes requested</option><option value="accepted">Complete</option><option value="pending">Pending</option></select><label htmlFor="completion-feedback">Feedback and reason</label><textarea id="completion-feedback" value={reason} onChange={e => setReason(e.target.value)} maxLength={2000} /><Button disabled={pending || !reason.trim() || revision !== page.revision} onClick={() => void save()}>Record completion feedback</Button>{revision !== page.revision && <div><p>This submission changed. Review the current formal evidence above before using the latest review state.</p><Button variant="outline" onClick={() => { setRevision(page.revision); setFailure(undefined); setSaved(false) }}>Use latest review state</Button></div>}{!!failure && <Failure error={failure} />}{saved && <p role="status">Completion feedback recorded.</p>}</section>
}

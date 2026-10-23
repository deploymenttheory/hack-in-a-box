import { useEffect, useMemo, useState } from 'react'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, APIError, privateKey, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { localDate } from './event-time'
import { SurveyReportView } from './survey-results'
import { answerText, nativeDenied, unknownOutcome, useActivePage, useSurveyLeaveWarning, type SurveyConfiguration } from './survey-context'

type ReportReview = components['schemas']['SurveyReviewedReport']
type ReportCommand = components['schemas']['SurveyReportCommand']
export function SurveyReportReview({ eventId, configuration, timezone, writable, onDirty, deny }: { eventId: string; configuration: SurveyConfiguration; timezone: string; writable: boolean; onDirty: (dirty: boolean) => void; deny: (error: unknown) => void }) {
  const identity = useIdentity(), cache = useQueryClient(), id = configuration.id
  const key = useMemo(() => [...privateKey(identity.person.id, eventId, 'survey-report-review'), identity.draft_scope, id], [identity.person.id, identity.draft_scope, eventId, id])
  const query = useQuery({ queryKey: key, gcTime: 0, retry: false, refetchOnWindowFocus: 'always', refetchInterval: 15_000, queryFn: async () => { try { return unwrap(await api.GET('/api/v1/events/{event_id}/surveys/{survey_id}/report/review', { params: { path: { event_id: eventId, survey_id: id } } })) } catch (error) { if (error instanceof APIError && error.status === 404) return null; throw error } } })
  useEffect(() => () => { cache.removeQueries({ queryKey: key }) }, [cache, key])
  useEffect(() => { if (nativeDenied(query.error)) deny(query.error) }, [query.error, deny])
  const latest = query.data ?? undefined
  const loaded = query.data !== undefined
  const networkBlocked = !!query.error
  function receive(value: ReportReview) { cache.setQueryData(key, value) }
  return <section><h2>Frozen aggregates and explicitly reviewed text</h2><p>Wait until close. The first deliberate review freezes one cohort for this question version. There are no filters or live totals. Prose corrections retain that same aggregate. Raw answers never become published text automatically.</p>
    {networkBlocked && <p role="alert">Current review authority could not be refreshed. Local text and the exact retry command remain in this page; writes are paused until a successful refresh.</p>}
    {!loaded ? query.error ? <Failure error={query.error} retry={() => void query.refetch()} /> : <Loading /> : <>
      {latest && <><p>Organiser preview of the exact retained review. Wider release exists only after its explicit publication.</p><SurveyReportView report={{ ...latest.report, reviewed_text_version: latest.published_at ? latest.version : 0, published_at: latest.published_at }} timezone={timezone} /></>}
      <ReportEditor key={`${id}:${writable}`} eventId={eventId} configuration={configuration} initial={latest} writable={writable} networkBlocked={networkBlocked} receive={receive} refresh={async () => { const result = await query.refetch(); if (result.error) throw result.error; return result.data ?? undefined }} onDirty={onDirty} deny={deny} />
    </>}
  </section>
}
function ReportEditor({ eventId, configuration, initial, writable, networkBlocked, receive, refresh, onDirty, deny }: { eventId: string; configuration: SurveyConfiguration; initial?: ReportReview; writable: boolean; networkBlocked: boolean; receive: (value: ReportReview) => void; refresh: () => Promise<ReportReview | undefined>; onDirty: (value: boolean) => void; deny: (error: unknown) => void }) {
  const identity = useIdentity(), active = useActivePage(), [saved, setSaved] = useState(initial), [summary, setSummary] = useState(initial?.report.reviewed_summary ?? ''), [reviewed, setReviewed] = useState(false), [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState(''), [uncertain, setUncertain] = useState<{ command: ReportCommand; revision: number }>(), [conflict, setConflict] = useState(false)
  const dirty = summary !== (saved?.report.reviewed_summary ?? ''), changedElsewhere = (initial?.version ?? 0) !== (saved?.version ?? 0) || initial?.review_id !== saved?.review_id
  useEffect(() => { onDirty(dirty || !!uncertain); return () => onDirty(false) }, [dirty, uncertain, onDirty])
  useSurveyLeaveWarning(dirty || !!uncertain)
  function adopt(value: ReportReview) { setSaved(value); setSummary(value.report.reviewed_summary); setReviewed(false); setUncertain(undefined); setConflict(false); receive(value) }
  async function act(action: ReportCommand['action'], retained?: { command: ReportCommand; revision: number }) {
    const intent = retained ?? { command: { action, review_id: action === 'review_text' ? crypto.randomUUID() : saved!.review_id, summary: action === 'review_text' ? summary : '', reviewed_text: action === 'review_text' }, revision: saved?.version ?? 0 }
    setPending(true); setFailure(undefined); setNotice('')
    try {
      // Refresh current native review authority before explicitly replaying an
      // uncertain command; fixed review_id/version keeps the retry idempotent.
      if (retained) { await refresh(); if (!active.current) return }
      const value = unwrap(await api.PUT('/api/v1/events/{event_id}/surveys/{survey_id}/report/review', { params: { path: { event_id: eventId, survey_id: configuration.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${intent.revision}"` } }, body: intent.command }))
      if (!active.current) return
      adopt(value); setNotice(action === 'review_text' ? `Reviewed summary version ${value.version} retained against the frozen aggregate. It is not yet published.` : action === 'approve' ? 'This exact reviewed summary is approved, not yet published.' : 'This reviewed summary is published. Its frozen aggregate is unchanged.')
    } catch (error) {
      if (!active.current) return
      if (nativeDenied(error)) { deny(error); return }
      setFailure(error); if (unknownOutcome(error)) setUncertain(intent); else { setUncertain(undefined); setConflict(true); await refresh().catch(currentError => { if (nativeDenied(currentError)) deny(currentError); else setFailure(currentError) }) }
    } finally { if (active.current) setPending(false) }
  }
  async function reconcile() {
    setPending(true); setFailure(undefined)
    try {
      const value = await refresh(); if (!active.current || !uncertain) return
      const command = uncertain.command
      const matches = value?.review_id === command.review_id && (command.action === 'review_text' ? value.version === uncertain.revision + 1 && value.reviewer_id === identity.person.id && value.report.reviewed_summary === command.summary.trim() : value.version === uncertain.revision && (command.action === 'approve' ? !!value.approved_at : !!value.published_at))
      if (value && matches) { adopt(value); setNotice('The current exact review confirms the operation. No second write was sent.') }
      else setNotice('This exact operation is not confirmed by the current review. Retain its identity for an explicit retry, or deliberately discard it.')
    } catch (error) { if (nativeDenied(error)) deny(error); else setFailure(error) } finally { if (active.current) setPending(false) }
  }
  function load() { if ((dirty || uncertain) && !window.confirm('Discard local reviewed text and the retained uncertain command, then load the current review?')) return; setSaved(initial); setSummary(initial?.report.reviewed_summary ?? ''); setUncertain(undefined); setConflict(false); setReviewed(false); setFailure(undefined) }
  const blocked = networkBlocked || pending || !writable || !!uncertain || conflict || changedElsewhere
  return <section><h3>Review and publish summary</h3>{!writable && <p>Current native report policy does not permit changes. Review opens only after the current live event and configured survey close; local unsent text is cleared when this capability is lost.</p>}<fieldset disabled={blocked}><legend>Deliberately reviewed prose</legend><label htmlFor="survey-reviewed-summary">Redacted summary for wider release</label><textarea id="survey-reviewed-summary" rows={6} maxLength={20000} value={summary} onChange={e => { setSummary(e.target.value); setReviewed(false) }} /><label className="check"><input type="checkbox" checked={reviewed} onChange={e => setReviewed(e.target.checked)} />I reviewed and redacted this wording for wider release. It contains no identifying response details.</label></fieldset>
    <p>Below the minimum cohort, nonempty prose is rejected. Individual raw answers and text counts are never copied automatically. Publication requires a separately explicit approval of the exact reviewed version.</p><p role="status" aria-label="Survey report status">{notice || (uncertain ? 'The outcome is unknown. The same review identity is retained only in this page; no automatic retry will occur.' : dirty ? 'Unsent reviewed text, kept only in this page.' : 'No unsent reviewed text.')}</p>{!!failure && <Failure error={failure} />}
    {(conflict || changedElsewhere) && <p role="alert">The current review changed. Inspect it before another write.</p>}
    <div className="actions"><Button disabled={blocked || !reviewed || !!saved && !dirty || !configuration.published_at} onClick={() => void act('review_text')}>Freeze cohort and save reviewed summary</Button><Button variant="outline" disabled={blocked || dirty || !saved || !!saved.approved_at} onClick={() => void act('approve')}>Approve exact reviewed summary</Button><Button disabled={blocked || dirty || !saved?.approved_at || !!saved.published_at} onClick={() => void act('publish')}>Publish reviewed report</Button>{uncertain && <><Button variant="outline" disabled={pending || !writable} onClick={() => void reconcile()}>Check exact report operation</Button><Button variant="outline" disabled={pending || !writable || networkBlocked} onClick={() => void act(uncertain.command.action, uncertain)}>Retry the same report command</Button></>}{(dirty || uncertain || conflict || changedElsewhere) && <Button variant="outline" disabled={pending} onClick={load}>Load current report review</Button>}</div>
  </section>
}
export function SurveyPrivateResponses({ eventId, configuration, timezone, deny }: { eventId: string; configuration: SurveyConfiguration; timezone: string; deny: (error: unknown) => void }) {
  const identity = useIdentity(), cache = useQueryClient(), [selected, setSelected] = useState<string>()
  const key = useMemo(() => [...privateKey(identity.person.id, eventId, 'survey-authored-responses'), identity.draft_scope, configuration.id], [identity.person.id, identity.draft_scope, eventId, configuration.id])
  const query = useInfiniteQuery({ queryKey: key, gcTime: 0, retry: false, refetchOnWindowFocus: 'always', refetchInterval: 15_000, initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => unwrap(await api.GET('/api/v1/events/{event_id}/surveys/{survey_id}/responses', { params: { path: { event_id: eventId, survey_id: configuration.id }, query: { after: pageParam, limit: 20 } } })), getNextPageParam: page => page.next_person ?? undefined })
  useEffect(() => () => { cache.removeQueries({ queryKey: key }) }, [cache, key])
  useEffect(() => { if (nativeDenied(query.error)) deny(query.error) }, [query.error, deny])
  return <section><h2>Restricted authored responses</h2><p>Current organisers only. These responses are identifiable and must not be copied into a wider report without explicit review and redaction. Downloads remain private after they leave this app.</p>
    {query.error ? <Failure error={query.error} /> : query.isPending ? <Loading /> : <><a className="primary-link" href={`/api/v1/events/${eventId}/surveys/${configuration.id}/responses/export.csv`} download="restricted-survey-responses.csv">Download restricted responses CSV</a><p>This explicit export is audited and rechecks current organiser and privacy authority. It contains names and answers, not a published report.</p>
      {query.data.pages.flatMap(page => page.items).map(item => <article className="panel" key={item.person_id}><h3>{item.person_name}</h3><p>Response revision {item.response.revision} · Question version {item.response.form_version} · {localDate(item.response.submitted_at, timezone)}</p><dl>{configuration.definition.form.fields.map(field => <div key={field.id}><dt>{field.label}</dt><dd className="help-problem">{answerText(item.response.answers[field.id])}</dd></div>)}</dl><Button variant="outline" onClick={() => setSelected(selected === item.person_id ? undefined : item.person_id)}>Review retained response history</Button>{selected === item.person_id && <ResponseHistory eventId={eventId} configuration={configuration} personId={item.person_id} timezone={timezone} deny={deny} />}</article>)}
      {query.data.pages.every(page => !page.items.length) && <p>No retained responses. This is not a claim of zero learning or adoption.</p>}{query.hasNextPage && <Button variant="outline" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>More restricted responses</Button>}
    </>}
  </section>
}
function ResponseHistory({ eventId, configuration, personId, timezone, deny }: { eventId: string; configuration: SurveyConfiguration; personId: string; timezone: string; deny: (error: unknown) => void }) {
  const identity = useIdentity(), cache = useQueryClient()
  const key = useMemo(() => [...privateKey(identity.person.id, eventId, 'survey-response-history'), identity.draft_scope, configuration.id, personId], [identity.person.id, identity.draft_scope, eventId, configuration.id, personId])
  const query = useInfiniteQuery({ queryKey: key, gcTime: 0, retry: false, initialPageParam: undefined as number | undefined, queryFn: async ({ pageParam }) => unwrap(await api.GET('/api/v1/events/{event_id}/surveys/{survey_id}/responses/{person_id}/history', { params: { path: { event_id: eventId, survey_id: configuration.id, person_id: personId }, query: { before_revision: pageParam, limit: 20 } } })), getNextPageParam: page => page.length === 20 ? page.at(-1)?.revision : undefined })
  useEffect(() => () => { cache.removeQueries({ queryKey: key }) }, [cache, key])
  useEffect(() => { if (nativeDenied(query.error)) deny(query.error) }, [query.error, deny])
  return <section aria-label="Retained response history">{query.error ? <Failure error={query.error} /> : query.isPending ? <Loading /> : <><ol>{query.data.pages.flatMap(page => page).map(response => <li key={response.revision}><h4>Revision {response.revision} · {localDate(response.submitted_at, timezone)}</h4><dl>{configuration.definition.form.fields.map(field => <div key={field.id}><dt>{field.label}</dt><dd className="help-problem">{answerText(response.answers[field.id])}</dd></div>)}</dl></li>)}</ol>{query.hasNextPage && <Button variant="outline" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>Older response revisions</Button>}</>}</section>
}

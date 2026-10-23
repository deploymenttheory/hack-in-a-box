import { useEffect, useMemo } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from '@tanstack/react-router'
import { api, privateKey, unwrap } from './api/client'
import { useIdentity } from './session'
import { Failure, Loading } from './components/states'
import { Button } from './components/ui/button'
import { localDate } from './event-time'
import type { SurveyReport } from './survey-context'
export function SurveyResultsPage() {
  const { eventId, surveyId } = useParams({ strict: false }) as { eventId: string; surveyId: string }, identity = useIdentity(), cache = useQueryClient()
  const key = useMemo(() => [...privateKey(identity.person.id, eventId, 'survey-published-report'), identity.draft_scope, surveyId], [identity.person.id, identity.draft_scope, eventId, surveyId])
  const query = useQuery({ queryKey: key, gcTime: 0, staleTime: 0, retry: false, refetchOnWindowFocus: 'always', refetchInterval: 15_000, queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/surveys/{survey_id}/report', { params: { path: { event_id: eventId, survey_id: surveyId } } })) })
  const event = useQuery({ queryKey: [...key, 'event'], gcTime: 0, retry: false, refetchOnWindowFocus: 'always', refetchInterval: 15_000, queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}', { params: { path: { event_id: eventId } } })) })
  useEffect(() => () => { cache.removeQueries({ queryKey: key }) }, [cache, key])
  return <section className="workspace survey-workspace"><Link to="/events/$eventId/surveys" params={{ eventId }}>← Surveys and feedback</Link><h1>Reviewed survey report</h1><Button variant="outline" onClick={() => void query.refetch()}>Refresh reviewed report</Button>{query.error || event.error ? <Failure error={query.error || event.error} retry={() => void query.refetch()} /> : query.isPending || event.isPending || !query.isFetchedAfterMount ? <Loading /> : <SurveyReportView report={query.data} timezone={event.data.configuration.timezone} />}</section>
}
export function SurveyReportView({ report, timezone }: { report: SurveyReport; timezone: string }) {
  return <section className="survey-report"><p>Fixed question version {report.survey_version} · Reviewed text version {report.reviewed_text_version || 'Not published'}{report.published_at && ` · Published ${localDate(report.published_at, timezone)}`}</p><p>{report.response_visibility}</p><p>Respondents: {report.respondents === null ? 'Suppressed; unknown to this report reader' : report.respondents}. Counts are self-reports, not independently demonstrated learning or adoption. Missing data is not zero achievement.</p>
    <p>This is one frozen cohort, with no live filters. Reviewed prose corrections do not recalculate its totals. These results do not calculate monetary ROI.</p>
    {report.reviewed_summary && <section><h3>Explicitly reviewed summary</h3><p className="help-problem">{report.reviewed_summary}</p></section>}
    {report.questions.map(question => <article className="panel" key={question.question_id}><h3>{question.label}</h3><p>Question version {question.question_version}</p>{question.measure && <p>Self-reported {question.measure.construct === 'self_reported_adoption' ? 'adoption' : 'learning'} · objective {question.measure.measure.objective.id} version {question.measure.measure.objective.version} · measure {question.measure.measure.measure_id}</p>}
      {question.suppressed ? <p>Suppressed or text-only. Counts, complementary cells and denominators are not disclosed.</p> : <><p>Answered denominator: {question.answered ?? 'Unknown'} respondents · Unknown/not answered: {question.unknown ?? 'Unknown'}. Multiple-choice selections may total more than the respondent count.</p>
        <div className="survey-bars" aria-hidden="true">{question.cells.map(cell => <div key={cell.value}><span>{cell.value}: {cell.count ?? 'Unknown'}</span>{cell.count !== null && <span className="survey-bar" style={{ width: `${Math.max(0, (cell.count ?? 0) / Math.max(1, ...question.cells.map(c => c.count ?? 0)) * 100)}%` }} />}</div>)}</div>
        <table><caption>{question.label}: accessible counts</caption><thead><tr><th scope="col">Answer</th><th scope="col">Respondents selecting this value</th></tr></thead><tbody>{question.cells.map(cell => <tr key={cell.value}><th scope="row">{cell.value}</th><td>{cell.count === null ? 'Unknown/suppressed' : cell.count}</td></tr>)}</tbody></table>
      </>}
    </article>)}
  </section>
}

import { useCallback } from 'react'
import { Link, useParams } from '@tanstack/react-router'
import { api, unwrap } from './api/client'
import { useIdentity } from './session'
import { Failure, Loading } from './components/states'
import { CollectionHeading } from './components/patterns'
import { localDate } from './event-time'
import { useSurveyPrivateRead } from './survey-private-read'
import type { SurveyReport } from './survey-context'

export function SurveyResultsPage() {
  const { eventId, surveyId } = useParams({ strict: false }) as { eventId: string; surveyId: string }, identity = useIdentity()
  return <PublishedResults key={`${identity.person.id}:${identity.draft_scope}:${eventId}:${surveyId}`} eventId={eventId} surveyId={surveyId} />
}
function PublishedResults({ eventId, surveyId }: { eventId: string; surveyId: string }) {
  const identity = useIdentity(), key = `${identity.person.id}:${identity.draft_scope}:${eventId}:${surveyId}:published-report`
  const load = useCallback(async (signal: AbortSignal) => {
    const [report, event] = await Promise.all([api.GET('/api/v1/events/{event_id}/surveys/{survey_id}/report', { signal, cache: 'no-store', params: { path: { event_id: eventId, survey_id: surveyId } } }).then(unwrap), api.GET('/api/v1/events/{event_id}', { signal, cache: 'no-store', params: { path: { event_id: eventId } } }).then(unwrap)])
    return { report, event }
  }, [eventId, surveyId])
  const query = useSurveyPrivateRead(key, load)
  return <section className="fleet-survey-workspace"><Link to="/events/$eventId/surveys" params={{ eventId }}>Surveys and feedback</Link><CollectionHeading title="Reviewed survey report" scope={{ event: query.data?.event.name, entity: 'Published frozen aggregate' }} description="A separately reviewed report; no raw response or live administration denominator replaces its suppressed cohort." secondary={[{ label: 'Refresh reviewed report', icon: 'refresh', onSelect: () => { void query.read(true).catch(() => {}) } }]} />
    {query.denied || !query.data && query.error ? <Failure error={query.error} retry={() => { void query.read(true).catch(() => {}) }} /> : !query.data ? <Loading /> : <>{!!query.error && <p role="alert">Current report authority could not be refreshed. Previously authorised facts are retained; no private draft replaces this report.</p>}<SurveyReportView report={query.data.report} timezone={query.data.event.configuration.timezone} /></>}
  </section>
}
export function SurveyReportView({ report, timezone }: { report: SurveyReport; timezone: string }) {
  return <section className="fleet-survey-report"><dl className="fleet-survey-facts"><div><dt>Fixed questions</dt><dd>Version {report.survey_version}</dd></div><div><dt>Reviewed text</dt><dd>Version {report.reviewed_text_version || 'Not published'}</dd></div><div><dt>Published</dt><dd>{report.published_at ? localDate(report.published_at, timezone) : 'Not published'}</dd></div></dl><p>{report.response_visibility}</p><p>Respondents: {report.suppressed || report.respondents === null ? 'Suppressed; unknown to this report reader' : report.respondents}. Counts are self-reports, not independently demonstrated learning or adoption. Missing data is not zero achievement.</p><p>This is one frozen cohort, with no live filters. Reviewed prose corrections do not recalculate its totals. These results do not calculate monetary ROI.</p>
    {!report.suppressed && report.reviewed_summary && <section className="fleet-survey-section"><h3>Explicitly reviewed summary</h3><p className="fleet-survey-reviewed-prose">{report.reviewed_summary}</p></section>}
    {report.questions.map(question => <section className="fleet-survey-section" key={question.question_id}><h3>{question.label}</h3><p>Question version {question.question_version}</p>{question.measure && <p>Self-reported {question.measure.construct === 'self_reported_adoption' ? 'adoption' : 'learning'} · objective {question.measure.measure.objective.id} version {question.measure.measure.objective.version} · measure {question.measure.measure.measure_id}</p>}
      {report.suppressed || question.suppressed ? <p>Suppressed or text-only. Counts, complementary cells and denominators are not disclosed.</p> : <><p>Answered denominator: {question.answered ?? 'Unknown'} respondents · Unknown/not answered: {question.unknown ?? 'Unknown'}. Multiple-choice selections may total more than the respondent count.</p><div className="fleet-survey-table"><table><caption>{question.label}: accessible counts</caption><thead><tr><th scope="col">Answer</th><th scope="col">Respondents selecting this value</th></tr></thead><tbody>{question.cells.map(cell => <tr key={cell.value}><th scope="row">{cell.value}</th><td>{cell.count === null ? 'Unknown/suppressed' : cell.count}</td></tr>)}</tbody></table></div></>}
    </section>)}
  </section>
}

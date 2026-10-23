import { useEffect, useMemo } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from '@tanstack/react-router'
import { api, privateKey, unwrap } from './api/client'
import { useIdentity } from './session'
import { Failure, Loading } from './components/states'
import { Button } from './components/ui/button'

// Native read-only destination for approved invitations. Response editing and
// organiser survey controls belong to the separate survey workspace delivery.
export function SurveyReader() {
  const { eventId, surveyId } = useParams({ strict: false }) as { eventId: string; surveyId: string }
  const identity = useIdentity(), cache = useQueryClient()
  const key = useMemo(() => [...privateKey(identity.person.id, eventId, 'own-survey'), surveyId], [identity.person.id, eventId, surveyId])
  const survey = useQuery({ queryKey: key, gcTime: 0, staleTime: 0, retry: false, refetchOnWindowFocus: 'always', refetchInterval: 15_000,
    queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/surveys/{survey_id}', { params: { path: { event_id: eventId, survey_id: surveyId } } })) })
  useEffect(() => () => { cache.removeQueries({ queryKey: key }) }, [cache, key])
  useEffect(() => { if (survey.isError) cache.removeQueries({ queryKey: key }) }, [survey.isError, cache, key])
  return <section className="workspace"><Link to="/inbox">← Your inbox</Link><h1>Post-event survey</h1><Button variant="outline" onClick={() => void survey.refetch()}>Refresh survey</Button>
    {survey.isPending || !survey.isFetchedAfterMount ? <Loading /> : survey.error ? <Failure error={survey.error} retry={() => void survey.refetch()} /> : survey.data && <article className="panel">
      <p className="eyebrow">Fixed question version {survey.data.version}</p><h2>{survey.data.form.name}</h2><p role="note">{survey.data.response_visibility}</p>
      <p>Opens {new Date(survey.data.opens_at).toLocaleString()} · Closes {new Date(survey.data.closes_at).toLocaleString()}</p>
      <p>{survey.data.can_respond ? 'This survey is open. Response editing is not available in this reader yet.' : 'Response editing is currently closed or unavailable.'}</p>
      <p>{survey.data.response ? `Your saved response: revision ${survey.data.response.revision}, submitted ${new Date(survey.data.response.submitted_at).toLocaleString()}.` : 'You have not submitted a response.'}</p>
      <ol>{survey.data.form.fields.map(field => <li key={field.id}><h3>{field.label}</h3>{field.help && <p>{field.help}</p>}
        {survey.data?.response && <p className="help-problem">{answerText(survey.data.response.answers[field.id])}</p>}
        {survey.data?.measures.filter(m => m.question_id === field.id).map(m => <p key={m.measure.measure_id}>Self-reported {m.construct === 'self_reported_adoption' ? 'adoption' : 'learning'} · objective version {m.measure.objective.version}. This does not independently evidence adoption.</p>)}
      </li>)}</ol>
    </article>}
  </section>
}
function answerText(value: unknown): string {
  if (value === undefined || value === null) return 'Unknown or not answered'
  if (typeof value === 'boolean') return value ? 'Yes' : 'No'
  if (Array.isArray(value)) return value.map(String).join(', ') || 'No choices selected'
  return String(value)
}

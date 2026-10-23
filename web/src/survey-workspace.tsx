import { useEffect, useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams, useSearch } from '@tanstack/react-router'
import { api, privateKey, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { Failure, Loading } from './components/states'
import { SurveyAuthoring } from './survey-authoring'
import { SurveyPrivateResponses, SurveyReportReview } from './survey-review'
import { SurveyInvitations } from './survey-invitations'
import { SurveyComparison } from './survey-comparison'
import { SurveyOutcomeObservations } from './survey-outcome-observations'
import { localDate } from './event-time'
import { nativeDenied, type SurveyConfiguration, type SurveyDashboard } from './survey-context'

export function SurveyManagePage() {
  const { eventId, surveyId } = useParams({ strict: false }) as { eventId: string; surveyId: string }, identity = useIdentity()
  return <Manage key={`${identity.person.id}:${identity.draft_scope}:${eventId}:${surveyId}`} eventId={eventId} surveyId={surveyId} />
}
function Manage({ eventId, surveyId }: { eventId: string; surveyId: string }) {
  const identity = useIdentity(), cache = useQueryClient(), [tab, setTab] = useState('overview'), [dirty, setDirty] = useState(false), [denied, setDenied] = useState<unknown>()
  const key = useMemo(() => [...privateKey(identity.person.id, eventId, 'survey-management'), identity.draft_scope, surveyId], [identity.person.id, identity.draft_scope, eventId, surveyId])
  const event = useQuery({ queryKey: [...key, 'event'], gcTime: 0, retry: false, refetchOnWindowFocus: 'always', refetchInterval: 15_000, queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}', { params: { path: { event_id: eventId } } })) })
  const dashboard = useQuery({ queryKey: [...key, 'dashboard'], gcTime: 0, retry: false, refetchOnWindowFocus: 'always', refetchInterval: 15_000, queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/surveys/{survey_id}/dashboard', { params: { path: { event_id: eventId, survey_id: surveyId } } })) })
  useEffect(() => () => { cache.removeQueries({ queryKey: key }) }, [cache, key])
  const error = denied || (nativeDenied(event.error) ? event.error : undefined) || (nativeDenied(dashboard.error) ? dashboard.error : undefined)
  const networkError = event.error || dashboard.error
  useEffect(() => { if (error) cache.removeQueries({ queryKey: key }) }, [cache, key, error])
  function deny(error: unknown) { setDenied(error); cache.removeQueries({ queryKey: ['private', identity.person.id, eventId] }) }
  function receive(configuration: SurveyConfiguration) { cache.setQueryData([...key, 'dashboard'], (value: SurveyDashboard | undefined) => value && ({ ...value, configuration })); void dashboard.refetch() }
  if (error) return <Failure error={error} retry={() => { setDenied(undefined); void event.refetch(); void dashboard.refetch() }} />
  if (!event.data || !dashboard.data) return networkError ? <Failure error={networkError} retry={() => { void event.refetch(); void dashboard.refetch() }} /> : <Loading />
  const current = dashboard.data, configuration = current.configuration
  const sections = ['overview', 'questions', 'invitations', 'restricted responses', 'reviewed report', 'compare editions']
  return <section className="workspace survey-workspace survey-private"><Link to="/events/$eventId/surveys" params={{ eventId }}>← Surveys and feedback</Link><p className="eyebrow">{event.data.name} · Current organisers only</p><h1>{configuration.definition.form.name}</h1><p>{current.response_visibility}</p>{!current.can_manage && <p className="notice">This event is read-only. Unsaved editing state is cleared; authorised history remains available until its privacy deadline.</p>}
    {!!networkError && <p role="alert">Current survey authority could not be refreshed. Local drafts and retry identities remain in this page; writes are paused until a successful refresh.</p>}
    <button onClick={() => { void event.refetch(); void dashboard.refetch() }}>Refresh survey authority</button>
    <nav className="tabs" aria-label="Survey sections">{sections.map(section => <button key={section} aria-current={tab === section ? 'page' : undefined} onClick={() => { if (section === tab) return; if (!dirty || window.confirm('Leave unsaved survey text or an uncertain operation? The private page state and retained retry command will be cleared.')) { setTab(section); setDirty(false) } }}>{section[0].toUpperCase() + section.slice(1)}</button>)}</nav>
    <fieldset disabled={!!networkError} className="survey-network-scope">
    {tab === 'questions' ? <SurveyAuthoring key={`${surveyId}:${current.can_manage}`} event={event.data} initial={configuration} writable={current.can_manage} onDirty={setDirty} receive={receive} deny={deny} /> : tab === 'restricted responses' ? <SurveyPrivateResponses timezone={event.data.configuration.timezone} eventId={eventId} configuration={configuration} deny={deny} /> : tab === 'reviewed report' ? <SurveyReportReview timezone={event.data.configuration.timezone} eventId={eventId} configuration={configuration} writable={current.can_manage && current.can_review_report} onDirty={setDirty} deny={deny} /> : tab === 'invitations' ? <SurveyInvitations event={event.data} configuration={configuration} writable={current.can_manage} onDirty={setDirty} deny={deny} /> : tab === 'compare editions' ? <SurveyComparison event={event.data} configuration={configuration} deny={deny} /> : <>
      <SurveyParticipation dashboard={current} /><section><h2>Configured objectives and success measures</h2><p>Definition references stay fixed. Baselines and targets are planning inputs, not achieved outcomes. Missing values are unknown and no monetary ROI is calculated.</p>{configuration.definition.measures.length === 0 ? <p>No success measure is linked to this questionnaire. This does not mean zero achievement.</p> : configuration.definition.measures.map(measure => <ObjectiveMeasure key={`${measure.question_id}:${measure.measure.measure_id}`} eventId={eventId} measure={measure} />)}</section>
      <SurveyOutcomeObservations eventId={eventId} timezone={event.data.configuration.timezone} deny={deny} />
      <p><Link to="/events/$eventId/surveys/$surveyId/results" params={{ eventId, surveyId }}>Open the separately published, suppressed report</Link></p>
    </>}
    </fieldset>
  </section>
}
export function SurveyParticipation({ dashboard }: { dashboard: SurveyDashboard }) {
  return <section><h2>Restricted participation and completion</h2><p>Observed {localDate(dashboard.observed_at, dashboard.timezone)}. Event configuration revision {dashboard.event_revision}. These are retained/current-authority administrative counts, not a published aggregate or proof of learning.</p><table><caption>Response completion denominators at the observation time</caption><thead><tr><th scope="col">Population</th><th scope="col">Submitted responses</th><th scope="col">Invited denominator</th></tr></thead><tbody><tr><th scope="row">Retained invited people</th><td>{dashboard.submitted_responses}</td><td>{dashboard.retained_invitations}</td></tr><tr><th scope="row">Currently eligible retained invitees</th><td>{dashboard.current_eligible_responses}</td><td>{dashboard.current_eligible_invitations}</td></tr></tbody></table><p>Units: people/responses. Source: native retained invitations and response heads. Personal removal can remove historical invitation mappings; these counts do not claim the original published cohort size. If its denominator is zero, a completion percentage is undefined, not zero achievement.</p><p>Published reports use their own one-time frozen, suppressed denominator. Their cohorts are never recalculated from these current counts.</p></section>
}
function ObjectiveMeasure({ eventId, measure }: { eventId: string; measure: SurveyConfiguration['definition']['measures'][number] }) {
  const identity = useIdentity()
  const query = useQuery({ queryKey: [...privateKey(identity.person.id, eventId, 'survey-exact-objective'), identity.draft_scope, measure.measure.objective.id, measure.measure.objective.version], gcTime: 0, retry: false,
    queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/objectives/{objective_id}', { params: { path: { event_id: eventId, objective_id: measure.measure.objective.id }, query: { version: measure.measure.objective.version } } })) })
  const definition = !query.error ? query.data?.definition : undefined, current = definition?.measures.find(item => item.id === measure.measure.measure_id)
  return <article className="panel"><h3>{definition?.description ?? 'Exact retained objective'}</h3><p>Objective {measure.measure.objective.id} version {measure.measure.objective.version} · measure {measure.measure.measure_id}</p>{query.error ? <Failure error={query.error} /> : query.isPending ? <Loading /> : current ? <><h4>{current.description}</h4><dl><div><dt>Unit</dt><dd>{current.unit || 'Not specified'}</dd></div><div><dt>Measurement method/source</dt><dd>{current.method || 'Not specified'} · {current.evidence_source || 'No independent evidence source specified'}</dd></div><div><dt>Baseline</dt><dd>{current.baseline ? `${current.baseline.value} (${current.baseline.basis})` : 'Unknown / not configured'}</dd></div><div><dt>Planning target</dt><dd>{current.target ?? 'Unknown / not configured'}</dd></div><div><dt>Review horizon</dt><dd>{current.review_horizon || 'Not configured'}</dd></div></dl><p>This question collects {measure.construct === 'self_reported_adoption' ? 'self-reported adoption' : 'self-reported learning'}, separately from demonstrated outcome evidence.</p></> : <p>The exact linked measure is unavailable. No substitute or zero value is used.</p>}</article>
}
export function SurveyCreatePage() {
  const { eventId } = useParams({ strict: false }) as { eventId: string }, identity = useIdentity(), source = useSearch({ strict: false }) as { fromEvent?: string; fromSurvey?: string }
  return <Create key={`${identity.person.id}:${identity.draft_scope}:${eventId}:${source.fromEvent}:${source.fromSurvey}`} eventId={eventId} fromEvent={source.fromEvent} fromSurvey={source.fromSurvey} />
}
function Create({ eventId, fromEvent, fromSurvey }: { eventId: string; fromEvent?: string; fromSurvey?: string }) {
  const identity = useIdentity(), [denied, setDenied] = useState<unknown>(), [configuration, setConfiguration] = useState<SurveyConfiguration>()
  const event = useQuery({ queryKey: [...privateKey(identity.person.id, eventId, 'new-survey-event'), identity.draft_scope], gcTime: 0, retry: false, refetchInterval: 15_000, queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}', { params: { path: { event_id: eventId } } })) })
  const permission = useQuery({ queryKey: [...privateKey(identity.person.id, eventId, 'new-survey-authority'), identity.draft_scope], gcTime: 0, retry: false, refetchInterval: 15_000, queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/surveys/configurations', { params: { path: { event_id: eventId }, query: { limit: 1 } } })) })
  const source = useQuery({ queryKey: [...privateKey(identity.person.id, fromEvent ?? eventId, 'survey-question-copy-source'), identity.draft_scope, fromSurvey], gcTime: 0, retry: false, enabled: !!fromEvent && !!fromSurvey, refetchOnWindowFocus: 'always', refetchInterval: 15_000, queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/surveys/{survey_id}/configuration', { params: { path: { event_id: fromEvent!, survey_id: fromSurvey! } } })) })
  const networkError = event.error || permission.error || source.error
  const denial = denied || [event.error, permission.error, source.error].find(nativeDenied)
  if (denial) return <Failure error={denial} />
  if (!event.data || !permission.data || !!fromEvent && !!fromSurvey && !source.data) return networkError ? <Failure error={networkError} retry={() => { void event.refetch(); void permission.refetch(); if (fromSurvey) void source.refetch() }} /> : <Loading />
  if (!permission.data.can_manage) return <section className="workspace survey-workspace"><h1>Survey creation is unavailable</h1><p>Current native authority does not permit creating a survey. No unsent draft is retained.</p></section>
  return <section className="workspace survey-workspace"><Link to="/events/$eventId/surveys" params={{ eventId }}>← Surveys and feedback</Link><h1>Create post-event survey</h1>{!!networkError && <p role="alert">Current authority could not be refreshed. The local draft remains in this page; writes are paused.</p>}<button onClick={() => { void event.refetch(); void permission.refetch(); if (fromSurvey) void source.refetch() }}>Refresh survey authority</button><fieldset disabled={!!networkError} className="survey-network-scope"><NewSurveyEditor key={permission.data.can_manage.toString()} event={event.data} configuration={configuration} source={source.data} crossEdition={fromEvent !== eventId} receive={setConfiguration} deny={setDenied} /></fieldset>{configuration && <Link className="primary-link" to="/events/$eventId/surveys/$surveyId/manage" params={{ eventId, surveyId: configuration.id }}>Open saved survey workspace</Link>}</section>
}
function NewSurveyEditor({ event, configuration, source, crossEdition, receive, deny }: { event: EventRecord; configuration?: SurveyConfiguration; source?: SurveyConfiguration; crossEdition: boolean; receive: (value: SurveyConfiguration) => void; deny: (error: unknown) => void }) {
  const identity = useIdentity(), [initial] = useState<SurveyConfiguration>(() => {
    const id = crypto.randomUUID(), opens = new Date(Math.max(Date.now() + 3_600_000, new Date(event.configuration.live_end).getTime() + 3_600_000))
    return { id, version: 0, definition: { form: source ? { ...structuredClone(source.definition.form), id } : { id, name: 'Post-event feedback', audience: 'attendee', fields: [] }, opens_at: opens.toISOString(), closes_at: new Date(opens.getTime() + 7 * 86_400_000).toISOString(), minimum_cohort: source?.definition.minimum_cohort ?? 5, measures: source && !crossEdition ? structuredClone(source.definition.measures) : [] }, author_id: identity.person.id, created_at: new Date().toISOString(), approved_at: null, published_at: null, cancelled: false }
  })
  const [, setDirty] = useState(false), [copiedVersion] = useState(source?.version)
  return <>{copiedVersion && <p>Questions deliberately reused from source question version {copiedVersion}. This is a new unapproved draft.{crossEdition && ' Exact success measures must be selected in this edition; no foreign measure is silently retargeted.'}{source?.version !== copiedVersion && ' The current source has changed; review the retained copy before approving.'}</p>}<SurveyAuthoring event={event} initial={configuration ?? initial} writable onDirty={setDirty} receive={receive} deny={deny} /></>
}

export function surveyCreateSearch(value: Record<string, unknown>) {
 const id=(v: unknown)=>typeof v==='string'&&/^[a-f0-9-]{36}$/i.test(v)?v:undefined
 return {fromEvent:id(value.fromEvent),fromSurvey:id(value.fromSurvey)}
}

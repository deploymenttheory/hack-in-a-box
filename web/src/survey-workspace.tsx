import { useCallback, useState } from 'react'
import { Link, useNavigate, useParams, useSearch } from '@tanstack/react-router'
import { api, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { Failure, Loading } from './components/states'
import { SurveyAuthoring } from './survey-authoring'
import { SurveyPrivateResponses, SurveyReportReview } from './survey-review'
import { SurveyInvitations } from './survey-invitations'
import { SurveyComparison } from './survey-comparison'
import { SurveyOutcomeObservations } from './survey-outcome-observations'
import { SurveyConfiguredObjectives } from './survey-objectives'
import { localDate } from './event-time'
import { CollectionHeading } from './components/patterns'
import { Button } from './components/ui/button'
import { SurveyAuthority, useSurveyPrivateRead } from './survey-private-read'
import { planningDepartureAllowed } from './planning-operations'
import { type SurveyConfiguration, type SurveyDashboard } from './survey-context'

export function SurveyManagePage() {
  const { eventId, surveyId } = useParams({ strict: false }) as { eventId: string; surveyId: string }, identity = useIdentity()
  return <Manage key={`${identity.person.id}:${identity.draft_scope}:${eventId}:${surveyId}`} eventId={eventId} surveyId={surveyId} />
}
function Manage({ eventId, surveyId }: { eventId: string; surveyId: string }) {
  const identity = useIdentity(), [tab, setTab] = useState('overview'), [dirty, setDirty] = useState(false)
  const key = `${identity.person.id}:${identity.draft_scope}:${eventId}:${surveyId}:management`
  const load = useCallback(async (signal: AbortSignal) => {
    const [event, dashboard] = await Promise.all([api.GET('/api/v1/events/{event_id}', { signal, cache: 'no-store', params: { path: { event_id: eventId } } }).then(unwrap), api.GET('/api/v1/events/{event_id}/surveys/{survey_id}/dashboard', { signal, cache: 'no-store', params: { path: { event_id: eventId, survey_id: surveyId } } }).then(unwrap)])
    return { event, dashboard }
  }, [eventId, surveyId])
  const authority = useSurveyPrivateRead(key, load)
  function deny(error: unknown) { setTab('overview'); setDirty(false); authority.revoke(error) }
  function retry() { setTab('overview'); setDirty(false); void authority.read(true).catch(() => {}) }
  function receive(configuration: SurveyConfiguration) { if (authority.data) authority.accept({ ...authority.data, dashboard: { ...authority.data.dashboard, configuration } }); void authority.read().catch(() => {}) }
  if (authority.denied || !authority.data && authority.error) return <Failure error={authority.error} retry={retry} />
  if (!authority.data) return <Loading />
  const event = { data: authority.data.event }, dashboard = { data: authority.data.dashboard }, networkError = authority.error
  const current = dashboard.data, configuration = current.configuration
  const sections = ['overview', 'questions', 'invitations', 'restricted responses', 'reviewed report', 'compare editions']
  return <SurveyAuthority value={authority.authority}><section className="fleet-survey-workspace survey-private"><Link to="/events/$eventId/surveys" params={{ eventId }}>Surveys and feedback</Link><CollectionHeading title={configuration.definition.form.name} scope={{ event: event.data.name, entity: "Current organisers only" }} description={current.response_visibility} secondary={[{ label: "Refresh survey authority", icon: "refresh", onSelect: () => { void authority.read().catch(() => {}) } }]} />{!current.can_manage && <p className="notice">This event is read-only. Unsaved editing state is cleared; authorised history remains available until its privacy deadline.</p>}
    {!!networkError && <p role="alert">Current survey authority could not be refreshed. Local drafts and retry identities remain in this page; writes are paused until a successful refresh.</p>}
    <nav className="fleet-survey-tabs" aria-label="Survey sections">{sections.map(section => <Button icon={section === 'overview' ? 'layout' : section === 'questions' ? 'list' : section === 'invitations' ? 'send' : section === 'restricted responses' ? 'shield' : 'columns'} variant="ghost" key={section} aria-current={tab === section ? 'page' : undefined} onClick={() => { if (section === tab || !planningDepartureAllowed()) return; if (!dirty || window.confirm('Leave unsaved survey text or an uncertain private payload? Private drafts are cleared; original safe command metadata stays unconfirmed.')) { setTab(section); setDirty(false) } }}>{section[0].toUpperCase() + section.slice(1)}</Button>)}</nav>
    <fieldset disabled={!!networkError} className="fleet-survey-fields">
    {tab === 'questions' ? <SurveyAuthoring key={`${surveyId}:${current.can_manage}`} event={event.data} initial={configuration} nextAction={{ label: 'Review invitation messaging', onSelect: () => setTab('invitations') }} writable={current.can_manage} onDirty={setDirty} receive={receive} deny={deny} /> : tab === 'restricted responses' ? <SurveyPrivateResponses timezone={event.data.configuration.timezone} eventId={eventId} configuration={configuration} deny={deny} /> : tab === 'reviewed report' ? <SurveyReportReview timezone={event.data.configuration.timezone} eventId={eventId} configuration={configuration} writable={current.can_manage && current.can_review_report} onDirty={setDirty} deny={deny} /> : tab === 'invitations' ? <SurveyInvitations event={event.data} configuration={configuration} writable={current.can_manage} onDirty={setDirty} deny={deny} /> : tab === 'compare editions' ? <SurveyComparison event={event.data} configuration={configuration} deny={deny} /> : <>
      <SurveyParticipation dashboard={current} /><SurveyConfiguredObjectives eventId={eventId} timezone={event.data.configuration.timezone} deny={deny} /><section><h2>Exact survey-linked objective snapshots</h2><p>Definition references stay fixed. Baselines and targets are planning inputs, not achieved outcomes. Missing values are unknown and no monetary ROI is calculated.</p>{configuration.definition.measures.length === 0 ? <p>No success measure is linked to this questionnaire. This does not mean zero achievement.</p> : configuration.definition.measures.map(measure => <ObjectiveMeasure key={`${measure.question_id}:${measure.measure.measure_id}`} eventId={eventId} measure={measure} />)}</section>
      <SurveyOutcomeObservations eventId={eventId} timezone={event.data.configuration.timezone} deny={deny} />
      <p><Link to="/events/$eventId/surveys/$surveyId/results" params={{ eventId, surveyId }}>Open the separately published, suppressed report</Link></p>
    </>}
    </fieldset>
  </section></SurveyAuthority>
}
export function SurveyParticipation({ dashboard }: { dashboard: SurveyDashboard }) {
  return <section><h2>Restricted participation and completion</h2><p>Observed {localDate(dashboard.observed_at, dashboard.timezone)}. Event configuration revision {dashboard.event_revision}. These are retained/current-authority administrative counts, not a published aggregate or proof of learning.</p><table><caption>Response completion denominators at the observation time</caption><thead><tr><th scope="col">Population</th><th scope="col">Submitted responses</th><th scope="col">Invited denominator</th></tr></thead><tbody><tr><th scope="row">Retained invited people</th><td>{dashboard.submitted_responses}</td><td>{dashboard.retained_invitations}</td></tr><tr><th scope="row">Currently eligible retained invitees</th><td>{dashboard.current_eligible_responses}</td><td>{dashboard.current_eligible_invitations}</td></tr></tbody></table><p>Units: people/responses. Source: native retained invitations and response heads. Personal removal can remove historical invitation mappings; these counts do not claim the original published cohort size. If its denominator is zero, a completion percentage is undefined, not zero achievement.</p><p>Published reports use their own one-time frozen, suppressed denominator. Their cohorts are never recalculated from these current counts.</p></section>
}
function ObjectiveMeasure({ eventId, measure }: { eventId: string; measure: SurveyConfiguration['definition']['measures'][number] }) {
  const identity = useIdentity(), key = `${identity.person.id}:${identity.draft_scope}:${eventId}:${measure.measure.objective.id}:${measure.measure.objective.version}:survey-exact-objective`
  const load = useCallback(async (signal: AbortSignal) => unwrap(await api.GET('/api/v1/events/{event_id}/objectives/{objective_id}', { signal, cache: 'no-store', params: { path: { event_id: eventId, objective_id: measure.measure.objective.id }, query: { version: measure.measure.objective.version } } })), [eventId, measure.measure.objective.id, measure.measure.objective.version])
  const query = useSurveyPrivateRead(key, load)
  const definition = !query.error ? query.data?.definition : undefined, current = definition?.measures.find(item => item.id === measure.measure.measure_id)
  return <article className="fleet-survey-section"><h3>{definition?.description ?? 'Exact retained objective'}</h3><p>Objective {measure.measure.objective.id} version {measure.measure.objective.version} · measure {measure.measure.measure_id}</p>{query.error ? <Failure error={query.error} /> : query.pending ? <Loading /> : current ? <><h4>{current.description}</h4><dl><div><dt>Unit</dt><dd>{current.unit || 'Not specified'}</dd></div><div><dt>Measurement method/source</dt><dd>{current.method || 'Not specified'} · {current.evidence_source || 'No independent evidence source specified'}</dd></div><div><dt>Baseline</dt><dd>{current.baseline ? `${current.baseline.value} (${current.baseline.basis})` : 'Unknown / not configured'}</dd></div><div><dt>Planning target</dt><dd>{current.target ?? 'Unknown / not configured'}</dd></div><div><dt>Review horizon</dt><dd>{current.review_horizon || 'Not configured'}</dd></div></dl><p>This question collects {measure.construct === 'self_reported_adoption' ? 'self-reported adoption' : 'self-reported learning'}, separately from demonstrated outcome evidence.</p></> : <p>The exact linked measure is unavailable. No substitute or zero value is used.</p>}</article>
}
export function SurveyCreatePage() {
  const { eventId } = useParams({ strict: false }) as { eventId: string }, identity = useIdentity(), source = useSearch({ strict: false }) as { fromEvent?: string; fromSurvey?: string }
  return <Create key={`${identity.person.id}:${identity.draft_scope}:${eventId}:${source.fromEvent}:${source.fromSurvey}`} eventId={eventId} fromEvent={source.fromEvent} fromSurvey={source.fromSurvey} />
}
function Create({ eventId, fromEvent, fromSurvey }: { eventId: string; fromEvent?: string; fromSurvey?: string }) {
  const identity = useIdentity(), [configuration, setConfiguration] = useState<SurveyConfiguration>()
  const key = `${identity.person.id}:${identity.draft_scope}:${eventId}:${fromEvent}:${fromSurvey}:create-survey`
  const load = useCallback(async (signal: AbortSignal) => {
    const [event, permission, source] = await Promise.all([api.GET('/api/v1/events/{event_id}', { signal, cache: 'no-store', params: { path: { event_id: eventId } } }).then(unwrap), api.GET('/api/v1/events/{event_id}/surveys/configurations', { signal, cache: 'no-store', params: { path: { event_id: eventId }, query: { limit: 1 } } }).then(unwrap), fromEvent && fromSurvey ? api.GET('/api/v1/events/{event_id}/surveys/{survey_id}/configuration', { signal, cache: 'no-store', params: { path: { event_id: fromEvent, survey_id: fromSurvey } } }).then(unwrap) : Promise.resolve(undefined)])
    return { event, permission, source }
  }, [eventId, fromEvent, fromSurvey])
  const authority = useSurveyPrivateRead(key, load), current = authority.data
  function deny(error: unknown) { setConfiguration(undefined); authority.revoke(error) }
  if (authority.denied) return <Failure error={authority.error} retry={() => { setConfiguration(undefined); void authority.read(true).catch(() => {}) }} />
  if (!current) return authority.error ? <Failure error={authority.error} retry={() => void authority.read(true).catch(() => {})} /> : <Loading />
  if (!current.permission.can_manage) return <section className="fleet-survey-workspace"><h1>Survey creation is unavailable</h1><p>Current native authority does not permit creating a survey. No unsent editor is mounted.</p></section>
  return <SurveyAuthority value={authority.authority}><section className="fleet-survey-workspace survey-private"><Link to="/events/$eventId/surveys" params={{ eventId }}>Surveys and feedback</Link><CollectionHeading title="Create post-event survey" scope={{ event: current.event.name, entity: "Unapproved draft" }} description="Build and review fixed questions before approval and publication. This does not send invitations." secondary={[{ label: "Refresh survey authority", icon: "refresh", onSelect: () => { void authority.read().catch(() => {}) } }]} />{!!authority.error && <p role="alert">Current authority could not be refreshed. The local draft remains in this page; writes are paused.</p>}<fieldset disabled={!!authority.error} className="fleet-survey-fields"><NewSurveyEditor event={current.event} configuration={configuration} source={current.source} crossEdition={fromEvent !== eventId} receive={setConfiguration} deny={deny} /></fieldset>{configuration && <Link to="/events/$eventId/surveys/$surveyId/manage" params={{ eventId, surveyId: configuration.id }}>Open saved survey workspace</Link>}</section></SurveyAuthority>
}
function NewSurveyEditor({ event, configuration, source, crossEdition, receive, deny }: { event: EventRecord; configuration?: SurveyConfiguration; source?: SurveyConfiguration; crossEdition: boolean; receive: (value: SurveyConfiguration) => void; deny: (error: unknown) => void }) {
  const identity = useIdentity(), navigate = useNavigate(), [initial] = useState<SurveyConfiguration>(() => {
    const id = crypto.randomUUID(), opens = new Date(Math.max(Date.now() + 3_600_000, new Date(event.configuration.live_end).getTime() + 3_600_000))
    return { id, version: 0, definition: { form: source ? { ...structuredClone(source.definition.form), id } : { id, name: 'Post-event feedback', audience: 'attendee', fields: [] }, opens_at: opens.toISOString(), closes_at: new Date(opens.getTime() + 7 * 86_400_000).toISOString(), minimum_cohort: source?.definition.minimum_cohort ?? 5, measures: source && !crossEdition ? structuredClone(source.definition.measures) : [] }, author_id: identity.person.id, created_at: new Date().toISOString(), approved_at: null, published_at: null, cancelled: false }
  })
  const [, setDirty] = useState(false), [copiedVersion] = useState(source?.version)
  return <>{copiedVersion && <p>Questions deliberately reused from source question version {copiedVersion}. This is a new unapproved draft.{crossEdition && ' Exact success measures must be selected in this edition; no foreign measure is silently retargeted.'}{source?.version !== copiedVersion && ' The current source has changed; review the retained copy before approving.'}</p>}<SurveyAuthoring creation nextAction={configuration ? { label: "Open saved survey workspace", onSelect: () => { void navigate({ to: "/events/$eventId/surveys/$surveyId/manage", params: { eventId: event.id, surveyId: configuration.id } }) } } : undefined} event={event} initial={configuration ?? initial} writable onDirty={setDirty} receive={receive} deny={deny} /></>
}

export function surveyCreateSearch(value: Record<string, unknown>) {
 const id=(v: unknown)=>typeof v==='string'&&/^[a-f0-9-]{36}$/i.test(v)?v:undefined
 return {fromEvent:id(value.fromEvent),fromSurvey:id(value.fromSurvey)}
}

import { useCallback, useEffect, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { api, APIError, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { localDate } from './event-time'
import { Field, Select } from './components/ui/fields'
import { CollectionHeading } from './components/patterns'
import { useSurveyPrivateRead, useSurveyPrivatePages } from './survey-private-read'
import { equivalentQuestion } from './survey-question-equivalence'
import { type SurveyConfiguration } from './survey-context'

export function SurveyComparison({ event, configuration, deny }: { event: EventRecord; configuration: SurveyConfiguration; deny: (error: unknown) => void }) {
  const identity = useIdentity(), [otherEvent, setOtherEvent] = useState(''), [otherSurvey, setOtherSurvey] = useState(''), [question, setQuestion] = useState(configuration.definition.form.fields[0]?.id ?? '')
  const key = `${identity.person.id}:${identity.draft_scope}:${event.id}:${configuration.id}:survey-comparison`
  const loadEditions = useCallback(async (signal: AbortSignal, after?: string) => unwrap(await api.GET('/api/v1/events', { signal, cache: 'no-store', params: { query: { after, limit: 50 } } })), [])
  const nextEdition = useCallback((page: Awaited<ReturnType<typeof loadEditions>>) => page.next_cursor ?? undefined, [])
  const editions = useSurveyPrivatePages(`${key}:events`, loadEditions, nextEdition)
  const loadConfigurations = useCallback(async (signal: AbortSignal, after?: string) => unwrap(await api.GET('/api/v1/events/{event_id}/surveys/configurations', { signal, cache: 'no-store', params: { path: { event_id: otherEvent }, query: { after, limit: 50 } } })), [otherEvent])
  const nextConfiguration = useCallback((page: Awaited<ReturnType<typeof loadConfigurations>>) => page.next_id ?? undefined, [])
  const configurations = useSurveyPrivatePages(`${key}:${otherEvent}:configurations`, loadConfigurations, nextConfiguration, !!otherEvent)
  const loadFirst = useCallback(async (signal: AbortSignal) => unwrap(await api.GET('/api/v1/events/{event_id}/surveys/{survey_id}/report', { signal, cache: 'no-store', params: { path: { event_id: event.id, survey_id: configuration.id } } })), [event.id, configuration.id])
  const first = useSurveyPrivateRead(`${key}:current-report`, loadFirst)
  const loadOther = useCallback(async (signal: AbortSignal) => {
    const [definition, report, edition] = await Promise.all([api.GET('/api/v1/events/{event_id}/surveys/{survey_id}/configuration', { signal, cache: 'no-store', params: { path: { event_id: otherEvent, survey_id: otherSurvey } } }).then(unwrap), api.GET('/api/v1/events/{event_id}/surveys/{survey_id}/report', { signal, cache: 'no-store', params: { path: { event_id: otherEvent, survey_id: otherSurvey } } }).then(unwrap), api.GET('/api/v1/events/{event_id}', { signal, cache: 'no-store', params: { path: { event_id: otherEvent } } }).then(unwrap)])
    if (definition.id !== otherSurvey || report.survey_id !== otherSurvey) throw new Error('The selected native report identity changed.')
    const measurement = await measureSnapshot(otherEvent, definition, question, signal)
    return { definition, report, edition, measurement }
  }, [otherEvent, otherSurvey, question])
  const second = useSurveyPrivateRead(`${key}:${otherEvent}:${otherSurvey}:${question}:other`, loadOther, !!otherEvent && !!otherSurvey)
  const loadMeasure = useCallback((signal: AbortSignal) => measureSnapshot(event.id, configuration, question, signal), [event.id, configuration, question])
  const currentMeasure = useSurveyPrivateRead(`${key}:${question}:${configuration.version}:measure`, loadMeasure)
  useEffect(() => { if (first.denied && first.error instanceof APIError && first.error.status !== 404) deny(first.error) }, [first.denied, first.error, deny])
  const other = !second.error ? second.data : undefined, a = !first.error ? first.data?.questions.find(item => item.question_id === question) : undefined, b = other?.report.questions.find(item => item.question_id === question)
  const equivalent = other && !currentMeasure.error && !first.error && equivalentQuestion(configuration, other.definition, question, currentMeasure.data ?? undefined, other.measurement ?? undefined) && first.data?.survey_version === configuration.version && other.report.survey_version === other.definition.version && a?.question_version === configuration.version && b?.question_version === other.definition.version
  const choices = editions.data?.pages.flatMap(page => page.items).filter(item => item.id !== event.id && item.can_read_staff) ?? []
  return <section className="fleet-survey-section"><CollectionHeading title="Compare supported metrics between editions" scope={{ event: event.name, entity: "Separately published frozen cohorts" }} secondary={[{ label: "Check comparison access", icon: "refresh", onSelect: () => { void editions.refetch(); void configurations.refetch(); void first.read(true).catch(() => {}); if (otherEvent && otherSurvey) void second.read(true).catch(() => {}); void currentMeasure.read(true).catch(() => {}) } }]} /><p>Only an exact retained question identity with equivalent definitions, conditional context, audience and measure provenance can be compared. Similar labels alone are insufficient. Counts retain separate cohorts and denominators; they are never pooled or treated as independently evidenced adoption.</p>
    <Field label="Other edition you currently organise"><Select value={otherEvent} onChange={e => { setOtherEvent(e.target.value); setOtherSurvey('') }}><option value="">Choose another edition</option>{choices.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</Select></Field>{editions.hasNextPage && <Button variant="outline" onClick={() => void editions.fetchNextPage()}>More editions</Button>}{!!editions.error && <Failure error={editions.error} />}
    {otherEvent && <><Field label="Published survey in that edition"><Select value={otherSurvey} onChange={e => setOtherSurvey(e.target.value)}><option value="">Choose a published survey</option>{configurations.data?.pages.flatMap(page => page.items).filter(item => item.published).map(item => <option key={item.id} value={item.id}>{item.name} · version {item.version}</option>)}</Select></Field>{configurations.hasNextPage && <Button variant="outline" onClick={() => void configurations.fetchNextPage()}>More edition surveys</Button>}{!!configurations.error && <Failure error={configurations.error} />}<p><Link to="/events/$eventId/surveys/new" params={{ eventId: otherEvent }} search={{ fromEvent: event.id, fromSurvey: configuration.id }}>Reuse these questions in a new draft for that edition</Link></p><p>Reusing questions preserves stable question identities. Exact success measures must be deliberately reselected in their owning edition; copying does not approve or publish anything.</p></>}
    <Field label="Retained question to compare"><Select value={question} onChange={e => setQuestion(e.target.value)}>{configuration.definition.form.fields.map(field => <option key={field.id} value={field.id}>{field.label}</option>)}</Select></Field>
    {!!first.error && <p role="status">The current published report is unavailable. No draft or private response replaces it.</p>}{(second.pending || currentMeasure.pending) && <Loading />}{!!second.error && <Failure error={second.error} />}{!!currentMeasure.error && <Failure error={currentMeasure.error} />}
    {other && (!equivalent ? <p role="status">These question definitions, versions or measure provenance are not equivalent. Do not combine or compare their metrics.</p> : a && b && <><p>Equivalent question definitions retained at separate versions {a.question_version} and {b.question_version}. Suppressed/unknown values remain unavailable.</p><table><caption>Separate edition counts for the equivalent question</caption><thead><tr><th scope="col">Metric</th><th scope="col">{event.name} · version {a.question_version}</th><th scope="col">Other edition · version {b.question_version}</th></tr></thead><tbody><tr><th scope="row">Answered denominator (people)</th><td>{a.answered ?? 'Suppressed / unknown'}</td><td>{b.answered ?? 'Suppressed / unknown'}</td></tr><tr><th scope="row">Unknown / not answered</th><td>{a.unknown ?? 'Suppressed / unknown'}</td><td>{b.unknown ?? 'Suppressed / unknown'}</td></tr>{a.cells.map(cell => <tr key={cell.value}><th scope="row">{cell.value}</th><td>{cell.count ?? 'Suppressed / unknown'}</td><td>{b.cells.find(item => item.value === cell.value)?.count ?? 'Suppressed / unknown'}</td></tr>)}</tbody></table><p>Sources: each edition’s separately published frozen survey report. Observation/publication dates: {first.data?.published_at ? localDate(first.data.published_at, event.configuration.timezone) : 'Unknown'} and {other.report.published_at ? localDate(other.report.published_at, other.edition.configuration.timezone) : 'Unknown'}. Estimates and self-reports remain separate from demonstrated outcomes; no unsupported monetary ROI is calculated.</p></>)}
  </section>
}

async function measureSnapshot(eventId: string, configuration: SurveyConfiguration, question: string, signal: AbortSignal) {
 const reference=configuration.definition.measures.find(item=>item.question_id===question)?.measure
 if(!reference)return null
 const value=unwrap(await api.GET('/api/v1/events/{event_id}/objectives/{objective_id}',{signal,cache:'no-store',params:{path:{event_id:eventId,objective_id:reference.objective.id},query:{version:reference.objective.version}}}))
 const measure=value.definition.measures.find(item=>item.id===reference.measure_id)
 if(!measure||value.reference.id!==reference.objective.id||value.reference.version!==reference.objective.version)throw new Error('The exact measure definition is unavailable.')
 return {objective_description:value.definition.description,measure}
}

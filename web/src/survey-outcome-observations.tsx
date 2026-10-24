import { useCallback, useEffect } from 'react'
import { api, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { localDate } from './event-time'
import { useSurveyPrivatePages } from './survey-private-read'

type Observation = components['schemas']['OutcomeObservation']
const bases = [
  { value: 'estimate', label: 'Estimates' },
  { value: 'self_report', label: 'Self-reported follow-through' },
  { value: 'demonstrated_adoption', label: 'Demonstrated adoption records' },
] as const

export function SurveyOutcomeObservations({ eventId, timezone, deny }: { eventId: string; timezone: string; deny: (error: unknown) => void }) {
  const identity = useIdentity(), key = `${identity.person.id}:${identity.draft_scope}:${eventId}:survey-outcome-observations`
  const load = useCallback(async (signal: AbortSignal, cursor?: string) => unwrap(await api.GET('/api/v1/events/{event_id}/outcomes/observations', { signal, cache: 'no-store', params: { path: { event_id: eventId }, query: { cursor, limit: 25 } } })), [eventId])
  const next = useCallback((page: Awaited<ReturnType<typeof load>>) => page.next_cursor ?? undefined, [])
  const query = useSurveyPrivatePages(key, load, next)
  useEffect(() => { if (query.denied) deny(query.error) }, [query.denied, query.error, deny])
  const observations = query.error ? [] : query.data?.pages.flatMap(page => page.items) ?? []
  return <section aria-label="Native follow-through observations"><h2>Follow-through evidence and observation provenance</h2><p>These are actual retained outcome records, separate from survey self-reports and planning targets. Each record keeps its own basis and exact measure definition. No counts are pooled, missing values stay unknown, and no monetary ROI is inferred.</p>
    {query.error ? <Failure error={query.error} retry={() => void query.refetch()} /> : query.isPending ? <Loading /> : <>
      {observations.length === 0 && <p>No retained follow-through observations are available. This does not mean zero adoption or zero achievement; review may still be pending.</p>}
      {bases.map(basis => { const rows = observations.filter(item => item.evidence.basis === basis.value); return rows.length > 0 && <section key={basis.value} aria-label={basis.label}><h3>{basis.label}</h3>{rows.map(item => <ObservationRecord key={`${item.outcome_id}:${item.evidence.id}`} item={item} timezone={timezone} />)}</section> })}
      {query.hasNextPage && <Button variant="outline" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>More native follow-through observations</Button>}
    </>}
  </section>
}
function ObservationRecord({ item, timezone }: { item: Observation; timezone: string }) {
  const evidence = item.evidence
  return <article className="fleet-survey-section"><h4>{evidence.claim}</h4><p>Retained outcome version {item.outcome_version}. This observation does not change a survey aggregate.</p><details><summary>Retained record identifiers</summary><p>Outcome {item.outcome_id} · Evidence {evidence.id}</p></details>
    <table><caption>Recorded observation values</caption><thead><tr><th scope="col">Metric</th><th scope="col">Recorded value</th></tr></thead><tbody><tr><th scope="row">Value</th><td>{evidence.value ?? 'Unknown / not recorded'}</td></tr><tr><th scope="row">Denominator</th><td>{evidence.denominator ?? 'Unknown / not recorded'}</td></tr><tr><th scope="row">Units</th><td>{item.measures.length ? item.measures.map(m => m.definition.unit || 'Not specified').join('; ') : 'No exact measure linked'}</td></tr></tbody></table>
    <dl><div><dt>Reported evidence source</dt><dd className="fleet-survey-reviewed-prose">{evidence.source || 'Not recorded'}</dd></div><div><dt>Observation date</dt><dd>{evidence.observed_at ? localDate(evidence.observed_at, timezone) : 'Unknown / not recorded'}</dd></div><div><dt>Record created</dt><dd>{localDate(item.recorded_at, timezone)} · {timezone}. This is not a substitute for a missing observation date.</dd></div></dl>
    {item.measures.map(measure => <section key={`${measure.reference.objective.id}:${measure.reference.objective.version}:${measure.reference.measure_id}`}><h5>{measure.definition.description}</h5><p>Exact retained objective version {measure.reference.objective.version}</p><details><summary>Exact measure reference</summary><p>Objective {measure.reference.objective.id} · measure {measure.reference.measure_id}</p></details><dl><div><dt>Measurement method</dt><dd>{measure.definition.method || 'Not specified'}</dd></div><div><dt>Configured evidence source</dt><dd>{measure.definition.evidence_source || 'Not specified'}</dd></div><div><dt>Retained baseline</dt><dd>{measure.definition.baseline ? `${measure.definition.baseline.value} (${measure.definition.baseline.basis})` : 'Unknown / not configured'}</dd></div><div><dt>Retained planning target</dt><dd>{measure.definition.target ?? 'Unknown / not configured'}</dd></div><div><dt>Review horizon</dt><dd>{measure.definition.review_horizon || 'Not specified'}</dd></div></dl></section>)}
  </article>
}

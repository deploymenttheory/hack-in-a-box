import { useCallback, useEffect } from 'react'
import { api, unwrap } from './api/client'
import { useIdentity } from './session'
import { Failure, Loading } from './components/states'
import { Button } from './components/ui/button'
import { localDate } from './event-time'
import { useSurveyPrivatePages } from './survey-private-read'

export function SurveyConfiguredObjectives({ eventId, timezone, deny }: { eventId: string; timezone: string; deny: (error: unknown) => void }) {
  const identity = useIdentity(), key = `${identity.person.id}:${identity.draft_scope}:${eventId}:survey-configured-objectives`
  const load = useCallback(async (signal: AbortSignal, cursor?: string) => unwrap(await api.GET('/api/v1/events/{event_id}/objectives', { signal, cache: 'no-store', params: { path: { event_id: eventId }, query: { cursor, limit: 25 } } })), [eventId])
  const next = useCallback((page: Awaited<ReturnType<typeof load>>) => page.next_cursor ?? undefined, [])
  const query = useSurveyPrivatePages(key, load, next)
  useEffect(() => { if (query.denied) deny(query.error) }, [query.denied, query.error, deny])
  const items = query.error ? [] : query.data?.pages.flatMap(page => page.items) ?? []
  return <section aria-label="Configured event objectives"><h2>Configured event planning objectives</h2><p>These are current native planning definitions, including objectives without a survey question. Targets, baseline estimates and evidence instructions are not achieved outcomes. Retained survey and follow-through definitions remain separate when planning versions change.</p>
    {query.error ? <Failure error={query.error} retry={() => void query.refetch()} /> : query.isPending ? <Loading /> : <>
      {items.length === 0 && <p>No event planning objectives are configured. This does not mean zero achievement.</p>}
      {items.map(item => <article className="fleet-survey-section" key={`${item.reference.id}:${item.reference.version}`}><h3>{item.definition.description}</h3><p>Current planning version {item.reference.version}{item.retired && ' · Retired'} · Definition recorded {localDate(item.at, timezone)}. This is not an outcome observation date.</p><details><summary>Planning reference</summary><p>{item.reference.id} version {item.reference.version}</p></details>
        {item.definition.measures.length === 0 && <p>No success measure is configured for this objective. Its achieved result is unknown.</p>}
        {item.definition.measures.map(measure => <section key={measure.id}><h4>{measure.description}</h4><table><caption>Current planning definition</caption><thead><tr><th scope="col">Field</th><th scope="col">Configured value</th></tr></thead><tbody><tr><th scope="row">Unit</th><td>{measure.unit || 'Not specified'}</td></tr><tr><th scope="row">Baseline</th><td>{measure.baseline ? `${measure.baseline.value} (${measure.baseline.basis === 'self_report' ? 'self-report' : 'estimate'})` : 'Unknown / not configured'}</td></tr><tr><th scope="row">Planning target</th><td>{measure.target ?? 'Unknown / not configured'}</td></tr><tr><th scope="row">Measurement method</th><td>{measure.method || 'Not specified'}</td></tr><tr><th scope="row">Expected evidence source</th><td>{measure.evidence_source || 'Not specified'}</td></tr><tr><th scope="row">Review horizon</th><td>{measure.review_horizon || 'Not specified'}</td></tr></tbody></table></section>)}
      </article>)}
      {query.hasNextPage && <Button variant="outline" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>More configured event objectives</Button>}
    </>}
  </section>
}

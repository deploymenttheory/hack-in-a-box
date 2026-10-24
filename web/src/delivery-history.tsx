import { CollectionHeading, CollectionPagination, CollectionRecords, CollectionToolbar } from './components/patterns/collection'
import { StatusBadge } from './components/ui/feedback'
import { Select, Label } from './components/ui/fields'
import { CommunicationStep } from './communication-layout'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useParams } from '@tanstack/react-router'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, privateKey, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Failure, Loading } from './components/states'
import { DeliveryContext, stateName, denied, useDelivery, useDeliveryDenial } from './delivery-context'
import { DeliveryReview } from './delivery-review'

type Channel = 'smtp' | 'teams' | 'in_app'
const channelName = { smtp: 'SMTP email', teams: 'Teams shared channel', in_app: 'Private in-app inbox' }

export function DeliveryHistoryPage() {
  const { eventId, deliveryId } = useParams({ strict: false }), identity = useIdentity()
  return eventId ? <Workspace key={`${identity.person.id}:${identity.draft_scope}:${eventId}:${deliveryId ?? ''}`} eventID={eventId} deliveryID={deliveryId} /> : <Failure error={new Error('Missing event')} />
}

function Workspace({ eventID, deliveryID }: { eventID: string; deliveryID?: string }) {
  const identity = useIdentity(), cache = useQueryClient(), [failure, setFailure] = useState<unknown>()
  const key = useMemo(() => [...privateKey(identity.person.id, eventID, 'delivery-history'), identity.draft_scope, deliveryID ?? 'index'], [identity.person.id, identity.draft_scope, eventID, deliveryID])
  const event = useQuery({ queryKey: [...key, 'event'], gcTime: 0, retry: false, refetchOnWindowFocus: 'always', refetchInterval: 10_000,
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/events/{event_id}', { signal, cache: 'no-store', params: { path: { event_id: eventID } } })) })
  const deny = useCallback((error: unknown) => { if (denied(error)) setFailure(error) }, [])
  useEffect(() => () => { void cache.cancelQueries({ queryKey: key }); cache.removeQueries({ queryKey: key }) }, [cache, key])
  const inaccessible = failure || (denied(event.error) ? event.error : undefined)
  useEffect(() => { if (inaccessible) cache.removeQueries({ queryKey: key }) }, [inaccessible, cache, key])
  if (inaccessible) return <section className="workspace"><Link to="/events/$eventId" params={{ eventId: eventID }}>← Back to event</Link><h1>Delivery history</h1><Failure error={inaccessible} retry={() => { setFailure(undefined); void event.refetch() }} /></section>
  if (!event.data) return event.error ? <Failure error={event.error} retry={() => void event.refetch()} /> : <Loading />
  return <DeliveryContext.Provider value={{ event: event.data, key, deny }}><section className="workspace delivery-history fleet-communications"><Link to="/events/$eventId" params={{ eventId: eventID }}>← Back to event</Link><CollectionHeading title="Delivery history and failure review" scope={{ event: event.data.name, entity: 'Communications' }} description={`Times in ${event.data.configuration.timezone}`} /><p>Provider acceptance is not confirmed delivery or reading. Cancelling pending work cannot recall an earlier accepted or uncertain send.</p>{deliveryID ? <DeliveryReview id={deliveryID} /> : <HistoryIndex />}</section></DeliveryContext.Provider>
}

function HistoryIndex() {
  const { event, key } = useDelivery(), [campaign, setCampaign] = useState(''), [channel, setChannel] = useState('')
  const providers = useQuery({ queryKey: [...key, 'campaigns'], gcTime: 0, retry: false, refetchInterval: 10_000, refetchOnWindowFocus: 'always',
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/events/{event_id}/communications/campaigns', { signal, cache: 'no-store', params: { path: { event_id: event.id } } })) })
  const query = useInfiniteQuery({ queryKey: [...key, 'deliveries', campaign, channel], gcTime: 0, retry: false, initialPageParam: undefined as components['schemas']['DeliveryHistoryCursor'] | undefined, refetchInterval: 10_000, refetchOnWindowFocus: 'always',
    queryFn: async ({ signal, pageParam }) => unwrap(await api.GET('/api/v1/events/{event_id}/communications/deliveries', { signal, cache: 'no-store', params: { path: { event_id: event.id }, query: { campaign_id: campaign || undefined, channel: channel ? channel as Channel : undefined, before_at: pageParam?.at, before_id: pageParam?.id, limit: 30 } } })), getNextPageParam: page => page.next_cursor ?? undefined })
  useDeliveryDenial(query.error); useDeliveryDenial(providers.error)
  if (denied(query.error) || denied(providers.error)) return null
  const pages = query.data?.pages, latest = pages?.[0]
  return <><CommunicationStep title="Provider health and readiness" icon="shield" description="Local configuration checks only. Opening this page makes no provider request and does not prove live delivery.">{providers.isPending ? <Loading /> : providers.error ? <Failure error={providers.error} retry={() => void providers.refetch()} /> : <ul>{providers.data?.providers.map(provider => <li key={provider.channel}><strong>{channelName[provider.channel as Channel]}</strong>: {provider.available ? 'Configured and locally ready' : 'Unavailable'} · {provider.reason.replaceAll('_', ' ')}{provider.channel === 'teams' && <p>Shared-channel receipt must be checked with the workflow owner. A provider acceptance does not confirm channel receipt.</p>}</li>)}</ul>}</CommunicationStep>
    {latest?.paused && <p className="notice">Communication automation is paused. Current recovery checks must permit any later retry or resend; existing outcomes are retained.</p>}{latest?.archived && <p className="notice">This event is read-only. Delivery history remains available; recovery decisions are frozen.</p>}
    <section aria-label="Retained recipient selections"><h2>Retained recipient selections</h2><p>The selection timestamp records the original intent, separately from prepared attempts and receipts. Recipient addresses and message bodies are excluded.</p>
      <CollectionToolbar source="cursor" filters={<div className="fleet-delivery-filters"><div><Label htmlFor="delivery-campaign">Campaign</Label><Select id="delivery-campaign" value={campaign} onChange={e => setCampaign(e.target.value)}><option value="">All campaigns and provider checks</option>{providers.data?.campaigns.map(c => <option key={c.campaign.definition.id} value={c.campaign.definition.id}>{c.campaign.definition.name}</option>)}</Select></div><div><Label htmlFor="delivery-channel">Channel</Label><Select id="delivery-channel" value={channel} onChange={e => setChannel(e.target.value)}><option value="">All channels</option>{Object.entries(channelName).map(([value, name]) => <option key={value} value={value}>{name}</option>)}</Select></div></div>} />
      {query.isPending ? <Loading /> : query.error ? <Failure error={query.error} retry={() => void query.refetch()} /> : <><CollectionRecords label="Retained delivery selections" rows={pages?.flatMap(page => page.items) ?? []} rowKey={item => item.id} rowLabel={item => item.recipient_name || (item.channel === 'teams' ? 'Shared event channel' : 'Retained recipient')}
        destination={item => ({ href: `/events/${encodeURIComponent(event.id)}/communications/deliveries/${encodeURIComponent(item.id)}`, label: item.recipient_name || (item.channel === 'teams' ? 'Shared event channel' : 'Retained recipient') })}
        columns={[{ id: 'channel', label: 'Channel', cell: item => channelName[item.channel] }, { id: 'state', label: 'Outcome', essential: true, cell: item => <StatusBadge tone={item.state === 'ambiguous' ? 'warning' : 'neutral'}>{stateName(item.state)}</StatusBadge> }, { id: 'selected', label: 'Selected', cell: item => new Date(item.selected_at).toLocaleString() }, { id: 'attempts', label: 'Prepared attempts', cell: item => item.attempts }]} />
        {!pages?.some(page => page.items.length) && <p className="empty-state">No retained delivery evidence matches these filters. No send is implied.</p>}
        <CollectionPagination label="Retained delivery history pages" source={{ kind: 'cursor', loaded: pages?.reduce((sum, page) => sum + page.items.length, 0) ?? 0, busy: query.isFetchingNextPage, next: query.hasNextPage ? () => void query.fetchNextPage() : undefined }} />
      </>}
    </section></>
}

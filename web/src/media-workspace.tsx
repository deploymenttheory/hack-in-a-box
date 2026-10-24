import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { Link, useParams } from '@tanstack/react-router'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, APIError, getSession, privateKey, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Failure, Loading } from './components/states'
import { Button } from './components/ui/button'
import { CollectionHeading, CollectionPagination, ContextLink } from './components/patterns'
import { usePrivateRequestScope } from './private-request-scope'
import { MediaLifetime } from './media-lifetime'
import { MediaDecisionHistory } from './media-history'
import { MediaGallery } from './media-gallery'
import { MediaCollector, MediaEditor } from './media-editor'

type Item = components['schemas']['MediaItem']
export type MediaContext = { eventID: string; revision: number; refresh: () => Promise<void>; deny: (error: unknown) => void; writable: boolean; authorityCurrent: boolean; lifetime:MediaLifetime }
export const mediaDenied = (error: unknown) => error instanceof APIError && [401, 403, 404].includes(error.status)

export function MediaWorkspacePage() {
  const { eventId, mediaId } = useParams({ strict: false }), identity = useIdentity()
  return eventId ? <MediaWorkspace key={`${identity.person.id}:${identity.draft_scope}:${eventId}:${mediaId ?? ''}`} eventID={eventId} mediaID={mediaId} /> : <Failure error={new Error('Missing event')} />
}
export function OwnPhotoConsentPage() {
  const { eventId } = useParams({ strict: false }), identity = useIdentity()
  return eventId ? <section className="workspace media-workspace" key={`${identity.person.id}:${identity.draft_scope}:${eventId}`}><h1>Your event photo consent</h1><p>This self-only choice does not grant gallery access. Withdrawing consent blocks future protected media access; it cannot recall copies already downloaded or recordings hosted elsewhere.</p><PhotoConsent eventID={eventId} refresh={async () => {}} deny={() => {}} /></section> : <Failure error={new Error('Missing event')} />
}
function MediaWorkspace({ eventID, mediaID }: { eventID: string; mediaID?: string }) {
  const identity = useIdentity(), cache = useQueryClient(), [denial, setDenial] = useState<unknown>()
  const key = useMemo(() => [...privateKey(identity.person.id, eventID, 'media'), identity.draft_scope], [identity.person.id, identity.draft_scope, eventID])
  const scope=usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${eventID}:media-workspace`),readEpoch=useRef(0)
  const [projectionEpoch,bumpProjection]=useState(0)
  const lifetime=useMemo(()=>new MediaLifetime(scope.key,scope,()=>bumpProjection(value=>value+1)),[scope])
  const deny=useCallback((error:unknown)=>{
    if(!mediaDenied(error))return
    readEpoch.current++;lifetime.deny();setDenial(error)
    cache.setQueryData(key,{pages:[{revision:0,can_collect:false,can_review:false,items:[],next_cursor:null}],pageParams:[undefined]})
    const owns=(q:{queryKey:readonly unknown[]})=>q.queryKey[0]==='private'&&q.queryKey[1]===identity.person.id&&q.queryKey[2]===eventID&&(['media-history','media-guidance'].includes(String(q.queryKey[3]))||(q.queryKey[3]==='media'&&q.queryKey.includes('detail')))
    void cache.cancelQueries({predicate:owns});cache.removeQueries({predicate:owns})
  },[cache,eventID,identity.person.id,key,lifetime])
  const query = useInfiniteQuery({ queryKey: key, gcTime: 0, retry: false, initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam, signal }) => {const ticket=scope.capture(),epoch=readEpoch.current;try{const page=unwrap(await api.GET('/api/v1/events/{event_id}/media', { signal, cache: 'no-store', params: { path: { event_id: eventID }, query: { after: pageParam, limit: 30 } } }));ticket.assert();if(epoch!==readEpoch.current)throw new DOMException('Media audience ended','AbortError');lifetime.acceptPage(page);return page}catch(error){if(ticket.current()&&epoch===readEpoch.current)deny(error);throw error}}, getNextPageParam: page => page.next_cursor ?? undefined, refetchInterval: denial?false:10_000, refetchOnWindowFocus: denial?false:'always' })
  const detail = useQuery({ queryKey: [...key, 'detail', mediaID], enabled: !!mediaID && !query.error&&!denial, retry: false, gcTime: 0, refetchInterval: denial?false:10_000, refetchOnWindowFocus: denial?false:'always', queryFn: async ({ signal }) => {const ticket=scope.capture(),epoch=readEpoch.current;try{const item=unwrap(await api.GET('/api/v1/events/{event_id}/media/{media_id}', { signal, cache: 'no-store', params: { path: { event_id: eventID, media_id: mediaID! } } }));ticket.assert();if(epoch!==readEpoch.current)throw new DOMException('Media audience ended','AbortError');lifetime.acceptItem(item);return item}catch(error){if(ticket.current()&&epoch===readEpoch.current)deny(error);throw error}} })
  useLayoutEffect(()=>()=>{lifetime.deny();void cache.cancelQueries({queryKey:key});cache.removeQueries({queryKey:key})},[cache,key,lifetime])
  async function refresh() { const result = await query.refetch();if(result.error)return;if(mediaID){const current=await detail.refetch();if(current.error)return}setDenial(undefined) }
  const readError = query.error || (mediaID ? detail.error : undefined), currentError = denial || (mediaDenied(query.error) ? query.error : mediaID && mediaDenied(detail.error) ? detail.error : undefined), page = query.data?.pages[0]
  const authorityCurrent = !readError
  if (currentError) return <section className="workspace"><h1>Event media</h1><Link to="/events/$eventId/photo-consent" params={{ eventId: eventID }}>Manage your own photo consent</Link><p>Current media access could not be verified. Images, local forms and private review details have been cleared.</p><Failure error={currentError} retry={() => void refresh()} /></section>
  if (!page || (mediaID && !detail.data)) return readError ? <Failure error={readError} retry={() => void refresh()} /> : <Loading />
  const context: MediaContext = { eventID, revision: page.revision, refresh, deny, writable: page.can_collect && authorityCurrent&&lifetime.canWrite(), authorityCurrent,lifetime }
  const currentCollect=page.can_collect&&lifetime.canWrite()
  const items = (mediaID ? [detail.data!] : query.data!.pages.flatMap(p => p.items)).map(item=>lifetime.record(item.id)?.item??item)
  return <section className="fleet-collection"><ContextLink back target={{label:'Back to event',href:`/events/${eventID}`}}/><CollectionHeading title="Event media" scope={{entity:mediaID?'Current media record':'Current authorised gallery'}}/>
    <p>Only explicitly reviewed media enters the gallery. Identified people must currently consent. Revocation blocks future access here; it cannot recall copies someone already downloaded or remove recordings hosted elsewhere.</p>
    {!!readError && <><Failure error={readError} retry={() => void refresh()} /><p role="status">Current access could not be refreshed. Private drafts and uncertain intents remain only in this page. Media is hidden and writes are paused until access is checked again.</p></>}
    <div hidden={!authorityCurrent}><PhotoConsent eventID={eventID} refresh={refresh} deny={deny} /></div>
    {!mediaID && currentCollect && <div hidden={!authorityCurrent}><MediaCollector key={lifetime.collector.key} context={context} /></div>}
    <div hidden={!authorityCurrent}><MediaGallery eventID={eventID} title={page.can_review&&currentCollect ? 'Organiser review and gallery' : currentCollect ? 'Your submissions and released gallery' : 'Reviewed gallery'} items={items} renderContent={item=><MediaContent key={`${item.id}:${lifetime.record(item.id)?.epoch??projectionEpoch}`} item={item} context={context}/>}/></div>
    {!mediaID && <CollectionPagination source={{kind:'cursor',loaded:items.length,busy:query.isFetchingNextPage,next:query.hasNextPage?()=>void query.fetchNextPage():undefined}}/>}
  </section>
}
function PhotoConsent({ eventID, refresh, deny }: { eventID: string; refresh: () => Promise<void>; deny: (error: unknown) => void }) {
  const identity = useIdentity(), [failure, setFailure] = useState<unknown>(), [pending, setPending] = useState(false), [uncertain, setUncertain] = useState(false)
  const query = useQuery({ queryKey: [...privateKey(identity.person.id, eventID, 'photo-consent'), identity.draft_scope], gcTime: 0, retry: false, refetchInterval: 10_000, queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/events/{event_id}/photo-consent', { signal, cache: 'no-store', params: { path: { event_id: eventID } } })) })
  const generation = useRef(0)
  useEffect(() => () => { generation.current++ }, [])
  useEffect(() => { if (query.error) generation.current++ }, [query.error])
  async function choose(choice: 'yes' | 'no' | 'unknown') {
    if (!query.data || pending || uncertain) return
    const ticket = generation.current, version = query.data.revision
    setPending(true); setFailure(undefined)
    try {
      const current = await getSession()
      if (ticket !== generation.current || current.person.id !== identity.person.id || current.draft_scope !== identity.draft_scope) return
      unwrap(await api.PUT('/api/v1/events/{event_id}/photo-consent', { params: { path: { event_id: eventID }, header: { 'X-CSRF-Token': current.csrf_token, 'If-Match': `"${version}"` } }, body: { choice } }))
      if (ticket === generation.current) { await query.refetch(); await refresh() }
    } catch (error) { if (ticket === generation.current) { setFailure(error); deny(error); if (!(error instanceof APIError && error.status < 500)) setUncertain(true); await query.refetch(); await refresh() } }
    finally { if (ticket === generation.current) setPending(false) }
  }
  if (query.error) return <Failure error={query.error} retry={() => void query.refetch()} />
  if (!query.data) return <Loading />
  return <section className="panel media-consent"><h2>Your photo consent</h2><p>Current choice: <strong>{query.data.choice}</strong>. This changes only your event photo consent, without editing other registration details.</p><div className="actions"><Button disabled={pending || uncertain || !query.data.can_opt_in} onClick={() => void choose('yes')}>Allow identifiable event media</Button><Button variant="outline" disabled={pending || uncertain || !query.data.can_revoke} onClick={() => void choose('no')}>Do not allow identifiable media</Button><Button variant="outline" disabled={pending || uncertain || !query.data.can_revoke} onClick={() => void choose('unknown')}>Withdraw my consent choice</Button></div>{uncertain && <p role="status">The save outcome is uncertain. Your current choice was refreshed above. Check it before making another explicit choice. <Button variant="outline" onClick={() => { setUncertain(false); setFailure(undefined) }}>I have checked my current choice</Button></p>}{!!failure && <Failure error={failure} />}</section>
}
function MediaContent({item,context}:{item:Item;context:MediaContext}) {
 return <>{item.kind==='photo'&&item.can_read_original&&<ProtectedPhoto key={context.authorityCurrent?'current':'paused'} item={item} context={context}/ >}{item.recording_url&&<p><a href={item.recording_url} target="_blank" rel="noopener noreferrer">Open reviewed external recording</a> · Hosted elsewhere; this app does not host or recall third-party copies.</p>}{item.can_edit&&<MediaEditor item={item} context={context}/>} {item.can_read_history&&<MediaDecisionHistory eventID={context.eventID} mediaID={item.id}/>}</>
}
function ProtectedPhoto({ item, context }: { item: Item; context: MediaContext }) {
  const [url, setURL] = useState(''), [error, setError] = useState<unknown>(), [pending, setPending] = useState(false), epoch = useRef(0), controller = useRef<AbortController | null>(null)
  const clear = useCallback(() => { epoch.current++; controller.current?.abort(); controller.current = null; setURL(''); setPending(false) }, [])
  useEffect(() => { const hide = () => clear(); window.addEventListener('session-expired', hide); window.addEventListener('blur', hide); document.addEventListener('visibilitychange', hide); window.addEventListener('beforeprint', hide); return () => { hide(); window.removeEventListener('session-expired', hide); window.removeEventListener('blur', hide); document.removeEventListener('visibilitychange', hide); window.removeEventListener('beforeprint', hide) } }, [clear])
  useEffect(() => { if (!url) return; const timer = window.setTimeout(clear, 30_000); return () => { window.clearTimeout(timer); URL.revokeObjectURL(url) } }, [url, clear])
  async function view(original: boolean) {
    clear(); setPending(true); setError(undefined); const ticket = epoch.current, abort = new AbortController(); controller.current = abort
    try {
      const result = original ? await api.GET('/api/v1/events/{event_id}/media/{media_id}/original', { signal: abort.signal, cache: 'no-store', parseAs: 'blob', params: { path: { event_id: context.eventID, media_id: item.id } } }) : await api.GET('/api/v1/events/{event_id}/media/{media_id}/thumbnail', { signal: abort.signal, cache: 'no-store', parseAs: 'blob', params: { path: { event_id: context.eventID, media_id: item.id } } })
      if (ticket !== epoch.current || abort.signal.aborted || !context.authorityCurrent) return
      const blob = unwrap(result), address = URL.createObjectURL(new Blob([blob], { type: 'image/png' }))
      if (original) { const anchor = document.createElement('a'); anchor.href = address; anchor.download = `${item.id}-original`; anchor.click(); window.setTimeout(() => URL.revokeObjectURL(address), 1000) } else setURL(address)
    } catch (failure) { if (ticket === epoch.current) { setError(failure); context.deny(failure) } } finally { if (ticket === epoch.current) setPending(false) }
  }
  if (!context.authorityCurrent) return null
  return <div className="media-photo"><div className="actions"><Button variant="outline" disabled={pending} onClick={() => void view(false)}>View protected photo</Button><Button variant="outline" disabled={pending} onClick={() => void view(true)}>Download original</Button>{url && <Button variant="outline" onClick={clear}>Hide photo</Button>}</div>{url && <img src={url} alt={item.title} />}{!!error && <Failure error={error} />}</div>
}

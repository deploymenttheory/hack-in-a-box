import { useState } from 'react'
import { useInfiniteQuery } from '@tanstack/react-query'
import { api, privateKey, unwrap } from './api/client'
import { useIdentity } from './session'
import { usePrivateRequestScope } from './private-request-scope'
import { Button } from './components/ui/button'
import { CollectionPagination, CollectionRecords } from './components/patterns'
import { Failure, Loading } from './components/states'

// The native current can_read_history capability is separate from write access.
// A historical metadata entry is never a byte grant or original write receipt.
export function MediaDecisionHistory({eventID,mediaID}:{eventID:string;mediaID:string}) {
 const [open,setOpen]=useState(false)
 return <section className="media-private py-4 border-t border-border"><Button icon="list" variant="outline" onClick={()=>setOpen(value=>!value)}>{open?'Hide':'Read'} media decision history</Button>{open&&<HistoryReader eventID={eventID} mediaID={mediaID}/>}</section>
}
function HistoryReader({eventID,mediaID}:{eventID:string;mediaID:string}) {
 const identity=useIdentity(),[mountID]=useState(()=>crypto.randomUUID()),scope=usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${eventID}:${mediaID}:history:${mountID}`)
 const query=useInfiniteQuery({queryKey:[...privateKey(identity.person.id,eventID,'media-history'),identity.draft_scope,mediaID,mountID],gcTime:0,retry:false,refetchOnMount:'always',refetchInterval:10_000,refetchOnWindowFocus:'always',initialPageParam:undefined as number|undefined,queryFn:async({pageParam,signal})=>{const ticket=scope.capture();const page=unwrap(await api.GET('/api/v1/events/{event_id}/media/{media_id}/history',{signal,cache:'no-store',params:{path:{event_id:eventID,media_id:mediaID},query:{before_version:pageParam,limit:25}}}));ticket.assert();return page},getNextPageParam:page=>page.next_before_version??undefined})
 const rows=query.data?.pages.flatMap(page=>page.items)??[]
 return <section aria-label="Media decision metadata" className="py-4"><h3>Decision history</h3><p>Version, action and recorded time only. Earlier decisions do not restore consent, approval, gallery release or access to historical photographs. This history does not establish an unresolved command's original receipt.</p>{query.isPending||!query.isFetchedAfterMount?<Loading/>:query.error?<Failure error={query.error} retry={()=>void query.refetch()}/>:<><CollectionRecords label="Authorised media decisions" rows={rows} rowKey={row=>String(row.version)} rowLabel={row=>`Version ${row.version}`} columns={[{id:'action',label:'Recorded action',essential:true,cell:row=>row.action.replaceAll('_',' ')},{id:'changed',label:'Recorded time',cell:row=>new Date(row.changed_at).toLocaleString()}]}/>{!rows.length&&<p>No earlier decision metadata is available.</p>}<CollectionPagination source={{kind:'cursor',loaded:rows.length,busy:query.isFetchingNextPage,next:query.hasNextPage?()=>void query.fetchNextPage():undefined}}/></>}</section>
}

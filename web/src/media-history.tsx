import { useState } from 'react'
import { useInfiniteQuery } from '@tanstack/react-query'
import { APIError, api, privateKey, unwrap } from './api/client'
import { useIdentity } from './session'
import { usePrivateRequestScope } from './private-request-scope'
import { MediaHistoryContent, MediaHistoryDisclosure, type MediaHistoryState } from './media-history-presentation'

// The native current can_read_history capability is separate from write access.
// A historical metadata entry is never a byte grant or original write receipt.
export function MediaDecisionHistory({eventID,mediaID}:{eventID:string;mediaID:string}) {
 const [open,setOpen]=useState(false)
 return <MediaHistoryDisclosure open={open} onOpenChange={setOpen}>{open&&<HistoryReader eventID={eventID} mediaID={mediaID}/>}</MediaHistoryDisclosure>
}
function HistoryReader({eventID,mediaID}:{eventID:string;mediaID:string}) {
 const identity=useIdentity(),[mountID]=useState(()=>crypto.randomUUID()),scope=usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${eventID}:${mediaID}:history:${mountID}`)
 const query=useInfiniteQuery({queryKey:[...privateKey(identity.person.id,eventID,'media-history'),identity.draft_scope,mediaID,mountID],gcTime:0,retry:false,refetchOnMount:'always',refetchInterval:10_000,refetchOnWindowFocus:'always',initialPageParam:undefined as number|undefined,queryFn:async({pageParam,signal})=>{const ticket=scope.capture();const page=unwrap(await api.GET('/api/v1/events/{event_id}/media/{media_id}/history',{signal,cache:'no-store',params:{path:{event_id:eventID,media_id:mediaID},query:{before_version:pageParam,limit:25}}}));ticket.assert();return page},getNextPageParam:page=>page.next_before_version??undefined})
 const state:MediaHistoryState=query.isPending||!query.isFetchedAfterMount?{kind:'loading'}:query.error?{kind:query.error instanceof APIError&&[401,403,404].includes(query.error.status)?'denied':'failure',message:query.error instanceof APIError?query.error.message:'We could not load decision metadata. Please try again.'}:{kind:'ready',rows:query.data?.pages.flatMap(page=>page.items)??[],busy:query.isFetchingNextPage,hasNext:query.hasNextPage}
 return <MediaHistoryContent state={state} retry={()=>void query.refetch()} next={()=>void query.fetchNextPage()}/>
}

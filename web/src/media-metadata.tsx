import { useState } from 'react'
import { useInfiniteQuery } from '@tanstack/react-query'
import { api, privateKey, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Checkbox, Field, Input, Textarea } from './components/ui/fields'
import { Failure } from './components/states'
import type { MediaContext } from './media-workspace'
import type { PrivateRequestScope } from './private-request-scope'
export type MediaMetadata=components['schemas']['MediaMetadata']
export const emptyMediaMetadata=():MediaMetadata=>({kind:'photo',title:'',caption:'',captured_at:null,recording_url:'',people:[],challenge_related:false})
export function mediaDraftError(metadata:MediaMetadata,reason:string){
 const bytes=(value:string)=>new TextEncoder().encode(value).length
 if(!metadata.title.trim()||bytes(metadata.title)>200)return 'Provide a media title up to 200 UTF-8 bytes.'
 if(bytes(metadata.caption)>4000)return 'Keep the caption within 4000 UTF-8 bytes.'
 if(!reason.trim()||bytes(reason)>2000)return 'Provide a reason up to 2000 UTF-8 bytes.'
 if(metadata.people.length>100)return 'Tag no more than 100 identifiable participants.'
 if(metadata.kind==='recording'){try{const url=new URL(metadata.recording_url);if(url.protocol!=='https:'||url.username||url.password||/\s/.test(metadata.recording_url)||bytes(metadata.recording_url)>2048)throw new Error()}catch{return 'Provide a valid HTTPS recording destination without credentials or spaces.'}}
}
export function MediaMetadataFields({metadata,change,context,scope,prefix}:{metadata:MediaMetadata;change:(value:MediaMetadata)=>void;context:MediaContext;scope:PrivateRequestScope;prefix:string}){
 const identity=useIdentity(),[mount]=useState(()=>crypto.randomUUID())
 const guide=useInfiniteQuery({queryKey:[...privateKey(identity.person.id,context.eventID,'media-guidance'),identity.draft_scope,mount],gcTime:0,retry:false,refetchOnWindowFocus:'always',refetchInterval:10_000,initialPageParam:undefined as string|undefined,queryFn:async({pageParam,signal})=>{const ticket=scope.capture();ticket.assert();try{const result=unwrap(await api.GET('/api/v1/events/{event_id}/media/consent-guidance',{signal,cache:'no-store',params:{path:{event_id:context.eventID},query:{after:pageParam,limit:50}}}));ticket.assert();return result}catch(error){if(ticket.current())context.deny(error);throw error}},getNextPageParam:page=>page.next_cursor??undefined})
 return <div className="grid gap-5 min-w-0"><Field id={`${prefix}-title`} label="Media title" description="Up to 200 UTF-8 bytes. Keep sensitive personal details out of titles and captions."><Input required maxLength={200} value={metadata.title} onChange={event=>change({...metadata,title:event.target.value})}/></Field><Field id={`${prefix}-caption`} label="Caption" description="Up to 4000 UTF-8 bytes."><Textarea rows={3} maxLength={4000} value={metadata.caption} onChange={event=>change({...metadata,caption:event.target.value})}/></Field><Field id={`${prefix}-captured`} label="Captured at (UTC, optional)"><Input type="datetime-local" value={metadata.captured_at?new Date(metadata.captured_at).toISOString().slice(0,16):''} onChange={event=>change({...metadata,captured_at:event.target.value?`${event.target.value}:00Z`:null})}/></Field><label className="fleet-label flex gap-2"><Checkbox checked={metadata.challenge_related} onChange={event=>change({...metadata,challenge_related:event.target.checked})}/>Contains challenge or brief content (blocked until actual common release)</label>{metadata.kind==='recording'&&<Field id={`${prefix}-url`} label="Recording HTTPS link" description="Stored as a link. Never fetched, embedded or hosted by this app."><Input type="url" maxLength={2048} required value={metadata.recording_url} onChange={event=>change({...metadata,recording_url:event.target.value})}/></Field>}<fieldset className="grid gap-3 border-0 p-0"><legend className="fleet-label">Identifiable participants</legend><p>Only current event names and photo-consent indicators are shown. Unknown and “no” do not permit release.</p>{guide.error?<Failure error={guide.error} retry={()=>void guide.refetch()}/>:guide.isPending?<p role="status">Loading current consent guidance…</p>:guide.data?.pages.flatMap(page=>page.items).map(person=><label key={person.person_id} className="fleet-label flex gap-2"><Checkbox checked={metadata.people.includes(person.person_id)} onChange={event=>change({...metadata,people:event.target.checked?[...metadata.people,person.person_id]:metadata.people.filter(id=>id!==person.person_id)})}/>{person.display_name||'Unavailable participant'} · {person.choice}</label>)}{guide.hasNextPage&&<Button icon="next" type="button" variant="outline" pending={guide.isFetchingNextPage} onClick={()=>void guide.fetchNextPage()}>More consent guidance</Button>}</fieldset></div>
}

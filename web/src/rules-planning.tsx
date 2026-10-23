import { useCallback, useState } from 'react'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, APIError, privateKey, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { usePrivateRequestScope } from './private-request-scope'
import { CollectionPagination, CollectionRecords, CollectionToolbar } from './components/patterns'
import { Button } from './components/ui/button'
import { Select, Label } from './components/ui/fields'
import { Failure, Loading } from './components/states'
import { ContentEditor } from './content-editor'
import { contentTemplates, draftPage, type ContentPage } from './content-model'
import { EventGuidance } from './event-guidance'

const purposes = ['rules','support','conduct']
export function RulesPlanningWorkspace({ event, onDirty, openSection }: { event: EventRecord; onDirty: (value: boolean) => void; openSection: (section: string) => void }) {
 const identity=useIdentity(),cache=useQueryClient(),scope=usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${event.id}:rules`)
 const [selected,setSelected]=useState<ContentPage>(),[dirty,setDirty]=useState(false),[starter,setStarter]=useState('rules'),[search,setSearch]=useState(''),[query,setQuery]=useState(''),[read,setRead]=useState(0),[blocked,setBlocked]=useState<unknown>()
 const prefix=privateKey(identity.person.id,event.id,'content-authoring')
 const deny=useCallback((error:unknown)=>{scope.invalidate();setBlocked(error);setSelected(undefined);setDirty(false);onDirty(false);void cache.cancelQueries({queryKey:privateKey(identity.person.id,event.id,'content-authoring')});cache.removeQueries({queryKey:privateKey(identity.person.id,event.id,'content-authoring')})},[scope,onDirty,cache,identity.person.id,event.id])
 const pages=useInfiniteQuery({queryKey:[...prefix,'rules',query,read],initialPageParam:undefined as string|undefined,enabled:!blocked,retry:false,refetchOnWindowFocus:'always',refetchInterval:15_000,queryFn:async({pageParam,signal})=>{const ticket=scope.capture();ticket.assert();try{const result=unwrap(await api.GET('/api/v1/events/{event_id}/content',{signal,params:{path:{event_id:event.id},query:{preview:true,q:query||undefined,cursor:pageParam,limit:50}}}));ticket.assert();return result}catch(error){if(ticket.current()&&error instanceof APIError&&[401,403,404].includes(error.status))deny(error);throw error}},getNextPageParam:page=>page.next_cursor??undefined})
 const teams=useQuery({queryKey:privateKey(identity.person.id,event.id,'teams'),enabled:!blocked,retry:false,queryFn:async()=>{const ticket=scope.capture();ticket.assert();try{const value=unwrap(await api.GET('/api/v1/events/{event_id}/teams',{signal:ticket.signal,params:{path:{event_id:event.id}}}));ticket.assert();return value}catch(error){if(ticket.current()&&error instanceof APIError&&[401,403,404].includes(error.status))deny(error);throw error}}})
 const items=pages.data?.pages.flatMap(page=>page.items)??[],visible=items.filter(page=>page.kind==='page'&&purposes.includes(page.document.purpose??'')),starters=contentTemplates([],[]).filter(template=>purposes.includes(template.input.document.purpose??''))
 const markDirty=useCallback((value:boolean)=>{setDirty(value);onDirty(value)},[onDirty])
 function choose(next?:ContentPage){if(!dirty||window.confirm('Leave this page with unsaved content?'))setSelected(next)}
 if(blocked)return <section><h2>Rules planning unavailable</h2><p>Current access changed. Private rules inputs were cleared.</p><Failure error={blocked}/><Button icon="refresh" onClick={()=>{scope.renew();setBlocked(undefined)}}>Check current rules access</Button></section>
 return <section className="fleet-rules-planning"><h2>Rules and contacts planning</h2><p>Prepare native rules and support routes. Saving, approval and release apply to exact versions; existing readers keep their released version until a new one is deliberately published.</p>
  {pages.error&&<div role="status"><p>Current rules access could not be checked. Mounted unsent input is retained; writes are paused.</p><Button icon="refresh" variant="outline" onClick={()=>void pages.refetch()}>Check rules connection and access</Button></div>}
  {selected?<>{!teams.data?teams.isPending?<Loading/>:<Failure error={teams.error} retry={()=>void teams.refetch()}/>:<ContentEditor planning key={selected.id} initial={selected} event={{...event,can_edit:event.can_edit&&!pages.error&&!teams.error}} teams={teams.data.teams} onDirty={markDirty} onDeny={deny} close={()=>choose()} openSection={section=>{if(!dirty||window.confirm('Leave this page with unsaved content?'))openSection(section)}}/>}</>:<>
   {event.can_edit&&<section className="fleet-planning-section"><h3>Start a guidance draft</h3><Label htmlFor="rules-starter">Guidance starting point</Label><Select id="rules-starter" value={starter} onChange={e=>setStarter(e.target.value)}>{starters.map(template=><option key={template.id} value={template.id}>{template.name}</option>)}</Select><Button icon="add" disabled={!!pages.error||teams.isPending||!!teams.error} onClick={()=>{const template=starters.find(item=>item.id===starter);if(template)choose(draftPage(template.input))}}>Prepare guidance draft</Button><p>Starter placeholders are private drafts. Native approval rejects unresolved contacts or missing required acknowledgement.</p></section>}
   <CollectionToolbar source="cursor" search={{kind:'server',label:'Search authorised authored content',maxLength:200,value:search,onChange:setSearch,onSearch:()=>{setQuery(search);setRead(value=>value+1)},onClear:()=>{setSearch('');setQuery('');setRead(value=>value+1)}}}/>
   {pages.isPending?<Loading/>:!pages.error&&<><p role="status">{visible.length} rules/contact pages in {items.length} loaded authorised content records. Purpose is filtered within loaded records; more pages may contain additional guidance.</p><CollectionRecords label="Loaded rules and contact planning pages" rows={visible} rowKey={page=>page.id} rowLabel={page=>page.document.title} destination={page=>({label:page.document.title,href:'#content-title',onNavigate:()=>choose(page)})} columns={[{id:'purpose',label:'Purpose',essential:true,cell:page=>page.document.purpose},{id:'version',label:'Current version',essential:true,cell:page=>page.version},{id:'state',label:'Review state',essential:true,cell:page=>page.state},{id:'released',label:'Released version',cell:page=>page.released_version??'No released version'}]} actions={page=><Button icon="next" variant="ghost" onClick={()=>choose(page)}>Plan {page.document.title}</Button>}/>{!visible.length&&<p>No rules/contact pages occur in these loaded records. Continue paging to inspect further authorised content.</p>}<CollectionPagination source={{kind:'cursor',loaded:items.length,busy:pages.isFetchingNextPage,next:pages.hasNextPage?()=>void pages.fetchNextPage():undefined}}/></>}
   <EventGuidance event={event} readiness/>
  </>}
 </section>
}

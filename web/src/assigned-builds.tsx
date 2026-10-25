import {EventReadScopeProvider,useEventAuthority,useEventReadBoundary} from './event-authority'
import {useCallback} from 'react'
import {useParams} from '@tanstack/react-router'
import {api,unwrap} from './api/client'
import {useIdentity} from './session'
import {useTodayRead} from './today-read'
import {CollectionHeading,CollectionRecords} from './components/patterns'
import {ActionLink,Button} from './components/ui/button'
import {Failure,Loading} from './components/states'
import {EventBrand} from './branding'

type AssignedBuild={id:string;teamID:string;runID:string;revision:number}
export function AssignedBuildsPage(){
 const {eventId}=useParams({strict:false}),identity=useIdentity()
 return <EventBrand eventID={eventId!}><AssignedBuildsEvent key={`${identity.person.id}:${identity.draft_scope}:${eventId}`} eventID={eventId!}/></EventBrand>
}
function AssignedBuildsEvent({eventID}:{eventID:string}){
 const authority=useEventAuthority(eventID)
 if(authority.denial)return <Failure error={authority.denial} retry={()=>void authority.refresh()}/>
 if(!authority.verified||!authority.query.data)return <Loading/>
 return <EventReadScopeProvider value={authority.boundary}><AssignedBuilds key={authority.generation} eventID={eventID}/></EventReadScopeProvider>
}
function AssignedBuilds({eventID}:{eventID:string}){
 const identity=useIdentity(),boundary=useEventReadBoundary(eventID)
 const load=useCallback(async(signal:AbortSignal)=>{
  const [event,rounds]=await Promise.all([api.GET('/api/v1/events/{event_id}',{signal,cache:'no-store',params:{path:{event_id:eventID}}}).then(unwrap),api.GET('/api/v1/events/{event_id}/judging',{signal,cache:'no-store',params:{path:{event_id:eventID}}}).then(unwrap)])
  // Organiser responses can contain the complete configured cohort. Discovery
  // here is explicitly this actor's assigned judge context, never that cohort.
  const builds:AssignedBuild[]=rounds.items.flatMap(round=>round.assignments.filter(a=>a.judge_ids.includes(identity.person.id)).map(a=>({id:`${round.run_id}:${a.team_id}`,teamID:a.team_id,runID:round.run_id,revision:round.revision})))
  return {eventName:event.name,builds}
 },[eventID,identity.person.id])
 const signature=useCallback((data:{builds:AssignedBuild[]})=>JSON.stringify(data.builds.map(b=>b.id).sort()),[])
 const read=useTodayRead(`${identity.person.id}:${identity.draft_scope}:${eventID}:assigned-builds`,load,boundary?.scope,signature)
 if(read.denied)return <section><p>Current judge assignment access is unavailable. Earlier assigned-build links have been cleared.</p><ActionLink icon="back" href={`/events/${eventID}`}>Back to event</ActionLink><Button icon="refresh" onClick={()=>void read.refresh()}>Check current judge assignments</Button></section>
 if(!read.data)return read.error?<Failure error={read.error} retry={()=>void read.refresh()}/>:<Loading/>
 return <section className="fleet-resource-workspace"><ActionLink icon="back" variant="ghost" href={`/events/${eventID}`}>Back to event</ActionLink><CollectionHeading title="Your assigned judging builds" scope={{event:read.data.eventName,entity:'Current final-round assignments'}} description="Only your current conflict-free assignments appear. Opening a build checks the exact round, team and formal evidence again." secondary={[{label:'Read current judge assignments',icon:'refresh',onSelect:()=>void read.refresh(),disabled:read.pending}]}/>{!!read.error&&<Failure error={read.error}/>}
  <CollectionRecords label="Your current assigned builds" disabled={read.pending||!!read.error} rows={read.data.builds} rowKey={b=>b.id} rowLabel={b=>`Team ${b.teamID}`} columns={[{id:'round',label:'Final round',essential:true,cell:b=>b.runID},{id:'revision',label:'Assignment revision',cell:b=>b.revision}]} actions={b=><ActionLink icon="code" href={`/events/${eventID}/teams/${b.teamID}/build?judge_run=${b.runID}`}>Open assigned build</ActionLink>}/>{!read.data.builds.length&&<p>No current conflict-free final judging assignment is available. Assignment to an earlier round does not grant access to a different team.</p>}
 </section>
}

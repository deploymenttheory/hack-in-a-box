import { useCallback } from 'react'
import { useParams } from '@tanstack/react-router'
import { api, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { useTodayRead } from './today-read'
import { TodayActionSafety } from './today-safety'
import { EventBrand } from './branding'
import { SubmissionFreeze } from './submission-freeze'
import { CollectionHeading, ContextLink } from './components/patterns'
import { Failure, Loading } from './components/states'
import { localDate } from './event-time'
import { EvidenceList } from './deliverable-evidence'
import type { SubmissionPage } from './deliverable-model'
type BuildSource={event:EventRecord;submission:SubmissionPage}
export function SubmittedBuildPage(){
 const {eventId,teamId}=useParams({strict:false}),identity=useIdentity()
 return <EventBrand eventID={eventId!}><TodayActionSafety eventID={eventId!}><SubmittedBuild key={`${identity.person.id}:${identity.draft_scope}:${eventId}:${teamId}`} eventID={eventId!} teamID={teamId!}/></TodayActionSafety></EventBrand>
}
function SubmittedBuild({eventID,teamID}:{eventID:string;teamID:string}){
 const load=useCallback(async(signal:AbortSignal)=>{const [event,submission]=await Promise.all([api.GET('/api/v1/events/{event_id}',{signal,cache:'no-store',params:{path:{event_id:eventID}}}).then(unwrap),api.GET('/api/v1/events/{event_id}/teams/{team_id}/submission',{signal,cache:'no-store',params:{path:{event_id:eventID,team_id:teamID}}}).then(unwrap)]);return {event,submission}},[eventID,teamID])
 const signature=useCallback((source:BuildSource)=>JSON.stringify([source.event.can_edit,source.submission.can_read_draft,source.submission.can_review,source.submission.final]),[])
 const read=useTodayRead(`${eventID}:${teamID}:submitted-build`,load,undefined,signature)
 if(read.denied)return <section><p>Current submission access is unavailable. Private build evidence has been cleared.</p><ContextLink back target={{href:`/events/${eventID}`,label:'Back to event'}}/></section>
 if(!read.data)return read.error?<Failure error={read.error} retry={()=>void read.refresh()}/>:<Loading/>
 const {event,submission}=read.data
 return <section className="fleet-resource-workspace fleet-freeze-private"><ContextLink back target={{href:`/events/${eventID}`,label:'Back to event'}}/><CollectionHeading title="Submitted build" scope={{event:event.name,entity:`Team ${teamID}`}} description="Formal artifacts and reviewed code evidence have distinct status. Current team membership or an assigned assessor role is required for every read."/>{!!read.error&&<Failure error={read.error} retry={()=>void read.refresh()}/>}
  <section className="fleet-resource-section"><h2>Formal submission</h2><p>Native outcome: {submission.outcome.replaceAll('_',' ')} · Formal submissions and reviews open {localDate(submission.live_start,event.configuration.timezone)} · Shared cutoff {localDate(submission.cutoff,event.configuration.timezone)}</p>{submission.outcome==='withdrawn'?<p>This entry is withdrawn, unused and unscored. Disbanded work is not transferred to another entry.</p>:submission.outcome==='not_submitted'?<p>No formal entry was submitted by cutoff. A private working draft is not a formal submission.</p>:submission.outcome==='open'?<p>Early repository work and private drafts do not constitute formal acceptance. The native submission window controls when an entry can be submitted.</p>:<p>The native final is frozen; its repository SHA still requires the separately reviewed code evidence below.</p>}
  {submission.final?<><p>Formal version {submission.final.revision} · Submitted {localDate(submission.final.submitted_at,event.configuration.timezone)}{submission.final.late&&' · Audited late native correction'}</p><EvidenceList eventID={eventID} items={submission.final.evidence}/></>:<p>No formal evidence accepted.</p>}
  <div className="fleet-pattern-actions">{(submission.can_read_draft||submission.can_submit||event.can_edit)&&<ContextLink target={{href:`/events/${eventID}?section=deliverables`,label:submission.can_read_draft?'Continue working draft or live submission':'Review native formal submission'}}/>}{submission.can_read_draft&&<ContextLink target={{href:`/events/${eventID}?section=repositories`,label:'Review your repository invitation and access'}}/>}</div></section>
  <SubmissionFreeze key={signature(read.data)} eventID={eventID} teamID={teamID} timezone={event.configuration.timezone} parent={read.authority} paused={read.pending||!!read.error}/>
 </section>
}

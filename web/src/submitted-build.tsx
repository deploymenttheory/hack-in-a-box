import {EventReadScopeProvider,useEventAuthority,useEventReadBoundary} from './event-authority'
import { useCallback } from 'react'
import { useParams, useSearch } from '@tanstack/react-router'
import { api, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { useTodayRead } from './today-read'
import { TodayActionSafety } from './today-safety'
import { EventBrand } from './branding'
import { SubmissionFreeze } from './submission-freeze'
import { ActionLink, Button } from './components/ui/button'
import { CollectionHeading, ContextLink } from './components/patterns'
import { Failure, Loading } from './components/states'
import { localDate } from './event-time'
import { EvidenceList } from './deliverable-evidence'
import type { SubmissionPage } from './deliverable-model'
type BuildSource={event:EventRecord;submission:SubmissionPage|null;judgeName?:string;judgeRevision?:number}
export function SubmittedBuildPage(){
 const {eventId,teamId}=useParams({strict:false}),identity=useIdentity(),search=useSearch({strict:false}) as {judge_run?:string}
 return <EventBrand eventID={eventId!}><TodayActionSafety eventID={eventId!}><SubmittedBuildEvent key={`${identity.person.id}:${identity.draft_scope}:${eventId}:${teamId}:${search.judge_run??'ordinary'}`} eventID={eventId!} teamID={teamId!} judgeRun={search.judge_run}/></TodayActionSafety></EventBrand>
}
function SubmittedBuildEvent(props:{eventID:string;teamID:string;judgeRun?:string}){
 const authority=useEventAuthority(props.eventID)
 if(authority.denial)return <Failure error={authority.denial} retry={()=>void authority.refresh()}/>
 if(!authority.verified||!authority.query.data)return <Loading/>
 return <EventReadScopeProvider value={authority.boundary}><SubmittedBuild key={authority.generation} {...props}/></EventReadScopeProvider>
}
function SubmittedBuild({eventID,teamID,judgeRun}:{eventID:string;teamID:string;judgeRun?:string}){
 const boundary=useEventReadBoundary(eventID)
 const load=useCallback(async(signal:AbortSignal)=>{
  // Discovery is not a grant. This exact owner also checks current conflicts,
  // assignment and frozen output before any broader artifact request begins.
  let judgeName:string|undefined,judgeRevision:number|undefined
  if(judgeRun){const brief=unwrap(await api.GET('/api/v1/events/{event_id}/judging/{run_id}/teams/{team_id}',{signal,cache:'no-store',params:{path:{event_id:eventID,run_id:judgeRun,team_id:teamID}}}));judgeName=brief.team_name;judgeRevision=brief.submission_revision}
  const [event,submission]=await Promise.all([api.GET('/api/v1/events/{event_id}',{signal,cache:'no-store',params:{path:{event_id:eventID}}}).then(unwrap),api.GET('/api/v1/events/{event_id}/teams/{team_id}/submission',{signal,cache:'no-store',params:{path:{event_id:eventID,team_id:teamID}}}).then(unwrap)])
  // Discard a mismatched current projection rather than keeping an old judge
  // target visible. No scores, private guidance or other cards enter this owner.
  return {event,submission:judgeRevision!==undefined&&submission.final?.revision!==judgeRevision?null:submission,judgeName,judgeRevision}
 },[eventID,teamID,judgeRun])
 const signature=useCallback((source:BuildSource)=>JSON.stringify([source.event.can_edit,source.submission?.can_read_draft,source.submission?.can_review,source.submission?.final,source.judgeRevision]),[])
 const read=useTodayRead(`${eventID}:${teamID}:submitted-build`,load,boundary?.scope,signature)
 if(read.denied)return <section><p>Current submission access is unavailable. Private build evidence has been cleared.</p><ActionLink icon="back" variant="ghost" href={`/events/${eventID}`}>Back to event</ActionLink></section>
 if(!read.data)return read.error?<Failure error={read.error} retry={()=>void read.refresh()}/>:<Loading/>
 const {event,submission}=read.data
 if(!submission)return <section><p>The current formal version changed while checking the assigned judge context. Earlier artifact projections have been cleared.</p><ActionLink icon="back" href={`/events/${eventID}/builds`}>Back to assigned builds</ActionLink><Button icon="refresh" onClick={()=>void read.refresh()}>Read current assigned build</Button></section>
 return <section className="fleet-resource-workspace fleet-freeze-private"><ActionLink icon="back" variant="ghost" href={`/events/${eventID}`}>Back to event</ActionLink><CollectionHeading title="Submitted build" scope={{event:event.name,entity:read.data.judgeName??`Team ${teamID}`}} description="Formal artifacts and reviewed code evidence have distinct status. Current team membership or an assigned assessor role is required for every read."/>{!!read.error&&<Failure error={read.error} retry={()=>void read.refresh()}/>}
  <section className="fleet-resource-section"><h2>Formal submission</h2><p>Submission status: {submission.outcome.replaceAll('_',' ')} · Formal submissions and reviews open {localDate(submission.live_start,event.configuration.timezone)} · Shared cutoff {localDate(submission.cutoff,event.configuration.timezone)}</p>{submission.outcome==='withdrawn'?<p>This entry is withdrawn, unused and unscored. Disbanded work is not transferred to another entry.</p>:submission.outcome==='not_submitted'?<p>No formal entry was submitted by cutoff. A private working draft is not a formal submission.</p>:submission.outcome==='open'?<p>Early repository work and private drafts do not constitute formal acceptance. The submission window controls when an entry can be submitted.</p>:<p>The formal version is frozen; its repository SHA still requires the separately reviewed code evidence below.</p>}
  {submission.final?<><p>Formal version {submission.final.revision} · Submitted {localDate(submission.final.submitted_at,event.configuration.timezone)}{submission.final.late&&' · Audited late correction'}</p><EvidenceList eventID={eventID} items={submission.final.evidence}/></>:<p>No formal evidence accepted.</p>}
  <div className="fleet-pattern-actions">{(submission.can_read_draft||submission.can_submit||event.can_edit)&&<ContextLink target={{href:`/events/${eventID}?section=deliverables`,label:submission.can_read_draft?'Continue working draft or live submission':'Review formal submission'}}/>}{submission.can_read_draft&&<ContextLink target={{href:`/events/${eventID}?section=repositories`,label:'Review your repository invitation and access'}}/>}</div></section>
  <SubmissionFreeze key={signature(read.data)} eventID={eventID} teamID={teamID} timezone={event.configuration.timezone} parent={read.authority} paused={read.pending||!!read.error}/>
 </section>
}

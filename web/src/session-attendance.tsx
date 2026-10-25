import { useCallback, useRef, useState } from 'react'
import { APIError, api, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { useTodayRead } from './today-read'
import { usePrivateRequestScope, type PrivateRequestScope } from './private-request-scope'
import { useCommunicationCommands } from './communication-tests'
import { SessionDecision, SessionFailure, type BookableSession } from './session-booking-view'
import { matchSessionReceipt, SessionBookingError, type SessionReceipt } from './session-booking-receipt'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
type Roster = components['schemas']['SessionRoster']
type Entry = components['schemas']['SessionAttendanceEntry']
export function SessionAttendance({eventID,session,parent,disabled,acknowledged}:{eventID:string;session:BookableSession;parent:PrivateRequestScope;disabled:boolean;acknowledged:(receipt:SessionReceipt|undefined)=>void}){
 const identity=useIdentity()
 const load=useCallback(async(signal:AbortSignal)=>unwrap(await api.GET('/api/v1/events/{event_id}/sessions/{session_id}/attendance',{signal,cache:'no-store',params:{path:{event_id:eventID,session_id:session.id}}})),[eventID,session.id])
 const read=useTodayRead(`${identity.person.id}:${identity.draft_scope}:${eventID}:${session.id}:attendance`,load,parent)
 if(read.denied)return <p>Current facilitator roster access was removed. Private attendance names and reviews have been cleared.</p>
 if(!read.data)return read.error?<Failure error={read.error} retry={()=>void read.refresh()}/>:<Loading/>
 return <SessionAttendanceOwner key={read.generation} eventID={eventID} session={session} read={read} disabled={disabled} acknowledged={acknowledged}/>
}
function SessionAttendanceOwner({eventID,session,read,disabled,acknowledged}:{eventID:string;session:BookableSession;read:ReturnType<typeof useTodayRead<Roster>>;disabled:boolean;acknowledged:(receipt:SessionReceipt|undefined)=>void}){
 const identity=useIdentity(),commands=useCommunicationCommands(`${identity.person.id}:${identity.draft_scope}:${eventID}:session-bookings:commands`),scope=usePrivateRequestScope(`${eventID}:${session.id}:attendance-write`,read.authority),opener=useRef<HTMLElement|null>(null)
 const [review,setReview]=useState<{person:Entry;present:boolean;requestID:string;sessionRevision:number}>(),[failure,setFailure]=useState<unknown>()
 const blocked=commands.attempts.some(a=>a.command.resourceID===session.id&&a.blocks),paused=disabled||blocked||read.pending||!!read.error
 const current=read.data?.items.find(person=>person.person_id===review?.person.person_id)
 const stale=!!review&&(!current||current.revision!==review.person.revision||session.revision!==review.sessionRevision||!session.can_record_attendance)
 async function confirm(){
  if(!review||paused||stale)return
  const captured=review,ticket=scope.capture();ticket.assert()
  const action=captured.present?'present':'absent',id=commands.begin({operation:'Session attendance correction',revision:captured.person.revision,command:{action,requestID:captured.requestID,resourceID:session.id,subjectID:captured.person.person_id,expectedVersion:captured.sessionRevision}})
  try{ticket.assert();const result=unwrap(await api.PUT('/api/v1/events/{event_id}/sessions/{session_id}/attendance/{person_id}',{params:{path:{event_id:eventID,session_id:session.id,person_id:captured.person.person_id},header:{'X-CSRF-Token':identity.csrf_token,'If-Match':`"${captured.person.revision}"`}},body:{request_id:captured.requestID,present:captured.present}}));if(!matchSessionReceipt(result,{requestID:captured.requestID,sessionID:session.id,personID:captured.person.person_id,action,revision:captured.person.revision}))throw new SessionBookingError('The attendance acknowledgement does not match the original correction. Its outcome remains unknown.');commands.complete(id);if(ticket.current()){setReview(undefined);acknowledged(result);await read.refresh();requestAnimationFrame(()=>opener.current?.focus())}}
  catch(error){if(error instanceof APIError&&[400,401,403,404,409,422].includes(error.status))commands.complete(id);else commands.uncertain(id);if(ticket.current()){setReview(undefined);setFailure(error);await read.refresh()}}
 }
 return <section className="fleet-planning-section"><h3>Scoped facilitator attendance</h3><p>This session roster grants no unrelated logistics, competition output or assessment access.</p>{!session.can_record_attendance&&<p>Roster is read-only. Attendance recording is not currently permitted by the server.</p>}<Button icon="refresh" variant="outline" disabled={read.pending||commands.pending} onClick={()=>void read.refresh()}>Read current attendance</Button>{!!read.error&&<Failure error={read.error}/>}
  <ul className="fleet-planning-records">{read.data?.items.map(person=><li key={person.person_id}><span>{person.display_name} · {person.present?'Present':'Not recorded present'} · Revision {person.revision}</span><div hidden={!!review&&review.person.person_id===person.person_id}><Button icon="check" variant="ghost" disabled={paused||!session.can_record_attendance} onClick={e=>{opener.current=e.currentTarget;setFailure(undefined);acknowledged(undefined);setReview({person:structuredClone(person),present:!person.present,requestID:crypto.randomUUID(),sessionRevision:session.revision})}}>Review {person.present?'absence':'presence'} for {person.display_name}</Button></div></li>)}</ul>
  {review&&<SessionDecision title={`Correct attendance for ${review.person.display_name}`} pending={commands.pending} disabled={paused||stale} confirm={()=>void confirm()} correct={()=>setReview(undefined)} finalFocus={opener}><p>Record {review.person.display_name} as {review.present?'present':'absent'} in {session.title}.</p><p>Captured attendance revision {review.person.revision} · Session revision {review.sessionRevision}. This corrects optional session attendance only.</p>{stale&&<p role="alert">Current attendance or session authority changed. Cancel and review the current roster.</p>}</SessionDecision>}
  {!!failure&&<SessionFailure error={failure}/>}
 </section>
}

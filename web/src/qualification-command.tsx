import {useEffect,useRef,useState,type ReactNode} from 'react'
import {APIError} from './api/client'
import {useIdentity} from './session'
import {useCommunicationCommands} from './communication-tests'
import {usePrivateRequestScope,type PrivateRequestScope} from './private-request-scope'
import {usePlanningDepartureBarrier} from './planning-operations'
import {ConfirmationDialog} from './components/ui/dialog'
import {Button} from './components/ui/button'
import {Failure} from './components/states'
import {originalQualificationReceipt,readQualification,sendQualification} from './qualification-api'
import {matchesQualificationReceipt,qualificationFingerprint,type QualificationCommand,type QualificationIntent,type QualificationPage} from './qualification-model'

export const qualificationNamespace=(actor:string,session:string,eventID:string,runID:string)=>`${actor}:${session}:${eventID}:qualification:${runID}`
type Reviewed={body:QualificationCommand;intent:QualificationIntent;summary:ReactNode}
type Options={eventID:string;runID:string;page:QualificationPage;parent:PrivateRequestScope;paused:boolean;refresh:()=>Promise<unknown>;assertActionReady:(page:QualificationPage,body:QualificationCommand)=>void}

// This owner handles request identity and private lifetimes only. Native
// capability/source readiness remains with its caller and is rechecked against
// an actual current GET immediately before the first write.
export function useQualificationCommand(options:Options){
 const {eventID,runID,page,parent,paused,refresh,assertActionReady}=options,identity=useIdentity()
 const owner=qualificationNamespace(identity.person.id,identity.draft_scope,eventID,runID),commands=useCommunicationCommands(owner),scope=usePrivateRequestScope(`${owner}:task`,parent)
 const [review,setReview]=useState<Reviewed>(),[captured,setCaptured]=useState<Reviewed>(),[preparing,setPreparing]=useState(false),[failure,setFailure]=useState<unknown>(),[ack,setAck]=useState<QualificationIntent>()
 const sending=useRef(false),opener=useRef<HTMLElement|null>(null),fallback=useRef<HTMLHeadingElement|null>(null)
 const pending=preparing||commands.pending,blocked=paused||pending||commands.unconfirmed
 usePlanningDepartureBarrier(pending)
 async function prepare(body:QualificationCommand,summary:ReactNode,source:HTMLElement){
  if(blocked||sending.current)return
  const ticket=scope.capture();sending.current=true;setPreparing(true);setFailure(undefined)
  try{
   ticket.assert();assertActionReady(page,body)
   const command=structuredClone(body),fingerprint=await qualificationFingerprint(command,page.revision);ticket.assert()
   const value={body:command,intent:{requestID:command.request_id,runID,action:command.action,revision:page.revision,version:command.version,fingerprint},summary}
   opener.current=source;setCaptured(value);setReview(value)
  }catch(error){if(ticket.current())setFailure(error)}finally{sending.current=false;if(ticket.current())setPreparing(false)}
 }
 async function confirm(retry=false){
  const target=retry?captured:review
  if(!target||sending.current||paused||commands.pending||!retry&&commands.unconfirmed)return
  const ticket=scope.capture();sending.current=true;setPreparing(true);setFailure(undefined);let attemptID:string|undefined
  try{
   ticket.assert()
   if(!retry){
    const current=await readQualification(eventID,runID,ticket.signal);ticket.assert()
    if(current.round.run_id!==runID||current.revision!==target.intent.revision||current.round.version!==target.intent.version)throw new Error('The current panel starting point changed. Nothing was sent; read and review the current source.')
    assertActionReady(current,target.body)
   }
   ticket.assert()
   const existing=retry?commands.attempts.find(value=>value.command.requestID===target.intent.requestID)?.id:undefined
   attemptID=commands.begin({operation:'Private qualification panel command',revision:target.intent.revision,command:{action:target.intent.action,requestID:target.intent.requestID,revision:target.intent.revision,expectedVersion:target.intent.version,resourceID:runID,intentFingerprint:target.intent.fingerprint}},existing)
   const result=await sendQualification(eventID,runID,target.intent.revision,identity.csrf_token,target.body)
   if(!result.receipt||!matchesQualificationReceipt(result.receipt,target.intent))throw new Error('The response did not match this exact original request. Its outcome remains unknown.')
   commands.complete(attemptID)
   if(ticket.current()){setAck(target.intent);setReview(undefined);setCaptured(undefined);await refresh()}
  }catch(error){
   if(attemptID){if(error instanceof APIError&&error.status>=400&&error.status<500)commands.complete(attemptID);else commands.uncertain(attemptID)}
   if(ticket.current()){if(attemptID)setReview(undefined);setFailure(error);if(error instanceof APIError&&[401,403,404].includes(error.status)){scope.invalidate();await refresh()}}
  }finally{sending.current=false;if(ticket.current())setPreparing(false)}
 }
 return {prepare,pending,blocked,retryBlocked:paused||pending,unknown:commands.unconfirmed,attempts:commands.attempts,ack,failure,review,captured,headingRef:(element:HTMLHeadingElement|null)=>{fallback.current=element},returnFocus:()=>{const source=opener.current;return source?.isConnected&&!source.matches(':disabled')&&source.getClientRects().length?source:fallback.current??false},confirm,cancel:()=>{if(!pending)setReview(undefined)}}
}

export function QualificationCommandReview({headingRef,failure,ack,attempts,unknown,captured,retryBlocked,confirm,review,pending,blocked,cancel,returnFocus}:ReturnType<typeof useQualificationCommand>){
 return <><h3 ref={headingRef} tabIndex={-1}>Reviewed panel commands</h3>{!!failure&&<Failure error={failure}/>} {ack&&<p role="status">Original {ack.action.replaceAll('_',' ')} acknowledged for request {ack.requestID}, expected panel version {ack.version}. Read current state separately; no announcement was released.</p>}{attempts.map(attempt=><aside key={attempt.id} role="status"><h4>{attempt.state==='pending'?'Awaiting original acknowledgement':'Original panel outcome unknown'}</h4><p>Connection loss does not establish failure or success. A current panel read cannot acknowledge this earlier request. No automatic replay occurs.</p></aside>)}{unknown&&captured&&<Button icon="refresh" variant="outline" disabled={retryBlocked} onClick={()=>void confirm(true)}>Retry captured original panel request</Button>}{review&&<ConfirmationDialog open title="Review private panel command" description="Confirm the exact captured reason, evidence, source and panel version. This does not publish an announcement." confirmLabel="Confirm reviewed panel command" pending={pending} disabled={blocked&&!pending} onConfirm={()=>void confirm()} onOpenChange={open=>{if(!open)cancel()}} finalFocus={returnFocus} printExcluded>{review.summary}<p>Action {review.intent.action.replaceAll('_',' ')} · Panel version {review.intent.version} · Event revision {review.intent.revision}</p><p>Reason: {review.body.reason}</p></ConfirmationDialog>}</>
}

// Mount above the private panel reader so safe original identity survives native
// denial/task replacement. Native receipt lookup still authorises the current
// actor/run; retained metadata grants no private source or retry body.
export function QualificationCommandHistory({eventID,runID}:{eventID:string;runID:string}){
 const identity=useIdentity(),owner=qualificationNamespace(identity.person.id,identity.draft_scope,eventID,runID),commands=useCommunicationCommands(owner),scope=usePrivateRequestScope(`${owner}:original-history`)
 const [reading,setReading]=useState(false),[failure,setFailure]=useState<unknown>(),[message,setMessage]=useState('')
 const opener=useRef<HTMLButtonElement|null>(null),outcome=useRef<HTMLParagraphElement|null>(null)
 useEffect(()=>{if(!message)return;const frame=requestAnimationFrame(()=>{const source=opener.current;(source?.isConnected&&!source.matches(':disabled')&&source.getClientRects().length?source:outcome.current)?.focus()});return()=>cancelAnimationFrame(frame)},[message])
 async function recover(attempt:typeof commands.attempts[number],source:HTMLButtonElement){
  const metadata=attempt.command
  if(reading||!metadata.requestID||!metadata.intentFingerprint||metadata.expectedVersion===undefined)return
  const ticket=scope.capture();opener.current=source;setReading(true);setFailure(undefined)
  try{
   const result=await originalQualificationReceipt(eventID,runID,metadata.requestID,ticket.signal);ticket.assert()
   const intent={requestID:metadata.requestID,runID,action:metadata.action,revision:attempt.revision,version:metadata.expectedVersion,fingerprint:metadata.intentFingerprint}
   if(!result.receipt||!matchesQualificationReceipt(result.receipt,intent))throw new Error('No exact original acknowledgement is available. Current facts do not settle this request.')
   commands.complete(attempt.id);setMessage(`Original ${metadata.action.replaceAll('_',' ')} acknowledged at panel version ${result.receipt.version}. Current panel state must be read separately.`)
  }catch(error){if(ticket.current())setFailure(error)}finally{if(ticket.current())setReading(false)}
 }
 return <>{commands.attempts.length>0&&<aside aria-label="Original panel command history"><h2>Earlier panel request metadata</h2><p>Safe request identity only; no private scores, reasons, evidence or replay body survives the closed task.</p>{commands.attempts.map(attempt=><p key={attempt.id}>Request {attempt.command.requestID} · Expected event revision {attempt.revision} · Expected panel version {attempt.command.expectedVersion} · {attempt.state==='pending'?'Awaiting acknowledgement':'Outcome unknown'}{attempt.state==='unknown'&&<Button icon="refresh" variant="outline" disabled={reading} onClick={event=>void recover(attempt,event.currentTarget)}>Read original panel acknowledgement</Button>}</p>)}{!!failure&&<Failure error={failure}/>}</aside>}{message&&<p role="status" tabIndex={-1} ref={outcome}>{message}</p>}</>
}

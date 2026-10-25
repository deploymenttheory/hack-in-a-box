import { useCallback, useEffect, useRef, useState } from 'react'
import { useBlocker } from '@tanstack/react-router'
import { APIError } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { useTodayRead } from './today-read'
import { usePrivateRequestScope, type PrivateRequestScope } from './private-request-scope'
import { useCommunicationCommands } from './communication-tests'
import { currentJudgeStates, sendJudgeCommand } from './judge-api'
import { judgeNamespace } from './judge-editor'
import { judgeFingerprint, matchJudgeReceipt, type JudgeCommand, type JudgeIntent } from './judge-model'
import { JudgeReview } from './judge-view'
import { Button } from './components/ui/button'
import { Field, Textarea } from './components/ui/fields'
import { Failure, Loading } from './components/states'

type CardState=components['schemas']['JudgeCardState']
const stateAuthority=(v:components['schemas']['JudgeCardStates'])=>JSON.stringify(v.items.map(s=>s.author_id))
export function JudgeReopen({eventID,runID,teamID,parent,paused,onRisk}:{onRisk?:(risk:{dirty:boolean;pending:boolean;unknown:boolean})=>void;eventID:string;runID:string;teamID:string;parent:PrivateRequestScope;paused:boolean}){
 const load=useCallback((signal:AbortSignal)=>currentJudgeStates(eventID,runID,teamID,signal),[eventID,runID,teamID]),read=useTodayRead(`${eventID}:${runID}:${teamID}:states`,load,parent,stateAuthority)
 return <section className="fleet-judge-section" aria-label="Organiser score lock coordination"><h3>Score locks and reasoned reopening</h3><p>Metadata only. Private judge scores and notes remain unavailable to the organiser. Reopening invalidates affected unannounced result approvals.</p>{!!read.error&&<Failure error={read.error} retry={()=>void read.refresh()}/>}<Button icon="refresh" variant="outline" onClick={()=>void read.refresh()}>Read current card states</Button>{!read.data&&!read.error&&<Loading/>}{read.data&&<ReopenEditor key={read.generation} eventID={eventID} runID={runID} teamID={teamID} parent={read.authority} paused={paused} onRisk={onRisk} read={read}/>}</section>
}
function ReopenEditor({eventID,runID,teamID,parent,paused,onRisk,read}:{onRisk?:(risk:{dirty:boolean;pending:boolean;unknown:boolean})=>void;eventID:string;runID:string;teamID:string;parent:PrivateRequestScope;paused:boolean}&{read:ReturnType<typeof useTodayRead<components['schemas']['JudgeCardStates']>>}){
 const identity=useIdentity(),owner=`${judgeNamespace(identity.person.id,identity.draft_scope,eventID)}:${teamID}`,commands=useCommunicationCommands(owner),scope=usePrivateRequestScope(`${owner}:reopen`,parent)
 const [reason,setReason]=useState(''),[review,setReview]=useState<{state:CardState;reason:string;body:JudgeCommand;intent:JudgeIntent}>(),[error,setError]=useState<unknown>(),[preparing,setPreparing]=useState(false),[ack,setAck]=useState(''),[reasonError,setReasonError]=useState('')
 const opener=useRef<HTMLElement|null>(null),sending=useRef(false),heading=useRef<HTMLHeadingElement>(null),reasonInput=useRef<HTMLTextAreaElement>(null)
 const pending=preparing||commands.pending,blocked=paused||read.pending||!!read.error||pending||commands.unconfirmed
 useEffect(()=>{onRisk?.({dirty:!!reason,pending,unknown:commands.unconfirmed});return()=>onRisk?.({dirty:false,pending:false,unknown:false})},[onRisk,reason,pending,commands.unconfirmed])
 useBlocker({shouldBlockFn:()=>scope.capture().current()&&(pending||(!!reason&&!window.confirm('Discard the unsent reopen reason?'))),enableBeforeUnload:!!reason||pending||commands.unconfirmed})
 useEffect(()=>{if(reasonError)reasonInput.current?.focus()},[reasonError])
 async function prepare(state:CardState,button:HTMLElement){
 if(blocked||!state.can_reopen)return
 if(!reason.trim()||reason.length>4000){setReasonError('Provide a recorded reason within 4,000 characters.');return}
 setReasonError('')
 opener.current=button;setPreparing(true);const ticket=scope.capture()
 try{const body:JudgeCommand={request_id:crypto.randomUUID(),action:'reopen',author_id:state.author_id,submission_revision:state.submission_revision,repository_decision_id:state.repository_decision_id,reason}
 const fingerprint=await judgeFingerprint(body,state.version,runID,teamID);ticket.assert()
 setReview({state:structuredClone(state),reason,body,intent:{requestID:body.request_id,action:'reopen',revision:state.version,runID,teamID,fingerprint}})
 }catch(e){if(ticket.current())setError(e)}finally{if(ticket.current())setPreparing(false)}
 }
 async function confirm(){
 if(!review||blocked||sending.current)return
 const captured=review,ticket=scope.capture();sending.current=true;setPreparing(true);let id:string|undefined
 try{const now=await currentJudgeStates(eventID,runID,teamID,ticket.signal);ticket.assert();const state=now.items.find(s=>s.author_id===captured.state.author_id)
 if(!state?.can_reopen||state.version!==captured.state.version||state.submission_revision!==captured.state.submission_revision||state.repository_decision_id!==captured.state.repository_decision_id)throw new Error('Current reopen metadata changed. Nothing was sent; read current card states and review again.')
 id=commands.begin({operation:'Reasoned judge reopen',revision:captured.intent.revision,command:{requestID:captured.intent.requestID,action:'reopen',revision:captured.intent.revision,resourceID:runID,subjectID:teamID,intentFingerprint:captured.intent.fingerprint}})
 const receipt=await sendJudgeCommand(eventID,captured.intent.revision,identity.csrf_token,captured.body,runID,teamID)
 if(!matchJudgeReceipt(receipt,captured.intent,identity.person.id))throw new Error('The reopen response did not match the original reviewed request. Its outcome remains unknown.')
 commands.complete(id)
 if(ticket.current()){setAck(`Reasoned reopen acknowledged · Version ${receipt.revision} · Request ${receipt.request_id}`);setReason('');setReview(undefined);await read.refresh()}
 }catch(e){if(id){if(e instanceof APIError&&e.status>=400&&e.status<500)commands.complete(id);else commands.uncertain(id)}if(ticket.current()){if(id)setReview(undefined);setError(e);if(e instanceof APIError&&[401,403,404].includes(e.status)){scope.invalidate();setReason('');setReview(undefined);await read.refresh()}}}finally{sending.current=false;if(ticket.current())setPreparing(false)}
 }
 const current=review&&read.data?.items.find(s=>s.author_id===review.state.author_id),stale=!!review&&(!current?.can_reopen||current.version!==review.state.version||current.submission_revision!==review.state.submission_revision||current.repository_decision_id!==review.state.repository_decision_id)
 return <section><h4 tabIndex={-1} ref={heading}>Current independent card states</h4>{!!error&&<Failure error={error}/>}<div hidden={!!review||read.denied} inert={blocked}><Field label="Reason for reopening" error={reasonError} description="Required and recorded with this exact author/version. No private score is needed to coordinate a correction."><Textarea ref={reasonInput} required maxLength={4000} value={reason} onChange={e=>setReason(e.target.value)}/></Field>{read.data?.items.map(state=><p key={state.author_id}>Judge {state.author_id} · {state.state} · Version {state.version} · {state.source_available?state.source_changed?'Reviewed source changed':'Current source confirmed':'Current source unavailable'}{state.can_reopen&&<Button icon="refresh" variant="outline" disabled={blocked} onClick={e=>void prepare(state,e.currentTarget)}>Review reopening judge {state.author_id}</Button>}</p>)}</div>{ack&&<p role="status">{ack}</p>}{commands.unconfirmed&&<p role="alert">Original reopen outcome unknown. Use the original judge command history above to read its exact acknowledgement. Reading current card states does not settle it.</p>} {review&&<JudgeReview title="Review reasoned score reopening" pending={pending} disabled={paused||read.pending||!!read.error||stale||commands.unconfirmed} confirm={()=>void confirm()} correct={()=>{if(!pending){setReview(undefined);requestAnimationFrame(()=>(opener.current?.isConnected&&!opener.current.matches(':disabled')&&opener.current.getClientRects().length?opener.current:heading.current)?.focus())}}} opener={opener} fallback={heading}><p>Judge {review.state.author_id} · Submitted card version {review.state.version} · Current final revision {review.state.submission_revision}.</p><p>{review.reason}</p><p>The judge must review their draft and explicitly resubmit. A reopened draft is not a completed judge assessment.</p></JudgeReview>}</section>
}

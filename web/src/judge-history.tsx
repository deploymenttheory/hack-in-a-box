import { useEffect, useRef, useState } from 'react'
import { useIdentity } from './session'
import { useCommunicationCommands, useCommunicationCommandNamespace } from './communication-tests'
import { usePrivateRequestScope } from './private-request-scope'
import { originalJudgeReceipt } from './judge-api'
import { judgeNamespace } from './judge-editor'
import { matchJudgeReceipt, type JudgeIntent } from './judge-model'
import { Button } from './components/ui/button'
import { Failure } from './components/states'

// Session-level fixed-label metadata survives private task teardown. It stores
// no scores, notes, team names, reasons or captured retry body.
export function JudgeCommandHistory({eventID}:{eventID:string}){
 const identity=useIdentity(),prefix=judgeNamespace(identity.person.id,identity.draft_scope,eventID),attempts=useCommunicationCommandNamespace(prefix)
 const scope=usePrivateRequestScope(`${prefix}:original-recovery`),[error,setError]=useState<unknown>(),[message,setMessage]=useState<{text:string;requestID:string}>()
 const [reading,setReading]=useState(false),origin=useRef<HTMLButtonElement|null>(null),outcome=useRef<HTMLParagraphElement|null>(null)
 useEffect(()=>{if(!message)return;const frame=requestAnimationFrame(()=>{const source=origin.current;(source?.isConnected&&!source.matches(':disabled')&&source.getClientRects().length?source:outcome.current)?.focus()});return()=>cancelAnimationFrame(frame)},[message])
 return <>{attempts.length>0&&<aside className="fleet-judge-section" aria-label="Original judge command history"><h2>Earlier unconfirmed judge commands</h2><p>Only original request metadata survives a closed or denied task. Current reads and new commands never confirm an earlier outcome.</p>{attempts.map(a=><JudgeHistoryRow key={a.id} eventID={eventID} prefix={prefix} attempt={a} reading={reading} recover={async button=>{
 origin.current=button;
 if(reading||!a.command.requestID||!a.command.intentFingerprint)return
 setReading(true);setError(undefined);const ticket=scope.capture()
 try{const out=await originalJudgeReceipt(eventID,a.command.requestID,ticket.signal);ticket.assert()
 const intent:JudgeIntent={requestID:a.command.requestID,action:a.command.action as JudgeIntent['action'],revision:a.revision,runID:a.command.resourceID,teamID:a.command.subjectID,fingerprint:a.command.intentFingerprint}
 if(!out.receipt||!matchJudgeReceipt(out.receipt,intent,identity.person.id))throw new Error('No exact original acknowledgement is available. Its original outcome remains unknown.')
 setMessage({text:`Original ${out.receipt.action} acknowledged at revision ${out.receipt.revision}. Read current state separately.`,requestID:out.receipt.request_id});return true
 }catch(failure){if(ticket.current())setError(failure)}finally{if(ticket.current())setReading(false)}
 }}/>)}{!!error&&<Failure error={error}/>}</aside>}{message&&<p role="status" tabIndex={-1} ref={outcome}>{message.text}</p>}</>
}
type Attempt=ReturnType<typeof useCommunicationCommandNamespace>[number]
function JudgeHistoryRow({eventID,prefix,attempt:a,reading,recover}:{eventID:string;prefix:string;attempt:Attempt;reading:boolean;recover:(button:HTMLButtonElement)=>Promise<true|undefined>}){
 // Practice metadata uses the same explicit namespace as its private editor.
 const owner=`${prefix}:${a.command.resourceID?a.command.subjectID:'practice'}`,commands=useCommunicationCommands(owner)
 return <p>{a.operation} · Expected version {a.revision} · Request {a.command.requestID} · {a.state==='pending'?'Awaiting acknowledgement':'Outcome unknown'}{a.state==='unknown'&&<Button icon="refresh" variant="outline" disabled={reading} onClick={event=>void recover(event.currentTarget).then(confirmed=>{if(confirmed)commands.complete(a.id)})}>Read original acknowledgement</Button>}<span className="fleet-visually-hidden">Event {eventID}</span></p>
}

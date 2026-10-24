import { useEffect, useRef, useState } from 'react'
import { api, APIError, getSession, unwrap } from './api/client'
import { useIdentity } from './session'
import { useCommunicationCommands, CommunicationCommandHistory } from './communication-tests'
import { usePlanningDepartureBarrier } from './planning-operations'
import { Alert } from './components/ui/surfaces'
import type { PrivateRequestScope } from './private-request-scope'
import type { PresentationReader } from './presentation-reader'
import type { ConsolePage, CueCommand } from './presentation-model'
export type PresentationIntent={revision:number;reason:string;source:ConsolePage} & (
 |{kind:'control';command:CueCommand}
 |{kind:'pair';label:string;minutes:number}
 |{kind:'revoke';pairingID:string;label:string})
type State={kind:'idle'|'pending'|'unknown'|'rejected'|'accepted';message?:string}
export function usePresentationOperation(eventID:string,reader:PresentationReader,scope:PrivateRequestScope){
 const identity=useIdentity(),history=useCommunicationCommands(`${identity.person.id}:${identity.draft_scope}:${eventID}:presentation`),lock=useRef(false),hard=useRef(false)
 const [state,setState]=useState<State>({kind:'idle'}),[secret,setSecret]=useState('')
 useEffect(()=>{const clear=()=>setSecret(''),hidden=()=>{if(document.visibilityState!=='visible')clear()};window.addEventListener('beforeprint',clear);window.addEventListener('pagehide',clear);document.addEventListener('visibilitychange',hidden);return()=>{window.removeEventListener('beforeprint',clear);window.removeEventListener('pagehide',clear);document.removeEventListener('visibilitychange',hidden)}},[])
 const remembered=history.pending||history.unconfirmed
 const effective:State=history.pending?{kind:'pending',message:'An earlier room change is awaiting its response. Do not start a replacement.'}:history.unconfirmed?{kind:'unknown',message:'An earlier room change is unconfirmed. Read the current room before deliberately choosing a new starting point. Nothing is replayed.'}:state
 usePlanningDepartureBarrier(effective.kind==='pending')
 async function send(input:PresentationIntent){
  if(lock.current||hard.current||remembered)return false
  const ticket=scope.capture();if(!ticket.current())return false
  const intent=structuredClone(input);lock.current=true;setState({kind:'pending',message:'Checking current room control access…'});setSecret('')
  let sent=false,acknowledged=false,attempt:string|undefined
  try{
   const session=await getSession({signal:ticket.signal});ticket.assert()
   if(session.person.id!==identity.person.id||session.draft_scope!==identity.draft_scope)throw new DOMException('Session changed','AbortError')
   const current=await reader.read();ticket.assert()
   if(!current.can_control)throw new APIError(403,'presentation_unavailable')
   if(current.revision!==intent.revision)throw new APIError(409,'presentation_source_changed')
   ticket.assert();attempt=history.begin({operation:intent.kind==='control'?`Room ${intent.command.action}`:intent.kind==='pair'?'Display pairing':'Display revocation',revision:intent.revision,command:{action:intent.kind==='control'?intent.command.action:intent.kind,revision:intent.revision,...(intent.kind==='revoke'?{resourceID:intent.pairingID}:{})}})
   hard.current=true;sent=true
   const params={path:{event_id:eventID},header:{'X-CSRF-Token':session.csrf_token,'If-Match':`"${intent.revision}"`}}
   const response=intent.kind==='control'?unwrap(await api.POST('/api/v1/events/{event_id}/presentation',{params,body:intent.command})):intent.kind==='pair'?unwrap(await api.POST('/api/v1/events/{event_id}/presentation/pairings',{params,body:{label:intent.label,minutes:intent.minutes,reason:intent.reason}})):unwrap(await api.POST('/api/v1/events/{event_id}/presentation/pairings/{pairing_id}/revoke',{params:{...params,path:{...params.path,pairing_id:intent.pairingID}},body:{reason:intent.reason}}))
   if(response.revision!==intent.revision+1)throw new Error('Unrecognised presentation response')
   if(intent.kind==='pair'&&(!('token'in response)||typeof response.token!=='string'||!response.token))throw new Error('Pairing secret unavailable')
   // The direct response belongs to this request. A later GET has no original
   // intent identity and must never settle the safe unresolved capsule.
   acknowledged=true;history.complete(attempt);hard.current=false
   if(!ticket.current())return true
   if(intent.kind==='pair'){
    setSecret((response as {token:string}).token)
    await reader.read().catch(()=>undefined);ticket.assert()
   }else reader.receive(response as ConsolePage)
   setState({kind:'accepted',message:intent.kind==='control'&&intent.command.action==='save'?'Rehearsal saved. The room display is unchanged.':intent.kind==='control'&&intent.command.action==='return_to_schedule'?'Returned to the planned schedule.':intent.kind==='pair'?'Display paired. Transfer the secret once, then dismiss it.':'The server accepted this reviewed room change.'});return true
  }catch(failure){
   if(acknowledged)return true
   const definite=failure instanceof APIError&&failure.status>=400&&failure.status<500&&failure.status!==408
   if(sent&&attempt){if(definite){history.complete(attempt);hard.current=false}else history.uncertain(attempt)}
   if(!ticket.current())return false
   reader.deny(failure)
   if(!sent||definite){hard.current=false;setState({kind:'rejected',message:failure instanceof APIError?failure.status===409?'The source revision changed. Review the current room and your preserved unsent work before another action.':failure.message:'The change was not sent. Check current access before reviewing it again.'})}
   else setState({kind:'unknown',message:intent.kind==='pair'?'The pairing response was lost. A display may have been paired, but its once-only secret cannot be recovered from the list. Read current pairings and review any revocation or new pairing deliberately.':'The response was lost. This room change may have completed. A current room read cannot confirm the earlier change. Nothing is replayed.'})
   return false
  }finally{lock.current=false}
 }
 async function newBase(){if(lock.current||history.pending)return undefined;lock.current=true;const ticket=scope.capture();try{ticket.assert();const page=await reader.read();ticket.assert();if(!page.can_control)return undefined;history.selectNewBase();hard.current=false;setState({kind:'idle'});return page}catch{return undefined}finally{lock.current=false}}
 return {send,newBase,secret,dismissSecret:()=>setSecret(''),pending:effective.kind==='pending',unknown:effective.kind==='unknown',state:effective,
 feedback:<><CommunicationCommandHistory attempts={history.attempts}/>{effective.kind!=='idle'&&<Alert tone={effective.kind==='unknown'?'warning':effective.kind==='accepted'?'success':'neutral'} role={effective.kind==='rejected'?'alert':'status'}><p>{effective.message}</p>{effective.kind==='unknown'&&<p>No retry or recovery receipt is available for the original room command.</p>}</Alert>}</>}
}

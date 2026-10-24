import { useRef, useState } from 'react'
import { APIError, api, getSession, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { usePlanningDepartureBarrier } from './planning-operations'
import { CommunicationCommandHistory, useCommunicationCommands } from './communication-tests'
import { Alert } from './components/ui/surfaces'
import { Button } from './components/ui/button'
import type { MediaContext } from './media-workspace'
import type { PrivateRequestScope } from './private-request-scope'
export type MediaOperation=
 | {kind:'command';id:string;revision:number;command:components['schemas']['MediaCommand']}
 | {kind:'upload';revision:number;key:string;upload:components['schemas']['MediaPhotoUpload']}
type State={kind:'idle'|'pending'|'rejected'|'unknown'|'saved';message?:string;intent?:string}
export function useMediaOperation(context:MediaContext,scope:PrivateRequestScope,owner:string){
 const identity=useIdentity(),history=useCommunicationCommands(`${identity.person.id}:${identity.draft_scope}:${context.eventID}:media:${owner}`)
 const attempt=useRef<string|undefined>(undefined)
 const [state,setState]=useState<State>({kind:'idle'}),[retryAvailable,setRetryAvailable]=useState(false),lock=useRef(false),hard=useRef(false),retained=useRef<MediaOperation|undefined>(undefined)
 const remembered=history.unconfirmed||history.pending,stateNow=state.kind==='idle'&&remembered?{kind:'unknown' as const,message:'An earlier media intent is unconfirmed. Its private payload is unavailable in this task. Current reads do not establish its original receipt.'}:state
 usePlanningDepartureBarrier(state.kind==='pending')
 function intentOf(op:MediaOperation){return op.kind==='upload'?op.key:op.command.command_id}
 async function send(input?:MediaOperation){
  if(lock.current||(!input&&!retained.current)||(input&&(hard.current||remembered||state.kind==='unknown')))return false
  const ticket=scope.capture();if(!ticket.current())return false
  const op=input?structuredClone(input):retained.current!
  const retry=!input,id=op.kind==='upload'?op.upload.id:op.id,intent=intentOf(op),expected=op.kind==='upload'?1:op.command.version+1
  lock.current=true;setState({kind:'pending',intent,message:'Checking current authority for this exact reviewed intent…'})
  let sent=false,acknowledged=false
  try{
   const session=await getSession({signal:ticket.signal});ticket.assert()
   if(session.person.id!==identity.person.id||session.draft_scope!==identity.draft_scope)throw new DOMException('Session changed','AbortError')
   const page=unwrap(await api.GET('/api/v1/events/{event_id}/media',{signal:ticket.signal,cache:'no-store',params:{path:{event_id:context.eventID},query:{limit:1}}}));ticket.assert();context.lifetime.acceptPage(page);ticket.assert()
   if(!page.can_collect)throw new APIError(403,'media_write_unavailable')
   if(op.kind==='command'&&op.command.version>0){
    const current=unwrap(await api.GET('/api/v1/events/{event_id}/media/{media_id}',{signal:ticket.signal,cache:'no-store',params:{path:{event_id:context.eventID,media_id:id}}}));ticket.assert();context.lifetime.acceptItem(current);ticket.assert()
    if(!current.can_edit)throw new APIError(403,'media_write_unavailable')
    // An identical retry uses native owned/write authority and the original key.
    // Current can_publish may be false precisely because the first publish won.
    if(!retry&&(current.version!==op.command.version||page.revision!==op.revision))throw new APIError(409,'media_source_changed')
    if(!retry&&((op.command.action==='review'&&!current.can_review)||(op.command.action==='publish'&&!current.can_publish)||(op.command.action==='withdraw'&&!current.can_withdraw)))throw new APIError(409,'media_action_unavailable')
   }else if(!retry&&page.revision!==op.revision)throw new APIError(409,'media_source_changed')
   ticket.assert();retained.current=op;setRetryAvailable(true)
   if(!retry)attempt.current=history.begin({operation:op.kind==='upload'?'Media upload':`Media ${op.command.action}`,revision:op.revision,command:{intentID:intent,action:op.kind==='upload'?'upload':op.command.action,resourceID:id,expectedVersion:expected,revision:op.kind==='upload'?0:op.command.version}})
   else if(attempt.current)history.begin({operation:'Media identical retry',revision:op.revision,command:{action:'identical retry'}},attempt.current)
   hard.current=true;sent=true
   const header={'X-CSRF-Token':session.csrf_token,'If-Match':`"${op.revision}"`}
   const result=op.kind==='upload'?unwrap(await api.POST('/api/v1/events/{event_id}/media/upload',{params:{path:{event_id:context.eventID},header:{...header,'Idempotency-Key':op.key}},body:op.upload})):unwrap(await api.PUT('/api/v1/events/{event_id}/media/{media_id}',{params:{path:{event_id:context.eventID,media_id:op.id},header},body:op.command}))
   if(result.id!==id||result.acknowledged_version!==expected)throw new Error('Original acknowledgement unavailable')
   // Acknowledgement names the original accepted version, independently of the
   // CURRENT safe projection. Refresh failure must not erase this actual receipt.
   acknowledged=true;hard.current=false;history.complete(attempt.current!);retained.current=undefined
   if(!ticket.current())return true
   setRetryAvailable(false)
   setState({kind:'saved',message:`Original intent accepted at media version ${result.acknowledged_version}. Current safe projection version ${result.version}.`})
   await context.refresh();return true
  }catch(error){
   if(acknowledged)return true
   const definite=error instanceof APIError&&error.status>=400&&error.status<500&&error.status!==408
   if(sent&&attempt.current){if(definite&&!retry)history.complete(attempt.current);else history.uncertain(attempt.current)}
   if(!ticket.current())return false
   context.deny(error)
   if(!sent||definite){if(!retry){hard.current=false;if(attempt.current)history.complete(attempt.current)};retained.current=retry?op:undefined;setRetryAvailable(retry);setState(retry?{kind:'unknown',intent,message:'The original attempt remains unconfirmed. This retry did not establish its receipt.'}:{kind:'rejected',message:error instanceof APIError?error.message:'The intent was not sent. Review the current source before continuing.'})}
   else {void context.refresh();setState({kind:'unknown',intent,message:'The response was lost. The native command may have completed. Read current media, then deliberately retry this identical intent only while current ownership permits it. Nothing is replayed automatically.'})}
   return false
  }finally{lock.current=false;if(!scope.capture().current())retained.current=undefined}
 }
 return {state:stateNow,pending:state.kind==='pending',unknown:stateNow.kind==='unknown',send,
  feedback:<><CommunicationCommandHistory attempts={history.attempts}/>{stateNow.kind!=='idle'&&<Alert role={stateNow.kind==='rejected'?'alert':'status'} tone={stateNow.kind==='saved'?'success':stateNow.kind==='unknown'?'warning':'neutral'}><p>{stateNow.message}</p>{stateNow.intent&&<p className="fleet-reference">Local intent {stateNow.intent} · diagnostic reference</p>}{stateNow.kind==='unknown'&&<div className="fleet-pattern-actions"><Button icon="refresh" variant="outline" onClick={()=>void context.refresh()}>Check current media</Button>{retryAvailable&&<Button variant="outline" disabled={!context.writable} onClick={()=>void send()}>Retry identical media intent</Button>}</div>}</Alert>}</>}
}

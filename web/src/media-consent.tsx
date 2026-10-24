import { useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, APIError, getSession, privateKey, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { PrivateRequestScope, usePrivateRequestScope } from './private-request-scope'
import { CommunicationCommandHistory, useCommunicationCommands } from './communication-tests'
import { useMentorTaskNavigation } from './mentor-task'
import { PhotoConsentReview } from './media-consent-review'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
type Consent=components['schemas']['OwnPhotoConsent']
type Choice='yes'|'no'|'unknown'
export function PhotoConsent({eventID,refresh}:{eventID:string;refresh:()=>Promise<void>}){
 const identity=useIdentity(),root=useRef<HTMLElement>(null),scope=usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${eventID}:own-photo-consent`)
 const history=useCommunicationCommands(`${identity.person.id}:${identity.draft_scope}:${eventID}:media:own-consent`),lock=useRef(false),authority=useRef<Consent|undefined>(undefined)
 const initial=useMemo(()=>new PrivateRequestScope(`${scope.key}:editor`,scope),[scope]),editor=useRef(initial),[editorScope,setEditorScope]=useState(initial)
 const [ended,setEnded]=useState(false)
 const [review,setReview]=useState<{choice:Choice;revision:number}>(),[pending,setPending]=useState(false),[failure,setFailure]=useState<unknown>(),[notice,setNotice]=useState('')
 function observe(current:Consent){const previous=authority.current;authority.current=current;if(!editor.current.capture().current()||(previous&&((previous.can_opt_in&&!current.can_opt_in)||(previous.can_revoke&&!current.can_revoke)))){editor.current.invalidate();editor.current=new PrivateRequestScope(`${scope.key}:editor:${crypto.randomUUID()}`,scope);setEditorScope(editor.current);setReview(undefined)}}
 const query=useQuery({queryKey:[...privateKey(identity.person.id,eventID,'photo-consent'),identity.draft_scope],gcTime:0,retry:false,refetchOnMount:'always',refetchInterval:10_000,queryFn:async({signal})=>{const ticket=scope.capture();ticket.assert();try{const current=unwrap(await api.GET('/api/v1/events/{event_id}/photo-consent',{signal,cache:'no-store',params:{path:{event_id:eventID}}}));ticket.assert();observe(current);return current}catch(error){if(ticket.current()&&error instanceof APIError&&[401,403,404].includes(error.status)){authority.current=undefined;editor.current.invalidate();setReview(undefined)}throw error}}})
 useLayoutEffect(()=>{const end=()=>{editor.current.invalidate();setReview(undefined);setEnded(true)};const events=['session-ending','session-expired','mentor-draft-invalidated'];for(const name of events)window.addEventListener(name,end);return()=>{for(const name of events)window.removeEventListener(name,end)}},[])
 const blocked=history.unconfirmed||history.pending,navigation=useMentorTaskNavigation({root,dirty:!!review,pending,unknown:history.unconfirmed})
 function cancel(){navigation.discard(()=>setReview(undefined))}
 function choose(choice:Choice){if(!query.data||query.error||blocked||pending)return;navigation.capture();setReview({choice,revision:query.data.revision})}
 async function confirm(){
  if(!review||lock.current||blocked||query.error)return
  const captured={...review},ticket=editorScope.capture();if(!ticket.current())return
  lock.current=true;setPending(true);setFailure(undefined);let attempt:string|undefined,sent=false,accepted=false
  try{
   const session=await getSession({signal:ticket.signal});ticket.assert();if(session.person.id!==identity.person.id||session.draft_scope!==identity.draft_scope)throw new DOMException('Session changed','AbortError')
   const current=unwrap(await api.GET('/api/v1/events/{event_id}/photo-consent',{signal:ticket.signal,cache:'no-store',params:{path:{event_id:eventID}}}));ticket.assert();observe(current);ticket.assert()
   if(current.revision!==captured.revision)throw new APIError(409,'photo_consent_changed')
   if(captured.choice==='yes'?!current.can_opt_in:!current.can_revoke)throw new APIError(403,'photo_consent_unavailable')
   attempt=history.begin({operation:'Own photo-consent change',revision:captured.revision,command:{intentID:crypto.randomUUID(),action:'photo-consent',resourceID:identity.person.id}});sent=true
   unwrap(await api.PUT('/api/v1/events/{event_id}/photo-consent',{params:{path:{event_id:eventID},header:{'X-CSRF-Token':session.csrf_token,'If-Match':`"${captured.revision}"`}},body:{choice:captured.choice}}))
   // Direct success acknowledges this PUT, but its body is a current read made
   // after commit, not an immutable per-command choice/revision receipt.
   accepted=true;history.complete(attempt);if(!ticket.current())return
   setReview(undefined);navigation.restore();setNotice('Your change was accepted. The current choice is refreshed separately; an earlier approval is never restored.');await query.refetch();await refresh()
  }catch(error){
   if(accepted)return
   const definite=error instanceof APIError&&error.status>=400&&error.status<500&&error.status!==408
   if(attempt){if(definite)history.complete(attempt);else history.uncertain(attempt)}
   if(!ticket.current())return
   setFailure(error);setNotice(sent&&!definite?'The original outcome is unknown. Current choice reads do not establish it. This change will not be resent.':'The choice was not accepted. Read the current choice and review a new intent.')
   if(definite&&[401,403,404].includes((error as APIError).status)){authority.current=undefined;editor.current.invalidate();setReview(undefined)}
   await query.refetch();await refresh()
  }finally{lock.current=false;if(scope.capture().current())setPending(false)}
 }
 if(ended)return <section className="media-private"><h2>Your photo consent</h2><p>Private session ended. Sign in again to read your current choice.</p></section>
 return <section ref={root} className="media-private media-consent py-6 border-b border-border" aria-label="Your own photo consent"><h2>Your photo consent</h2><p>This self-only choice does not grant gallery access or change other registration fields.</p><CommunicationCommandHistory attempts={history.attempts}/>{query.error?<Failure error={query.error} retry={()=>void query.refetch()}/>:!query.data||!query.isFetchedAfterMount?<Loading/>:<><p>Current choice: <strong>{query.data.choice}</strong> · source revision {query.data.revision}</p>{review?<PhotoConsentReview {...review} pending={pending} disabled={blocked} confirm={()=>void confirm()} correct={()=>{setReview(undefined);navigation.restore()}} cancel={cancel}/>:<div className="fleet-pattern-actions"><Button icon="check" data-coverage-focus="photo-consent-yes" disabled={pending||blocked||!query.data.can_opt_in} onClick={()=>choose('yes')}>Allow identifiable event media</Button><Button icon="remove" variant="outline" data-coverage-focus="photo-consent-no" disabled={pending||blocked||!query.data.can_revoke} onClick={()=>choose('no')}>Do not allow identifiable media</Button><Button icon="back" variant="outline" data-coverage-focus="photo-consent-unknown" disabled={pending||blocked||!query.data.can_revoke} onClick={()=>choose('unknown')}>Withdraw my consent choice</Button></div>}</>}{notice&&<p role="status">{notice}</p>}{failure!=null&&<Failure error={failure}/>}<div className="fleet-pattern-actions"><Button icon="refresh" variant="outline" pending={pending} onClick={()=>void query.refetch()}>Check current photo consent</Button>{history.unconfirmed&&!query.error&&query.data&&<Button variant="outline" onClick={()=>{if(!window.confirm('Keep the earlier outcome unconfirmed and use the current revision for a different explicit choice? No earlier request will be replayed.'))return;history.selectNewBase();setReview(undefined);setNotice('Earlier original outcome remains unconfirmed. Review a different explicit choice from the current revision.')}}>Acknowledge unresolved choice and review a new choice</Button>}</div></section>
}

import { useCallback, useRef, useState } from 'react'
import { api, APIError, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { usePrivateRequestScope, type PrivateRequestScope } from './private-request-scope'
import { usePlanningDepartureBarrier, usePlanningUnknownCommands } from './planning-operations'
export type Huddle = components['schemas']['ModerationHuddle']
export type HuddleCommand = components['schemas']['ModerationCommand']
export type HuddleIntent = { id: string; teamID: string; revision: number; command: HuddleCommand; actor: string; expected: number; intentID?: string }
const canonical = (v: unknown): string => JSON.stringify(v, (_, item: unknown) => item && typeof item === 'object' && !Array.isArray(item) ? Object.fromEntries(Object.entries(item).sort(([a], [b]) => a.localeCompare(b))) : item)
export function exactHuddleReceipt(h: Huddle, i: HuddleIntent) {
 const c=i.command
 if(h.team_id!==i.teamID||h.definition.id!==i.id||h.version!==i.expected||h.actor_id!==i.actor||h.action!==c.action||h.reason!==c.reason)return false
 switch(c.action){
  case 'configure':return canonical(h.definition)===canonical(c.definition)
  case 'add_gap':return h.gaps.some(g=>g.id===c.gap_id&&g.action_id===c.action_id&&g.action_version===c.action_version&&g.reason===c.reason)
  case 'resolve_gap':return h.gaps.some(g=>g.id===c.gap_id&&g.action_version===c.action_version&&g.resolution===c.reason&&g.resolved_by===i.actor&&canonical(g.evidence)===canonical(c.evidence))
  case 'propose_change':return h.proposals.some(p=>p.id===c.proposal_id&&canonical(p.card)===canonical(c.card)&&p.original.category_id===c.category_id&&p.proposed_value===c.proposed_value&&p.reason===c.reason&&canonical(p.evidence)===canonical(c.evidence))
  case 'accept_change':case 'reject_change':return h.proposals.some(p=>p.id===c.proposal_id&&p.state===(c.action==='accept_change'?'accepted':'rejected')&&p.decided_by===i.actor&&p.decision_reason===c.reason)
  case 'sign_off':return h.state==='signed'&&!!h.signoff_id
  case 'complete_second_review':return h.second_review.reviewer_id===i.actor&&!!h.second_review.card&&!!h.second_review.completed_at
  case 'open':return h.state==='open'&&h.cards.length===2
  case 'cancel':return h.state==='cancelled'
 }
}
export async function readHuddle(eventID:string,teamID:string,id:string,signal?:AbortSignal){return unwrap(await api.GET('/api/v1/events/{event_id}/teams/{team_id}/moderation-huddles/{huddle_id}',{signal,params:{path:{event_id:eventID,team_id:teamID,huddle_id:id}}}))}
export function useHuddleWrite(eventID:string,teamID:string,refresh:()=>Promise<unknown>,confirmed:()=>void,parentScope?:PrivateRequestScope){
 const identity=useIdentity(),[busy,setBusy]=useState(false),[denied,setDenied]=useState(false),[failure,setFailure]=useState<unknown>(),[uncertain,setUncertain]=useState<HuddleIntent>(),[notice,setNotice]=useState('No decision sent.'),lock=useRef(false),hardUnknown=useRef(false)
 const scope=usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${eventID}:${teamID}:huddle-command`,parentScope),history=usePlanningUnknownCommands(`huddle:${teamID}`)
 const blocked=!!uncertain||(!busy&&history.unconfirmed)
 usePlanningDepartureBarrier(busy)
 async function send(id:string,command:HuddleCommand,reviewedRevision?:number){
  if(lock.current||hardUnknown.current||blocked)return
  const ticket=scope.capture();if(!ticket.current())return
  lock.current=true;setBusy(true);setFailure(undefined)
  const captured=structuredClone(command);let intent:HuddleIntent|undefined,attempted=false
  try{
   const page=unwrap(await api.GET('/api/v1/events/{event_id}/review-coverage',{signal:ticket.signal,params:{path:{event_id:eventID},query:{team_id:teamID,limit:1}}}));ticket.assert()
   if(reviewedRevision!=null&&page.revision!==reviewedRevision)throw new APIError(409,'review_required')
   if(!page.teams.some(t=>t.id===teamID))throw new APIError(403,'forbidden')
   if(captured.action==='configure'&&!page.organiser)throw new APIError(403,'forbidden')
   if(captured.action!=='configure'||captured.version>0){
    let cursor:string|undefined,summary:components['schemas']['ReviewHuddleSummary']|undefined
    do{const agenda=unwrap(await api.GET('/api/v1/events/{event_id}/teams/{team_id}/review-agenda',{signal:ticket.signal,params:{path:{event_id:eventID,team_id:teamID},query:{cursor,limit:100}}}));ticket.assert();summary=agenda.agenda.items.find(item=>item.id===id);cursor=agenda.agenda.next_cursor??undefined;if(summary)break}while(cursor)
    if(!summary||summary.version!==captured.version)throw new APIError(409,'review_required')
    const permitted=captured.action==='configure'?summary.can_reconfigure:summary.can_open&&summary.can_record&&(!['open','add_gap','resolve_gap','accept_change','reject_change','sign_off','cancel'].includes(captured.action)||summary.can_manage)
    if(!permitted)throw new APIError(403,'forbidden')
    const h=captured.action==='configure'?undefined:await readHuddle(eventID,teamID,id,ticket.signal);ticket.assert()
    if(h&&h.version!==captured.version)throw new APIError(409,'review_required')
    if(captured.action==='complete_second_review'&&h?.second_review.reviewer_id!==identity.person.id)throw new APIError(403,'forbidden')
   }
   intent={id,teamID,revision:page.revision,command:captured,actor:identity.person.id,expected:captured.version+1,intentID:crypto.randomUUID()}
   ticket.assert();history.record({operation:`Huddle ${captured.action.replaceAll('_',' ')}`,revision:intent.revision,command:{intentID:intent.intentID,resource:id,version:captured.version}});attempted=true
   const result=unwrap(await api.PUT('/api/v1/events/{event_id}/teams/{team_id}/moderation-huddles/{huddle_id}',{params:{path:{event_id:eventID,team_id:teamID,huddle_id:id},header:{'If-Match':`"${intent.revision}"`,'X-CSRF-Token':identity.csrf_token}},body:captured}));ticket.assert()
   if(!exactHuddleReceipt(result,intent))throw new Error('Unexpected decision receipt')
   hardUnknown.current=false;history.resolve(intent.intentID!);setNotice(`Confirmed ${captured.action.replaceAll('_',' ')} version ${result.version}.`);await refresh().catch(error=>{if(ticket.current())setFailure(error)});if(ticket.current())confirmed()
  }catch(error){if(ticket.current()){setFailure(error);if(error instanceof APIError&&[401,403,404].includes(error.status)){scope.invalidate();setDenied(true);setUncertain(undefined);setNotice('Current authority was denied. Private decision details cleared; no deferred request may continue.');setBusy(false);return}if(attempted&&intent&&(!(error instanceof APIError)||error.status===408||error.status>=500)){hardUnknown.current=true;setUncertain(intent);setNotice('Outcome uncertain. No retry was made. Check the exact immutable history before making another decision.')}else{if(intent?.intentID)history.resolve(intent.intentID);setNotice('No decision confirmed. Review fresh current sources and versions; your request was not replayed.');await refresh()}}}finally{if(ticket.current()){setBusy(false);lock.current=false}}
 }
 async function check(){
  if(!uncertain||lock.current)return
  const ticket=scope.capture();if(!ticket.current())return
  lock.current=true;setBusy(true);setFailure(undefined)
  const captured=uncertain
  try{
   let cursor:number|undefined,found=false
   do{const page=unwrap(await api.GET('/api/v1/events/{event_id}/teams/{team_id}/moderation-huddles/{huddle_id}/history',{signal:ticket.signal,params:{path:{event_id:eventID,team_id:teamID,huddle_id:captured.id},query:{cursor,limit:50}}}));ticket.assert();found=page.items.some(h=>exactHuddleReceipt(h,captured));cursor=page.next_cursor??undefined;if(found)break}while(cursor!=null)
   if(found){hardUnknown.current=false;history.resolve(captured.intentID!);setUncertain(undefined);setNotice('Exact recorded decision confirmed from immutable history.');await refresh().catch(error=>{if(ticket.current())setFailure(error)});if(ticket.current())confirmed()}else setNotice('No exact receipt is visible under current authority. No request was replayed. The original intent remains unresolved.')
  }catch(error){if(ticket.current()){setFailure(error);if(error instanceof APIError&&[401,403,404].includes(error.status)){scope.invalidate();setDenied(true);setUncertain(undefined);setBusy(false)}}}finally{if(ticket.current()){setBusy(false);lock.current=false}}
 }
 const invalidateAuthority=useCallback(()=>{scope.invalidate();lock.current=true;hardUnknown.current=true;setDenied(true);setBusy(false);setUncertain(undefined);setFailure(undefined);setNotice('Current authority was denied. Private decision details were cleared; an attempted command remains fenced by minimal metadata.')},[scope])
 return {busy,denied,failure,uncertain,blocked,notice,send,check,invalidateAuthority,discard(){if(busy||!window.confirm('Leave the original decision outcome unconfirmed? Its minimal reference will remain; no request will be replayed.'))return;setUncertain(undefined);confirmed()}}
}

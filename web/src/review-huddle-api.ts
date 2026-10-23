import { useCallback, useEffect, useRef, useState } from 'react'
import { api, APIError, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
export type Huddle = components['schemas']['ModerationHuddle']
export type HuddleCommand = components['schemas']['ModerationCommand']
export type HuddleIntent = { id: string; revision: number; command: HuddleCommand; actor: string; expected: number }
const canonical = (v: unknown): string => JSON.stringify(v, (_, item: unknown) => item && typeof item === 'object' && !Array.isArray(item) ? Object.fromEntries(Object.entries(item).sort(([a], [b]) => a.localeCompare(b))) : item)
export function exactHuddleReceipt(h: Huddle, i: HuddleIntent) {
 const c=i.command
 if(h.definition.id!==i.id||h.version!==i.expected||h.actor_id!==i.actor||h.action!==c.action||h.reason!==c.reason)return false
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
export function useHuddleWrite(eventID:string,teamID:string,refresh:()=>Promise<unknown>,confirmed:()=>void){
 const identity=useIdentity(),[busy,setBusy]=useState(false),[failure,setFailure]=useState<unknown>(),[uncertain,setUncertain]=useState<HuddleIntent>(),[notice,setNotice]=useState('No decision sent.'),generation=useRef(0)
 useEffect(()=>()=>{generation.current++},[])
 async function send(id:string,command:HuddleCommand,reviewedRevision?:number){
  if(busy||uncertain)return
  const ticket=generation.current;setBusy(true);setFailure(undefined)
  let intent:HuddleIntent|undefined
  try{
   const page=unwrap(await api.GET('/api/v1/events/{event_id}/review-coverage',{params:{path:{event_id:eventID},query:{team_id:teamID,limit:1}}}))
   if(ticket!==generation.current)return
   if(reviewedRevision!=null&&page.revision!==reviewedRevision)throw new APIError(409,'review_required')
   intent={id,revision:page.revision,command:structuredClone(command),actor:identity.person.id,expected:command.version+1}
   const result=unwrap(await api.PUT('/api/v1/events/{event_id}/teams/{team_id}/moderation-huddles/{huddle_id}',{params:{path:{event_id:eventID,team_id:teamID,huddle_id:id},header:{'If-Match':`"${intent.revision}"`,'X-CSRF-Token':identity.csrf_token}},body:intent.command}))
   if(ticket!==generation.current)return
   if(!exactHuddleReceipt(result,intent))throw new Error('Unexpected decision receipt')
   setNotice(`Confirmed ${command.action.replaceAll('_',' ')} version ${result.version}.`);await refresh();if(ticket===generation.current)confirmed()
  }catch(error){if(ticket===generation.current){setFailure(error);if(intent&&(!(error instanceof APIError)||error.status>=500)){setUncertain(intent);setNotice('Outcome uncertain. No retry was made. Check the exact immutable history before making another decision.')}else{setNotice('No decision confirmed. Review fresh current sources and versions; your request was not replayed.');await refresh()}}}finally{if(ticket===generation.current)setBusy(false)}
 }
 async function check(){
  if(!uncertain||busy)return
  const ticket=generation.current;setBusy(true);setFailure(undefined)
  try{
   let cursor:number|undefined,found=false
   do{const page=unwrap(await api.GET('/api/v1/events/{event_id}/teams/{team_id}/moderation-huddles/{huddle_id}/history',{params:{path:{event_id:eventID,team_id:teamID,huddle_id:uncertain.id},query:{cursor,limit:50}}}));if(ticket!==generation.current)return;found=page.items.some(h=>exactHuddleReceipt(h,uncertain));cursor=page.next_cursor??undefined;if(found)break}while(cursor!=null)
   if(found){setUncertain(undefined);setNotice('Exact recorded decision confirmed from immutable history.');await refresh();if(ticket===generation.current)confirmed()}else setNotice('No exact receipt is visible under current authority. No request was replayed. Keep this intent unresolved or explicitly discard it after reviewing current server state.')
  }catch(error){if(ticket===generation.current)setFailure(error)}finally{if(ticket===generation.current)setBusy(false)}
 }
 const invalidateAuthority=useCallback(()=>{generation.current++;setBusy(false);setUncertain(undefined);setFailure(undefined);setNotice('Current authority was denied. Pending private decision intent was cleared; no deferred request may continue.')},[])
 return {busy,failure,uncertain,notice,send,check,invalidateAuthority,discard(){setUncertain(undefined);setNotice('Uncertain intent explicitly released. Re-open fresh current context before submitting any new decision.');confirmed()}}
}

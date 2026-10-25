import { useBlocker } from '@tanstack/react-router'
import { useCallback, useEffect, useRef, useState } from 'react'
import { api, APIError, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { useTodayRead } from './today-read'
import { useEventReadBoundary } from './event-authority'
import { usePrivateRequestScope, type PrivateRequestScope } from './private-request-scope'
import { useCommunicationCommands } from './communication-tests'
import { usePlanningDepartureBarrier } from './planning-operations'
import { Button } from './components/ui/button'
import { Field, Input, Textarea, Checkbox } from './components/ui/fields'
import { Failure, Loading } from './components/states'
import { FreezeEvidence, FreezeReview } from './submission-freeze-view'
import { freezeFingerprint, trimFreezeEvidence, matchesFreezeReceipt, type FreezePage, type FreezeIntent, type FreezeAcknowledgement, type FreezeCommand } from './submission-freeze-model'
import { localDate } from './event-time'
import { useRepositoryDraft } from './repository-drafts'

// Only this domain's safe original UUID/revision/fingerprint metadata survives a
// private owner. Source bodies, branch drafts and reasons remain local.
export function SubmissionFreeze({eventID,teamID,timezone,parent,paused=false}:{eventID:string;teamID:string;timezone:string;parent?:PrivateRequestScope;paused?:boolean}) {
 const identity=useIdentity(),boundary=useEventReadBoundary(eventID)
 const load=useCallback(async(signal:AbortSignal)=>unwrap(await api.GET('/api/v1/events/{event_id}/teams/{team_id}/submission-freeze',{signal,cache:'no-store',params:{path:{event_id:eventID,team_id:teamID}}})),[eventID,teamID])
 const privacy=useCallback((p:FreezePage)=>JSON.stringify([p.can_configure,p.can_reconcile,p.can_amend,p.source?.submission_revision,p.source?.managed_evidence_ids,p.observations,p.decisions,p.current]),[])
 const read=useTodayRead(`${identity.person.id}:${identity.draft_scope}:${eventID}:${teamID}:code-cutoff`,load,parent??boundary?.scope,privacy)
 const commands=useCommunicationCommands(`${identity.person.id}:${identity.draft_scope}:${eventID}:submission-freeze:${teamID}`),scope=usePrivateRequestScope(`${eventID}:${teamID}:code-recovery`,read.authority)
 const [ack,setAck]=useState<FreezeAcknowledgement>(),[failure,setFailure]=useState<unknown>(),[reading,setReading]=useState(false),recovery=useRef(0)
 usePlanningDepartureBarrier(commands.pending)
 async function recover(attempt:typeof commands.attempts[number]) {
  const {requestID,action,intentFingerprint}=attempt.command
  if(!requestID||!intentFingerprint||reading||commands.pending)return
  const ticket=scope.capture(),number=++recovery.current;setReading(true);setFailure(undefined)
  try{ticket.assert();const result=unwrap(await api.GET('/api/v1/events/{event_id}/teams/{team_id}/submission-freeze/commands/{request_id}',{signal:ticket.signal,cache:'no-store',params:{path:{event_id:eventID,team_id:teamID,request_id:requestID}}}));ticket.assert();if(!result.receipt)throw new FreezeMismatch('No original acknowledgement is available. The original outcome remains unknown; a current read does not prove failure.');if(!matchesFreezeReceipt(result.receipt,{requestID,action,revision:attempt.revision,fingerprint:intentFingerprint}))throw new FreezeMismatch('The acknowledgement does not match the original reviewed command. Its outcome remains unknown.');commands.complete(attempt.id);setAck({requestID,action,revision:result.receipt.revision,bodyAvailable:!!result.receipt.decision});await read.refresh()}
  catch(error){if(ticket.current()){setFailure(error);if(error instanceof APIError&&[401,403,404].includes(error.status))await read.refresh()}}
  finally{if(number===recovery.current)setReading(false)}
 }
 if(read.denied)return <section className="fleet-freeze-private"><p>Current code evidence access is unavailable. Private code history and reviews have been cleared.</p><Button icon="refresh" onClick={()=>void read.refresh()}>Check current code evidence access</Button></section>
 if(!read.data)return read.error?<Failure error={read.error} retry={()=>void read.refresh()}/>:<Loading/>
 return <section className="fleet-freeze-private" aria-label="Submission code workspace"><div className="fleet-pattern-actions"><Button icon="refresh" variant="outline" disabled={read.pending||commands.pending||reading} onClick={()=>void read.refresh()}>Read current code evidence</Button></div>{!!read.error&&<Failure error={read.error}/>}
  {commands.attempts.map(a=><aside key={a.id}><p>{a.state==='pending'?'Awaiting original code acknowledgement':'Original code change outcome unknown'} · Request {a.command.requestID} · Expected revision {a.revision}. Current code evidence does not settle this request.</p>{a.state==='unknown'&&<Button icon="refresh" variant="outline" disabled={read.pending||commands.pending||reading} onClick={()=>void recover(a)}>Read original code acknowledgement</Button>}</aside>)}
  {ack&&<p role="status">Original code change acknowledged: {ack.action} · Revision {ack.revision} · Request {ack.requestID}. Read current evidence for the selected formal version.{!ack.bodyAvailable&&ack.action!=='configure'&&' The decision body is not currently disclosed.'}</p>}
  {!!failure&&<FreezeFailure error={failure}/>}
  <FreezeEvidence page={read.data} timezone={timezone}/>
  <FreezeEditor key={privacy(read.data)} page={read.data} eventID={eventID} teamID={teamID} timezone={timezone} parent={read.authority} paused={paused||read.pending||!!read.error||!!boundary?.paused||reading} refresh={()=>read.refresh()} commands={commands} acknowledged={setAck}/>
 </section>
}
export class FreezeMismatch extends Error {}
function FreezeFailure({error}:{error:unknown}) {return error instanceof FreezeMismatch?<p role="alert">{error.message}</p>:<Failure error={error}/>}
type Push=components['schemas']['SubmissionFreezePush']
function FreezeEditor({page,eventID,teamID,timezone,parent,paused,refresh,commands,acknowledged}:{page:FreezePage;eventID:string;teamID:string;timezone:string;parent:PrivateRequestScope;paused:boolean;refresh:()=>Promise<void>;commands:ReturnType<typeof useCommunicationCommands>;acknowledged:(ack:FreezeAcknowledgement|undefined)=>void}) {
 const identity=useIdentity(),scope=usePrivateRequestScope(`${eventID}:${teamID}:code-command`,parent),opener=useRef<HTMLElement|null>(null),lock=useRef(false)
 const [branch,setBranch]=useState(page.branch),[sha,setSHA]=useState(''),[reason,setReason]=useState(''),[external,setExternal]=useState(''),[observations,setObservations]=useState<string[]>([]),[pushIDs,setPushIDs]=useState<string[]>([]),[pushes,setPushes]=useState<Push[]>([]),[cursor,setCursor]=useState<string|null>(),[reading,setReading]=useState(false),[review,setReview]=useState<FreezeIntent>(),[failure,setFailure]=useState<unknown>(),[fresh,setFresh]=useState<FreezePage>()
 const blocked=commands.pending||commands.unconfirmed,disabled=paused||blocked||reading
 const unsent=branch!==page.branch||!!sha||!!reason||!!external||!!review
 useRepositoryDraft(unsent||blocked)
 useBlocker({shouldBlockFn:()=>commands.pending||(unsent&&!window.confirm('Discard unsaved code evidence inputs and review?')),enableBeforeUnload:unsent||commands.pending})
 useEffect(()=>()=>{lock.current=false},[])
 async function prepare(action:FreezeCommand['action'],button:HTMLElement){
  if(disabled||lock.current)return
  const ticket=scope.capture();opener.current=button;setFailure(undefined)
  const command:FreezeCommand=action==='configure'?{request_id:crypto.randomUUID(),action,branch}:{request_id:crypto.randomUUID(),action,source_id:page.source!.id,sha,reason:trimFreezeEvidence(reason),external_evidence:trimFreezeEvidence(external),observation_ids:[...observations],push_ids:[...pushIDs],native_revision:page.native_revision,submission_revision:page.current_submission_revision!,prior_decision_id:action==='amend'?page.current?.id??null:null}
  try{ticket.assert();const fingerprint=await freezeFingerprint(command,page.revision);ticket.assert();setReview({command,revision:page.revision,fingerprint})}catch(error){if(ticket.current())setFailure(error)}
 }
 async function confirm(){
  if(!review||disabled||lock.current||review.revision!==page.revision)return
  const allowed=review.command.action==='configure'?page.can_configure:review.command.action==='reconcile'?page.can_reconcile:page.can_amend
  if(!allowed)return
  const captured=review,ticket=scope.capture();ticket.assert();lock.current=true
  const id=commands.begin({operation:'Submission code change',revision:captured.revision,command:{action:captured.command.action,requestID:captured.command.request_id,resourceID:teamID,intentFingerprint:captured.fingerprint}})
  try{ticket.assert();const receipt=unwrap(await api.POST('/api/v1/events/{event_id}/teams/{team_id}/submission-freeze',{params:{path:{event_id:eventID,team_id:teamID},header:{'X-CSRF-Token':identity.csrf_token,'If-Match':`"${captured.revision}"`}},body:captured.command}));if(!matchesFreezeReceipt(receipt,{requestID:captured.command.request_id,action:captured.command.action,revision:captured.revision,fingerprint:captured.fingerprint}))throw new FreezeMismatch('The response did not acknowledge the exact reviewed code command. Its outcome remains unknown.');commands.complete(id);if(ticket.current()){setReview(undefined);acknowledged({requestID:receipt.request_id,action:receipt.action,revision:receipt.revision,bodyAvailable:!!receipt.decision});setSHA('');setReason('');setExternal('');await refresh();requestAnimationFrame(()=>opener.current?.focus())}}
  catch(error){if(error instanceof APIError&&[400,401,403,404,409,422].includes(error.status))commands.complete(id);else commands.uncertain(id);if(ticket.current()){setReview(undefined);setFailure(error);await refresh()}}
  finally{lock.current=false}
 }
 async function readPushes(more=false){if(reading||commands.pending)return;const ticket=scope.capture();setReading(true);try{ticket.assert();const p=unwrap(await api.GET('/api/v1/events/{event_id}/teams/{team_id}/submission-freeze/push-evidence',{signal:ticket.signal,cache:'no-store',params:{path:{event_id:eventID,team_id:teamID},query:{limit:25,after:more?cursor??undefined:undefined}}}));ticket.assert();setPushes(p.items);setCursor(p.next_cursor)}catch(error){if(ticket.current()){setFailure(error);if(error instanceof APIError&&[401,403,404].includes(error.status))await refresh()}}finally{if(ticket.current())setReading(false)}}
 async function readFresh(){const ticket=scope.capture();setReading(true);try{ticket.assert();await refresh();ticket.assert();const p=unwrap(await api.GET('/api/v1/events/{event_id}/teams/{team_id}/submission-freeze',{signal:ticket.signal,cache:'no-store',params:{path:{event_id:eventID,team_id:teamID}}}));ticket.assert();setFresh(p)}catch(error){if(ticket.current())setFailure(error)}finally{if(ticket.current())setReading(false)}}
 return <section className="fleet-freeze-editor">{page.can_configure&&<form hidden={!!review} onSubmit={e=>{e.preventDefault();void prepare('configure',(e.nativeEvent as SubmitEvent).submitter!)}}><Field label="Submission branch" description="Choose the managed repository branch before cutoff. This does not submit your entry."><Input required maxLength={1000} value={branch} disabled={disabled} onChange={e=>setBranch(e.target.value)}/></Field><Button type="submit" icon="repository" disabled={disabled||!branch.trim()}>Review submission branch</Button></form>}
  {(page.can_reconcile||page.can_amend)&&<form hidden={!!review} onSubmit={e=>{e.preventDefault();void prepare(page.can_reconcile?'reconcile':'amend',(e.nativeEvent as SubmitEvent).submitter!)}}><h3>Organiser code evidence decision</h3><p>Review actual observations and retained pushes. Commit author dates alone are insufficient. A separate amendment binds an explicitly corrected formal version while preserving the original cutoff history.</p><fieldset disabled={disabled}><Field label="Reviewed commit SHA"><Input required pattern="([a-f0-9]{40}|[a-f0-9]{64})" value={sha} onChange={e=>setSHA(e.target.value)}/></Field><Field label="Decision reason"><Textarea required maxLength={4000} value={reason} onChange={e=>setReason(e.target.value)}/></Field><Field label="Independent supporting evidence" description="Explain how selected facts establish the cutoff decision; later observations do not automatically prove historical state."><Textarea maxLength={8000} value={external} onChange={e=>setExternal(e.target.value)}/></Field>
  {!!page.observations.length&&<fieldset><legend>Retained candidate observations</legend>{page.observations.map(o=><Field key={o.id} label={`${o.status.replaceAll('_',' ')} · ${o.sha||'No SHA'} · ${localDate(o.observed_at,timezone)}`}><Checkbox checked={observations.includes(o.id)} disabled={!o.sha} onChange={e=>setObservations(ids=>e.target.checked?[...ids,o.id]:ids.filter(id=>id!==o.id))}/></Field>)}</fieldset>}
  <Button icon="refresh" variant="outline" disabled={reading} onClick={()=>void readPushes()}>Read authenticated push evidence</Button><ul>{pushes.map(p=><li key={p.id}><Field label={`${p.sha} · ${p.ref}`} description={`Push ${p.id} · Received ${localDate(p.received_at,timezone)} · Provider time ${p.provider_at?localDate(p.provider_at,timezone):'not supplied'} · Payload hash ${p.payload_hash}`}><Checkbox checked={pushIDs.includes(p.id)} onChange={e=>setPushIDs(ids=>e.target.checked?[...ids,p.id]:ids.filter(id=>id!==p.id))}/></Field></li>)}</ul>{cursor&&<Button icon="next" variant="outline" onClick={()=>void readPushes(true)}>Next push evidence page</Button>}
  <Button type="submit" icon="check" disabled={!reason.trim()||!sha||(!external.trim()&&!observations.length&&!pushIDs.length)}>Review {page.can_reconcile?'code reconciliation':'separate code amendment'}</Button></fieldset></form>}
  {commands.unconfirmed&&<Button icon="refresh" variant="outline" disabled={paused||commands.pending||reading} onClick={()=>void readFresh()}>Review a fresh code starting point</Button>}
  {fresh&&<FreezeReview title="Review a fresh code starting point" pending={false} disabled={paused||commands.pending||fresh.revision!==page.revision} confirm={()=>{commands.selectNewBase();setFresh(undefined)}} cancel={()=>setFresh(undefined)} opener={opener}><FreezeEvidence page={fresh} timezone={timezone}/><p>The earlier request remains unknown and available for original acknowledgement lookup. This read does not settle it. A future command requires a new review and request UUID; nothing is replayed.</p></FreezeReview>}
  {review&&<FreezeReview title={review.command.action==='configure'?'Review submission branch':'Review organiser code decision'} pending={commands.pending} disabled={paused||blocked||review.revision!==page.revision} confirm={()=>void confirm()} cancel={()=>setReview(undefined)} opener={opener}><p>Team {teamID} · Freeze revision {review.revision}</p>{review.command.action==='configure'?<p>Configured branch: <strong>{review.command.branch}</strong>. No formal submission or historical SHA is accepted by this change.</p>:<><p>Source {review.command.source_id} · Submission state revision {review.command.native_revision} · Current formal version {review.command.submission_revision}</p><p>Selected SHA <code>{review.command.sha}</code></p><p>{review.command.reason}</p><p>{review.command.external_evidence}</p><p>Observations: {review.command.observation_ids?.join(', ')||'None'}<br/>Pushes: {review.command.push_ids?.join(', ')||'None'}<br/>Prior decision: {review.command.prior_decision_id??'None'}</p></>}</FreezeReview>}
  {!!failure&&<FreezeFailure error={failure}/>}
 </section>
}

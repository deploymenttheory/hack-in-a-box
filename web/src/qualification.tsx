import {useCallback,useLayoutEffect,useRef,useState} from 'react'
import {useParams} from '@tanstack/react-router'
import {api,unwrap} from './api/client'
import {useIdentity} from './session'
import {EventBrand} from './branding'
import {EventReadScopeProvider,useEventAuthority,useEventReadBoundary} from './event-authority'
import {useTodayRead} from './today-read'
import type {PrivateRequestScope} from './private-request-scope'
import {DetailWorkspace,Inspector} from './components/patterns'
import {ActionLink,Button} from './components/ui/button'
import {Failure,Loading} from './components/states'
import {readHuddle} from './review-huddle-api'
import {QualificationOverview,QualificationCardComparison,QualificationSelection,QualificationPreparationPreview,QualificationEvidenceList,QualificationEvidenceDestination} from './qualification-view'
import type {QualificationPage as PanelPage,QualificationCard,QualificationIntent,QualificationCommand} from './qualification-model'
import {QualificationCommandHistory} from './qualification-command'
import {CollectionHeading,CollectionRecords} from './components/patterns'
import {Link} from '@tanstack/react-router'
import {QualificationEditor} from './qualification-editor'
import {QualificationHistory} from './qualification-history'

export function QualificationDirectoryPage(){
 const {eventId}=useParams({strict:false}),identity=useIdentity(),event=useEventAuthority(eventId!)
 if(event.denial||event.query.error&&!event.query.data)return <Failure error={event.denial??event.query.error} retry={()=>void event.refresh()}/>
 if(!event.verified||!event.query.data)return <Loading/>
 return <EventReadScopeProvider value={event.boundary}><EventBrand eventID={eventId!}><QualificationDirectory key={`${identity.person.id}:${identity.draft_scope}:${eventId}:${event.generation}`} eventID={eventId!} eventName={event.query.data.name} parent={event.scope} paused={event.boundary.paused}/></EventBrand></EventReadScopeProvider>
}
function QualificationDirectory({eventID,eventName,parent,paused}:{eventID:string;eventName:string;parent:PrivateRequestScope;paused:boolean}){
 const load=useCallback(async(signal:AbortSignal)=>unwrap(await api.GET('/api/v1/events/{event_id}/mentor-qualification',{signal,cache:'no-store',params:{path:{event_id:eventID}}})),[eventID])
 const read=useTodayRead(`${eventID}:qualification-directory`,load,parent)
 return <section><CollectionHeading title="Moderation and selections" scope={{event:eventName,entity:'Current native panel contexts'}} description="Open a currently permitted final mentor panel. A listed context does not grant private evidence or a scoring action."/>{!!read.error&&<Failure error={read.error} retry={()=>void read.refresh()}/>}<div hidden={read.denied} inert={paused||read.pending||!!read.error}>{!read.data?<Loading/>:<><CollectionRecords label="Current moderation panels" rows={read.data.items} rowKey={run=>run.run_id} rowLabel={run=>run.name} columns={[{id:'version',label:'Recorded panel version',essential:true,cell:run=>run.version},{id:'lock',label:'Record state',cell:run=>run.record_locked?'Signed private record':'Open private preparation'}]} actions={run=><Link to="/events/$eventId/panel/$runId" params={{eventId:eventID,runId:run.run_id}}>Open {run.name}</Link>}/>{!read.data.items.length&&<p>No current panel contexts are disclosed. Ask the organiser to review current panel membership, conflicts, common cutoff and published final criteria.</p>}</>}</div><ActionLink href={`/events/${eventID}/review`} icon="back">Back to support and review coverage</ActionLink></section>
}

export function QualificationPanelPage(){
 const {eventId,runId}=useParams({strict:false}),identity=useIdentity()
 return <><QualificationCommandHistory eventID={eventId!} runID={runId!}/><EventBrand eventID={eventId!}><QualificationEvent key={`${identity.person.id}:${identity.draft_scope}:${eventId}:${runId}`} eventID={eventId!} runID={runId!}/></EventBrand></>
}
function QualificationEvent({eventID,runID}:{eventID:string;runID:string}){
 const authority=useEventAuthority(eventID)
 if(authority.denial)return <Failure error={authority.denial} retry={()=>void authority.refresh()}/>
 if(!authority.verified||!authority.query.data)return <Loading/>
 const event=authority.query.data
 return <EventReadScopeProvider value={authority.boundary}><QualificationReader key={authority.generation} eventID={eventID} runID={runID} eventName={event.name} timezone={event.configuration.timezone}/></EventReadScopeProvider>
}
function QualificationReader({eventID,runID,eventName,timezone}:{eventID:string;runID:string;eventName:string;timezone:string}){
 const identity=useIdentity(),boundary=useEventReadBoundary(eventID)
 const [continuation,setContinuation]=useState<{requestID:string;action:string;version:number;generation:number;teamID:string;cardVersion:number;freezeBinding:string}>()

 const load=useCallback(async(signal:AbortSignal)=>{const value=unwrap(await api.GET('/api/v1/events/{event_id}/mentor-qualification/{run_id}',{signal,cache:'no-store',params:{path:{event_id:eventID,run_id:runID}}}));if(value.round.run_id!==runID)throw new Error('The current native panel did not match this requested run.');return value},[eventID,runID])
 const signature=useCallback((page:PanelPage)=>JSON.stringify([page.round.panel,page.capabilities,page.tie_actions,page.current,!!page.round.reason,!!page.round.signoff?.reason,page.round.eligibility.map(value=>[value.team_id,value.binding,value.actor_id,value.evidence.map(reference=>[reference.kind,reference.id,reference.version])]),page.round.votes.map(value=>[value.tie_id,value.seat,value.team_id,value.actor_id,value.evidence.map(reference=>[reference.kind,reference.id,reference.version])]),page.round.decisions.map(value=>[value.tie_id,value.seat,value.team_id,value.actor_id,value.evidence.map(reference=>[reference.kind,reference.id,reference.version])]),page.teams.map(t=>[t.team_id,t.eligible,t.outcome,t.freeze_binding,t.blockers,t.source?.binding,t.can_save_card,t.can_sign_card,t.can_decide_eligibility]),page.round.cards.map(c=>[c.team_id,c.version,c.source.binding,c.scores.map(s=>s.evidence.map(e=>[e.kind,e.id,e.version]))])]),[])
 const read=useTodayRead(`${identity.person.id}:${identity.draft_scope}:${eventID}:${runID}:qualification`,load,boundary?.scope,signature)
 if(read.denied)return <section><p>Current panel access is unavailable. Private selections, evidence and earlier review context have been cleared.</p><ActionLink icon="back" href={`/events/${eventID}/review`}>Back to support and review coverage</ActionLink><Button icon="refresh" onClick={()=>{setContinuation(undefined);void read.refresh()}}>Check current panel access</Button></section>
 if(!read.data)return read.error?<Failure error={read.error} retry={()=>void read.refresh()}/>:<Loading/>

 const next=continuation?.generation===read.generation&&read.data.round.version===continuation.version+1&&read.data.teams.some(team=>team.team_id===continuation.teamID&&team.freeze_binding===continuation.freezeBinding)&&read.data.round.cards.some(card=>card.team_id===continuation.teamID&&card.version===continuation.cardVersion)?continuation:undefined
 const acknowledged=(intent:QualificationIntent,body:QualificationCommand)=>{
  const teamID=body.card?.team_id??body.team_id
  if(!teamID||!body.freeze_binding||!['save_card','sign_card'].includes(body.action))return
  // Carry only exact acknowledgement identity and navigation coordinates across
  // the disposed private task. Every score, reason and action comes from the
  // separately authorised current read, never the retained command body.
  setContinuation({requestID:intent.requestID,action:intent.action,version:intent.version,generation:read.generation+1,teamID,cardVersion:(body.card?.version??body.card_version??0)+1,freezeBinding:body.freeze_binding})
 }
 return <QualificationWorkspace initialSelected={next?.teamID} acknowledgement={next} onAcknowledged={acknowledged} key={read.generation} eventID={eventID} eventName={eventName} timezone={timezone} page={read.data} parent={read.authority} paused={read.pending||!!read.error||!!boundary?.paused} refresh={()=>read.refresh()} failure={read.error}/>
}
function QualificationWorkspace({eventID,eventName,timezone,page,parent,paused,refresh,failure,initialSelected,acknowledgement,onAcknowledged}:{eventID:string;eventName:string;timezone:string;page:PanelPage;parent:PrivateRequestScope;paused:boolean;refresh:()=>Promise<unknown>;failure:unknown;initialSelected?:string;acknowledgement?:{requestID:string;action:string;version:number};onAcknowledged:(intent:QualificationIntent,body:QualificationCommand)=>void}){
 const [selected,select]=useState<string|undefined>(initialSelected)
 const root=useRef<HTMLElement>(null)
 useLayoutEffect(()=>{if(!acknowledgement)return;const frame=requestAnimationFrame(()=>(root.current?.querySelector<HTMLElement>('[data-qualification-next]')??root.current?.querySelector<HTMLElement>('h3[tabindex="-1"]'))?.focus());return()=>cancelAnimationFrame(frame)},[acknowledgement])
 const risk=useRef<()=>boolean>(()=>true)
 const change=(id?:string)=>{if(risk.current())select(id)}
 const team=page.teams.find(value=>value.team_id===selected),card=page.round.cards.find(value=>value.team_id===selected),teamName=(id:string)=>`Team ${id}`,categoryName=(id:string)=>page.criteria.categories.find(value=>value.id===id)?.label??`Category ${id}`
 return <section ref={root} data-private-qualification>{acknowledgement&&<p role="status">Original {acknowledgement.action.replaceAll('_',' ')} acknowledged for request {acknowledgement.requestID}, expected panel version {acknowledgement.version}. The displayed card is separately read current state; no announcement was released.</p>}<ActionLink icon="back" variant="ghost" href={`/events/${eventID}/review`}>Back to support and review coverage</ActionLink>{!!failure&&<Failure error={failure} retry={refresh}/>}<DetailWorkspace collection={<QualificationOverview page={page} eventName={eventName} teamName={teamName} disabled={paused} inspect={id=>change(id)} actions={<Button icon="refresh" variant="outline" disabled={paused} onClick={refresh}>Read current panel source</Button>}/>} detail={team&&<Inspector focusOnOpen={!acknowledgement} identity={team.team_id} title={teamName(team.team_id)} scope={{event:eventName,entity:'Current native qualification evidence'}} back={{href:`/events/${eventID}/panel/${page.round.run_id}`,label:'Back to panel',onNavigate:()=>change(undefined)}}><QualificationEvidenceDestination eventID={eventID} teamID={team.team_id}/><ActionLink icon="book" href={`/events/${eventID}/review/${team.team_id}`}>Inspect required reviews and evidence gaps</ActionLink><QualificationCardComparison card={card} page={page} timezone={timezone} source={(card?.source??team.source)?<QualificationSourceEvidence eventID={eventID} teamID={team.team_id} sourceRef={(card?.source??team.source)!} parent={parent}/>:<p>No current canonical source is disclosed here. Open the permitted team review to inspect required evidence and blockers.</p>}/></Inspector>}/><QualificationEditor onAcknowledged={onAcknowledged} key={selected??'panel'} eventID={eventID} page={page} teamID={selected} parent={parent} paused={paused} refresh={refresh} registerDeparture={fn=>{risk.current=fn}}/>{page.preview&&<QualificationSelection proposal={page.preview} teamName={teamName} categoryName={categoryName}/>}<QualificationHistory eventID={eventID} runID={page.round.run_id} parent={parent} paused={paused}/><QualificationPreparationPreview key={`${page.round.run_id}:${page.round.signoff?.binding??'unsigned'}`} page={page} teamName={teamName} categoryName={categoryName}/></section>
}
function QualificationSourceEvidence({eventID,teamID,sourceRef,parent}:{eventID:string;teamID:string;sourceRef:QualificationCard['source'];parent:PrivateRequestScope}){
 const load=useCallback(async(signal:AbortSignal)=>{
  const huddle=await readHuddle(eventID,teamID,sourceRef.huddle_id,signal)
  const originals=unwrap(await api.GET('/api/v1/events/{event_id}/teams/{team_id}/scorecards/panel',{signal,cache:'no-store',params:{path:{event_id:eventID,team_id:teamID},query:{context_id:sourceRef.huddle_id}}}))
  return {huddle,originals}
 },[eventID,teamID,sourceRef.huddle_id])
 const read=useTodayRead(`${eventID}:${teamID}:${sourceRef.huddle_id}:panel-source`,load,parent)
 if(read.denied)return <p>This source is not disclosed under current source-specific authority. A panel selection does not grant private huddle access.</p>
 if(!read.data)return read.error?<Failure error={read.error} retry={()=>void read.refresh()}/>:<Loading/>
 const {huddle:source,originals}=read.data
 if(source.version!==sourceRef.huddle_version||source.signoff_id!==sourceRef.signoff_id||source.team_id!==teamID||source.submission_revision!==sourceRef.submission_revision||source.checkpoint.rubric.id!==sourceRef.rubric.id||source.checkpoint.rubric.version!==sourceRef.rubric.version||source.checkpoint.rubric.digest!==sourceRef.rubric.digest||!source.signoff_context||source.signoff_context.cards.length!==originals.length||originals.some(card=>card.team_id!==teamID||card.run_id!==source.signoff_context!.run_id||card.purpose!=='mentor_final'||card.state!=='submitted'||card.submission_revision!==sourceRef.submission_revision||card.rubric.digest!==sourceRef.rubric.digest||!source.signoff_context!.cards.some(ref=>ref.id===card.reference.id&&ref.version===card.reference.version)))return <p>The native huddle changed after this card source was selected. Read the current panel before another decision.</p>
 return <section><section aria-label="Independent original final evidence"><h4>Independent original final evidence</h4>{originals.map(card=><section key={card.reference.id}><h5>Reviewer {card.author_id} · Original card version {card.reference.version}</h5>{card.scores.map(score=><section key={score.category_id}><h6>{card.criteria.categories.find(category=>category.id===score.category_id)?.label??score.category_id}</h6><p>{score.pending?'Pending/unobserved; no numeric score':`Original score ${score.value}`}</p><QualificationEvidenceList items={score.evidence}/></section>)}</section>)}</section><p>Signed huddle version {source.version} · Required second review {source.second_review.completed_at?'completed':'not recorded'}</p>{source.gaps.map(gap=><section key={gap.id}><h4>{gap.resolved_at?'Resolved evidence gap':'Unresolved evidence gap'}</h4><p>{gap.reason}</p>{gap.resolution&&<p>Resolution: {gap.resolution}</p>}<QualificationEvidenceList items={gap.evidence}/></section>)}{source.proposals.map(proposal=><section key={proposal.id}><h4>Recorded score change · {proposal.state}</h4><p>Original {proposal.original.pending?'pending':proposal.original.value??'unscored'} → Proposed {proposal.proposed_value}</p><p>Reason: {proposal.reason}</p><p>Decision: {proposal.decision_reason||'No recorded decision'}</p><QualificationEvidenceList items={proposal.evidence}/></section>)}</section>
}

import {useCallback,useState} from 'react'
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
import type {QualificationPage as PanelPage,QualificationCard} from './qualification-model'

export function QualificationPanelPage(){
 const {eventId,runId}=useParams({strict:false}),identity=useIdentity()
 return <EventBrand eventID={eventId!}><QualificationEvent key={`${identity.person.id}:${identity.draft_scope}:${eventId}:${runId}`} eventID={eventId!} runID={runId!}/></EventBrand>
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
 const load=useCallback(async(signal:AbortSignal)=>unwrap(await api.GET('/api/v1/events/{event_id}/mentor-qualification/{run_id}',{signal,cache:'no-store',params:{path:{event_id:eventID,run_id:runID}}})),[eventID,runID])
 const signature=useCallback((page:PanelPage)=>JSON.stringify([page.round.panel,page.current,page.teams.map(t=>[t.team_id,t.eligible,t.outcome,t.freeze_binding,t.blockers]),page.round.cards.map(c=>[c.team_id,c.version,c.source.binding,c.scores.map(s=>s.evidence.map(e=>[e.kind,e.id,e.version]))])]),[])
 const read=useTodayRead(`${identity.person.id}:${identity.draft_scope}:${eventID}:${runID}:qualification`,load,boundary?.scope,signature)
 if(read.denied)return <section><p>Current panel access is unavailable. Private selections, evidence and earlier review context have been cleared.</p><ActionLink icon="back" href={`/events/${eventID}/review`}>Back to support and review coverage</ActionLink><Button icon="refresh" onClick={()=>void read.refresh()}>Check current panel access</Button></section>
 if(!read.data)return read.error?<Failure error={read.error} retry={()=>void read.refresh()}/>:<Loading/>
 return <QualificationWorkspace key={read.generation} eventID={eventID} eventName={eventName} timezone={timezone} page={read.data} parent={read.authority} paused={read.pending||!!read.error||!!boundary?.paused} refresh={()=>void read.refresh()} failure={read.error}/>
}
function QualificationWorkspace({eventID,eventName,timezone,page,parent,paused,refresh,failure}:{eventID:string;eventName:string;timezone:string;page:PanelPage;parent:PrivateRequestScope;paused:boolean;refresh:()=>void;failure:unknown}){
 const [selected,select]=useState<string>()
 const team=page.teams.find(value=>value.team_id===selected),card=page.round.cards.find(value=>value.team_id===selected),teamName=(id:string)=>`Team ${id}`,categoryName=(id:string)=>page.criteria.categories.find(value=>value.id===id)?.label??`Category ${id}`
 return <section data-private-qualification><ActionLink icon="back" variant="ghost" href={`/events/${eventID}/review`}>Back to support and review coverage</ActionLink>{!!failure&&<Failure error={failure} retry={refresh}/>}<DetailWorkspace collection={<QualificationOverview page={page} eventName={eventName} teamName={teamName} disabled={paused} inspect={select} actions={<Button icon="refresh" variant="outline" disabled={paused} onClick={refresh}>Read current panel source</Button>}/>} detail={team&&<Inspector identity={team.team_id} title={teamName(team.team_id)} scope={{event:eventName,entity:'Current native qualification evidence'}} back={{href:`/events/${eventID}/panel/${page.round.run_id}`,label:'Back to panel',onNavigate:()=>select(undefined)}}><QualificationEvidenceDestination eventID={eventID} teamID={team.team_id}/><ActionLink icon="book" href={`/events/${eventID}/review/${team.team_id}`}>Inspect required reviews and evidence gaps</ActionLink><QualificationCardComparison card={card} page={page} timezone={timezone} source={card?<QualificationSourceEvidence eventID={eventID} card={card} parent={parent}/>:<p>No current canonical source is disclosed here. Open the permitted team review to inspect required evidence and blockers.</p>}/></Inspector>}/>{page.preview&&<QualificationSelection proposal={page.preview} teamName={teamName} categoryName={categoryName}/>}<QualificationPreparationPreview key={`${page.round.run_id}:${page.round.signoff?.binding??'unsigned'}`} page={page} teamName={teamName} categoryName={categoryName}/></section>
}
function QualificationSourceEvidence({eventID,card,parent}:{eventID:string;card:QualificationCard;parent:PrivateRequestScope}){
 const load=useCallback(async(signal:AbortSignal)=>readHuddle(eventID,card.team_id,card.source.huddle_id,signal),[eventID,card.team_id,card.source.huddle_id])
 const read=useTodayRead(`${eventID}:${card.team_id}:${card.source.huddle_id}:panel-source`,load,parent)
 if(read.denied)return <p>This source is not disclosed under current source-specific authority. A panel selection does not grant private huddle access.</p>
 if(!read.data)return read.error?<Failure error={read.error} retry={()=>void read.refresh()}/>:<Loading/>
 const source=read.data
 if(source.version!==card.source.huddle_version||source.signoff_id!==card.source.signoff_id)return <p>The native huddle changed after this card source was selected. Read the current panel before another decision.</p>
 return <section><p>Signed huddle version {source.version} · Required second review {source.second_review.completed_at?'completed':'not recorded'}</p>{source.gaps.map(gap=><section key={gap.id}><h4>{gap.resolved_at?'Resolved evidence gap':'Unresolved evidence gap'}</h4><p>{gap.reason}</p>{gap.resolution&&<p>Resolution: {gap.resolution}</p>}<QualificationEvidenceList items={gap.evidence}/></section>)}{source.proposals.map(proposal=><section key={proposal.id}><h4>Recorded score change · {proposal.state}</h4><p>Original {proposal.original.pending?'pending':proposal.original.value??'unscored'} → Proposed {proposal.proposed_value}</p><p>Reason: {proposal.reason}</p><p>Decision: {proposal.decision_reason||'No recorded decision'}</p><QualificationEvidenceList items={proposal.evidence}/></section>)}</section>
}

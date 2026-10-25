import { JudgeCommandHistory } from './judge-history'
import { JudgeReopen } from './judge-reopen'
import { useCallback, useRef, useState } from 'react'
import { Link, useParams } from '@tanstack/react-router'
import { api, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { EventReadScopeProvider, useEventAuthority } from './event-authority'
import { useTodayRead } from './today-read'
import { usePrivateRequestScope, type PrivateRequestScope } from './private-request-scope'
import { CollectionHeading, CollectionRecords, DetailWorkspace, Inspector } from './components/patterns'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { JudgeEditor } from './judge-editor'
import type { components } from './api/schema.gen'
import './judge.css'

type Discovery={rounds:components['schemas']['JudgeRounds'];today:components['schemas']['TodayPage']}
const discoveryAuthority=(v:Discovery)=>JSON.stringify([v.today.roles,v.rounds.items.map(r=>[r.run_id,r.assignments.map(a=>a.team_id)])])
export function JudgeWorkspacePage(){const {eventId}=useParams({strict:false}) as {eventId:string};return <section className="fleet-judge-workspace"><JudgeCommandHistory eventID={eventId}/><JudgeEvent eventID={eventId}/></section>}
export function JudgePracticePage(){const {eventId}=useParams({strict:false}) as {eventId:string};return <section className="fleet-judge-workspace"><JudgeCommandHistory eventID={eventId}/><JudgeEvent eventID={eventId} practice/></section>}
function JudgeEvent({eventID,practice=false}:{eventID:string;practice?:boolean}){
 const identity=useIdentity(),event=useEventAuthority(eventID)
 if(event.denial||event.query.error&&!event.query.data)return <Failure error={event.denial??event.query.error} retry={()=>void event.refresh()}/>
 if(!event.verified||!event.query.data)return <Loading/>
 return <EventReadScopeProvider value={event.boundary}><div><Link to="/events/$eventId" params={{eventId:eventID}}>Back to event</Link>{!!event.query.error&&<Failure error={event.query.error} retry={()=>void event.refresh()}/>}<div hidden={!!event.query.error||event.checking} inert={event.boundary.paused}>{practice?<JudgePracticeOwner key={`${identity.person.id}:${identity.draft_scope}:${eventID}:${event.generation}`} event={event.query.data} parent={event.scope} paused={event.boundary.paused}/>:<JudgeDesk key={`${identity.person.id}:${identity.draft_scope}:${eventID}:${event.generation}`} event={event.query.data} parent={event.scope} paused={event.boundary.paused}/>}</div></div></EventReadScopeProvider>
}
function JudgeDesk({event,parent,paused}:{event:EventRecord;parent:PrivateRequestScope;paused:boolean}){
 const identity=useIdentity(),owner=`${identity.person.id}:${identity.draft_scope}:${event.id}:judge-discovery`
 const load=useCallback(async(signal:AbortSignal,assert:()=>void)=>{
  const today=unwrap(await api.GET('/api/v1/events/{event_id}/today',{signal,cache:'no-store',params:{path:{event_id:event.id}}}));assert()
  const rounds=unwrap(await api.GET('/api/v1/events/{event_id}/judging',{signal,cache:'no-store',params:{path:{event_id:event.id}}}));assert()
  return {today,rounds}
 },[event.id])
 const read=useTodayRead(owner,load,parent,discoveryAuthority),[selected,select]=useState<{run:string;team:string}>(),risk=useRef<()=>boolean>(()=>true),opener=useRef<HTMLElement|null>(null)
 const rows=read.data?.rounds.items.flatMap(round=>round.assignments.filter(a=>a.judge_ids.includes(identity.person.id)).map((assignment,index)=>({run:round.run_id,team:assignment.team_id,required:round.required_judges,label:`Assigned build ${index+1}`})))??[]
 const change=(next:typeof selected,button?:HTMLElement)=>{if(!risk.current())return;opener.current=button??opener.current;select(next);if(!next)requestAnimationFrame(()=>opener.current?.focus())}
 const current=selected&&rows.find(row=>row.run===selected.run&&row.team===selected.team)
 const program=read.data?.today.program
 return <><CollectionHeading title="Judge review desk" scope={{event:event.name,entity:'Your current independent assignments'}} description="Final scoring opens after the shared submission cutoff, against an eligible frozen final and reconciled code evidence. Declared own-team/reporting conflicts require organiser remediation; a role alone never grants a build." secondary={[{label:'Read current assignments',icon:'refresh',disabled:read.pending,onSelect:()=>void read.refresh()}]}/><p><Link to="/events/$eventId/judging/practice" params={{eventId:event.id}}>Open isolated synthetic practice</Link></p>{program&&<section className="fleet-judge-section" aria-label="Published presentation order"><h2>Published room programme</h2><p>Current: {program.current?.title??'No published current cue'} · Next: {program.next?.title??'No published next cue'}. The published programme is a room observation; assignment order below is not a presentation slot.</p></section>}{read.error&&<Failure error={read.error} retry={()=>void read.refresh()}/>}<div hidden={read.denied} inert={!!read.error||read.pending||paused}>{!read.data?<Loading/>:<DetailWorkspace collection={<><h2>Assigned builds</h2><CollectionRecords label="Current judge assignments" rows={rows} rowKey={r=>`${r.run}:${r.team}`} rowLabel={r=>r.label} columns={[{id:'team',label:'Native team',essential:true,cell:r=><code>{r.team}</code>},{id:'judges',label:'Required independent judges',cell:r=>r.required}]} actions={r=><Button icon="next" variant="ghost" onClick={e=>change({run:r.run,team:r.team},e.currentTarget)}>{r.label}</Button>}/>{!rows.length&&<p>No current eligible assignments. Ask your organiser to confirm assignments, declared conflicts and finalist eligibility. Practice is separate.</p>}</>} detail={current?<Inspector identity={`${current.run}:${current.team}`} title="Review assigned build" scope={{event:event.name,entity:current.label}} back={{label:'All assigned builds',href:`/events/${event.id}/judging`,onNavigate:()=>change(undefined)}}><JudgeBuild key={`${current.run}:${current.team}:${read.generation}`} event={event} runID={current.run} teamID={current.team} parent={read.authority} paused={paused||read.pending||!!read.error} registerDeparture={fn=>{risk.current=fn}} next={()=>{const index=rows.indexOf(current);const following=rows[index+1];change(following?{run:following.run,team:following.team}:undefined)}}/></Inspector>:undefined}/>}</div>{event.can_edit&&read.data&&!read.denied&&<JudgeCoordination event={event} rounds={read.data.rounds} parent={read.authority} paused={paused||read.pending||!!read.error}/>}</>
}
const briefAuthority=(v:components['schemas']['JudgeBrief'])=>JSON.stringify([v.run_id,v.team_id,v.submission_revision,v.reference,v.repository,v.submission.evidence.map(e=>e.deliverable_id)])
function JudgeBuild({event,runID,teamID,parent,paused,registerDeparture,next}:{event:EventRecord;runID:string;teamID:string;parent:PrivateRequestScope;paused:boolean;registerDeparture:(fn:()=>boolean)=>void;next:()=>void}){
 const load=useCallback(async(signal:AbortSignal)=>unwrap(await api.GET('/api/v1/events/{event_id}/judging/{run_id}/teams/{team_id}',{signal,cache:'no-store',params:{path:{event_id:event.id,run_id:runID,team_id:teamID}}})),[event.id,runID,teamID])
 const read=useTodayRead(`${event.id}:${runID}:${teamID}:judge-brief`,load,parent,briefAuthority)
 return <>{read.error&&<><p>Scoring is paused until current assignment, declared conflicts, cutoff and exact frozen evidence are confirmed. Unsent input stays only in this mounted task during a temporary failure; definite denial clears it.</p><Failure error={read.error} retry={()=>void read.refresh()}/></>}{!read.data?<>{!read.error&&<Loading/>}</>:<div hidden={!!read.error} inert={read.pending||!!read.error||paused}><JudgeEditor key={read.generation} eventID={event.id} brief={read.data} parent={read.authority} paused={paused||read.pending||!!read.error} refresh={()=>read.refresh()} registerDeparture={registerDeparture} next={next}/></div>}</>
}
const practiceAuthority=(v:components['schemas']['JudgePractice'])=>JSON.stringify([v.fictional_entry_id,v.rubric])
function JudgePracticeOwner({event,parent,paused}:{event:EventRecord;parent:PrivateRequestScope;paused:boolean}){
 const scope=usePrivateRequestScope(`${event.id}:judge-practice`,parent)
 const load=useCallback(async(signal:AbortSignal)=>unwrap(await api.GET('/api/v1/events/{event_id}/judge-practice',{signal,cache:'no-store',params:{path:{event_id:event.id}}})),[event.id])
 const read=useTodayRead(`${event.id}:judge-practice`,load,scope,practiceAuthority)
 return <><CollectionHeading title="Synthetic judge practice" scope={{event:event.name,entity:'Fictional entry · never a competition score'}} description="Available only before live start under current native judge authority. Practice uses the configured published rubric and has no real team or submission reference."/><p><Link to="/events/$eventId/judging" params={{eventId:event.id}}>Return to assigned competition builds</Link></p>{read.error&&<Failure error={read.error} retry={()=>void read.refresh()}/>}<div hidden={!!read.error} inert={read.pending||!!read.error||paused}>{read.data?<JudgeEditor key={read.generation} eventID={event.id} practice={read.data} parent={read.authority} paused={paused||read.pending||!!read.error} refresh={()=>read.refresh()}/>:!read.error&&<Loading/>}</div></>
}

function JudgeCoordination({event,rounds,parent,paused}:{event:EventRecord;rounds:Discovery['rounds'];parent:PrivateRequestScope;paused:boolean}){
 const [selected,select]=useState<{run:string;team:string}>(),risk=useRef({dirty:false,pending:false,unknown:false})
 const allowed=()=>!risk.current.pending&&(!(risk.current.dirty||risk.current.unknown)||window.confirm(risk.current.unknown?'Leave the earlier reopen outcome unresolved? Safe original metadata remains available.':'Discard this unsent reopen reason?'))
 const rows=rounds.items.flatMap(r=>r.assignments.map(a=>({run:r.run_id,team:a.team_id})))
 const current=selected&&rows.find(r=>r.run===selected.run&&r.team===selected.team)
 return <section className="fleet-judge-section" aria-label="Organiser judging coordination"><h2>Organiser lock coordination</h2><p>Current card state/version only. This does not grant access to independent judge scores.</p><CollectionRecords label="Assigned team lock coordination" rows={rows} rowKey={r=>`${r.run}:${r.team}`} rowLabel={r=>`Team ${r.team}`} columns={[]} actions={r=><Button icon="shield" variant="ghost" disabled={paused||risk.current.pending} onClick={()=>{if(allowed())select(r)}}>Open lock coordination for {r.team}</Button>}/>{current&&<JudgeReopen key={`${current.run}:${current.team}`} eventID={event.id} runID={current.run} teamID={current.team} parent={parent} paused={paused} onRisk={value=>{risk.current=value}}/>}</section>
}

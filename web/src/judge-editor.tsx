import { EvidenceList } from './deliverable-evidence'
import { sendJudgeCommand, originalJudgeReceipt } from './judge-api'
import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { useBlocker } from '@tanstack/react-router'
import { APIError, api, unwrap } from './api/client'
import { useIdentity } from './session'
import { usePrivateRequestScope, type PrivateRequestScope } from './private-request-scope'
import { useCommunicationCommands } from './communication-tests'
import { usePlanningDepartureBarrier } from './planning-operations'
import { Button } from './components/ui/button'
import { focusFirstInvalid } from './components/ui/fields'
import { Failure } from './components/states'
import { JudgeBriefing, JudgeReview, JudgeRubric, JudgeScoreFields, JudgeScoreSummary, JudgeUnknown } from './judge-view'
import { judgeFingerprint, matchJudgeReceipt, scoreInputs, validateJudgeScores, type JudgeBrief, type JudgeCommand, type JudgeIntent, type JudgePractice, type JudgePracticeCommand, type JudgeReceipt, type ScoreInput } from './judge-model'

type Props={eventID:string;parent:PrivateRequestScope;paused:boolean;refresh:()=>Promise<unknown>;registerDeparture?:(fn:()=>boolean)=>void;next?:()=>void}&({brief:JudgeBrief;practice?:never}|{practice:JudgePractice;brief?:never})
type Reviewed={intent:JudgeIntent;body:JudgeCommand|JudgePracticeCommand;inputs:ScoreInput[];source:string}
export const judgeNamespace=(actor:string,draft:string,eventID:string)=>`${actor}:${draft}:judging:${eventID}`
function sourceOf(v:JudgeBrief|JudgePractice){return 'run_id'in v?JSON.stringify([v.run_id,v.team_id,v.submission_revision,v.reference,v.repository,v.submission.evidence.map(e=>e.deliverable_id)]):JSON.stringify([v.fictional_entry_id,v.rubric])}
export function JudgeEditor(props:Props){
 const {eventID,paused,refresh,registerDeparture,next}=props,identity=useIdentity(),value=props.brief??props.practice,card=props.brief?.own_card??props.practice?.card,version=card?.version??0
 const criteria=value.criteria,reference=props.brief?.reference??props.practice!.rubric,source=sourceOf(value)
 const owner=`${judgeNamespace(identity.person.id,identity.draft_scope,eventID)}:${props.brief?.team_id??'practice'}`
 const commands=useCommunicationCommands(owner),scope=usePrivateRequestScope(owner,props.parent)
 const [inputs,setInputs]=useState(()=>scoreInputs(criteria,card?.scores)),[baseline,setBaseline]=useState(()=>JSON.stringify(scoreInputs(criteria,card?.scores))),[baseVersion,setBaseVersion]=useState(version)
 const [errors,setErrors]=useState<Record<string,string>>({}),[review,setReview]=useState<Reviewed>(),[newBase,setNewBase]=useState(false),[preparing,setPreparing]=useState(false),[failure,setFailure]=useState<unknown>(),[receipt,setReceipt]=useState<JudgeReceipt>(),[captured,setCaptured]=useState<Reviewed>()
 const sending=useRef(false),opener=useRef<HTMLElement|null>(null),form=useRef<HTMLFormElement>(null)
 const dirty=JSON.stringify(inputs)!==baseline,stale=version!==baseVersion,locked=card?.state==='submitted'&&version===baseVersion,pending=preparing||commands.pending
 const blocked=paused||pending||commands.unconfirmed||stale||card?.state==='source_changed'
 const depart=()=>{if(!scope.capture().current())return true;if(pending)return false;return !(dirty||commands.unconfirmed)||window.confirm(commands.unconfirmed?'Leave this original outcome unresolved? Only safe request metadata will remain; it will not be replayed.':'Discard your private unsent judge scores?')}
 useBlocker({shouldBlockFn:()=>!depart(),enableBeforeUnload:dirty||pending||commands.unconfirmed})
 usePlanningDepartureBarrier(pending)
 useLayoutEffect(()=>{registerDeparture?.(depart);return()=>registerDeparture?.(()=>true)})
 useEffect(()=>{if(Object.keys(errors).length&&form.current)focusFirstInvalid(form.current)},[errors])
 function correct(){if(pending)return;setReview(undefined);setNewBase(false);requestAnimationFrame(()=>opener.current?.focus())}
 async function prepare(action:Exclude<JudgeIntent['action'],'reopen'>,button:HTMLElement){
  if(blocked||sending.current)return
  const checked=validateJudgeScores(criteria,inputs,action==='submit');setErrors(checked.errors)
  if(action!=='reset'&&Object.keys(checked.errors).length)return
  opener.current=button;setFailure(undefined);setPreparing(true);sending.current=true
  const ticket=scope.capture()
  try{
   const requestID=crypto.randomUUID()
   const body:JudgeCommand|JudgePracticeCommand=props.brief?{request_id:requestID,action,submission_revision:props.brief.submission_revision,repository_decision_id:props.brief.repository.decision_id,scores:checked.scores}:{request_id:requestID,action,scores:action==='reset'?[]:checked.scores}
   const fingerprint=await judgeFingerprint(body,baseVersion,props.brief?.run_id,props.brief?.team_id);ticket.assert()
   const reviewed={intent:{requestID,action,revision:baseVersion,runID:props.brief?.run_id,teamID:props.brief?.team_id??props.practice?.fictional_entry_id,fingerprint},body,inputs:structuredClone(inputs),source}
   setCaptured(reviewed);setReview(reviewed)
  }catch(error){if(ticket.current())setFailure(error)}finally{sending.current=false;if(ticket.current())setPreparing(false)}
 }
 async function currentSource(signal:AbortSignal){return props.brief?unwrap(await api.GET('/api/v1/events/{event_id}/judging/{run_id}/teams/{team_id}',{signal,cache:'no-store',params:{path:{event_id:eventID,run_id:props.brief.run_id,team_id:props.brief.team_id}}})):unwrap(await api.GET('/api/v1/events/{event_id}/judge-practice',{signal,cache:'no-store',params:{path:{event_id:eventID}}}))}
 async function confirm(retry=false){
  const target=retry?captured:review
  if(!target||sending.current||paused||commands.pending||(!retry&&commands.unconfirmed))return
  sending.current=true;setPreparing(true);setFailure(undefined)
  const ticket=scope.capture();let attemptID:string|undefined
  try{
   ticket.assert()
   if(!retry){const current=await currentSource(ticket.signal);ticket.assert();const currentVersion=('own_card'in current?current.own_card:current.card)?.version??0
    if(sourceOf(current)!==target.source||currentVersion!==target.intent.revision)throw new Error('The current final, code receipt, rubric or saved card changed. Nothing was sent. Review the current starting point.')}
   ticket.assert()
   const existing=retry?commands.attempts.find(a=>a.command.requestID===target.intent.requestID)?.id:undefined
   attemptID=commands.begin({operation:props.practice?'Synthetic practice scores':'Independent judge scores',revision:target.intent.revision,command:{action:target.intent.action,requestID:target.intent.requestID,revision:target.intent.revision,resourceID:target.intent.runID,subjectID:target.intent.teamID,intentFingerprint:target.intent.fingerprint}},existing)
   const result=await sendJudgeCommand(eventID,target.intent.revision,identity.csrf_token,target.body,props.brief?.run_id,props.brief?.team_id)
   if(!matchJudgeReceipt(result,target.intent,identity.person.id))throw new Error('The response did not match the captured original request. Its outcome remains unknown.')
   commands.complete(attemptID) // exact safe metadata may settle after private teardown
   if(ticket.current()){setReceipt(result);setReview(undefined);setBaseVersion(result.revision);setBaseline(JSON.stringify(target.inputs));if(target.intent.action==='reset'){const empty=scoreInputs(criteria);setInputs(empty);setBaseline(JSON.stringify(empty))}await refresh()}
  }catch(error){
   if(attemptID){if(error instanceof APIError&&error.status>=400&&error.status<500)commands.complete(attemptID);else commands.uncertain(attemptID)}
   if(ticket.current()){setFailure(error);if(error instanceof APIError&&[401,403,404].includes(error.status)){scope.invalidate();await refresh()}}
  }finally{sending.current=false;if(ticket.current())setPreparing(false)}
 }
 async function recover(){
  const attempt=commands.attempts.find(a=>a.state==='unknown');if(!attempt?.command.requestID||!attempt.command.intentFingerprint)return
  const ticket=scope.capture();setFailure(undefined)
  try{const result=await originalJudgeReceipt(eventID,attempt.command.requestID,ticket.signal);ticket.assert()
   const intent:JudgeIntent={requestID:attempt.command.requestID,action:attempt.command.action as JudgeIntent['action'],revision:attempt.revision,runID:attempt.command.resourceID,teamID:attempt.command.subjectID,fingerprint:attempt.command.intentFingerprint}
   if(!result.receipt||!matchJudgeReceipt(result.receipt,intent,identity.person.id))throw new Error('No matching original acknowledgement is available. The original outcome remains unknown; current facts do not prove failure.')
   commands.complete(attempt.id);setReceipt(result.receipt);setReview(undefined);await refresh()
  }catch(error){if(ticket.current()){setFailure(error);if(error instanceof APIError&&[401,403,404].includes(error.status)){scope.invalidate();await refresh()}}}
 }
 return <section aria-label={props.practice?'Synthetic practice scoring':'Independent final scoring'}><h2>{props.brief?.team_name??props.practice?.name}</h2>{props.brief&&<JudgeBriefing brief={props.brief} artifacts={<EvidenceList items={props.brief.submission.evidence} eventID={eventID}/>}/>}<JudgeRubric criteria={criteria} version={reference.version}/>{card?.state==='source_changed'&&<p role="alert">The reviewed final/code source changed. Old scores are not current. Ask the organiser for a reasoned reopen and review the new frozen evidence.</p>}{locked&&<p role="status">{props.practice?'Synthetic practice submitted':'Competition scores submitted and locked'}. {props.practice?'Reset practice to rehearse again.':'Changes require an organiser’s recorded reason and invalidate affected unannounced result approvals.'}</p>}{stale&&<p role="alert">The current saved card has changed. Your unsent values remain below; review a fresh starting point before any new command.</p>}{!!failure&&<Failure error={failure}/>} {receipt&&<p role="status">{props.practice?'Practice':'Competition'} original acknowledgement: {receipt.state} · Revision {receipt.revision} · Request {receipt.request_id}. This confirms that exact request, not a later current version.</p>}
 {commands.attempts.map(a=><JudgeUnknown key={a.id} pending={a.state==='pending'} recover={()=>void recover()}/>)}
 <form ref={form} noValidate hidden={!!review||newBase} onSubmit={e=>{e.preventDefault();const button=(e.nativeEvent as SubmitEvent).submitter;if(button instanceof HTMLElement)void prepare('submit',button)}}><JudgeScoreFields criteria={criteria} inputs={inputs} change={setInputs} errors={errors} evidence={props.brief?.submission.evidence.map(e=>({id:e.deliverable_id,label:e.label}))} disabled={blocked||locked}/><div className="fleet-pattern-actions">{!locked&&<><Button icon="save" variant="outline" disabled={blocked} onClick={e=>void prepare('save',e.currentTarget)}>Review save draft</Button><Button icon="check" type="submit" disabled={blocked}>Review {props.practice?'practice':'competition'} submission</Button></>}{props.practice&&<Button icon="refresh" variant="outline" disabled={blocked} onClick={e=>void prepare('reset',e.currentTarget)}>Review practice reset</Button>}{next&&<Button icon="next" variant="ghost" disabled={pending} onClick={next}>Next assigned build</Button>}<Button icon="refresh" variant="outline" disabled={pending||paused} onClick={()=>void refresh()}>Read current source</Button></div></form>
 {(stale||commands.unconfirmed)&&<Button icon="refresh" variant="outline" disabled={pending||paused} onClick={e=>{opener.current=e.currentTarget;setNewBase(true)}}>Review current saved starting point</Button>}
 {commands.unconfirmed&&captured&&<Button icon="refresh" variant="outline" disabled={pending||paused} onClick={()=>void confirm(true)}>Retry captured original request</Button>}
 {review&&<JudgeReview title={review.intent.action==='reset'?'Review synthetic practice reset':review.intent.action==='save'?'Review independent draft':'Review locked submission'} pending={pending} disabled={paused||stale} confirm={()=>void confirm()} correct={correct} opener={opener}><p>{props.practice?'Fictional synthetic entry; never counted towards competition.':`Frozen final ${props.brief?.submission_revision}; exact reviewed code receipt and rubric ${reference.version}.`} Saved card revision {review.intent.revision}.</p><JudgeScoreSummary criteria={criteria} inputs={review.inputs}/><p>{review.intent.action==='submit'?'Submitting locks these scores. A saved draft or unknown request is not submitted competition scoring.':'Saving keeps this private draft editable; it does not satisfy required-judge completion.'}</p></JudgeReview>}
 {newBase&&<JudgeReview title="Review current saved starting point" pending={false} disabled={paused||pending} confirm={()=>{const values=scoreInputs(criteria,card?.scores);setInputs(values);setBaseline(JSON.stringify(values));setBaseVersion(version);setErrors({});commands.selectNewBase();setNewBase(false);requestAnimationFrame(()=>opener.current?.focus())}} correct={correct} opener={opener}><p>This replaces your local unsent editor with current saved card revision {version}. An earlier unknown request remains unknown and its original acknowledgement can still be read. This is not proof that it failed or permission to replay it.</p><JudgeScoreSummary criteria={criteria} inputs={scoreInputs(criteria,card?.scores)}/></JudgeReview>}
 {props.brief?.deliberation&&<section className="fleet-judge-section"><h3>Opened deliberation · submitted cards only</h3>{props.brief.submitted_cards.map(c=><section key={c.author_id}><h4>Submitted judge {c.author_id}</h4><JudgeScoreSummary criteria={criteria} inputs={scoreInputs(criteria,c.scores)}/></section>)}</section>}
 </section>
}

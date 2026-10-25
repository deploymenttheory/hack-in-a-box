import type {Page} from '@playwright/test'
import type {components} from '../src/api/schema.gen'
import {freezeFingerprint} from '../src/submission-freeze-model'
import {mockWorkspace,firstID,fixtureEvent,person} from './fixtures'
export const buildTeam='55555555-5555-4555-8555-555555555555',sourceID='66666666-6666-4666-8666-666666666666',evidenceID='77777777-7777-4777-8777-777777777777'
type S=components['schemas']
export async function freezeFixture(page:Page){
 await mockWorkspace(page)
 const event=fixtureEvent();event.stage='live';event.published_at='2027-06-13T09:00:00Z'
 const submission:S['SubmissionPage']={entry_id:sourceID,team_id:buildTeam,event_revision:1,revision:3,outcome:'frozen',frozen_at:event.configuration.submission_cutoff,can_read_draft:false,can_draft:false,can_submit:false,can_review:false,live_start:event.configuration.live_start,cutoff:event.configuration.submission_cutoff,organisers:[],definitions:[],final:{revision:1,actor_id:person.id,submitted_at:'2027-06-20T15:00:00Z',late:false,reason:'',review_status:'pending',definitions:[],evidence:[{deliverable_id:evidenceID,kind:'url',label:'Synthetic live demo',file_id:null,url:'https://demo.example.test/current'}]}}
 const frozen:S['SubmissionFreezePage']={revision:2,native_revision:3,native_outcome:'frozen',current_submission_revision:1,binding_current:true,branch:'main',repository_id:123456,repository_url:'https://github.com/synthetic-org/synthetic-repo',cutoff:submission.cutoff,can_configure:false,can_reconcile:true,can_amend:false,status:'unresolved',current_reference_status:'managed_reference',source:{id:sourceID,event_id:firstID,team_id:buildTeam,entry_id:sourceID,submission_revision:1,native_outcome:'frozen',cutoff:submission.cutoff,recorded_at:'2027-06-20T16:03:00Z',repository_id:123456,repository_url:'https://github.com/synthetic-org/synthetic-repo',branch:'main',configuration_revision:1,repository_provenance:sourceID,status:'unresolved',managed_evidence_ids:[evidenceID],uncaptured_evidence_ids:[],audiences:['organiser','mentor','judge']},observations:[{id:evidenceID,source_id:sourceID,observed_at:'2027-06-20T16:03:00Z',sha:'a'.repeat(40),status:'current_ref',provenance:'github_app_current_ref'}],decisions:[],current:null}
 const state={event,submission,frozen,denied:false,hold:undefined as Promise<void>|undefined,lost:false,lostBefore:false,missing:false,mismatch:false,writes:[] as S['SubmissionFreezeCommand'][],receipts:new Map<string,S['SubmissionFreezeReceipt']>()}
 const deny=(r:import('@playwright/test').Route)=>r.fulfill({status:403,json:{error:{code:'forbidden',message:'Synthetic',request_id:'synthetic'}}})
 await page.route(`**/api/v1/events/${firstID}`,r=>r.fulfill({json:event}))
 await page.route(`**/api/v1/events/${firstID}/teams/${buildTeam}/submission`,r=>state.denied?deny(r):r.fulfill({json:submission}))
 await page.route(`**/api/v1/events/${firstID}/teams/${buildTeam}/submission-freeze**`,async r=>{
  const req=r.request(),path=new URL(req.url()).pathname
  if(state.denied)return deny(r)
  if(path.includes('/commands/')){const receipt=state.missing?null:state.receipts.get(path.split('/').at(-1)!)??null;return r.fulfill({json:{receipt:receipt&&state.mismatch?{...receipt,fingerprint:'0'.repeat(64)}:receipt}})}
  if(path.endsWith('/push-evidence'))return r.fulfill({json:{items:[],next_cursor:null}})
  if(req.method()==='GET')return r.fulfill({json:frozen})
  const body=req.postDataJSON() as S['SubmissionFreezeCommand'];state.writes.push(body);if(state.lostBefore)return r.abort('failed');if(state.hold)await state.hold
  const expected=Number(req.headers()['if-match'].replaceAll('"',''))
  const decision:S['SubmissionFreezeDecision']|null=body.action==='configure'?null:{id:crypto.randomUUID(),source_id:body.source_id!,revision:expected+1,native_revision:body.native_revision!,submission_revision:body.submission_revision!,actor_id:person.id,at:'2027-06-20T16:10:00Z',action:body.action,sha:body.sha!,reason:body.reason!,external_evidence:body.external_evidence??'',managed_evidence_ids:[evidenceID],uncaptured_evidence_ids:[],audiences:['judge'],observation_ids:body.observation_ids??[],push_ids:body.push_ids??[],prior_decision_id:body.prior_decision_id??null}
  const receipt={request_id:body.request_id,expected_revision:expected,revision:expected+1,action:body.action,branch:body.branch??'',fingerprint:await freezeFingerprint(body,expected),decision};state.receipts.set(body.request_id,receipt);frozen.revision++;if(body.action==='configure')frozen.branch=body.branch!;else{frozen.current=decision;frozen.decisions.push(decision!);frozen.status='completed';frozen.can_reconcile=false;frozen.can_amend=true}
  if(state.lost)return r.abort('failed');return r.fulfill({json:state.mismatch?{...receipt,fingerprint:'0'.repeat(64)}:receipt})
 })
 return state
}

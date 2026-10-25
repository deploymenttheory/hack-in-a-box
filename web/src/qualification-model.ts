import type {components} from './api/schema.gen'
type Schemas=components['schemas']
export type QualificationPage=Schemas['QualificationPage']
export type QualificationCommand=Schemas['QualificationCommand']
export type QualificationReceipt=Schemas['QualificationReceipt']
export type QualificationCard=Schemas['QualificationFinalCard']
export type QualificationTeam=Schemas['QualificationTeamStatus']
export type QualificationProposal=Schemas['QualificationProposal']
export type QualificationEvidence=Schemas['ScorecardEvidence']
export const qualificationWords=(value:string)=>value.replaceAll('_',' ')
export function qualificationOutcome(team:QualificationTeam){
 if(team.outcome==='withdrawn'||team.outcome==='disbanded'||team.outcome==='inactive')return 'Unused entry · excluded from scoring'
 if(team.outcome==='not_submitted')return 'Not submitted · no formal final to score'
 if(team.outcome!=='frozen')return 'Formal final is not frozen'
 return team.eligible?'Eligible competing entry':'Not currently eligible for selection; inspect native blockers'
}
export function currentPrivateSelection(page:QualificationPage){return page.current&&page.round.signoff&&page.round.proposal?page.round.proposal:undefined}

export type QualificationIntent={requestID:string;runID:string;action:string;revision:number;version:number;fingerprint:string}
const nilUUID='00000000-0000-0000-0000-000000000000'
const nativeUUID=(value:string|undefined)=>value?.toLowerCase()??nilUUID
const nativeEvidence=(e:QualificationEvidence)=>({kind:e.kind,id:e.id?nativeUUID(e.id):null,version:e.version,note:e.note})
// Mirror the actual native Command JSON representation, including explicit
// null/zero fields and Go's HTML-sensitive escaping. Reasons remain verbatim:
// native validation checks whitespace but does not trim the retained command.
export async function qualificationFingerprint(body:QualificationCommand,revision:number){
 const command={request_id:nativeUUID(body.request_id),freeze_binding:body.freeze_binding??'',repository_decision_id:body.repository_decision_id?nativeUUID(body.repository_decision_id):null,version:body.version,action:body.action,reason:body.reason,
  panel:body.panel?{mentor_ids:body.panel.mentor_ids?.map(nativeUUID)??null,organiser_id:nativeUUID(body.panel.organiser_id)}:null,
  card:body.card?{team_id:nativeUUID(body.card.team_id),version:body.card.version,scores:body.card.scores?.map(score=>({category_id:nativeUUID(score.category_id),pending:score.pending,value:score.value,evidence:score.evidence?.map(nativeEvidence)??null}))??null}:null,
  team_id:nativeUUID(body.team_id),card_version:body.card_version??0,decision:body.decision??'',evidence:body.evidence?.map(nativeEvidence)??null,tie_id:body.tie_id??'',seat:body.seat??0}
 const encoded=JSON.stringify({Command:command,ExpectedRevision:revision}).replace(/[<>&\u2028\u2029]/g,c=>`\\u${c.charCodeAt(0).toString(16).padStart(4,'0')}`)
 const digest=await crypto.subtle.digest('SHA-256',new TextEncoder().encode(encoded))
 return [...new Uint8Array(digest)].map(value=>value.toString(16).padStart(2,'0')).join('')
}
export function matchesQualificationReceipt(receipt:QualificationReceipt,intent:QualificationIntent){
 return receipt.request_id===intent.requestID&&receipt.run_id===intent.runID&&receipt.action===intent.action&&receipt.expected_revision===intent.revision&&receipt.expected_version===intent.version&&receipt.version===intent.version+1&&receipt.fingerprint===intent.fingerprint
}

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

import type { components } from './api/schema.gen'
export type JudgeBrief = components['schemas']['JudgeBrief']
export type JudgePractice = components['schemas']['JudgePractice']
export type JudgeScore = components['schemas']['JudgeScore']
export type JudgeReceipt = components['schemas']['JudgeReceipt']
export type JudgeCriteria = components['schemas']['PublicRubricDefinition']
export type JudgeCommand = components['schemas']['JudgeCardCommand']
export type JudgePracticeCommand = components['schemas']['JudgePracticeCommand']
export type ScoreInput = { categoryID: string; value: string; note: string; deliverableID: string }
export function scoreInputs(criteria: JudgeCriteria, scores: JudgeScore[] = []): ScoreInput[] {
 return criteria.categories.map(category => {const score=scores.find(s=>s.category_id===category.id);return {categoryID:category.id,value:score?.value===null||score?.value===undefined?'':String(score.value),note:score?.evidence[0]?.note??'',deliverableID:score?.evidence[0]?.deliverable_id??''}})
}
export function validateJudgeScores(criteria: JudgeCriteria, inputs: ScoreInput[], submit: boolean) {
 const errors: Record<string,string>={}, scores:JudgeScore[]=[]
 for(const category of criteria.categories){
  const input=inputs.find(i=>i.categoryID===category.id)??{categoryID:category.id,value:'',note:'',deliverableID:''}
  const value=input.value===''?null:Number(input.value)
  if(value!==null&&(!Number.isInteger(value)||value<category.min_score||value>category.max_score))errors[category.id]=`Choose a whole score from ${category.min_score} to ${category.max_score}.`
  else if(submit&&value===null)errors[category.id]='Choose a score before submitting.'
  if(input.note.length>4000)errors[`${category.id}:evidence`]='Keep the evidence note within 4,000 characters.'
  else if(submit&&!input.deliverableID&&!input.note.trim())errors[`${category.id}:evidence`]='Reference a frozen deliverable or add your own observed evidence.'
  scores.push({category_id:category.id,value,evidence:input.note.trim()||input.deliverableID?[{deliverable_id:input.deliverableID||null,note:input.note}]:[]})
 }
 return {errors,scores}
}
const nil='00000000-0000-0000-0000-000000000000'
export type JudgeIntent = {requestID:string;action:'save'|'submit'|'reset';revision:number;runID?:string;teamID?:string;fingerprint:string}
// Match Go's explicit command struct, including its JSON escaping. This digest
// is exact request metadata; private scores/notes never enter session history.
export async function judgeFingerprint(body:JudgeCommand|JudgePracticeCommand,revision:number,runID?:string,teamID?:string){
 const scores=body.scores?.map(s=>({category_id:s.category_id,value:s.value,evidence:s.evidence.map(e=>({deliverable_id:e.deliverable_id??null,note:e.note}))}))??null
 const command='submission_revision' in body?{repository_decision_id:body.repository_decision_id??null,request_id:body.request_id,action:body.action,author_id:body.author_id??nil,submission_revision:body.submission_revision,scores,reason:body.reason??''}:{request_id:body.request_id,action:body.action,scores}
 const value=runID?{Run:runID,Team:teamID,Version:revision,Command:command}:{Version:revision,Practice:command}
 const encoded=JSON.stringify(value).replace(/[<>&\u2028\u2029]/g,c=>`\\u${c.charCodeAt(0).toString(16).padStart(4,'0')}`)
 const digest=await crypto.subtle.digest('SHA-256',new TextEncoder().encode(encoded))
 return [...new Uint8Array(digest)].map(byte=>byte.toString(16).padStart(2,'0')).join('')
}
export function matchJudgeReceipt(receipt:JudgeReceipt,intent:JudgeIntent,actorID:string){
 return receipt.request_id===intent.requestID&&receipt.actor_id===actorID&&receipt.run_id===(intent.runID??nil)&&receipt.team_id===(intent.teamID??nil)&&receipt.action===intent.action&&receipt.revision===intent.revision+1&&receipt.state===(intent.action==='submit'?'submitted':intent.action==='reset'?'reset':'draft')&&receipt.fingerprint===intent.fingerprint
}

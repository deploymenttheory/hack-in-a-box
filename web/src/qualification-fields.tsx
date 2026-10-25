import type {ReactNode} from 'react'
import {Field,Input,Textarea} from './components/ui/fields'
import {Button} from './components/ui/button'
import type {QualificationCard,QualificationEvidence,QualificationPage} from './qualification-model'

export type QualificationScoreInput={categoryID:string;pending:boolean;value:string;note:string;references:QualificationEvidence[]}
export function qualificationScoreInputs(page:QualificationPage,card?:QualificationCard):QualificationScoreInput[]{
 return page.criteria.categories.map(category=>{
  const score=card?.scores.find(value=>value.category_id===category.id),noteIndex=score?.evidence.findIndex(value=>value.kind==='note')??-1
  return {categoryID:category.id,pending:score?.pending??true,value:score?.value===null||score?.value===undefined?'':String(score.value),note:noteIndex<0?'':score!.evidence[noteIndex].note,references:score?.evidence.filter((_,index)=>index!==noteIndex)??[]}
 })
}
export function qualificationFieldErrors(page:QualificationPage,inputs:QualificationScoreInput[],reason:string){
 const errors:Record<string,string>={}
 if(!reason.trim()||new TextEncoder().encode(reason).byteLength>2000)errors.reason='Record a reason within the native 2,000-byte limit.'
 for(const category of page.criteria.categories){
  const input=inputs.find(value=>value.categoryID===category.id)
  if(!input){errors[category.id]='Choose Pending/unobserved or record a score.';continue}
  if(!input.pending&&(!input.value.trim()||!Number.isInteger(Number(input.value))||Number(input.value)<category.min_score||Number(input.value)>category.max_score))errors[category.id]=`Enter a whole score from ${category.min_score} to ${category.max_score}, or choose Pending/unobserved.`
  if(new TextEncoder().encode(input.note).byteLength>10000)errors[`${category.id}:evidence`]='Shorten the evidence note to the native 10,000-byte limit. Your entered text is retained.'
  if(input.references.length+(input.note.trim()?1:0)>50)errors[`${category.id}:evidence`]='Keep at most 50 evidence references for this category.'
 }
 return errors
}
export const qualificationInputScores=(inputs:QualificationScoreInput[])=>inputs.map(input=>({category_id:input.categoryID,pending:input.pending,value:input.pending?null:Number(input.value),evidence:[...(input.note.trim()?[{kind:'note' as const,id:null,version:null,note:input.note}]:[]),...input.references]}))

// Pure controlled production fields. Their native owner supplies current action
// readiness and separately authorised evidence selectors; this view grants none.
export function QualificationScoreFields({page,inputs,change,errors,disabled=false,evidencePicker}:{page:QualificationPage;inputs:QualificationScoreInput[];change:(inputs:QualificationScoreInput[])=>void;errors:Record<string,string>;disabled?:boolean;evidencePicker?:(categoryID:string,add:(evidence:QualificationEvidence)=>void)=>ReactNode}){
 const update=(id:string,values:Partial<QualificationScoreInput>)=>change(inputs.map(input=>input.categoryID===id?{...input,...values}:input))
 return <fieldset disabled={disabled}><legend>Reasoned moderated final scores</legend>{page.criteria.categories.map(category=>{
  const input=inputs.find(value=>value.categoryID===category.id)!
  return <section key={category.id} className="fleet-qualification-score-category"><h4>{category.label}</h4><p>{category.guidance}</p><details><summary>Read descriptive anchors</summary>{category.anchors.map((anchor,index)=><p key={index}>{anchor.score}: {anchor.description}</p>)}</details><label><input type="checkbox" checked={input.pending} onChange={event=>update(category.id,{pending:event.target.checked})}/> Pending/unobserved for {category.label}</label><Field id={`qualification-score-${category.id}`} label={`Moderated score for ${category.label}`} description={`Whole score ${category.min_score}–${category.max_score}; weight ${category.weight}`} error={errors[category.id]}><Input type="number" min={category.min_score} max={category.max_score} step={1} value={input.value} disabled={input.pending} onChange={event=>update(category.id,{value:event.target.value})}/></Field><Field id={`qualification-evidence-${category.id}`} label={`Moderated evidence for ${category.label}`} description="Explain the final observed evidence. Retained references preserve their exact source version." error={errors[`${category.id}:evidence`]}><Textarea value={input.note} onChange={event=>update(category.id,{note:event.target.value})}/></Field>{input.references.map((reference,index)=><div key={index}><p>{reference.kind} · {reference.id??'Recorded note'} · Version {reference.version??'not applicable'} · {reference.note}</p><Button type="button" icon="close" variant="ghost" onClick={()=>update(category.id,{references:input.references.filter((_,position)=>position!==index)})}>Remove evidence {index+1} for {category.label}</Button></div>)}{evidencePicker?.(category.id,evidence=>update(category.id,{references:[...input.references,evidence]}))}</section>
 })}</fieldset>
}

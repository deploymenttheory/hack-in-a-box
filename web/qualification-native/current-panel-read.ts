import type {Page} from '@playwright/test'
import type {QualificationPage} from '../src/qualification-model'

// Observe this owner's real next projection, never another actor's write ACK.
// An already pending GET may still contain only the first ballot.
export async function waitForCastingRead(page:Page,url:string,categoryID:string,seat:number){
 const response=await page.waitForResponse(async response=>{
  if(response.url()!==url||response.request().method()!=='GET'||response.status()!==200)return false
  try{
   const current=await response.json() as QualificationPage
   const ties=current.preview?.ties.filter(tie=>tie.category_id===categoryID)??[]
   return current.tie_actions.some(action=>ties.some(tie=>tie.id===action.tie_id)&&action.seat===seat&&action.votes_complete&&action.casting)
  }catch{return false}
 })
 const error=await response.finished()
 if(error)throw new Error('Current owner panel response did not finish')
}

import {expect,type Locator,type Request} from '@playwright/test'
import {observeActivation} from './keyboard'
import {matchesQualificationReceipt,qualificationFingerprint,type QualificationCommand,type QualificationPage} from '../src/qualification-model'

// Model an explicitly resumed user gesture only when the browser proves the
// first key was suppressed by a read lock and no command was sent. Accepted or
// uncertain requests never repeat. A second interruption fails, without a loop.
export async function confirmReviewedDialog(dialog:Locator,endpoint:string){
 const page=dialog.page(),confirm=dialog.getByRole('button',{name:'Confirm reviewed panel command',exact:true}),review=await dialog.textContent()??'',posts:Request[]=[]
 const observe=(request:Request)=>{if(request.method()==='POST'&&new URL(request.url()).pathname===endpoint)posts.push(request)}
 page.on('request',observe)
 try{
  const first=await observeActivation(confirm)
  let accepted=first.clicks
  if(first.keys===1&&first.connected&&first.paused&&first.clicks===0){
   expect(posts,'A read-suppressed gesture must not send any command').toHaveLength(0)
   await expect.poll(async()=>!await dialog.count()?'removed':await confirm.isEnabled()?'ready':'pending').not.toBe('pending')
   expect(await dialog.count(),'A changed or denied private review must not be resurrected').toBe(1)
   await expect(dialog).toHaveText(review);await expect(confirm).toBeEnabled()
   const resumed=await observeActivation(confirm)
   expect(resumed).toMatchObject({keys:1,clicks:1,paused:false,focused:true,connected:true});accepted+=resumed.clicks
  }else expect(first).toMatchObject({keys:1,clicks:1,paused:false,focused:true,connected:true})
  expect(accepted).toBe(1)
  await expect.poll(()=>posts.length,{message:'An accepted reviewed gesture must send exactly one original command'}).toBe(1)
  const request=posts[0],body=request.postDataJSON() as QualificationCommand,response=await request.response()
  expect(response,'An accepted request needs its actual original response').not.toBeNull()
  expect(response!.status()).toBe(200)
  const result=await response!.json() as QualificationPage,revision=Number(request.headers()['if-match']?.replaceAll('"',''))
  expect(result.receipt&&matchesQualificationReceipt(result.receipt,{requestID:body.request_id,runID:endpoint.split('/').at(-1)!,action:body.action,revision,version:body.version,fingerprint:await qualificationFingerprint(body,revision)}),'Only the exact original receipt establishes acknowledgement').toBe(true)
  await expect(dialog).toHaveCount(0)
  expect(posts).toHaveLength(1)
  return {acceptedClicks:accepted,posts:posts.length,resumed:first.clicks===0}
 }finally{page.off('request',observe)}
}

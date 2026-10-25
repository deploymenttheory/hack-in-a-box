import { expect, test } from '@playwright/test'
import { firstID } from './fixtures'
import { judgeFixture, otherJudge, prepareJudging, runID, teamA } from './judge-fixture'
const history=(page:import('@playwright/test').Page)=>page.getByRole('complementary',{name:'Original judge command history',exact:true})
async function fill(page:import('@playwright/test').Page){for(const label of ['Demo','Technical approach','UX','Feature fit','Innovation']){await page.getByLabel(`Score for ${label}`,{exact:true}).fill('8');await page.getByLabel(`Your evidence for ${label}`,{exact:true}).fill('Private independent observation')}}
async function confirm(page:import('@playwright/test').Page,label:string){await page.getByRole('button',{name:label,exact:true}).click();await page.getByRole('alertdialog').getByRole('button',{name:'Confirm reviewed scores',exact:true}).click()}
for(const mode of ['save','submit','practice-save','practice-submit','practice-reset','reopen'] as const)test(`HTTP408 retains original ${mode} recovery without replay`,async({page})=>{
 const data=judgeFixture(),practice=mode.startsWith('practice')
 if(mode==='reopen'){data.organiser=true;data.states=[{run_id:runID,team_id:teamA,author_id:otherJudge,version:4,state:'submitted',source_available:true,source_changed:false,can_reopen:true,submission_revision:3,repository_decision_id:null}]}
 await prepareJudging(page,data,practice)
 if(mode==='reopen'){await page.getByRole('button',{name:`Open lock coordination for ${teamA}`,exact:true}).click();await page.getByLabel('Reason for reopening',{exact:true}).fill('Reviewed independent correction')}
 else{if(!practice)await page.getByRole('button',{name:'Assigned build 1',exact:true}).click();await fill(page)}
 if(mode==='practice-reset'){await confirm(page,'Review practice submission');await expect(page.getByText('Synthetic practice submitted',{exact:false})).toBeVisible()}
 const before=data.requests.length;data.timeoutNext=true
 await confirm(page,mode==='reopen'?`Review reopening judge ${otherJudge}`:mode==='practice-reset'?'Review practice reset':mode==='practice-submit'?'Review practice submission':mode==='submit'?'Review competition submission':'Review save draft')
 await expect(history(page)).toContainText('Outcome unknown');expect(data.requests).toHaveLength(before+1)
 const requestID=data.requests.at(-1)!.body.request_id as string
 await expect(history(page)).toContainText(requestID)
 if(mode==='reopen')await page.getByRole('button',{name:'Read current card states',exact:true}).click();else await page.getByRole('button',{name:'Read current source',exact:true}).click()
 await expect(history(page)).toContainText(requestID);await expect(history(page)).toContainText('Outcome unknown')
 await history(page).getByRole('button',{name:'Read original acknowledgement',exact:true}).click();await expect(history(page)).toHaveCount(0);expect(data.requests).toHaveLength(before+1)
})

for(const response of ['ack','timeout'] as const)test(`metadata owns pending and ${response} departure after actual current assignment denial`,async({page})=>{
 const data=await prepareJudging(page);await page.getByRole('button',{name:'Assigned build 1',exact:true}).click();await fill(page)
 let release!:()=>void;data.responseGate=new Promise<void>(resolve=>{release=resolve});data.timeoutNext=response==='timeout'
 try{
 await confirm(page,'Review competition submission');await expect.poll(()=>data.requests.length).toBe(1);await expect(history(page)).toContainText('Awaiting acknowledgement')
 data.authorized=false;await page.evaluate(()=>window.dispatchEvent(new Event('focus')));await expect(page.getByLabel('Your evidence for Demo',{exact:true})).toHaveCount(0)
 await expect(history(page)).toContainText('Awaiting acknowledgement')
 expect(await page.evaluate(()=>window.dispatchEvent(new Event('planning-departure',{cancelable:true})))).toBe(false)
 expect(await page.evaluate(()=>window.dispatchEvent(new Event('beforeunload',{cancelable:true})))).toBe(false)
 await page.getByRole('link',{name:'Back to event',exact:true}).click();await expect(page).toHaveURL(new RegExp(`/events/${firstID}/judging$`))
 release();if(response==='ack'){await expect(history(page)).toHaveCount(0);expect(await page.evaluate(()=>window.dispatchEvent(new Event('planning-departure',{cancelable:true})))).toBe(true);expect(await page.evaluate(()=>window.dispatchEvent(new Event('beforeunload',{cancelable:true})))).toBe(true);await page.getByRole('link',{name:'Back to event',exact:true}).click();await expect(page).toHaveURL(new RegExp(`/events/${firstID}$`));expect(data.requests).toHaveLength(1);return}
 await expect(history(page)).toContainText('Outcome unknown')
 expect(await page.evaluate(()=>window.dispatchEvent(new Event('beforeunload',{cancelable:true})))).toBe(false)
 let prompts=0;page.on('dialog',dialog=>{prompts++;void dialog.dismiss()})
 expect(await page.evaluate(()=>window.dispatchEvent(new Event('planning-departure',{cancelable:true})))).toBe(false)
 await page.getByRole('link',{name:'Back to event',exact:true}).click();await expect(page).toHaveURL(new RegExp(`/events/${firstID}/judging$`));expect(prompts).toBe(2)
 page.removeAllListeners('dialog');page.once('dialog',dialog=>void dialog.accept());await page.getByRole('link',{name:'Back to event',exact:true}).click();await expect(page).toHaveURL(new RegExp(`/events/${firstID}$`))
 data.authorized=true;await page.goBack();await expect(history(page)).toContainText('Outcome unknown');await history(page).getByRole('button',{name:'Read original acknowledgement',exact:true}).click();await expect(history(page)).toHaveCount(0)
 expect(data.requests).toHaveLength(1);expect(await page.evaluate(()=>window.dispatchEvent(new Event('planning-departure',{cancelable:true})))).toBe(true)
 }finally{release()}
})

test('unknown original metadata still protects departure after private access is removed',async({page})=>{
 const data=await prepareJudging(page);await page.getByRole('button',{name:'Assigned build 1',exact:true}).click();await fill(page);data.loseNext=true;await confirm(page,'Review competition submission');await expect(history(page)).toContainText('Outcome unknown')
 data.authorized=false;await page.evaluate(()=>window.dispatchEvent(new Event('focus')));await expect(page.getByLabel('Your evidence for Demo',{exact:true})).toHaveCount(0);await expect(history(page)).toContainText('Outcome unknown');await expect(history(page)).not.toContainText('Private independent observation')
 page.on('dialog',dialog=>void dialog.dismiss());expect(await page.evaluate(()=>window.dispatchEvent(new Event('planning-departure',{cancelable:true})))).toBe(false);expect(data.requests).toHaveLength(1)
})

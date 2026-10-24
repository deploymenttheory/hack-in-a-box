import { expect, test, type Page } from '@playwright/test'
import type { components } from '../src/api/schema.gen'
import { firstID, mockWorkspace, person } from './fixtures'
const id='71111111-1111-4111-8111-111111111111'
type Schema=components['schemas']
async function setup(page:Page){
 await mockWorkspace(page)
 const outcome:Schema['OutcomeView']={id,version:4,current_version:4,command_id:crypto.randomUUID(),author_id:person.id,actor_id:person.id,at:'2026-10-01T12:00:00Z',reason:'Private native review',definition:{kind:'pitch',problem:'PRIVATE original <script>window.privateLeak=true</script>',audience:'Private audience',proposed_value:'Private estimate',solution:'PRIVATE original solution',backlog_url:'',theme_sensitive:false},provenance:{event_id:firstID,team_id:null},status:'accepted',owner_id:person.id,review_at:null,measures:[],evidence_added_id:null,capability:{can_revise:false,can_moderate:true,can_follow_up:true,can_adopt:false,archived:true}}
 const state={outcome,publication:null as Schema['OutcomePublication']|null,reviews:new Map<string,Schema['OutcomeReview']>(),publications:[] as Schema['OutcomePublicationCommand'][],plans:[] as Schema['OutcomeReviewCommand'][],deny:false,planMode:'' as ''|'after',planResponse:undefined as (()=>Promise<void>)|undefined,reviewReceipts:new Map<string,Schema['OutcomeReview']>(),hold:undefined as (()=>Promise<void>)|undefined}
 await page.route('**/api/v1/**',async route=>{
  const req=route.request(),url=new URL(req.url()),path=url.pathname
  const fail=(status:number)=>route.fulfill({status,json:{error:{code:status===404?'not_found':status===403?'forbidden':'revision_conflict',message:'Synthetic native policy',request_id:'synthetic'}}})
  if(path===`/api/v1/events/${firstID}/communications/campaigns`)return route.fulfill({json:{items:[],providers:[{channel:'in_app',available:true,reason:'ready',supports_self_test:true,self_test_available:true,self_test_reason:'ready',config_revision:'synthetic-in-app'},{channel:'smtp',available:false,reason:'not_configured',supports_self_test:true,self_test_available:false,self_test_reason:'not_configured',config_revision:''}]}})
  if(path===`/api/v1/events/${firstID}/objectives`)return route.fulfill({json:{items:[],next_cursor:null}})
  if(path==='/api/v1/inbox')return route.fulfill({json:{items:[],next_cursor:null,unread:0}})
  if(path===`/api/v1/events/${firstID}/media`)return route.fulfill({json:{items:[],next_cursor:null}})
  const review=path.match(/^\/api\/v1\/outcome-reviews\/([^/]+)$/)
  if(review){if(state.deny)return fail(403);const current=state.reviews.get(review[1]),command=url.searchParams.get('command_id'),found=command?state.reviewReceipts.get(command):current;return found&&found.id===review[1]?route.fulfill({json:{...found,current_version:current?.version??found.version}}):fail(404)}
  if(!path.startsWith(`/api/v1/outcomes/${id}`))return route.fallback()
  if(state.deny)return fail(403)
  const suffix=path.slice(`/api/v1/outcomes/${id}`.length)
  if(suffix==='/history')return route.fulfill({json:{items:[outcome],next_cursor:null}})
  if(suffix==='/evidence'||suffix==='/owners')return route.fulfill({json:{items:[],next_cursor:null}})
  if(suffix==='/reviews')return route.fulfill({json:{items:[...state.reviews.values()]}})
  if(suffix==='/publication'){
   if(req.method()==='GET')return state.publication?route.fulfill({json:state.publication}):fail(404)
   const body=req.postDataJSON() as Schema['OutcomePublicationCommand'];state.publications.push(body)
   if(body.source_version!==outcome.current_version||req.headers()['if-match']!==`"${state.publication?.version??0}"`)return fail(409)
   const version=(state.publication?.version??0)+1
   state.publication={outcome_id:id,version,current_version:version,source_version:body.source_version,command_id:body.id,fields:body.fields??state.publication!.fields,state:body.action==='save'?'draft':body.action==='publish'?'published':'withdrawn',published_version:body.action==='publish'?version:body.action==='withdraw'?null:state.publication?.published_version??null,reason:body.reason,actor_id:person.id,at:'2026-10-01T12:00:00Z'}
   return route.fulfill({json:state.publication})
  }
  if(suffix.startsWith('/reviews/')){
   const reviewID=suffix.split('/')[2],body=req.postDataJSON() as Schema['OutcomeReviewCommand'];state.plans.push(body)
   const previous=state.reviews.get(reviewID),version=(previous?.version??0)+1
   if(req.headers()['if-match']!==`"${previous?.version??0}"`||body.source_version!==outcome.current_version)return fail(409)
   const schedule=body.schedule??previous!.schedule
   const next:Schema['OutcomeReview']={id:reviewID,outcome_id:id,version,current_version:version,command_id:body.id,source_version:body.source_version,owner_id:person.id,schedule,message:body.message??previous!.message,due_at:'2026-12-01T12:00:00Z',state:body.action==='save'?'draft':body.action==='approve'?'approved':'cancelled',effective_state:body.action==='save'?'draft':body.action==='approve'?'approved':'cancelled',reason:body.reason,actor_id:person.id,at:'2026-10-01T12:00:00Z',blockers:[]}
   state.reviews.set(reviewID,next);state.reviewReceipts.set(body.id,structuredClone(next));if(state.planMode==='after'){state.planMode='';return route.abort()};await state.planResponse?.();return route.fulfill({json:next})
  }
  if(suffix===''){await state.hold?.();return route.fulfill({json:outcome})}
  return fail(404)
 })
 return state
}
test('organiser separately selects public fields and explicitly publishes then withdraws without private copying',async({page},info)=>{
 const state=await setup(page);await page.goto(`/ideas/${id}`)
 await page.getByRole('button',{name:'Open publication review'}).click()
 await expect(page.getByLabel('Reviewed public problem')).toHaveValue('')
 await expect(page.getByLabel('Reviewed public solution')).toHaveValue('')
 await page.getByLabel('Reviewed public problem').fill('Reviewed problem <script>window.publishedLeak=true</script>')
 await page.getByLabel('Reviewed public solution').fill('Reviewed positive solution')
 await page.getByLabel('Reason for this separate publication decision').fill('Review exactly these fields')
 await page.getByRole('button',{name:'Review publication draft',exact:true}).click(); await page.getByRole('button',{name:'Save publication draft',exact:true}).focus();await page.keyboard.press('Enter')
 await expect.poll(()=>state.publications.length).toBe(1)
 expect(JSON.stringify(state.publications[0])).not.toContain('PRIVATE original')
 await expect(page.getByRole('button',{name:'Review publication release',exact:true})).toBeDisabled()
 await page.getByLabel('Reason for this separate publication decision').fill('Approve the separate publication')
 await page.getByRole('button',{name:'Review publication release',exact:true}).click(); await page.getByRole('button',{name:'Publish exactly reviewed fields',exact:true}).click()
 await expect.poll(()=>state.publication?.state).toBe('published')
 await page.getByLabel('Reason for this separate publication decision').fill('Withdraw public projection')
 await page.getByRole('button',{name:'Review withdrawal',exact:true}).click(); await page.getByRole('button',{name:'Withdraw publication',exact:true}).click()
 await expect.poll(()=>state.publication?.state).toBe('withdrawn')
 expect(state.publications.map(c=>c.action)).toEqual(['save','publish','withdraw'])
 expect(await page.evaluate(()=>'privateLeak'in window||'publishedLeak'in window)).toBe(false)
 await page.screenshot({path:info.outputPath('organiser-outcome-publication.png'),fullPage:true})
 expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(page.viewportSize()!.width)
})
test('organiser optional timezone-aware review needs explicit approval and honest provider readiness then cancellation',async({page})=>{
 const state=await setup(page);await page.goto(`/ideas/${id}`)
 await page.getByRole('button',{name:'Configure optional 30-day checkpoint'}).click()
 await page.getByLabel('Review timezone (IANA)').fill('Europe/London')
 await page.getByLabel('Actual local review date and time').fill('2026-12-01T12:00')
 await expect(page.getByText('Unavailable: not_configured',{exact:false})).toBeVisible()
 await page.getByLabel('Reason for this explicit review decision').fill('Save only; no permission to send yet')
 await page.getByRole('button',{name:'Review plan draft',exact:true}).click(); await page.getByRole('button',{name:'Save review draft',exact:true}).click()
 await expect.poll(()=>state.plans.length).toBe(1)
 expect([...state.reviews.values()][0].state).toBe('draft')
 await expect(page.getByRole('button',{name:'Review plan approval',exact:true})).toBeDisabled()
 await page.getByLabel('Reason for this explicit review decision').fill('Explicitly approve current owner in-app only')
 await page.getByRole('button',{name:'Review plan approval',exact:true}).click(); await page.getByRole('button',{name:'Approve exact review',exact:true}).click()
 await expect.poll(()=>[...state.reviews.values()][0].state).toBe('approved')
 expect(state.plans[0].schedule?.timezone).toBe('Europe/London');expect(state.plans[0].schedule?.channels).toEqual(['in_app'])
 await page.getByLabel('Reason for this explicit review decision').fill('Cancel future review deliberately')
 await page.getByRole('button',{name:'Review cancellation',exact:true}).click(); await page.getByRole('button',{name:'Cancel review',exact:true}).click()
 await expect.poll(()=>[...state.reviews.values()][0].state).toBe('cancelled')
 expect(state.plans.map(c=>c.action)).toEqual(['save','approve','cancel'])
})

for (const result of ['pending', 'unknown'] as const) test(`original ${result} owner plan survives checkpoint editor replacement and settles only its original receipt`, async ({ page }) => {
 const state=await setup(page);await page.goto(`/ideas/${id}`)
 await page.getByRole('button',{name:'Configure optional 30-day checkpoint'}).click()
 await page.getByLabel('Actual local review date and time').fill('2026-12-01T12:00')
 await page.getByLabel('Reviewed message subject').fill('PRIVATE ORIGINAL PLAN SUBJECT')
 await page.getByLabel('Reason for this explicit review decision').fill('Original explicit synthetic plan')
 await page.getByRole('button',{name:'Review plan draft',exact:true}).click()
 let release=()=>{}, reached=()=>{}
 const held=new Promise<void>(resolve=>{release=resolve}),entered=new Promise<void>(resolve=>{reached=resolve})
 if(result==='pending')state.planResponse=async()=>{reached();await held}
 else state.planMode='after'
 await page.getByRole('button',{name:'Save review draft',exact:true}).click()
 if(result==='pending')await entered
 else await expect(page.getByRole('status',{name:'Outcome save state'}).filter({hasText:'Save outcome is unknown'})).toBeVisible()
 expect(state.plans).toHaveLength(1)
 page.once('dialog',dialog=>dialog.accept())
 await page.getByRole('button',{name:'Configure optional 60-day checkpoint'}).click()
 const editor=page.locator('section.fleet-outcome-section').filter({has:page.getByRole('heading',{name:'Configure 60-day checkpoint',exact:true})})
 await expect(editor.getByLabel('Reviewed message subject')).toBeDisabled()
 await expect(editor.getByLabel('Reviewed message subject')).not.toHaveValue('PRIVATE ORIGINAL PLAN SUBJECT')
 expect(state.plans).toHaveLength(1)
 if(result==='pending'){release();await expect(editor.getByLabel('Reviewed message subject')).toBeEnabled()}
 else {await page.getByRole('button',{name:'Check earlier exact save receipt'}).click();await expect(page.getByRole('status',{name:'Earlier command recovery state'})).toContainText('exact earlier command was saved');await expect(editor.getByLabel('Reviewed message subject')).toBeEnabled()}
 expect(state.plans).toHaveLength(1)
})

test('restored moderation capability waits for its own fresh owner choices and does not restore private editor input', async ({ page }) => {
 const state=await setup(page)
 let reads=0, hold=false, release=()=>{}, reached=()=>{}
 const held=new Promise<void>(resolve=>{release=resolve}),entered=new Promise<void>(resolve=>{reached=resolve})
 await page.route(`**/api/v1/outcomes/${id}/owners*`,async route=>{reads++;if(hold){reached();await held;return route.fulfill({json:{items:[],next_cursor:null}})}return route.fulfill({json:{items:[{id:person.id,display_name:'PRIVATE CURRENT OWNER CHOICE'}],next_cursor:null}})})
 await page.goto(`/ideas/${id}`)
 const assignment=()=>page.locator('section.fleet-outcome-action, section.panel').filter({has:page.getByRole('heading',{name:'Owner and review date',exact:true})})
 await expect(assignment().getByLabel('Eligible current owner')).toContainText('PRIVATE CURRENT OWNER CHOICE')
 await assignment().getByLabel('Reason for this change').fill('PRIVATE OLD ASSIGNMENT REASON')
 state.outcome.capability.can_moderate=false
 await page.getByRole('button',{name:'Refresh current outcome'}).click()
 await expect(page.getByRole('heading',{name:'Owner and review date',exact:true})).toHaveCount(0)
 hold=true;state.outcome.capability.can_moderate=true
 await page.getByRole('button',{name:'Refresh current outcome'}).click()
 await expect(assignment().getByLabel('Eligible current owner')).not.toContainText('PRIVATE CURRENT OWNER CHOICE')
 await entered
 await expect(assignment().getByLabel('Reason for this change')).toHaveValue('')
 release();await expect.poll(()=>reads).toBe(2)
 await expect(assignment().getByLabel('Eligible current owner')).not.toContainText('PRIVATE CURRENT OWNER CHOICE')
 expect(state.plans).toHaveLength(0);expect(state.publications).toHaveLength(0)
})

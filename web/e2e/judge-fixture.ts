import type { Page } from '@playwright/test'
import type { components } from '../src/api/schema.gen'
import { judgeFingerprint } from '../src/judge-model'
import { judgeGuideBrief, judgeGuideCriteria } from '../src/components/patterns/scenario'
import { firstID, mockWorkspace, person } from './fixtures'
export const teamA='3f8af336-a168-47ed-b67c-cba51d530413',teamB='e6bcb64c-8c03-458a-a184-302e87c034f5',runID='02054da1-8a61-496b-9f97-ff0fc7e1ca61',otherJudge='cbccbf46-057f-4224-b6e1-f92047d4f858'
export const criteria:components['schemas']['PublicRubricDefinition']={...judgeGuideCriteria,categories:['Demo','Technical approach','UX','Feature fit','Innovation'].map((label,i)=>({...judgeGuideCriteria.categories[0],label,id:`8ffbb915-b8cc-439b-b025-00ca75dcf07${i}`}))}
export function judgeFixture(){
 const make=(id:string,name:string):components['schemas']['JudgeBrief']=>({...structuredClone(judgeGuideBrief),team_id:id,team_name:name,criteria,reference:{...judgeGuideBrief.reference,digest:'a'.repeat(64)},submission:{definitions:[],evidence:[{...judgeGuideBrief.submission.evidence[0],label:'Frozen demonstration',url:'https://example.test/synthetic-frozen-build'}]}})
 return {briefs:{[teamA]:make(teamA,'Synthetic Aurora build'),[teamB]:make(teamB,'Synthetic Beacon build')} as Record<string,components['schemas']['JudgeBrief']>,assignments:[teamA,teamB],authorized:true,gate:false,failBrief:false,loseNext:false,corruptReceipt:false,organiser:false,practiceDenied:false,receiptAvailable:true,requests:[] as {body:Record<string,unknown>;revision:string;path:string}[],receipts:new Map<string,components['schemas']['JudgeReceipt']>(),practice:{fictional_entry_id:'8afac138-307a-4bf8-8e04-4ad02dd57977',name:'Fictional practice robot — synthetic entry',criteria,rubric:judgeGuideBrief.reference,card:null} as components['schemas']['JudgePractice'],states:[] as components['schemas']['JudgeCardState'][]}
}
export async function prepareJudging(page:Page,data=judgeFixture(),practice=false){
 const workspace=await mockWorkspace(page),event=workspace.events[0];event.stage='assessment';event.can_edit=data.organiser;event.can_read_staff=data.organiser;event.can_present=false;event.can_transition=data.organiser
 await page.route('**/api/v1/session',route=>route.fulfill({json:{person:{...person,platform_admin:false},draft_scope:'22222222-2222-4222-8222-222222222222',csrf_token:'synthetic-csrf-token-32-characters',can_create_events:false}}))
 await page.route(`**/api/v1/events/${firstID}/today`,route=>route.fulfill({json:{event,roles:[data.organiser?'organiser':'judge'],participation_state:'none',program:{sequence:2,server_now:'2027-06-20T17:00:00Z',current:{id:'41ea8918-4a5a-405e-a98b-bae24d46c872',title:'Aurora demonstration',text:'Published room cue',url:'',duration_seconds:120},next:{id:'7c8c2fd8-d95d-4788-b9d4-06dc221dc46a',title:'Beacon demonstration',text:'Published next cue',url:'',duration_seconds:120},mode:'held',remaining_seconds:120,timer_ends_at:null,override_active:false}}}))
 const respond=async(route:import('@playwright/test').Route)=>{
 const request=route.request(),path=new URL(request.url()).pathname
 const reject=(status:number)=>route.fulfill({status,json:{error:{code:status===403?'forbidden':status===409?'revision_conflict':'unavailable',message:status===409?'Current final scoring gate is not open.':'Synthetic current authority unavailable',request_id:'synthetic'}}})
 if(!data.authorized)return reject(403)
 if(path.includes('/judging-receipts/'))return route.fulfill({json:{receipt:data.receiptAvailable?data.receipts.get(path.split('/').at(-1)!)??null:null}})
 if(path.endsWith('/judging'))return route.fulfill({json:{items:[{run_id:runID,revision:0,deliberation:false,required_judges:2,assignments:data.assignments.map(team_id=>({team_id,judge_ids:[data.organiser?otherJudge:person.id],missing:0}))}]}})
 if(path.endsWith('/states')){if(!data.organiser)return reject(403);return route.fulfill({json:{items:data.states}})}
 const isPractice=path.endsWith('/judge-practice'),team=path.split('/').at(-1)!,brief=data.briefs[team]
 if(isPractice&&data.practiceDenied)return reject(403)
 if(!isPractice&&!brief)return reject(404)
 if(request.method()==='GET'){
 if(data.failBrief)return reject(503)
 if(data.gate&&!isPractice)return reject(409)
 return route.fulfill({json:isPractice?data.practice:brief})
 }
 const body=request.postDataJSON(),revision=request.headers()['if-match']
 data.requests.push({body,revision,path})
 const known=data.receipts.get(body.request_id)
 if(known)return route.fulfill({json:known})
 if(!request.headers()['x-csrf-token'])return reject(403)
 const expected=Number(revision.replaceAll('"',''))
 const card=isPractice?data.practice.card:brief.own_card
 if(expected!==(body.action==='reopen'?data.states.find(s=>s.author_id===body.author_id)?.version:card?.version??0))return reject(409)
 if(!isPractice&&(body.submission_revision!==brief.submission_revision||body.repository_decision_id!==brief.repository.decision_id))return reject(409)
 if(card?.state==='submitted'&&!['reset','reopen'].includes(body.action))return reject(409)
 const state=body.action==='submit'?'submitted':'draft'
 const scores=body.action==='reset'?[]:body.scores??card?.scores??[]
 const next={version:expected+1,state,scores,rubric:isPractice?data.practice.rubric:brief.reference}
 if(isPractice)data.practice.card=next
 else if(body.action==='reopen'){const metadata=data.states.find(s=>s.author_id===body.author_id)!;metadata.version++;metadata.state='draft';metadata.can_reopen=false}
 else brief.own_card={...next,run_id:runID,team_id:team,author_id:person.id,submission_revision:brief.submission_revision,repository:brief.repository,changed_at:'2027-06-20T17:00:00Z'}
 const fingerprint=await judgeFingerprint(body,expected,isPractice?undefined:runID,isPractice?undefined:team)
 const receipt={request_id:body.request_id,actor_id:person.id,run_id:isPractice?'00000000-0000-0000-0000-000000000000':runID,team_id:isPractice?data.practice.fictional_entry_id:team,action:isPractice?`practice_${body.action}`:body.action,revision:expected+1,state,fingerprint}
 data.receipts.set(body.request_id,receipt)
 if(data.loseNext){data.loseNext=false;return route.abort('failed')}
 return route.fulfill({json:data.corruptReceipt?{...receipt,actor_id:otherJudge}:receipt})
 }
 await page.route(`**/api/v1/events/${firstID}/judging**`,respond)
 await page.route(`**/api/v1/events/${firstID}/judge-practice`,respond)
 await page.goto(`/events/${firstID}/judging${practice?'/practice':''}`)
 return data
}

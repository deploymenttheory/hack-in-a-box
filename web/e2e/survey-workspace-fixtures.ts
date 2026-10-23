import type { Page } from '@playwright/test'
import type { components } from '../src/api/schema.gen'
import { fixtureEvent, firstID, mockWorkspace, person, secondID } from './fixtures'
export const surveyID='8d4158fd-3ef9-42eb-9c0c-a064f34db9cd', questionID='122700b0-acf4-4e13-ae54-d339af614225', textID='2d69935f-f2c5-4f74-b53b-a9d95a7191ca'
export const surveyPath=`/events/${firstID}/surveys/${surveyID}`, managePath=surveyPath+'/manage'
export const privateText='SYNTHETIC-PRIVATE-SURVEY-WORKSPACE-CANARY'
export type Configuration=components['schemas']['SurveyConfiguration']
type Response=components['schemas']['SurveyResponse']
type ResponseCommand=components['schemas']['SurveyResponseCommand']
type ReportReview=components['schemas']['SurveyReviewedReport']
export async function surveyFixture(page:Page,staff=false){
 await mockWorkspace(page)
 const event=fixtureEvent();event.name='Synthetic survey edition';event.stage='completed';event.can_read_staff=staff;event.can_edit=staff;event.can_present=false;event.configuration.timezone='Pacific/Auckland'
 const configuration:Configuration={id:surveyID,version:1,definition:{form:{id:surveyID,name:'Post-event learning',audience:'attendee',fields:[{id:questionID,label:'Did you learn something?',type:'single',required:true,help:'Use your own experience, not a team score.',choices:['yes','no']},{id:textID,label:'Private explanation',type:'text',required:false,help:'Only you and current organisers can read this response.',choices:[],visible_when:{field_id:questionID,equals:'yes'}}]},opens_at:'2027-06-21T09:00:00Z',closes_at:'2027-06-28T09:00:00Z',minimum_cohort:5,measures:[]},author_id:person.id,created_at:'2027-06-20T09:00:00Z',approved_at:'2027-06-20T09:05:00Z',published_at:'2027-06-20T09:10:00Z',cancelled:false}
 const state={allowed:true,staff,writable:true,open:true,reportOpen:true,readFailure:false,response:null as Response|null,dropResponse:0,commitDroppedResponse:true,dropConfiguration:0,dropReport:0,commitDroppedReport:true,otherVersion:2,review:null as ReportReview|null,publishedReport:null as ReportReview|null,otherChanged:false,small:false,dropAutomation:0}
 const commands=new Map<string,Response>(), puts:ResponseCommand[]=[],configurationWrites:unknown[]=[],reportWrites:components['schemas']['SurveyReportCommand'][]=[],configurations=new Map<string,Configuration>([[surveyID,configuration]])
 const automation:components['schemas']['AutomationPage']={revision:7,paused:false,bindings:[],occurrences:[],next_binding_cursor:null,next_occurrence_cursor:null}, automationWrites:components['schemas']['AutomationCommand'][]=[]
 const campaignID='46d32a82-0660-4509-8263-a702802a473e'
 const campaign:components['schemas']['CampaignView']={campaign:{definition:{id:campaignID,name:'Reviewed survey invitation',template_id:questionID,audience_id:textID,channels:['in_app'],include_calendar:false,timing:{kind:'fixed',wall_time:'2027-06-21T21:00:00',dst_choice:'',anchor:'',days:0,minutes:0,business_days:false}},version:3,preview_id:questionID,approval:{preview_id:questionID,template_version:1,audience_version:1,fingerprint:'synthetic-current-approved-survey',channel_revisions:{in_app:'synthetic-inbox'},binding:'synthetic-exact',actor_id:person.id,approved_at:'2027-06-20T09:00:00Z',due_at:configuration.definition.opens_at},cancelled:false},status:'approved',due_at:configuration.definition.opens_at,blockers:[]}
 const failure=(code='forbidden')=>({error:{code,message:'Native request is unavailable.',request_id:'synthetic'}})
 function report():components['schemas']['SurveyReport']{return{survey_id:surveyID,survey_version:1,response_visibility:'Responses are identifiable to current organisers, not anonymous.',suppressed:state.small,respondents:state.small?null:12,questions:[{question_id:questionID,question_version:1,label:'Did you learn something?',suppressed:state.small,answered:state.small?null:12,unknown:state.small?null:0,cells:state.small?[]:[{value:'yes',count:6},{value:'no',count:6}],measure:null},{question_id:textID,question_version:1,label:'Private explanation',suppressed:true,answered:null,unknown:null,cells:[],measure:null}],reviewed_summary:'',reviewed_text_version:0,published_at:null}}
 await page.route('**/api/v1/**',async route=>{
  const request=route.request(),url=new URL(request.url()),path=url.pathname,method=request.method()
  if(path==='/api/v1/session')return route.fulfill({json:{person:{...person,platform_admin:false},csrf_token:'synthetic-csrf-token-32-characters',can_create_events:false,draft_scope:'48e4056e-ce21-4b45-8fc1-6dc7dde63572'}})
  if(path==='/api/v1/inbox')return route.fulfill({json:{items:[],next_cursor:null,unread:0}})
  if(path==='/api/v1/events')return route.fulfill({json:{items:[event,{...event,id:secondID,name:'Other edition',can_read_staff:state.staff}],next_cursor:null}})
  if(path===`/api/v1/events/${firstID}`||path===`/api/v1/events/${secondID}`)return state.allowed?route.fulfill({json:{...event,id:path.endsWith(secondID)?secondID:firstID,can_read_staff:state.staff,can_edit:state.staff&&state.writable}}):route.fulfill({status:403,json:failure()})
  if(path===`/api/v1/events/${firstID}/communications/campaigns`)return state.allowed&&state.staff?route.fulfill({json:{revision:automation.revision,timezone:event.configuration.timezone,can_manage:state.writable,campaigns:[campaign],providers:[]}}):route.fulfill({status:403,json:failure()})
  if(path===`/api/v1/events/${firstID}/communications/automation`){
   if(!state.allowed||!state.staff)return route.fulfill({status:403,json:failure()})
   if(method==='GET')return route.fulfill({json:automation})
   if(!state.writable)return route.fulfill({status:403,json:failure()})
   const command=request.postDataJSON() as components['schemas']['AutomationCommand'];automationWrites.push(command)
   if(request.headers()['if-match']!==`"${automation.revision}"`)return route.fulfill({status:409,json:failure('conflict')})
   if(command.action==='save'&&command.definition)automation.bindings.push({definition:command.definition,version:1,campaign_approval:'synthetic-exact',actor_id:person.id,approved_at:null,due_at:configuration.definition.opens_at,cancelled:false,publication_approval_id:null})
   const binding=automation.bindings.find(b=>b.definition.id===command.binding_id)
   if(command.action==='approve'&&binding){binding.approved_at='2027-06-20T10:00:00Z';binding.version++}
   if(command.action==='cancel'&&binding){binding.cancelled=true;binding.version++}
   const occurrence=automation.occurrences.find(o=>o.id===command.occurrence_id)
   if(command.action==='review_occurrence'&&occurrence){occurrence.review_revision++;occurrence.state='pending'}
   if(command.action==='cancel_occurrence'&&occurrence)occurrence.state='cancelled'
   automation.revision++
   if(state.dropAutomation-->0)return route.abort('failed')
   return route.fulfill({json:{page:automation,preview:null}})
  }
  const match=path.match(/^\/api\/v1\/events\/([^/]+)\/surveys(?:\/([^/]+))?(.*)$/)
  if(match){
   if(!state.allowed)return route.fulfill({status:403,json:failure()})
   const eventId=match[1],id=match[2],suffix=match[3],current=configurations.get(id)
   if(state.readFailure&&method==='GET')return route.fulfill({status:503,json:failure('unavailable')})
   if(!id)return route.fulfill({json:{items:state.staff?[]:[{id:surveyID,version:1,name:configuration.definition.form.name,opens_at:configuration.definition.opens_at,closes_at:configuration.definition.closes_at,can_respond:state.open,has_responded:!!state.response}],next_id:null,response_visibility:'Responses are identifiable, not anonymous.'}})
   if(id==='configurations')return state.staff?route.fulfill({json:{items:[{id:surveyID,version:configuration.version,name:configuration.definition.form.name,audience:'attendee',opens_at:configuration.definition.opens_at,closes_at:configuration.definition.closes_at,approved:!!configuration.approved_at,published:!!configuration.published_at,cancelled:configuration.cancelled}],next_id:null,can_manage:state.writable}}):route.fulfill({status:403,json:failure()})
   if(suffix==='/configuration'){
    if(!state.staff)return route.fulfill({status:403,json:failure()})
    if(method==='GET'){
     if(!current)return route.fulfill({status:404,json:failure('not_found')})
     const value=structuredClone(current);if(eventId===secondID){value.version=state.otherVersion;if(state.otherChanged)value.definition.form.fields[0].choices.push('pending')}
     return route.fulfill({json:value})
    }
    if(!state.writable)return route.fulfill({status:403,json:failure()})
    const command=request.postDataJSON() as components['schemas']['SurveyCommand'];configurationWrites.push(command)
    if(Number(request.headers()['if-match']?.replaceAll('"',''))!==(current?.version??0))return route.fulfill({status:409,json:failure('conflict')})
    let value=current
    if(command.action==='save'&&command.definition){value={id,version:(current?.version??0)+1,definition:command.definition,author_id:person.id,created_at:'2027-06-20T09:00:00Z',approved_at:null,published_at:null,cancelled:false};configurations.set(id,value);if(id===surveyID)Object.assign(configuration,value)}
    if(!value)return route.fulfill({status:404,json:failure('not_found')})
    if(command.action==='approve')value.approved_at='2027-06-20T09:05:00Z'
    if(command.action==='publish')value.published_at='2027-06-20T09:10:00Z'
    if(command.action==='cancel')value.cancelled=true
    if(state.dropConfiguration-->0)return route.abort('failed')
    return route.fulfill({json:value})
   }
   if(!current)return route.fulfill({status:404,json:failure('not_found')})
   if(suffix==='/dashboard')return state.staff?route.fulfill({json:{configuration:current,can_manage:state.writable,can_review_report:state.reportOpen&&state.writable,event_revision:7,timezone:event.configuration.timezone,retained_invitations:12,current_eligible_invitations:12,submitted_responses:12,current_eligible_responses:12,observed_at:'2027-06-29T09:00:00Z',response_visibility:'Current organisers only; identifiable responses are not anonymous.'}}):route.fulfill({status:403,json:failure()})
   if(suffix==='/response'&&method==='PUT'){
    if(state.staff||!state.open)return route.fulfill({status:403,json:failure()})
    const command=request.postDataJSON() as ResponseCommand;puts.push(command)
    const previous=commands.get(command.command_id)
    if(previous)return route.fulfill({json:previous})
    if(Number(request.headers()['if-match']?.replaceAll('"',''))!==(state.response?.revision??0))return route.fulfill({status:409,json:failure('conflict')})
    const saved={revision:(state.response?.revision??0)+1,form_version:1,answers:command.answers,submitted_at:'2027-06-22T09:00:00Z'}
    const drop=state.dropResponse-->0
    if(!drop||state.commitDroppedResponse){state.response=saved;commands.set(command.command_id,saved)}
    if(drop)return route.abort('failed')
    return route.fulfill({json:saved})
   }
   if(!suffix)return !state.staff?route.fulfill({json:{id,version:current.version,form:current.definition.form,opens_at:current.definition.opens_at,closes_at:current.definition.closes_at,timezone:event.configuration.timezone,response_visibility:'Responses are identifiable to current event organisers, not anonymous. Other participants cannot read your response.',measures:current.definition.measures,can_respond:state.open&&state.writable&&!current.cancelled,response:state.response}}):route.fulfill({status:403,json:failure()})
   if(suffix==='/responses/export.csv')return state.staff?route.fulfill({headers:{'Content-Type':'text/csv','Content-Disposition':'attachment; filename=restricted-survey-responses.csv'},body:'person_id,answer_json\n'+person.id+','+privateText+'\n'}):route.fulfill({status:403,json:failure()})
   if(suffix==='/responses')return state.staff?route.fulfill({json:{items:[{person_id:person.id,person_name:'Synthetic respondent',response:{revision:1,form_version:1,answers:{[questionID]:'yes',[textID]:privateText},submitted_at:'2027-06-22T09:00:00Z'}}],next_person:null,response_visibility:'Identifiable organisers-only responses.'}}):route.fulfill({status:403,json:failure()})
   if(suffix.includes('/responses/')&&suffix.endsWith('/history'))return state.staff?route.fulfill({json:[{revision:1,form_version:1,answers:{[questionID]:'yes',[textID]:privateText},submitted_at:'2027-06-22T09:00:00Z'}]}):route.fulfill({status:403,json:failure()})
   if(suffix==='/report/review'){
    if(!state.staff)return route.fulfill({status:403,json:failure()})
    if(method==='GET')return state.review?route.fulfill({json:state.review}):route.fulfill({status:404,json:failure('not_found')})
    if(!state.writable||!state.reportOpen)return route.fulfill({status:403,json:failure()})
    const command=request.postDataJSON() as components['schemas']['SurveyReportCommand'];reportWrites.push(command);const drop=state.dropReport-->0,before=structuredClone(state.review),beforePublished=structuredClone(state.publishedReport)
    if(command.action==='review_text'){
     if(state.small&&command.summary)return route.fulfill({status:400,json:failure('invalid_request')})
     if(state.review?.review_id!==command.review_id)state.review={review_id:command.review_id,version:(state.review?.version??0)+1,report:{...report(),reviewed_summary:command.summary.trim()},reviewer_id:person.id,reviewed_at:'2027-06-29T09:00:00Z',approved_at:null,published_at:null}
    }
    if(!state.review)return route.fulfill({status:404,json:failure('not_found')})
    if(command.action==='approve')state.review.approved_at='2027-06-29T09:05:00Z'
    if(command.action==='publish'){state.review.published_at='2027-06-29T09:10:00Z';state.review.report.reviewed_text_version=state.review.version;state.review.report.published_at=state.review.published_at;state.publishedReport=structuredClone(state.review)}
    if(drop){if(!state.commitDroppedReport){state.review=before;state.publishedReport=beforePublished}return route.abort('failed')}
    return route.fulfill({json:state.review})
   }
   if(suffix==='/report'){const value=structuredClone(state.publishedReport?.report??{...report(),reviewed_summary:'Explicitly reviewed common themes.',reviewed_text_version:1,published_at:'2027-06-29T09:10:00Z'});if(eventId===secondID){value.survey_version=state.otherVersion;for(const q of value.questions)q.question_version=state.otherVersion}return route.fulfill({json:value})}
  }
  if(path===`/api/v1/events/${firstID}/objectives`)return state.allowed&&state.staff?route.fulfill({json:{items:[],next_cursor:null}}):route.fulfill({status:403,json:failure()})
  if(path==='/api/v1/time/resolve'){const body=request.postDataJSON();return route.fulfill({json:{instant:new Date(new Date(body.wall_time+'Z').getTime()-12*3_600_000).toISOString()}})}
  return route.fallback()
 })
 return{state,event,configuration,configurations,commands,puts,configurationWrites,reportWrites,automation,automationWrites,campaign}
}

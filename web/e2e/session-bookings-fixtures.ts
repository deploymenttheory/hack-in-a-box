import type { Page, Route } from '@playwright/test'
import type { components } from '../src/api/schema.gen'
import { mockWorkspace, firstID, person } from './fixtures'
export const sessionID='524b0d4d-e4d2-42bf-a873-46751e33264a',attendeeID='b242fb5d-66d9-45c1-a67a-0e055d03115c'
type S=components['schemas']
export async function sessionsFixture(page:Page, role:'attendee'|'facilitator'|'organiser'='attendee'){
 const base=await mockWorkspace(page),event=base.events[0]
 event.can_edit=role==='organiser';event.can_read_staff=role==='organiser';event.can_read_logistics=false;event.can_read_attendance=false;event.can_present=false;event.name='Synthetic session edition';event.stage='preparation';event.configuration.schedule=[{id:sessionID,title:'Synthetic workshop',audience:['attendee'],anchor:'live_start',days:0,minutes:0,business_days:false}]
 const definition:S['SessionDefinition']={capacity:1,duration_minutes:60,room:'Synthetic lab',meeting_url:'https://meeting.example.test/private-session',facilitator_ids:[person.id],booking_opens:{wall:'2027-06-01T09:00',dst_choice:''},booking_closes:{wall:'2027-06-20T09:00',dst_choice:''}}
 const session:S['BookableSession']={id:sessionID,revision:3,calendar_sequence:2,title:'Synthetic workshop',timezone:'Europe/London',start:'2027-06-20T09:00:00Z',end:'2027-06-20T10:00:00Z',booking_opens:'2027-06-01T08:00:00Z',booking_closes:'2027-06-20T08:00:00Z',capacity:1,booked:0,available:1,capacity_shortfall:0,cancelled:false,needs_review:false,room:'Synthetic lab',conflicts:[],can_reserve:role==='attendee',can_manage:role==='organiser',can_read_attendance:role!=='attendee',can_record_attendance:role!=='attendee',...(role==='organiser'?{definition}: {})}
 const state={session,denied:0,rosterDenied:0,originalDenied:0,lost:false,hold:undefined as Promise<void>|undefined,started:undefined as (()=>void)|undefined,fail409:false,exists:true,receiptMismatch:false,receiptAuthorityChange:false,lostBeforeCommit:false,notificationMismatch:'' as ''|'channels'|'approval',roster:[{person_id:attendeeID,display_name:'Synthetic booked attendee',present:false,revision:0}] as S['SessionAttendanceEntry'][],configVersion:0,approved:false,channels:['in_app'] as ('in_app'|'smtp')[]}
 const calls:{path:string;method:string;body?:Record<string,unknown>;revision?:string;csrf?:string}[]=[],receipts=new Map<string,S['SessionBookingReceipt']>()
 const fail=(route:Route,status:number)=>route.fulfill({status,json:{error:{code:status===409?'conflict':status===404?'not_found':'forbidden',request_id:'synthetic-session'}}})
 await page.route(`**/api/v1/events/${firstID}/sessions**`,async route=>{
  const req=route.request(),path=new URL(req.url()).pathname,method=req.method(),body=req.postData()?req.postDataJSON():undefined
  calls.push({path,method,body,revision:req.headers()['if-match'],csrf:req.headers()['x-csrf-token']});const reply=(json:unknown)=>route.fulfill({json,headers:{'Cache-Control':'no-store'}})
  if(state.denied)return fail(route,state.denied)
  if(path.includes('/receipts/')){if(state.receiptAuthorityChange){session.can_record_attendance=true;session.can_read_attendance=true;}if(state.originalDenied)return fail(route,state.originalDenied);const original=receipts.get(path.split('/').at(-1)!);return original?reply({...original,...(state.receiptMismatch?{person_id:person.id===original.person_id?attendeeID:person.id}:{})}):fail(route,404)}
  if(path.endsWith('/attendance'))return state.rosterDenied?fail(route,state.rosterDenied):reply({items:state.roster})
  if(path.includes('/attendance/')){if(state.rosterDenied)return fail(route,state.rosterDenied);const entry=state.roster.find(p=>path.endsWith(p.person_id))!;if(req.headers()['if-match']!==`"${entry.revision}"`)return fail(route,409);entry.present=body.present;entry.revision++;const receipt:S['SessionBookingReceipt']={request_id:body.request_id,session_id:sessionID,person_id:entry.person_id,action:'attendance',state:entry.present?'present':'absent',revision:entry.revision};receipts.set(receipt.request_id,receipt);state.started?.();await state.hold;return state.lost?route.abort('failed'):reply(receipt)}
  const preview=()=>({configuration:{id:state.configVersion?'71e751cc-07c9-41e5-b3fd-e27e5058d244':'00000000-0000-0000-0000-000000000000',version:state.configVersion,channels:state.channels,approved:state.approved},session_revision:session.revision,current_recipients:session.booked,providers:[{channel:'in_app',config_revision:'synthetic-provider-1',available:true,supports_self_test:false,self_test_available:false,reason:'',self_test_reason:'',setup_url:''}],can_approve:state.configVersion>0&&!state.approved,fingerprint:'f'.repeat(64)})
  if(path.endsWith('/notifications/approve')){state.approved=true;return reply({...preview(),...(state.notificationMismatch==='approval'?{configuration:{...preview().configuration,approved:false}}:{})})}
  if(path.endsWith('/notifications')){if(method==='PUT'){state.configVersion++;session.revision++;state.channels=body.channels;state.approved=false;return reply({...preview(),...(state.notificationMismatch==='channels'?{configuration:{...preview().configuration,channels:['smtp']}}:{})})}return reply(preview())}
  if(path.endsWith('/notification-history'))return reply({items:state.approved?[{id:sessionID,sequence:1,recipient_id:attendeeID,configuration_version:state.configVersion,session_revision:session.revision,booking_revision:1,calendar_sequence:session.calendar_sequence,source_state:'approved',reason:'',reports:[{channel:'in_app',state:'queued',queued_intents:1,reason:''}]}]:[]})
  if(path.endsWith('/cancel')){session.cancelled=true;session.revision++;session.calendar_sequence++;return reply(session)}
  if(path.endsWith('/booking')){if(state.lostBeforeCommit)return route.abort('failed');
   if(state.fail409){session.revision++;session.available=0;session.booked=1;return fail(route,409)}
   if(body.session_revision!==session.revision||req.headers()['if-match']!==`"${session.own_booking?.revision??0}"`)return fail(route,409)
   const action=body.action as 'reserve'|'cancel',status=action==='cancel'?'cancelled':session.available?'booked':'waitlisted',revision=(session.own_booking?.revision??0)+1
   session.own_booking={person_id:person.id,state:status,revision,calendar_sequence:session.calendar_sequence+revision,waitlist_position:status==='waitlisted'?1:undefined}
   if(status==='booked'){session.can_reserve=false;session.booked++;session.available=Math.max(0,session.capacity-session.booked);session.meeting_url=definition.meeting_url}if(status==='cancelled'){session.can_reserve=true;session.booked=Math.max(0,session.booked-1);session.available=Math.max(0,session.capacity-session.booked);session.meeting_url=undefined}
   const receipt:S['SessionBookingReceipt']={request_id:body.request_id,session_id:sessionID,person_id:person.id,action,state:status,revision};receipts.set(receipt.request_id,receipt);state.started?.();await state.hold;return state.lost?route.abort('failed'):reply(receipt)
  }
  if(path.endsWith('/sessions'))return reply({items:state.exists&&(new URL(req.url()).searchParams.get('own')!=='true'||session.own_booking)?[session]:[]})
  if(method==='PUT'){state.exists=true;Object.assign(session,{definition:body,revision:session.revision+1,capacity:body.capacity,capacity_shortfall:Math.max(0,session.booked-body.capacity),can_manage:true});return reply(session)}
  return state.exists?reply(session):fail(route,404)
 });return {state,calls,event,receipts}
}

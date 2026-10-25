import { useBlocker } from '@tanstack/react-router'
import { useCallback, useEffect, useRef, useState } from 'react'
import { APIError, api, unwrap, type EventRecord } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { useTodayRead } from './today-read'
import { usePrivateRequestScope, type PrivateRequestScope } from './private-request-scope'
import { useCommunicationCommands } from './communication-tests'
import { Inspector } from './components/patterns'
import { Button } from './components/ui/button'
import { Checkbox, Field, Input } from './components/ui/fields'
import { Failure, Loading } from './components/states'
import { SessionWindowFields, SessionDefinitionSummary, matchSessionDefinition } from './session-definition'
import { SessionBookingError } from './session-booking-receipt'
import { matchSessionNotificationConfiguration, matchSessionNotificationApproval } from './session-notification-receipt'
import { SessionDecision, SessionFacts, SessionFailure, type BookableSession } from './session-booking-view'

export type SessionSetupOutcome = { action: 'Session configuration' | 'Session cancellation' | 'Notification configuration' | 'Notification approval'; sessionID: string; revision: number; configurationVersion?: number }
type Definition = components['schemas']['SessionDefinition']
type Preview = components['schemas']['SessionNotificationPreview']
type Action = 'configure' | 'cancel_session' | 'notification_configuration' | 'notification_approval'
type Review = { action: Action; revision: number; definition?: Definition; channels?: ('smtp' | 'in_app')[]; preview?: Preview }
export function SessionSetup({ event, sessionID, parent, current, back, updated, onDirty, acknowledged }: { event: EventRecord; sessionID: string; parent: PrivateRequestScope; current?: BookableSession; back: () => void; updated: () => void; onDirty?: (dirty: boolean) => void; acknowledged: (outcome: SessionSetupOutcome | undefined) => void }) {
  const load = useCallback(async (signal: AbortSignal) => {
    try { return { session: unwrap(await api.GET('/api/v1/events/{event_id}/sessions/{session_id}', { signal, cache: 'no-store', params: { path: { event_id: event.id, session_id: sessionID } } })) } }
    catch (error) { if (error instanceof APIError && error.status === 404 && event.can_edit && !current) return { session: undefined }; throw error }
  }, [event.id,event.can_edit,sessionID,current])
  const signature = useCallback((value: { session: BookableSession | undefined }) => JSON.stringify([value.session?.can_manage,!!value.session?.definition]), [])
  const read = useTodayRead(`${event.id}:${sessionID}:setup`, load, parent, signature)
  if (read.denied || read.data?.session && !read.data.session.can_manage || !event.can_edit) return <p>Session configuration is no longer authorised.</p>
  if (!read.data) return read.error ? <Failure error={read.error} retry={() => void read.refresh()} /> : <Loading />
  return <SessionSetupEditor event={event} sessionID={sessionID} current={read.data.session} parent={read.authority} paused={read.pending || !!read.error} refresh={() => void read.refresh()} back={back} updated={updated} onDirty={onDirty} acknowledged={acknowledged} />
}
function SessionSetupEditor({ event, sessionID, current, parent, paused, refresh, back, updated, onDirty, acknowledged }: { event: EventRecord; sessionID: string; current?: BookableSession; parent: PrivateRequestScope; paused: boolean; refresh: () => void; back: () => void; updated: () => void; onDirty?: (dirty: boolean) => void; acknowledged: (outcome: SessionSetupOutcome | undefined) => void }) {
  const identity = useIdentity(), scope = usePrivateRequestScope(`${event.id}:${sessionID}:configuration-write`, parent), commands = useCommunicationCommands(`${identity.person.id}:${identity.draft_scope}:${event.id}:session-bookings:commands`)
  const [draft,setDraft] = useState<Definition>(() => structuredClone(current?.definition ?? { capacity: 10, duration_minutes: 60, room: '', meeting_url: '', facilitator_ids: [], booking_opens: { wall: '', dst_choice: '' }, booking_closes: { wall: '', dst_choice: '' } })), [base,setBase] = useState(current?.revision ?? 0), [dirty,setDirty] = useState(false), [review,setReview] = useState<Review>(), [failure,setFailure] = useState<unknown>(), [channels,setChannels] = useState<('smtp'|'in_app')[]>(['in_app']), [channelsDirty,setChannelsDirty] = useState(false), [preview,setPreview] = useState<Preview>(), [history,setHistory] = useState<components['schemas']['SessionNotificationHistory']>(), [reading,setReading] = useState(false)
  const opener = useRef<HTMLElement | null>(null)
  const staffLoad = useCallback(async (signal: AbortSignal, assert:()=>void) => {
    const [staffResult,attendeeResult]=await Promise.allSettled([
      api.GET('/api/v1/events/{event_id}/staff',{signal,cache:'no-store',params:{path:{event_id:event.id}}}).then(unwrap),
      event.can_read_attendance?api.GET('/api/v1/events/{event_id}/attendance',{signal,cache:'no-store',params:{path:{event_id:event.id}}}).then(unwrap):Promise.resolve(undefined),
    ])
    if(staffResult.status==='rejected')throw staffResult.reason
    if(attendeeResult.status==='rejected')throw attendeeResult.reason
    assert()
    const people=new Map<string,{person_id:string;display_name:string;active:boolean;role:string}>()
    for(const person of staffResult.value.items)if(person.active)people.set(person.person_id,{person_id:person.person_id,display_name:person.display_name,active:true,role:person.role})
    for(const person of attendeeResult.value?.items??[])if(person.active&&!people.has(person.person_id))people.set(person.person_id,{person_id:person.person_id,display_name:person.display_name,active:true,role:'confirmed attendee'})
    return {items:[...people.values()]}
  }, [event.id,event.can_read_attendance])
  const staff = useTodayRead(`${event.id}:${sessionID}:session-facilitators`, staffLoad, parent)
  const blocked = commands.attempts.some(a => a.command.resourceID === sessionID && a.blocks), pending = commands.pending, stale = base !== (current?.revision ?? 0), disabled = paused || blocked || reading
  const unsent = dirty || channelsDirty || !!review
  function discardDraft() { return !pending && (!unsent || window.confirm('Discard unsaved session setup inputs and review?')) }
  useBlocker({ shouldBlockFn: () => !discardDraft(), enableBeforeUnload: unsent || pending })
  useEffect(() => { onDirty?.(unsent); return () => onDirty?.(false) }, [unsent,onDirty])
  function changeChannels(channel:'smtp'|'in_app', checked:boolean) { setChannels(value=>checked?[...value,channel]:value.filter(item=>item!==channel)); setChannelsDirty(true); acknowledged(undefined) }
  function change<K extends keyof Definition>(key: K, value: Definition[K]) { setDraft(d => ({...d,[key]:value})); setDirty(true); acknowledged(undefined) }
  function prepare(action: Action, button: HTMLElement) { if (disabled || (action === 'notification_configuration' || action === 'notification_approval') && (!preview || preview.session_revision !== current?.revision)) return; opener.current=button; setReview({ action, revision: current?.revision ?? 0, definition: action === 'configure' ? structuredClone(draft) : undefined, channels: action === 'notification_configuration' ? [...channels] : undefined, preview: (action === 'notification_approval' || action === 'notification_configuration') && preview ? structuredClone(preview) : undefined }) }
  async function notifications(more = false) {
    if (!current || reading || pending) return
    const ticket=scope.capture(); setReading(true); setFailure(undefined)
    try { ticket.assert(); const path={event_id:event.id,session_id:sessionID}
      const fresh=unwrap(await api.GET('/api/v1/events/{event_id}/sessions/{session_id}/notifications',{signal:ticket.signal,cache:'no-store',params:{path}})); ticket.assert(); setPreview(fresh)
      const rows=unwrap(await api.GET('/api/v1/events/{event_id}/sessions/{session_id}/notification-history',{signal:ticket.signal,cache:'no-store',params:{path,query:{limit:25,after:more?history?.next_cursor:undefined}}})); ticket.assert(); setHistory(rows)
    } catch(error) { if(ticket.current()) {setFailure(error); if(error instanceof APIError && [401,403,404].includes(error.status)){setPreview(undefined);setHistory(undefined);scope.invalidate();refresh()} } }
    finally{if(ticket.current())setReading(false)}
  }
  async function confirm() {
    if (!review || disabled || review.revision !== (current?.revision ?? 0) || review.action === 'configure' && (stale || staff.pending || !!staff.error)) return
    const captured=review,ticket=scope.capture(); ticket.assert()
    const id=commands.begin({operation: captured.action === 'configure' ? 'Session configuration' : captured.action === 'cancel_session' ? 'Session cancellation' : 'Session notification decision',revision:captured.revision,command:{action:captured.action,resourceID:sessionID,intentID:crypto.randomUUID()}})
    setFailure(undefined);acknowledged(undefined)
    try {
      ticket.assert(); const params={path:{event_id:event.id,session_id:sessionID},header:{'X-CSRF-Token':identity.csrf_token,'If-Match':`"${captured.revision}"`}}
      if(captured.action==='configure' && captured.definition) {const result=unwrap(await api.PUT('/api/v1/events/{event_id}/sessions/{session_id}',{params,body:captured.definition}));if(result.id!==sessionID || result.revision!==captured.revision+1 || !matchSessionDefinition(result.definition,captured.definition))throw new SessionBookingError('The configuration response did not match the exact reviewed session. Its original outcome remains unknown.');if(ticket.current()){setBase(result.revision);setDirty(false);acknowledged({action:'Session configuration',sessionID,revision:result.revision})}}
      else if(captured.action==='cancel_session'){const result=unwrap(await api.POST('/api/v1/events/{event_id}/sessions/{session_id}/cancel',{params}));if(result.id!==sessionID||!result.cancelled||result.revision!==captured.revision+1)throw new SessionBookingError('The cancellation response did not match the exact reviewed session. Its original outcome remains unknown.');if(ticket.current())acknowledged({action:'Session cancellation',sessionID,revision:result.revision})}
      else if(captured.action==='notification_configuration' && captured.channels && captured.preview){const result=unwrap(await api.PUT('/api/v1/events/{event_id}/sessions/{session_id}/notifications',{params,body:{channels:captured.channels}}));if(!matchSessionNotificationConfiguration(result,captured.preview,captured.channels,captured.revision))throw new SessionBookingError('The notification configuration response did not match the reviewed channels and source. Its original outcome remains unknown.');if(ticket.current()){setChannelsDirty(false);setPreview(result);acknowledged({action:'Notification configuration',sessionID,revision:result.session_revision,configurationVersion:result.configuration.version})}}
      else if(captured.action==='notification_approval' && captured.preview){const result=unwrap(await api.POST('/api/v1/events/{event_id}/sessions/{session_id}/notifications/approve',{params,body:{version:captured.preview.configuration.version,fingerprint:captured.preview.fingerprint}}));if(!matchSessionNotificationApproval(result,captured.preview))throw new SessionBookingError('The notification approval response did not match the captured configuration, source and approval. Its original outcome remains unknown.');if(ticket.current()){setPreview(result);acknowledged({action:'Notification approval',sessionID,revision:result.session_revision,configurationVersion:result.configuration.version})}}
      else throw new Error('The captured review is incomplete.')
      commands.complete(id)
      if(ticket.current()){setReview(undefined);setPreview(undefined);setHistory(undefined);refresh();updated();requestAnimationFrame(()=>opener.current?.focus())}
    }catch(error){if(error instanceof APIError && [400,401,403,404,409,422].includes(error.status))commands.complete(id);else commands.uncertain(id);if(ticket.current()){setReview(undefined);setFailure(error);refresh()}}
  }
  const title=event.configuration.schedule.find(s=>s.id===sessionID)?.title ?? current?.title ?? 'Session configuration'
  return <Inspector identity={sessionID} title={title} scope={{event:event.name,entity:'Organiser session setup'}} back={{label:current?'Back to session':'Back to sessions',href:'#sessions',onNavigate:()=>{if(discardDraft())back()}}}>
    <p>Edit the agenda title, audience and time in event setup. Booking windows use {event.configuration.timezone}. For repeated clock times, choose the earlier or later occurrence.</p>
    {current && <SessionFacts session={current} />}{stale && <p className="notice">The session changed since this editor opened. Your unsent draft remains here. Read and explicitly adopt the current configuration before reviewing a change.</p>}
    <div className="fleet-pattern-actions"><Button icon="refresh" variant="outline" disabled={pending||reading} onClick={refresh}>Read current configuration</Button><Button icon="refresh" variant="outline" disabled={pending||reading||paused} onClick={()=>{if(!dirty||window.confirm('Replace the unsaved draft with current session configuration?')){if(current?.definition)setDraft(structuredClone(current.definition));setBase(current?.revision??0);setDirty(false);setPreview(undefined);setReview(undefined)}}}>Adopt current starting point</Button></div>
    <form hidden={!!review} onSubmit={e=>{e.preventDefault();prepare('configure',(e.nativeEvent as SubmitEvent).submitter as HTMLElement)}}><fieldset className="fleet-planning-authority" disabled={disabled}>
      <Field label="Session capacity" description="Reducing capacity retains existing places and exposes a shortfall."><Input type="number" min={1} max={10000} required value={draft.capacity} onChange={e=>change('capacity',e.target.valueAsNumber)}/></Field>
      <Field label="Duration in minutes"><Input type="number" min={1} max={1440} required value={draft.duration_minutes} onChange={e=>change('duration_minutes',e.target.valueAsNumber)}/></Field>
      <Field label="Room"><Input maxLength={200} value={draft.room} onChange={e=>change('room',e.target.value)}/></Field><Field label="Authorised meeting URL"><Input type="url" maxLength={2000} value={draft.meeting_url} onChange={e=>change('meeting_url',e.target.value)}/></Field>
      <SessionWindowFields definition={draft} change={change} />
      <fieldset><legend>Assigned facilitators</legend>{!!staff.error&&<Failure error={staff.error}/>}<p>Assign current staff or confirmed attendees to this session only. This does not add an event staff role, change event admission or grant unrelated logistics/assessment access.</p>{staff.data?.items.filter(s=>s.active).filter((s,i,all)=>all.findIndex(p=>p.person_id===s.person_id)===i).map(person=><Field key={person.person_id} label={`${person.display_name} · ${person.role}`} description={<span className="fleet-session-reference">Person reference {person.person_id}</span>}><Checkbox disabled={staff.pending||!!staff.error} checked={draft.facilitator_ids.includes(person.person_id)} onChange={e=>change('facilitator_ids',e.target.checked?[...draft.facilitator_ids,person.person_id]:draft.facilitator_ids.filter(id=>id!==person.person_id))}/></Field>)}{[...new Set([...(current?.definition?.facilitator_ids??[]),...draft.facilitator_ids])].filter(id=>!staff.data?.items.some(person=>person.person_id===id)).map(id=><Field key={id} label="Retained facilitator assignment" description={<span className="fleet-session-reference">Person reference {id}; no current name is projected.</span>}><Checkbox disabled={staff.pending||!!staff.error} checked={draft.facilitator_ids.includes(id)} onChange={e=>change('facilitator_ids',e.target.checked?[...draft.facilitator_ids,id]:draft.facilitator_ids.filter(value=>value!==id))}/></Field>)}</fieldset>
      <Button icon="check" type="submit" disabled={stale||staff.pending||!!staff.error}>Review session configuration</Button></fieldset></form>
    {current?.can_manage && <section className="fleet-planning-section"><h3>Session changes and calendar notifications</h3><p>Read the current notification preview before reviewing channels or approval. To leave an unknown original outcome unresolved and prepare a different change, return to the session and review a fresh starting point.</p><Button icon="close" variant="outline" disabled={disabled||current.cancelled} onClick={e=>prepare('cancel_session',e.currentTarget)}>Review session cancellation</Button><p>Cancellation releases session places; it does not withdraw event admission. Parent event read-only policy can suppress notification work.</p>
      <Field label="In-app session notices"><Checkbox checked={channels.includes('in_app')} disabled={disabled} onChange={e=>changeChannels('in_app',e.target.checked)}/></Field><Field label="Email and personal calendar notices"><Checkbox checked={channels.includes('smtp')} disabled={disabled} onChange={e=>changeChannels('smtp',e.target.checked)}/></Field>
      <div className="fleet-pattern-actions"><Button icon="check" variant="outline" disabled={disabled||!preview||preview.session_revision!==current.revision} onClick={e=>prepare('notification_configuration',e.currentTarget)}>Review notification channels</Button><Button icon="refresh" variant="outline" disabled={reading||pending||paused} onClick={()=>void notifications()}>Read notification preview and history</Button></div>
      {preview&&<><p>Configuration {preview.configuration.version} · {preview.configuration.approved?'Approved':'Not approved'} · Current recipients {preview.current_recipients} · Session revision {preview.session_revision}</p><ul>{preview.providers.map((provider,i)=><li key={i}>{provider.channel}: {provider.available ? 'Available' : 'Unavailable'} · {provider.reason || 'No additional reason'} · Provider configuration {provider.config_revision}</li>)}</ul><Button icon="check" disabled={disabled||!preview.can_approve||preview.session_revision!==current.revision} onClick={e=>prepare('notification_approval',e.currentTarget)}>Review exact notification approval</Button></>}
      {history&&<><ul className="fleet-planning-records">{history.items.map(row=><li key={row.id}>Source {row.source_state} · {row.reason||'No additional reason'} · Calendar sequence {row.calendar_sequence}<ul>{row.reports.map((report,i)=><li key={i}>{report.channel}: {report.state} · {report.queued_intents} retained intent(s) · {report.reason}</li>)}</ul></li>)}</ul>{!history.items.length&&<p>No retained source or queue history on this page.</p>}{history.next_cursor&&<Button icon="next" variant="outline" disabled={reading||pending} onClick={()=>void notifications(true)}>Next notification history page</Button>}</>}
    </section>}
    {review&&<SessionDecision title={review.action==='configure'?'Review session configuration':review.action==='cancel_session'?'Review whole-session cancellation':review.action==='notification_configuration'?'Review notification channels':'Review exact notification approval'} pending={pending} disabled={blocked||paused||review.revision!==(current?.revision??0)||review.action==='configure'&&(stale||staff.pending||!!staff.error)} confirm={()=>void confirm()} correct={()=>setReview(undefined)} finalFocus={opener}><h3>{title}</h3><p>Session revision {review.revision} · Event timezone {event.configuration.timezone}</p>{review.definition&&<SessionDefinitionSummary definition={review.definition} people={staff.data?.items.filter(person=>person.active).map(person=>({id:person.person_id,name:`${person.display_name} · ${person.role}`}))}/>}{review.channels&&<p>Channels: {review.channels.join(', ')}</p>}{review.preview&&<p>Configuration {review.preview.configuration.version} · {review.preview.current_recipients} current recipients · Source session revision {review.preview.session_revision}. This approves exactly the current preview fingerprint; it does not claim delivery.</p>}{review.action==='cancel_session'&&<p>Cancel this entire optional session and invalidate future session actions. Calendar updates remain subject to the approved notification and parent-event policies.</p>}</SessionDecision>}
    {!!failure&&<SessionFailure error={failure}/>}
  </Inspector>
}

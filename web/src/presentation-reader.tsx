import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { api, APIError, unwrap } from './api/client'
import { usePrivateRequestScope, PrivateRequestScope } from './private-request-scope'
import type { ConsolePage } from './presentation-model'
export const presentationDenied = (error: unknown) => error instanceof APIError && [401,403,404].includes(error.status)
// Private child snapshots and continuations share this synchronous projection
// lifetime. Route keys alone cannot fence a held preflight/POST continuation.
export class PresentationLifetime {
  private serial=0
  editor: PrivateRequestScope
  constructor(readonly root: PrivateRequestScope){this.editor=new PrivateRequestScope(`${root.key}:editor:0`,root)}
  expire(){this.editor.invalidate();this.editor=new PrivateRequestScope(`${this.root.key}:editor:${++this.serial}`,this.root)}
}
export function usePresentationReader(eventID:string, owner:string){
 const root=usePrivateRequestScope(owner),lifetime=useMemo(()=>new PresentationLifetime(root),[root])
 const [page,setPage]=useState<ConsolePage>(),[error,setError]=useState<unknown>(),[loading,setLoading]=useState(true)
 const current=useRef<ConsolePage|undefined>(undefined),epoch=useRef(0),request=useRef(0)
 const deny=useCallback((failure:unknown)=>{if(!presentationDenied(failure))return;epoch.current++;request.current++;lifetime.expire();current.current=undefined;setPage(undefined);setError(failure);setLoading(false)},[lifetime])
 const accept=useCallback((value:ConsolePage)=>{
   if(current.current?.can_control&&!value.can_control)lifetime.expire()
   // Read-only operating state does not keep former editable drafts/secrets.
   if(current.current&&value.revision<current.current.revision)throw new DOMException('Presentation revision superseded','AbortError')
   const safe=value.can_control?value:{...value,draft:[],pairings:[]}
   current.current=safe;setPage(safe);setError(undefined);setLoading(false)
   return safe
 },[lifetime])
 const read=useCallback(async()=>{
  const ticket=root.capture(),generation=epoch.current,serial=++request.current
  ticket.assert()
  try{
   const value=unwrap(await api.GET('/api/v1/events/{event_id}/presentation',{signal:ticket.signal,cache:'no-store',params:{path:{event_id:eventID}}}));ticket.assert()
   if(generation!==epoch.current||serial!==request.current)throw new DOMException('Presentation read superseded','AbortError')
   return accept(value)
  }catch(failure){if(ticket.current()&&generation===epoch.current){if(presentationDenied(failure))deny(failure);else if(serial===request.current){setError(failure);setLoading(false)}}throw failure}
 },[accept,deny,eventID,root])
 useEffect(()=>{void Promise.resolve().then(read).catch(()=>undefined);const timer=window.setInterval(()=>void read().catch(()=>undefined),2000);const focus=()=>void read().catch(()=>undefined);window.addEventListener('focus',focus);return()=>{window.clearInterval(timer);window.removeEventListener('focus',focus)}},[read])
 useLayoutEffect(()=>{const end=()=>deny(new APIError(401,'presentation_session_ended'));const names=['session-ending','session-expired','mentor-draft-invalidated'];names.forEach(name=>window.addEventListener(name,end));return()=>{names.forEach(name=>window.removeEventListener(name,end));lifetime.editor.invalidate()}},[deny,lifetime])
 return {page,error,loading,read,receive:(value:ConsolePage)=>{request.current++;return accept(value)},deny,lifetime,current:()=>current.current}
}
export type PresentationReader=ReturnType<typeof usePresentationReader>

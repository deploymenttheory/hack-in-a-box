import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { api, unwrap } from './api/client'
import { usePrivateRequestScope } from './private-request-scope'
import { Button } from './components/ui/button'
import { Failure } from './components/states'
import type { components } from './api/schema.gen'
import type { MediaContext } from './media-workspace'
export function ProtectedPhoto({item,context}:{item:components['schemas']['MediaItem'];context:MediaContext}){
 const [mount]=useState(()=>crypto.randomUUID()),scope=usePrivateRequestScope(`${context.lifetime.root.key}:${item.id}:protected:${mount}`,context.lifetime.record(item.id)?.scope)
 const [url,setURL]=useState(''),[error,setError]=useState<unknown>(),[pending,setPending]=useState(false),address=useRef(''),downloads=useRef(new Set<string>()),request=useRef<AbortController|undefined>(undefined),serial=useRef(0)
 const clear=useCallback(()=>{serial.current++;request.current?.abort();request.current=undefined;if(address.current)URL.revokeObjectURL(address.current);address.current='';for(const value of downloads.current)URL.revokeObjectURL(value);downloads.current.clear();setURL('');setPending(false)},[])
 useLayoutEffect(()=>{const events=['session-ending','session-expired','mentor-draft-invalidated','blur','beforeprint'];const hide=()=>clear();for(const name of events)window.addEventListener(name,hide);document.addEventListener('visibilitychange',hide);return()=>{clear();for(const name of events)window.removeEventListener(name,hide);document.removeEventListener('visibilitychange',hide)}},[clear])
 useEffect(()=>{if(!url)return;const timer=window.setTimeout(clear,30_000);return()=>window.clearTimeout(timer)},[url,clear])
 async function read(original:boolean){
  clear();const ticket=scope.capture();if(!ticket.current()||!context.authorityCurrent||!item.can_read_original)return
  const generation=serial.current,abort=new AbortController();request.current=abort;setPending(true);setError(undefined)
  try{
   const path=original?'/api/v1/events/{event_id}/media/{media_id}/original':'/api/v1/events/{event_id}/media/{media_id}/thumbnail'
   const response=await api.GET(path,{signal:abort.signal,cache:'no-store',parseAs:'blob',params:{path:{event_id:context.eventID,media_id:item.id}}});ticket.assert();if(generation!==serial.current||abort.signal.aborted)return
   const blob=unwrap(response),next=URL.createObjectURL(blob)
   if(!ticket.current()||generation!==serial.current){URL.revokeObjectURL(next);return}
   if(original){downloads.current.add(next);const anchor=document.createElement('a');anchor.href=next;anchor.download=`${item.id}-original`;anchor.click();window.setTimeout(()=>{URL.revokeObjectURL(next);downloads.current.delete(next)},1000)}else{address.current=next;setURL(next)}
  }catch(failure){if(ticket.current()&&generation===serial.current){setError(failure);context.deny(failure)}}finally{if(ticket.current()&&generation===serial.current)setPending(false)}
 }
 if(!context.authorityCurrent||!item.can_read_original)return null
 return <div className="media-photo print:hidden py-4"><div className="fleet-pattern-actions"><Button icon="shield" variant="outline" disabled={pending} onClick={()=>void read(false)}>View protected photo</Button><Button icon="download" variant="outline" disabled={pending} onClick={()=>void read(true)}>Download original</Button>{url&&<Button icon="close" variant="ghost" onClick={clear}>Hide photo</Button>}</div>{url&&<img src={url} alt={item.title} className="block max-w-full h-auto py-3"/>}{error!=null&&<Failure error={error}/>}</div>
}

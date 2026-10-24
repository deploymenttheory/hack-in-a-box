import { useId, useRef, type ReactNode } from 'react'
import { Button } from './components/ui/button'
import { LiveMessage } from './components/ui/feedback'
import { Alert } from './components/ui/surfaces'
import { CollectionPagination, CollectionRecords } from './components/patterns'

export type MediaHistoryState =
 | { kind: 'loading' }
 | { kind: 'failure' | 'denied'; message: string }
 | { kind: 'ready'; rows: readonly { version: number; action: string; changed_at: string }[]; busy: boolean; hasNext: boolean }

// Presentation only: the owner supplies current, authorised metadata and callbacks.
// No session, transport, private cache, byte URL or command receipt belongs here.
export function MediaHistoryDisclosure({ open, onOpenChange, children }: { open: boolean; onOpenChange: (open: boolean) => void; children?: ReactNode }) {
 const id=useId(),opener=useRef<HTMLButtonElement>(null)
 function close(){onOpenChange(false);opener.current?.focus()}
 return <section className="media-private print:hidden py-4 border-t border-border"><Button ref={opener} icon="list" variant="outline" aria-expanded={open} aria-controls={open?id:undefined} onClick={()=>open?close():onOpenChange(true)}>{open?'Hide':'Read'} media decision history</Button>{open&&<div id={id}>{children}<Button icon="close" variant="ghost" onClick={close}>Close decision history</Button></div>}</section>
}
export function MediaHistoryContent({ state, retry, next }: { state: MediaHistoryState; retry: () => void; next: () => void }) {
 return <section aria-label="Media decision metadata" className="py-4"><h3>Decision history</h3><p>Version, action and recorded time only. Earlier decisions do not restore consent, approval, gallery release or access to historical photographs. Viewing this history does not confirm an earlier change.</p>{state.kind==='loading'?<LiveMessage>Loading decision metadata…</LiveMessage>:state.kind==='failure'||state.kind==='denied'?<Alert role="alert" tone="destructive"><p>{state.message}</p><Button icon="refresh" onClick={retry}>Try again</Button></Alert>:state.kind==='ready'?<><CollectionRecords label="Authorised media decisions" rows={[...state.rows]} rowKey={row=>String(row.version)} rowLabel={row=>`Version ${row.version}`} columns={[{id:'action',label:'Recorded action',essential:true,cell:row=>row.action.replaceAll('_',' ')},{id:'changed',label:'Recorded time',cell:row=>new Date(row.changed_at).toLocaleString()}]}/>{!state.rows.length&&<p>No earlier decision metadata is available.</p>}<CollectionPagination source={{kind:'cursor',loaded:state.rows.length,busy:state.busy,next:state.hasNext?next:undefined}}/></>:null}</section>
}

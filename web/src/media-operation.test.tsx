import { act, renderHook } from '@testing-library/react'
import type { ReactNode } from 'react'
import { expect, test, vi } from 'vitest'
import { api, APIError, getSession } from './api/client'
import type { components } from './api/schema.gen'
import { CommunicationSafety, useCommunicationCommands } from './communication-tests'
import { MediaLifetime } from './media-lifetime'
import { PrivateRequestScope } from './private-request-scope'
import { useMediaOperation, type MediaOperation } from './media-operation'
import type { MediaContext } from './media-workspace'
vi.mock('@tanstack/react-router',()=>({useBlocker:()=>undefined}))
vi.mock('./session',()=>({useIdentity:()=>({person:{id:'media-owner'},draft_scope:'current-session'})}))
vi.mock('./api/client',async importOriginal=>{const actual=await importOriginal<typeof import('./api/client')>();return {...actual,getSession:vi.fn(),api:{GET:vi.fn(),PUT:vi.fn(),POST:vi.fn()}}})
const item:components['schemas']['MediaItem']={id:'44444444-4444-4444-8444-444444444444',version:1,kind:'recording',title:'Private synthetic title',caption:'Private synthetic caption',captured_at:null,state:'reviewed',consent_state:'reviewed',recording_url:'https://synthetic.example.test/recording',can_read_original:false,can_read_history:true,can_edit:true,can_review:true,can_publish:true,can_withdraw:true,people:[],challenge_related:false}
const page={revision:7,can_collect:true,can_review:true,items:[item],next_cursor:null}
const operation:MediaOperation={kind:'command',id:item.id,revision:7,command:{action:'publish',version:1,command_id:'exact-command',reason:'Private synthetic release reason'}}
const ok=(data:unknown)=>({data,response:new Response(null,{status:200})})
function setup(){
 vi.clearAllMocks()
 vi.mocked(getSession).mockResolvedValue({person:{id:'media-owner'},draft_scope:'current-session',csrf_token:'csrf'} as Awaited<ReturnType<typeof getSession>>)
 vi.mocked(api.GET).mockImplementation(async path=>ok(String(path).endsWith('/media')?page:item) as never)
 const lifetime=new MediaLifetime('synthetic-event',new PrivateRequestScope('synthetic-event'),()=>undefined);lifetime.acceptPage(page)
 const context:MediaContext={eventID:'synthetic-event',revision:7,refresh:vi.fn(async()=>undefined),deny:vi.fn(),writable:true,authorityCurrent:true,lifetime}
 const scope=lifetime.record(item.id)!.scope
 function Wrapper({children}:{children:ReactNode}){return <CommunicationSafety>{children}</CommunicationSafety>}
 return {...renderHook(()=>({write:useMediaOperation(context,scope,item.id),history:useCommunicationCommands('media-owner:current-session:synthetic-event:media:44444444-4444-4444-8444-444444444444')}),{wrapper:Wrapper}),context,lifetime}
}
test('a held session continuation cannot start a media write after current projection loss',async()=>{
 const owner=setup();let finish!:(value:Awaited<ReturnType<typeof getSession>>)=>void
 vi.mocked(getSession).mockImplementation(()=>new Promise(resolve=>{finish=resolve}))
 let pending!:Promise<boolean>;act(()=>{pending=owner.result.current.write.send(operation)})
 owner.lifetime.acceptPage({...page,can_collect:false,items:[{...item,can_edit:false,can_review:false,recording_url:''}]})
 await act(async()=>{finish({person:{id:'media-owner'},draft_scope:'current-session',csrf_token:'csrf'} as Awaited<ReturnType<typeof getSession>>);await pending})
 expect(api.PUT).not.toHaveBeenCalled();expect(api.GET).not.toHaveBeenCalled();expect(owner.result.current.history.attempts).toHaveLength(0)
})
test('408 and stale closures preserve only minimal unknown metadata without replay',async()=>{
 const owner=setup();vi.mocked(api.PUT).mockRejectedValue(new APIError(408,'request_timeout'))
 const original=owner.result.current.write.send
 await act(async()=>{await original(operation);await original({...operation,command:{...operation.command,command_id:'different'}})})
 expect(api.PUT).toHaveBeenCalledOnce();expect(owner.result.current.write.unknown).toBe(true)
 expect(owner.result.current.history.attempts[0].command).toEqual({intentID:'exact-command',action:'publish',revision:1,resourceID:'44444444-4444-4444-8444-444444444444',expectedVersion:2})
 expect(JSON.stringify(owner.result.current.history.attempts)).not.toContain('Private synthetic')
 await act(async()=>{await owner.context.refresh()});expect(owner.result.current.history.unconfirmed).toBe(true)
})
test('original acknowledged version is independent of a newer current safe projection',async()=>{
 const owner=setup();vi.mocked(api.PUT).mockResolvedValue(ok({...item,version:5,acknowledged_version:2,title:'Newer safe title'}) as never)
 await act(async()=>{expect(await owner.result.current.write.send(operation)).toBe(true)})
 expect(owner.result.current.write.state).toMatchObject({kind:'saved',message:'Original intent accepted at media version 2. Current safe projection version 5.'});expect(owner.result.current.history.attempts).toHaveLength(0)
})
test('deliberate native publish retry keeps the exact intent after current can_publish becomes false',async()=>{
 const owner=setup();vi.mocked(api.PUT).mockRejectedValueOnce(new Error('lost response'))
 await act(async()=>{await owner.result.current.write.send(operation)})
 const released={...item,version:2,state:'released' as const,can_publish:false}
 vi.mocked(api.GET).mockImplementation(async path=>ok(String(path).endsWith('/media')?{...page,revision:8,items:[released]}:released) as never)
 vi.mocked(api.PUT).mockResolvedValue(ok({...released,acknowledged_version:2}) as never)
 await act(async()=>{expect(await owner.result.current.write.send()).toBe(true)})
 expect(api.PUT).toHaveBeenCalledTimes(2);expect(vi.mocked(api.PUT).mock.calls[1]).toEqual(vi.mocked(api.PUT).mock.calls[0]);expect(owner.result.current.history.unconfirmed).toBe(false)
})
test('a late exact acknowledgement settles only safe original metadata after private access loss',async()=>{
 const owner=setup();let finish!:(value:unknown)=>void
 vi.mocked(api.PUT).mockImplementation(()=>new Promise(resolve=>{finish=resolve as (value:unknown)=>void}))
 let pending!:Promise<boolean>;await act(async()=>{pending=owner.result.current.write.send(operation);await Promise.resolve();await Promise.resolve()})
 expect(api.PUT).toHaveBeenCalledOnce();owner.lifetime.deny()
 await act(async()=>{finish(ok({...item,acknowledged_version:2,version:2}));await pending})
 expect(owner.result.current.write.state.kind).not.toBe('saved');expect(owner.result.current.history.unconfirmed).toBe(false);expect(owner.context.refresh).not.toHaveBeenCalled()
})

test('a held lost response after projection loss retains minimal unknown metadata instead of an invented acknowledgement',async()=>{
 const owner=setup();let fail!:(error:unknown)=>void
 vi.mocked(api.PUT).mockImplementation(()=>new Promise((_resolve,reject)=>{fail=reject}))
 let pending!:Promise<boolean>;await act(async()=>{pending=owner.result.current.write.send(operation);await Promise.resolve();await Promise.resolve()})
 owner.lifetime.deny();await act(async()=>{fail(new Error('lost response'));await pending})
 expect(owner.result.current.history.unconfirmed).toBe(true);expect(owner.result.current.history.attempts[0].command).toMatchObject({resourceID:item.id,expectedVersion:2});expect(owner.context.refresh).not.toHaveBeenCalled();expect(JSON.stringify(owner.result.current.history.attempts)).not.toContain('Private synthetic')
})

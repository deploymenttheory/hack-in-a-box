import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, expect, test, vi } from 'vitest'
import type { ReactNode } from 'react'
import { APIError, api, getSession } from './api/client'
import { CommunicationSafety, useCommunicationCommands } from './communication-tests'
import { IdentityContext } from './session'
import { usePrivateRequestScope } from './private-request-scope'
import { usePresentationOperation, type PresentationIntent } from './presentation-operation'
import type { PresentationReader } from './presentation-reader'
import type { ConsolePage } from './presentation-model'
vi.mock('./api/client',async original=>({...await original<typeof import('./api/client')>(),api:{GET:vi.fn(),POST:vi.fn()},getSession:vi.fn()}))
const identity={person:{id:'11111111-1111-4111-8111-111111111111',display_name:'Synthetic MC',platform_admin:false},draft_scope:'22222222-2222-4222-8222-222222222222',csrf_token:'synthetic',can_create_events:false}
const event='33333333-3333-4333-8333-333333333333',owner=`${identity.person.id}:${identity.draft_scope}:${event}:presentation`
const page:ConsolePage={revision:7,can_control:true,server_now:'2026-10-24T10:00:00Z',timezone:'UTC',draft:[],published:[],current_index:-1,mode:'held',remaining_seconds:0,timer_ends_at:null,override:null,override_mode:'held',override_remaining_seconds:0,override_timer_ends_at:null,pairings:[]}
const intent:PresentationIntent={kind:'pair',revision:7,source:page,reason:'PRIVATE-REASON-CANARY',label:'PRIVATE-LABEL-CANARY',minutes:120}
function Wrapper({children}:{children:ReactNode}){return <IdentityContext.Provider value={identity}><CommunicationSafety>{children}</CommunicationSafety></IdentityContext.Provider>}
function setup(read=vi.fn().mockResolvedValue(page)){
 const receive=vi.fn(),deny=vi.fn()
 const reader={read,receive,deny} as unknown as PresentationReader
 return {...renderHook(()=>{const scope=usePrivateRequestScope('presentation-operation-test'),history=useCommunicationCommands(owner),operation=usePresentationOperation(event,reader,scope);return{operation,history,scope}},{wrapper:Wrapper}),read,receive,deny}
}
beforeEach(()=>{vi.mocked(api.POST).mockReset();vi.mocked(getSession).mockResolvedValue(identity)})
test('held direct pairing acknowledgement settles only safe root metadata after the private owner ends',async()=>{
 let finish!:(value:unknown)=>void
 vi.mocked(api.POST).mockImplementation(()=>new Promise(resolve=>{finish=resolve}) as ReturnType<typeof api.POST>)
 const {result,receive}=setup();let pending!:Promise<boolean>
 act(()=>{pending=result.current.operation.send(intent)})
 await waitFor(()=>expect(result.current.history.pending).toBe(true))
 const serialized=JSON.stringify(result.current.history.attempts)
 expect(serialized).not.toContain('PRIVATE-');expect(serialized).toContain('Display pairing')
 act(()=>result.current.scope.invalidate())
 await act(async()=>{finish({data:{revision:8,pairing:{id:event},token:'PRIVATE-ONCE-ONLY-SECRET'},response:new Response('',{status:201})});await pending})
 expect(result.current.history.attempts).toEqual([])
 expect(result.current.operation.secret).toBe('');expect(receive).not.toHaveBeenCalled()
})
test('a timeout stays unresolved through current reads and reentrant old closures cannot replay it',async()=>{
 vi.mocked(api.POST).mockRejectedValue(new APIError(408,'timeout'))
 const {result,read}=setup();const original=result.current.operation.send
 await act(async()=>{await original(intent);await original(intent)})
 expect(api.POST).toHaveBeenCalledOnce();expect(result.current.history.unconfirmed).toBe(true)
 read.mockResolvedValue({...page,revision:8})
 await act(async()=>{await read();await original(intent)})
 expect(result.current.history.unconfirmed).toBe(true);expect(api.POST).toHaveBeenCalledOnce()
 expect(JSON.stringify(result.current.history.attempts)).not.toContain('PRIVATE-')
 await act(async()=>{await result.current.operation.newBase()})
 expect(result.current.history.attempts).toHaveLength(1)
 expect(result.current.history.unconfirmed).toBe(false);expect(api.POST).toHaveBeenCalledOnce()
})
test('a held preflight cannot start a write after its private authority lifetime ends',async()=>{
 let finish!:(value:ConsolePage)=>void
 const read=vi.fn(()=>new Promise<ConsolePage>(resolve=>{finish=resolve})),{result}=setup(read)
 let pending!:Promise<boolean>
 act(()=>{pending=result.current.operation.send(intent)})
 await waitFor(()=>expect(read).toHaveBeenCalledOnce())
 act(()=>result.current.scope.invalidate())
 await act(async()=>{finish(page);await pending})
 expect(api.POST).not.toHaveBeenCalled();expect(result.current.history.attempts).toEqual([])
})

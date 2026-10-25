import { act, renderHook } from '@testing-library/react'
import type { ReactNode } from 'react'
import { expect, test, vi } from 'vitest'
import { APIError, type Session } from './api/client'
import { CommunicationSafety, useCommunicationCommands } from './communication-tests'
import { PrivateRequestScope } from './private-request-scope'
import { useResultDecision } from './results-decision'
import { IdentityContext } from './session'
vi.mock('@tanstack/react-router', () => ({ useBlocker: vi.fn() }))
const identity: Session = { person: { id: '11111111-1111-4111-8111-111111111111', display_name: 'Synthetic mentor', platform_admin: false }, draft_scope: '22222222-2222-4222-8222-222222222222', csrf_token: 'synthetic', can_create_events: false }
function wrapper({ children }: { children: ReactNode }) { return <IdentityContext.Provider value={identity}><CommunicationSafety>{children}</CommunicationSafety></IdentityContext.Provider> }
function setup(status = 503) {
 const error = new APIError(status, status === 409 ? 'conflict' : 'unavailable'), parent = new PrivateRequestScope('synthetic-current-assignment'), send = vi.fn().mockRejectedValue(error), page = { revision: 7, version: 0 }
 const owner = 'synthetic-original-advisory', read = vi.fn(async () => page)
 const hook = renderHook(() => ({ decision: useResultDecision({ owner, parent, paused: false, page, revision: p => p.revision, version: p => p.version, read, fingerprint: async () => 'a'.repeat(64), ready: () => {}, send, matches: () => false, contextDenied: error => error instanceof APIError && error.status === 409, refresh: async () => page }), history: useCommunicationCommands(owner) }), { wrapper })
 async function lose(request: string) {
  await act(async () => { await hook.result.current.decision.prepare({ request_id: request, action: 'advise', reason: 'Synthetic private contribution' }, null, document.createElement('button')) })
  await act(async () => { await hook.result.current.decision.confirm() })
  return hook.result.current.history.attempts.find(a => a.command.requestID === request)!.id
 }
 return { ...hook, lose, error, send, parent, read }
}
test('current reads do not acknowledge a request and exact completion clears its obsolete transport failure', async () => {
 const { result, lose, error, send, read } = setup(), original = await lose('33333333-3333-4333-8333-333333333333')
 expect(result.current.decision.failure).toBe(error)
 await act(async () => { await read() })
 expect(result.current.decision.failure).toBe(error)
 expect(result.current.decision.unknown).toBe(true)
 // OriginalResultHistory completes only after matching exact native metadata.
 // Exercise the actual shared-store transition from that separate owner.
 act(() => result.current.history.complete(original))
 expect(result.current.decision.failure).toBeUndefined()
 expect(result.current.decision.failureStage).toBeUndefined()
 expect(send).toHaveBeenCalledOnce()
})
test('acknowledging an earlier attempt cannot dismiss a later unknown failure and session disposal cannot infer success', async () => {
 const { result, lose, error, parent } = setup(), first = await lose('44444444-4444-4444-8444-444444444444')
 await act(async () => { await result.current.decision.readBase(document.createElement('button')) })
 await act(async () => { await result.current.decision.chooseBase() })
 expect(result.current.history.attempts.some(attempt => attempt.id === first && attempt.state === 'unknown')).toBe(true)
 const second = await lose('55555555-5555-4555-8555-555555555555')
 act(() => result.current.history.complete(first))
 expect(result.current.decision.failure).toBe(error)
 act(() => { parent.invalidate(); result.current.history.complete(second) })
 expect(result.current.decision.failure).toBe(error)
 expect(result.current.decision.failureStage).toBe('unknown')
 expect(result.current.decision.notice).not.toMatch(/acknowledged/i)
})

test('a rejected stale write refreshes current facts without ending the still-authorised private editor', async () => {
 const { result } = setup(409)
 const body = { request_id: '66666666-6666-4666-8666-666666666666', action: 'advise', reason: 'Synthetic stale write' }
 await act(async () => { await result.current.decision.prepare(body, null, document.createElement('button')) })
 await act(async () => { await result.current.decision.confirm() })
 expect(result.current.decision.failureStage).toBe('rejected')
 await act(async () => { await result.current.decision.prepare({ ...body, request_id: '77777777-7777-4777-8777-777777777777' }, null, document.createElement('button')) })
 expect(result.current.decision.review?.body.request_id).toBe('77777777-7777-4777-8777-777777777777')
})

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
function setup() {
 const error = new APIError(503, 'unavailable'), parent = new PrivateRequestScope('synthetic-current-assignment'), send = vi.fn().mockRejectedValue(error), page = { revision: 7, version: 0 }
 const owner = 'synthetic-original-advisory', read = vi.fn(async () => page)
 const hook = renderHook(() => ({ decision: useResultDecision({ owner, parent, paused: false, page, revision: p => p.revision, version: p => p.version, read, fingerprint: async () => 'a'.repeat(64), ready: () => {}, send, matches: () => false, refresh: async () => page }), history: useCommunicationCommands(owner) }), { wrapper })
 async function lose(request: string) {
  await act(async () => { await hook.result.current.decision.prepare({ request_id: request, action: 'advise', reason: 'Synthetic private contribution' }, null, document.createElement('button')) })
  await act(async () => { await hook.result.current.decision.confirm() })
  return hook.result.current.history.attempts.find(a => a.command.requestID === request)!.id
 }
 return { ...hook, lose, error, send, parent, read }
}
test('current reads and new bases retain the transport failure until another owner acknowledges that exact original attempt', async () => {
 const { result, lose, error, send, read } = setup(), original = await lose('33333333-3333-4333-8333-333333333333')
 expect(result.current.decision.failure).toBe(error)
 await act(async () => { await read() })
 expect(result.current.decision.failure).toBe(error)
 act(() => result.current.history.selectNewBase())
 expect(result.current.decision.failure).toBe(error)
 // OriginalResultHistory completes only after matching exact native metadata.
 // Exercise the actual shared-store transition from that separate owner.
 act(() => result.current.history.complete(original))
 expect(result.current.decision.failure).toBeUndefined()
 expect(result.current.decision.failureStage).toBeUndefined()
 expect(send).toHaveBeenCalledOnce()
})
test('acknowledging an earlier attempt cannot dismiss a later unknown failure and session disposal cannot infer success', async () => {
 const { result, lose, error, parent } = setup(), first = await lose('44444444-4444-4444-8444-444444444444')
 act(() => result.current.history.selectNewBase())
 const second = await lose('55555555-5555-4555-8555-555555555555')
 act(() => result.current.history.complete(first))
 expect(result.current.decision.failure).toBe(error)
 act(() => { parent.invalidate(); result.current.history.complete(second) })
 expect(result.current.decision.failure).toBe(error)
 expect(result.current.decision.failureStage).toBe('unknown')
 expect(result.current.decision.notice).not.toMatch(/acknowledged/i)
})

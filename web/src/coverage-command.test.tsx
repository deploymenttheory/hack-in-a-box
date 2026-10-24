import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { expect, test, vi } from 'vitest'
import { APIError } from './api/client'
import { useCoveragePage } from './coverage-command'
import { PlanningOperations, usePlanningUnknownCommands } from './planning-operations'

vi.mock('@tanstack/react-router', () => ({ useBlocker: () => undefined }))

vi.mock('./session', () => ({ useIdentity: () => ({ person: { id: 'coverage-person' }, draft_scope: 'current-session' }) }))

function setup(write: (input: unknown, revision: number) => Promise<{ revision: number; can_manage?: boolean }>) {
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } })
  const load = vi.fn().mockResolvedValue({ revision: 7 })
  function Wrapper({ children }: { children: ReactNode }) { return <QueryClientProvider client={cache}><PlanningOperations>{children}</PlanningOperations></QueryClientProvider> }
  return { ...renderHook(() => { const page = useCoveragePage('coverage-event', 'roster', load, write); const history = usePlanningUnknownCommands(page.scopeKey); return { ...page, history } }, { wrapper: Wrapper }), load }
}

test('an ended private session cannot start a roster write even before the page unmounts', async () => {
  const write = vi.fn().mockResolvedValue({ revision: 8 })
  const { result } = setup(write)
  await waitFor(() => expect(result.current.state.kind).toBe('ready'))
  act(() => { window.dispatchEvent(new Event('session-ending')) })
  await act(async () => { await result.current.act({ action: 'publish' }, 7, true) })
  expect(write).not.toHaveBeenCalled()
})

test('a request timeout retains unknown intent through a fresh roster read and never resends', async () => {
  const write = vi.fn().mockRejectedValue(new APIError(408, 'request_timeout'))
  const { result, load } = setup(write)
  await waitFor(() => expect(result.current.state.kind).toBe('ready'))
  await act(async () => { await result.current.act({ action: 'publish' }, 7, true) })
  expect(result.current.command.kind).toBe('unknown')
  const originalIntent = result.current.command.intent
  load.mockResolvedValue({ revision: 8 })
  await act(async () => { await result.current.read() })
  expect(result.current.command).toMatchObject({ kind: 'unknown', intent: originalIntent, currentRevision: 8 })
  await act(async () => { await result.current.act({ action: 'publish' }, 8, true) })
  expect(write).toHaveBeenCalledOnce()
})

test('the original pre-render command closure cannot resend an uncertain write', async () => {
  const write = vi.fn().mockRejectedValue(new Error('connection reset'))
  const { result } = setup(write)
  await waitFor(() => expect(result.current.state.kind).toBe('ready'))
  const originalAct = result.current.act
  await act(async () => { await originalAct({ action: 'publish' }, 7, true); await originalAct({ action: 'publish' }, 7, true) })
  expect(result.current.command.kind).toBe('unknown')
  expect(write).toHaveBeenCalledOnce()
})

test('a late response after session end cannot refill the current roster or announce success', async () => {
  let finish!: (value: { revision: number }) => void
  const write = vi.fn(() => new Promise<{ revision: number }>(resolve => { finish = resolve }))
  const { result } = setup(write)
  await waitFor(() => expect(result.current.state.kind).toBe('ready'))
  let pending!: Promise<boolean>
  act(() => { pending = result.current.act({ action: 'publish' }, 7, true) })
  act(() => { window.dispatchEvent(new Event('session-ending')) })
  await act(async () => { finish({ revision: 8 }); await pending })
  expect(result.current.query.data?.revision).toBe(7)
  expect(result.current.command.kind).not.toBe('saved')
})


test('a fresh restricted projection fences old command closures and late privileged responses', async () => {
  let finish!: (value: { revision: number; can_manage: boolean }) => void
  const write = vi.fn(() => new Promise<{ revision: number; can_manage: boolean }>(resolve => { finish = resolve }))
  const { result, load } = setup(write)
  await waitFor(() => expect(result.current.state.kind).toBe('ready'))
  load.mockResolvedValue({ revision: 7, can_manage: true })
  await act(async () => { await result.current.read() })
  const originalAct = result.current.act
  let pending!: Promise<boolean>
  act(() => { pending = originalAct({ action: 'publish' }, 7, true) })
  load.mockResolvedValue({ revision: 7, can_manage: false })
  await act(async () => { await result.current.read() })
  await act(async () => { finish({ revision: 8, can_manage: true }); await pending; await originalAct({ action: 'publish' }, 7, true) })
  expect(result.current.query.data).toMatchObject({ revision: 7, can_manage: false })
  expect(result.current.command.kind).toBe('unknown')
  expect(write).toHaveBeenCalledOnce()
})

for (const boundary of ['restricted', 401, 403, 404] as const) {
  test(`an earlier private unknown payload is redacted at ${boundary} without losing the write barrier`, async () => {
    const write = vi.fn().mockRejectedValue(new APIError(503, 'unavailable'))
    const { result, load } = setup(write)
    await waitFor(() => expect(result.current.state.kind).toBe('ready'))
    load.mockResolvedValue({ revision: 7, can_manage: true })
    await act(async () => { await result.current.read() })
    const payload = { action: 'profile', availability: 'private-availability', capacity: 19 }
    await act(async () => { await result.current.act(payload, 7, true) })
    expect(JSON.stringify(result.current.history.attempts)).toContain('private-availability')
    if (boundary === 'restricted') load.mockResolvedValue({ revision: 8, can_manage: false })
    else load.mockRejectedValue(new APIError(boundary, 'access_lost'))
    await act(async () => { await result.current.read() })
    expect(JSON.stringify(result.current.history.attempts)).not.toContain('private-availability')
    expect(result.current.history.attempts[0]).toMatchObject({ revision: 7, command: { intentID: expect.any(String) } })
    expect(result.current.history.unconfirmed).toBe(true)
    await act(async () => { await result.current.act(payload, 8, true) })
    expect(write).toHaveBeenCalledOnce()
    expect(result.current.command.kind).not.toBe('saved')
  })
}

test('initial participant projection retains its ordinary unknown join intent through fresh reads', async () => {
  const write = vi.fn().mockRejectedValue(new APIError(503, 'unavailable'))
  const { result, load } = setup(write)
  await waitFor(() => expect(result.current.state.kind).toBe('ready'))
  load.mockResolvedValue({ revision: 7, can_manage: false })
  await act(async () => { await result.current.read() })
  await act(async () => { await result.current.act({ action: 'join', invitation_code: 'ordinary-code' }, 7, true) })
  await act(async () => { await result.current.read() })
  expect(JSON.stringify(result.current.history.attempts)).toContain('ordinary-code')
  expect(result.current.history.unconfirmed).toBe(true)
})

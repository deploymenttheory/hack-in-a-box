import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { expect, test, vi } from 'vitest'
import { APIError } from './api/client'
import { useCoveragePage } from './coverage-command'

vi.mock('./session', () => ({ useIdentity: () => ({ person: { id: 'coverage-person' }, draft_scope: 'current-session' }) }))

function setup(write: (input: unknown, revision: number) => Promise<{ revision: number }>) {
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } })
  const load = vi.fn().mockResolvedValue({ revision: 7 })
  function Wrapper({ children }: { children: ReactNode }) { return <QueryClientProvider client={cache}>{children}</QueryClientProvider> }
  return { ...renderHook(() => useCoveragePage('coverage-event', 'roster', load, write), { wrapper: Wrapper }), load }
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

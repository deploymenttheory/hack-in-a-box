import { act, renderHook } from '@testing-library/react'
import { expect, test, vi } from 'vitest'
import { APIError } from './api/client'
import { useAdmissionCommand } from './registration-command'
const intent = { label: 'Accept offer', resource: 'registration-1', revision: 7 }

test('unknown admission write retains original intent and read-only reconciliation never settles or replays it', async () => {
  const write = vi.fn().mockRejectedValue(new APIError(503, 'unavailable')), confirmed = vi.fn(() => 'Confirmed'), read = vi.fn().mockResolvedValue({ revision: 8, state: 'confirmed' })
  const { result } = renderHook(() => useAdmissionCommand('event:person:registration'))
  await act(async () => { await result.current.run(intent, write, confirmed) })
  expect(result.current.outcome).toEqual({ kind: 'uncertain', intent }); expect(result.current.blocked).toBe(true)
  await act(async () => { await result.current.readCurrent(read); await result.current.run({ ...intent, revision: 8 }, write, confirmed) })
  expect(read).toHaveBeenCalledOnce(); expect(write).toHaveBeenCalledOnce(); expect(confirmed).not.toHaveBeenCalled(); expect(result.current.outcome).toEqual({ kind: 'uncertain', intent })
})

test('stale revision requires a successful read and retained input can be deliberately reviewed again', async () => {
  const { result } = renderHook(() => useAdmissionCommand('stale'))
  const write = vi.fn().mockRejectedValueOnce(new APIError(409, 'revision_conflict')).mockResolvedValueOnce({ revision: 9 })
  await act(async () => { await result.current.run(intent, write, () => 'Saved') })
  expect(result.current.outcome.kind).toBe('stale')
  await act(async () => { await result.current.readCurrent(async () => { throw new Error('offline') }) })
  expect(result.current.blocked).toBe(true)
  await act(async () => { await result.current.readCurrent(async () => ({ revision: 8 })) })
  expect(result.current.blocked).toBe(false)
  await act(async () => { await result.current.run({ ...intent, revision: 8 }, write, () => 'Native revision 9') })
  expect(result.current.outcome).toEqual({ kind: 'confirmed', message: 'Native revision 9' }); expect(write).toHaveBeenCalledTimes(2)
})

test('business conflicts remain correctable and do not pretend to be stale versions', async () => {
  const { result } = renderHook(() => useAdmissionCommand('conflict'))
  await act(async () => { await result.current.run(intent, async () => { throw new APIError(409, 'assessment_conflict') }, () => 'Saved') })
  expect(result.current.outcome.kind).toBe('rejected'); expect(result.current.blocked).toBe(false)
})

test('synchronous double activation sends one request and scope invalidation fences native success effects', async () => {
  let complete!: (value: { revision: number }) => void
  const write = vi.fn(() => new Promise<{ revision: number }>(resolve => { complete = resolve })), confirmed = vi.fn(() => 'Native revision 8')
  const { result } = renderHook(() => useAdmissionCommand('late'))
  let running!: Promise<boolean>
  act(() => { running = result.current.run(intent, write, confirmed); void result.current.run(intent, write, confirmed) })
  expect(write).toHaveBeenCalledOnce()
  act(() => { window.dispatchEvent(new Event('session-ending')) })
  await act(async () => { complete({ revision: 8 }); await running })
  expect(confirmed).not.toHaveBeenCalled(); expect(result.current.outcome.kind).toBe('pending')
})

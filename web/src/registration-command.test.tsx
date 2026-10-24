import { act, render, renderHook, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { expect, test, vi } from 'vitest'
import { APIError } from './api/client'
import { useAdmissionCommand } from './registration-command'
import { PlanningOperations, planningDepartureAllowed } from './planning-operations'
vi.mock('@tanstack/react-router', () => ({ useBlocker: vi.fn() }))
const intent = { label: 'Accept offer', resource: 'registration-1', revision: 7 }

test('event history restores unknown intent after remount and denial destroys the retained capsule', async () => {
  const user = userEvent.setup(), write = vi.fn().mockRejectedValue(new APIError(503, 'unavailable'))
  function Owner() {
    const command = useAdmissionCommand('person:event:form')
    return <><p>{JSON.stringify(command.outcome)}</p><button onClick={() => void command.run(intent, write, () => 'Saved')}>Write</button><button onClick={() => void command.readCurrent(async () => { throw new APIError(403, 'forbidden') })}>Denied read</button></>
  }
  function Workspace() { const [open, setOpen] = useState(true); return <PlanningOperations><button onClick={() => setOpen(value => !value)}>Toggle owner</button>{open && <Owner />}</PlanningOperations> }
  render(<Workspace />)
  await user.click(screen.getByRole('button', { name: 'Write' })); await user.click(screen.getByRole('button', { name: 'Toggle owner' })); await user.click(screen.getByRole('button', { name: 'Toggle owner' }))
  expect(screen.getByText(JSON.stringify({ kind: 'uncertain', intent }))).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Write' })); expect(write).toHaveBeenCalledOnce()
  expect(screen.getByLabelText('Unconfirmed command history')).toHaveTextContent('Expected revision 7')
  await user.click(screen.getByRole('button', { name: 'Denied read' })); await user.click(screen.getByRole('button', { name: 'Toggle owner' })); await user.click(screen.getByRole('button', { name: 'Toggle owner' }))
  expect(screen.queryByLabelText('Unconfirmed command history')).not.toBeInTheDocument()
  expect(screen.getByText(JSON.stringify({ kind: 'idle' }))).toBeVisible()
})

test('a pending admission command rejects local planning departure until the response settles', async () => {
  let finish!: () => void
  const { result } = renderHook(() => useAdmissionCommand('pending:departure'))
  let running!: Promise<boolean>
  act(() => { running = result.current.run(intent, () => new Promise<void>(resolve => { finish = resolve }), () => 'Saved') })
  expect(planningDepartureAllowed()).toBe(false)
  await act(async () => { finish(); await running })
  expect(planningDepartureAllowed()).toBe(true)
})

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

import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { APIError } from './api/client'
import { useTodayRead } from './today-read'

afterEach(cleanup)
function deferred<T>() { let resolve!: (value: T) => void, reject!: (reason: Error) => void; const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no }); return { promise, resolve, reject } }
function Reader({ read }: { read: (signal: AbortSignal, assert: () => void) => Promise<string> }) { const query = useTodayRead('synthetic-actor:session:event', read); return <><p>{query.denied ? 'Access denied' : query.data ?? 'Checking current access'}</p><button onClick={() => void query.refresh()}>Refresh</button></> }

test('a session ending during a held refresh scrubs the prior projection and rejects its late continuation', async () => {
  const held = deferred<string>(), read = vi.fn().mockResolvedValueOnce('Private task canary').mockReturnValue(held.promise)
  render(<Reader read={read} />)
  await screen.findByText('Private task canary')
  fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
  act(() => window.dispatchEvent(new Event('session-ending')))
  expect(screen.queryByText('Private task canary')).not.toBeInTheDocument()
  await act(async () => { held.resolve('Late private task canary') })
  expect(screen.queryByText('Late private task canary')).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
  expect(read).toHaveBeenCalledTimes(2)
})

test('definitive denial discards prior data and a held explicit fresh read cannot borrow it', async () => {
  const held = deferred<string>(), read = vi.fn().mockResolvedValueOnce('Private assigned title').mockRejectedValueOnce(new APIError(403, 'forbidden')).mockReturnValue(held.promise)
  render(<Reader read={read} />)
  await screen.findByText('Private assigned title')
  fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
  await screen.findByText('Access denied')
  fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
  expect(screen.queryByText('Private assigned title')).not.toBeInTheDocument()
  await act(async () => { held.resolve('Independently current public title') })
  await screen.findByText('Independently current public title')
})

import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, expect, test, vi } from 'vitest'
import { TeamsWorkflowSetup } from './teams-workflow-setup'
import { api } from './api/client'

vi.mock('./session', () => ({ useIdentity: () => ({ person: { id: 'organiser' }, csrf_token: 'csrf' }) }))
const safe = { revision: 4, encryption_ready: true, connection: { id: 'connection', version: 1, label: 'Approved channel', owner: 'Owner', co_owner: 'Co-owner', active: true, available: true, updated_at: '2026-10-22T12:00:00Z' }, history: [] }
const success = (data: unknown) => ({ data, response: new Response(null, { status: 200 }) })
afterEach(() => vi.restoreAllMocks())
function mount(data = safe) {
  vi.spyOn(api, 'GET').mockResolvedValue(success(data) as never)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } }), changed = vi.fn()
  const view = render(<QueryClientProvider client={client}><TeamsWorkflowSetup eventID="event" canEdit changed={changed} /></QueryClientProvider>)
  return { client, changed, ...view }
}
test('write-only endpoint clears before a pending save and never enters shared caches or storage', async () => {
  let finish: (value: never) => void = () => {}
  const post = vi.spyOn(api, 'POST').mockImplementation(() => new Promise(resolve => { finish = resolve }) as never)
  const { client, changed } = mount()
  const input = await screen.findByLabelText('New secret workflow endpoint')
  const secret = 'https://synthetic.environment.api.powerplatform.com/?sig=SECRET-CANARY'
  fireEvent.change(input, { target: { value: secret } })
  fireEvent.click(screen.getByRole('button', { name: 'Save Teams connection' }))
  expect(input).toHaveValue('')
  expect(post.mock.calls[0][1]).toMatchObject({ body: { action: 'save', endpoint: secret } })
  expect(JSON.stringify(client.getQueryCache().getAll().map(q => q.state.data))).not.toContain('SECRET-CANARY')
  expect(client.getMutationCache().getAll()).toHaveLength(0)
  expect(document.body).not.toHaveTextContent('SECRET-CANARY')
  expect(JSON.stringify(localStorage)).not.toContain('SECRET-CANARY')
  expect(JSON.stringify(sessionStorage)).not.toContain('SECRET-CANARY')
  finish(success({ ...safe, revision: 5 }) as never)
  await waitFor(() => expect(changed).toHaveBeenCalledOnce())
})
test('fixed shared-channel test requires explicit confirmation and carries no campaign content', async () => {
  const post = vi.spyOn(api, 'POST').mockResolvedValue(success(safe) as never)
  const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
  mount()
  const button = await screen.findByRole('button', { name: 'Queue shared-channel TEST' })
  fireEvent.click(button); expect(post).not.toHaveBeenCalled()
  confirm.mockReturnValue(true); fireEvent.click(button)
  await waitFor(() => expect(post).toHaveBeenCalledOnce())
  expect(post.mock.calls[0][0]).toBe('/api/v1/events/{event_id}/teams-workflow/test')
  expect(post.mock.calls[0][1]).toEqual(expect.objectContaining({ body: { request_id: expect.any(String) } }))
  expect(screen.queryByRole('button', { name: /test to myself/i })).not.toBeInTheDocument()
})
test('unavailable encryption disables endpoint writes and test dispatch', async () => {
  mount({ ...safe, encryption_ready: false, connection: { ...safe.connection, available: false } })
  expect(await screen.findByLabelText('New secret workflow endpoint')).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Queue shared-channel TEST' })).toBeDisabled()
  expect(screen.getByRole('alert')).toHaveTextContent('encryption is unavailable')
})
test('session expiry clears the unsaved endpoint and a failed save does not echo raw provider errors', async () => {
  vi.spyOn(api, 'POST').mockRejectedValue(new Error('sig=SECRET-PROVIDER-ERROR'))
  mount(); const input = await screen.findByLabelText('New secret workflow endpoint')
  fireEvent.change(input, { target: { value: 'SECRET-UNSAVED' } })
  fireEvent(window, new Event('session-expired')); expect(input).toHaveValue('')
  fireEvent.change(input, { target: { value: 'SECRET-UNSAVED' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save Teams connection' }))
  await screen.findByRole('alert'); expect(input).toHaveValue('')
  expect(document.body).not.toHaveTextContent('SECRET-PROVIDER-ERROR')
})

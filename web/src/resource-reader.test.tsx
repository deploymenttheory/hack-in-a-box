import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, expect, test, vi } from 'vitest'
import { IdentityContext } from './session'
import { APIError, privateKey } from './api/client'
import { ResourceReader } from './resource-reader'
import type { ContentPage } from './content-model'
import { Theme } from './components/theme'
import { fixtureEvent, person } from '../e2e/fixtures'

const transport = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }))
vi.mock('./api/client', async original => ({ ...(await original<typeof import('./api/client')>()), api: { GET: transport.get, POST: transport.post } }))
vi.mock('@tanstack/react-router', () => ({ useBlocker: () => undefined }))

const caches: QueryClient[] = []
afterEach(() => { cleanup(); caches.splice(0).forEach(cache => cache.clear()); transport.get.mockReset(); transport.post.mockReset() })

const event = fixtureEvent(), resourceID = '719d60be-7844-4f24-ae5b-055e35005e5a'
function page(markdown = 'PRIVATE CACHED BODY'): ContentPage {
  return { id: resourceID, kind: 'brief', revision: 1, version: 1, state: 'released', document: { title: 'Private team instructions', markdown, audience: { confirmed: true, roles: [], teams: [] }, attachments: ['5ea320af-5711-407f-a4e7-471bc2bde0b9'], scheduled_at: null, require_acknowledgement: true }, released_version: 1, released_at: '2027-06-13T09:00:00Z', acknowledged_at: null, can_manage: false, attachment_details: [{ id: '5ea320af-5711-407f-a4e7-471bc2bde0b9', filename: 'private-brief.txt', media_type: 'text/plain', size: 33, can_download: true }], message: '' }
}
const response = (data: ContentPage) => ({ data, response: new Response(null, { status: 200 }) })
function heldRead() {
  let resolve!: (value: ReturnType<typeof response>) => void, reject!: (error: Error) => void
  const promise = new Promise<ReturnType<typeof response>>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
function setup() {
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 15_000 } } })
  caches.push(cache)
  function Readers({ second = false }: { second?: boolean }) {
    return <QueryClientProvider client={cache}><IdentityContext.Provider value={{ person: { ...person, platform_admin: false }, draft_scope: 'synthetic-reader-session', csrf_token: 'synthetic-csrf-token', can_create_events: false }}><Theme>
      <section aria-label="First reader"><ResourceReader id={resourceID} event={event} back={() => undefined} /></section>
      {second && <section aria-label="Second reader"><ResourceReader id={resourceID} event={event} back={() => undefined} /></section>}
    </Theme></IdentityContext.Provider></QueryClientProvider>
  }
  return { cache, Readers }
}
function expectPrivatePageAbsent(region: HTMLElement) {
  expect(within(region).queryByText('PRIVATE CACHED BODY')).not.toBeInTheDocument()
  expect(within(region).queryByRole('link', { name: 'private-brief.txt' })).not.toBeInTheDocument()
  expect(within(region).queryByRole('button', { name: 'I have read version 1' })).not.toBeInTheDocument()
}

test('a fresh reader cannot use warm private cache as current authority while its own GET is held or denied', async () => {
  const { cache, Readers } = setup(), read = heldRead()
  cache.setQueryData([...privateKey(person.id, event.id, 'resource-reader'), resourceID], page())
  transport.get.mockReturnValue(read.promise)
  render(<Readers />)
  const reader = screen.getByRole('region', { name: 'First reader' })
  expectPrivatePageAbsent(reader)
  await waitFor(() => expect(transport.get).toHaveBeenCalledOnce())
  await act(async () => { read.reject(new APIError(403, 'forbidden')) })
  await within(reader).findByText('You do not have access to this resource.')
  expectPrivatePageAbsent(reader)
})

test('acknowledging one verified reader cannot authorise a simultaneous reader whose GET has not completed', async () => {
  const { Readers } = setup(), secondRead = heldRead()
  transport.get.mockResolvedValueOnce(response(page())).mockReturnValue(secondRead.promise)
  transport.post.mockResolvedValue(response({ ...page(), acknowledged_at: '2027-06-13T10:00:00Z' }))
  const view = render(<Readers />)
  const first = screen.getByRole('region', { name: 'First reader' })
  await within(first).findByText('PRIVATE CACHED BODY')
  view.rerender(<Readers second />)
  await waitFor(() => expect(transport.get).toHaveBeenCalledTimes(2))
  const second = screen.getByRole('region', { name: 'Second reader' })
  expectPrivatePageAbsent(second)
  fireEvent.click(within(first).getByRole('button', { name: 'I have read version 1' }))
  fireEvent.click(await screen.findByRole('button', { name: 'Confirm acknowledgement' }))
  await within(first).findByText(/You acknowledged version 1/)
  expect(transport.post).toHaveBeenCalledOnce()
  expectPrivatePageAbsent(second)
  expect(within(second).queryByText(/You acknowledged version 1/)).not.toBeInTheDocument()
  await act(async () => { secondRead.resolve(response(page('Fresh independently authorised body'))) })
  await within(second).findByText('Fresh independently authorised body')
  expect(within(second).queryByText(/You acknowledged version 1/)).not.toBeInTheDocument()
})

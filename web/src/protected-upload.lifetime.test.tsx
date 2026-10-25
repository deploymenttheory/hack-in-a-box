import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { IdentityContext } from './session'
import { APIError } from './api/client'
import { ProtectedUpload } from './protected-upload'
import { CommunicationSafety, useCommunicationCommandNamespace } from './communication-tests'
import { Theme } from './components/theme'
import { ProtectedUploadSafety } from './protected-upload-safety'
import { planningDepartureAllowed } from './planning-operations'
import { fixtureEvent, person } from '../e2e/fixtures'

const transport = vi.hoisted(() => ({ post: vi.fn() }))
vi.mock('./api/client', async original => ({ ...(await original<typeof import('./api/client')>()), api: { POST: transport.post } }))
vi.mock('@tanstack/react-router', () => ({ useBlocker: () => undefined }))
afterEach(() => { cleanup(); transport.post.mockReset(); vi.restoreAllMocks() })
const event = fixtureEvent(), draftScope = 'synthetic-upload-session'
function Metadata() { const attempts = useCommunicationCommandNamespace(`${person.id}:${draftScope}:protected-uploads`); return <><pre data-testid="safe-memory">{JSON.stringify(attempts)}</pre></> }
function held<T>() { let resolve!: (value: T) => void, reject!: (reason: Error) => void; const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no }); return { promise, resolve, reject } }
function result() { return { data: { id: '84fa2668-0f91-4804-911c-9995c393f06b', event_id: event.id, owner_id: person.id, filename: 'PRIVATE-name.txt', media_type: 'text/plain', size_bytes: 13 }, response: new Response(null, { status: 200 }) } }
function setup() {
 const accepted = vi.fn(), busy = vi.fn()
 function Tree({ visible = true }: { visible?: boolean }) { return <CommunicationSafety><IdentityContext.Provider value={{ person, draft_scope: draftScope, csrf_token: 'synthetic', can_create_events: false }}><Theme><ProtectedUploadSafety><Metadata />{visible && <ProtectedUpload eventID={event.id} revision={7} id="synthetic-upload" label="Evidence" action="Upload selected file" disabled={false} busy={busy} accepted={accepted} />}</ProtectedUploadSafety></Theme></IdentityContext.Provider></CommunicationSafety> }
 return { Tree, accepted }
}
async function upload() {
 fireEvent.change(screen.getByLabelText('Choose evidence'), { target: { files: [new File(['PRIVATE-BYTES'], 'PRIVATE-name.txt', { type: 'text/plain' })] } })
 fireEvent.click(screen.getByRole('button', { name: 'Upload selected file' }))
 await waitFor(() => expect(transport.post).toHaveBeenCalledOnce())
}
test.each(['acknowledged', 'lost'] as const)('owner teardown retains safe upload metadata until the original response is %s', async outcome => {
 const f = setup(), write = held<ReturnType<typeof result>>()
 transport.post.mockReturnValue(write.promise)
 const view = render(<f.Tree />); await upload()
 view.rerender(<f.Tree visible={false} />)
 expect(screen.getByRole('complementary')).toHaveTextContent('Awaiting original acknowledgement')
 expect(screen.getByTestId('safe-memory')).not.toHaveTextContent('PRIVATE')
 expect(planningDepartureAllowed()).toBe(false)
 await act(async () => { if (outcome === 'acknowledged') write.resolve(result()); else write.reject(new APIError(503, 'unavailable')) })
 expect(f.accepted).not.toHaveBeenCalled()
 if (outcome === 'acknowledged') expect(screen.queryByRole('complementary')).not.toBeInTheDocument()
 else {
  expect(screen.getByRole('complementary')).toHaveTextContent('Original outcome remains unconfirmed')
  const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
  expect(planningDepartureAllowed()).toBe(false)
  confirm.mockReturnValue(true); expect(planningDepartureAllowed()).toBe(true)
 }
 expect(transport.post).toHaveBeenCalledOnce()
})

test('deliberate upload recovery repeats the original key and bytes and settles one retained attempt', async () => {
 const f = setup(); transport.post.mockRejectedValueOnce(new APIError(503, 'unavailable')).mockResolvedValueOnce(result())
 render(<f.Tree />); await upload()
 await screen.findByRole('region', { name: 'Unconfirmed upload' })
 expect(screen.getByRole('complementary')).toHaveTextContent('Original outcome remains unconfirmed')
 fireEvent.click(screen.getByRole('button', { name: 'Upload selected file' }))
 await waitFor(() => expect(f.accepted).toHaveBeenCalledOnce())
 expect(transport.post.mock.calls[1]).toEqual(transport.post.mock.calls[0])
 expect(screen.queryByRole('complementary')).not.toBeInTheDocument()
 expect(transport.post).toHaveBeenCalledTimes(2)
})

test('session expiry clears safe upload history and a late response cannot restore it or the attachment', async () => {
 const f = setup(), write = held<ReturnType<typeof result>>()
 transport.post.mockReturnValue(write.promise); render(<f.Tree />); await upload()
 expect(screen.getByRole('complementary')).toHaveTextContent('Awaiting original acknowledgement')
 act(() => { window.dispatchEvent(new Event('session-expired')) })
 expect(screen.queryByRole('complementary')).not.toBeInTheDocument()
 await act(async () => { write.resolve(result()) })
 expect(screen.queryByRole('complementary')).not.toBeInTheDocument()
 expect(f.accepted).not.toHaveBeenCalled()
})

test('an unrelated upload response remains unconfirmed and cannot attach private evidence', async () => {
 const f = setup(); transport.post.mockResolvedValue({ ...result(), data: { ...result().data, event_id: 'aebd84da-f3ee-48e4-b79c-d52302672565' } })
 render(<f.Tree />); await upload()
 await screen.findByRole('region', { name: 'Unconfirmed upload' })
 expect(screen.getByRole('complementary')).toHaveTextContent('Original outcome remains unconfirmed')
 expect(f.accepted).not.toHaveBeenCalled()
 expect(transport.post).toHaveBeenCalledOnce()
})

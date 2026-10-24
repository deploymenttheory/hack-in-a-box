import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterAll, afterEach, expect, test, vi } from 'vitest'
import { SurveyResponseExport } from './survey-export'
import { IdentityContext } from './session'
import { Theme } from './components/theme'
import { PrivateRequestScope } from './private-request-scope'
import { person, firstID } from '../e2e/fixtures'
const transport = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('./api/client', async original => ({ ...(await original<typeof import('./api/client')>()), api: { GET: transport.get } }))
const create = vi.fn(() => 'blob:synthetic-private-export'), revoke = vi.fn(), click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
const originalCreate = URL.createObjectURL, originalRevoke = URL.revokeObjectURL
URL.createObjectURL = create; URL.revokeObjectURL = revoke
afterEach(() => { cleanup(); transport.get.mockReset(); create.mockClear(); revoke.mockClear(); click.mockClear() })
function owner() {
  let release!: () => void
  const held = new Promise<{ data: Blob; response: Response }>(resolve => { release = () => resolve({ data: new Blob(['PRIVATE-EXPORT-CANARY']), response: new Response(null, { status: 200 }) }) })
  transport.get.mockReturnValue(held)
  const authority = new PrivateRequestScope('synthetic-export-authority')
  function View({ disabled = false }: { disabled?: boolean }) { return <IdentityContext.Provider value={{ person, csrf_token: 'synthetic-csrf', draft_scope: 'synthetic-export-session', can_create_events: false }}><Theme><SurveyResponseExport eventId={firstID} surveyId="8d4158fd-3ef9-42eb-9c0c-a064f34db9cd" disabled={disabled} authority={authority} deny={vi.fn()} /></Theme></IdentityContext.Provider> }
  return { View, release, authority }
}
async function begin() {
  fireEvent.click(screen.getByRole('button', { name: 'Review restricted CSV export' }))
  fireEvent.click(await screen.findByRole('button', { name: 'Download restricted responses CSV' }))
  await waitFor(() => expect(transport.get).toHaveBeenCalledOnce())
}
test('a held CSV cannot create a byte URL or download after same-mounted permission loss', async () => {
  const { View, release } = owner(), view = render(<View />); await begin(); view.rerender(<View disabled />)
  await act(async () => { release() })
  expect(create).not.toHaveBeenCalled(); expect(click).not.toHaveBeenCalled(); expect(document.body).not.toHaveTextContent('PRIVATE-EXPORT-CANARY')
})
test('a synchronous parent authority denial fences a held CSV before any prop or child unmount', async () => {
  const { View, release, authority } = owner(); render(<View />); await begin(); authority.invalidate()
  await act(async () => { release() })
  expect(create).not.toHaveBeenCalled(); expect(click).not.toHaveBeenCalled()
})
test('an authorised explicit export uses no-store and immediately revokes its byte URL', async () => {
  const { View, release } = owner(); render(<View />); await begin()
  expect(transport.get.mock.calls[0][1]).toMatchObject({ parseAs: 'blob', cache: 'no-store' })
  await act(async () => { release() })
  expect(create).toHaveBeenCalledOnce(); expect(click).toHaveBeenCalledOnce(); expect(revoke).toHaveBeenCalledWith('blob:synthetic-private-export')
  expect(document.body).not.toHaveTextContent('PRIVATE-EXPORT-CANARY')
})
afterAll(() => { URL.createObjectURL = originalCreate; URL.revokeObjectURL = originalRevoke; click.mockRestore() })

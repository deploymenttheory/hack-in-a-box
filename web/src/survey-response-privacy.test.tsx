import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, expect, test, vi } from 'vitest'
import { IdentityContext } from './session'
import { Theme } from './components/theme'
import { CommunicationSafety } from './communication-tests'
import { SurveyResponsePage } from './survey-response'
import type { components } from './api/schema.gen'
import { person, firstID } from '../e2e/fixtures'

const surveyID = '8d4158fd-3ef9-42eb-9c0c-a064f34db9cd', fieldID = '122700b0-acf4-4e13-ae54-d339af614225'
const transport = vi.hoisted(() => ({ get: vi.fn(), put: vi.fn() }))
vi.mock('./api/client', async original => ({ ...(await original<typeof import('./api/client')>()), api: { GET: transport.get, PUT: transport.put } }))
vi.mock('@tanstack/react-router', () => ({ useParams: () => ({ eventId: firstID, surveyId: '8d4158fd-3ef9-42eb-9c0c-a064f34db9cd' }), useBlocker: () => undefined, Link: ({ children }: { children: React.ReactNode }) => <a href="#survey-test">{children}</a> }))
const caches: QueryClient[] = []
afterEach(() => { cleanup(); caches.splice(0).forEach(cache => cache.clear()); transport.get.mockReset(); transport.put.mockReset() })
function participant(answer: string): components['schemas']['SurveyParticipant'] {
  return { id: surveyID, version: 1, timezone: 'UTC', form: { id: surveyID, name: 'Private own questionnaire', audience: 'attendee', fields: [{ id: fieldID, label: 'Private answer', type: 'text', required: false, help: 'Only you and current organisers.', choices: [] }] }, opens_at: '2027-06-21T09:00:00Z', closes_at: '2027-06-28T09:00:00Z', response_visibility: 'Identifiable, not anonymous.', measures: [], can_respond: true, response: { revision: 1, form_version: 1, answers: { [fieldID]: answer }, submitted_at: '2027-06-22T09:00:00Z' } }
}
const success = (answer: string) => ({ data: participant(answer), response: new Response(null, { status: 200 }) })
function mount() {
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } }); caches.push(cache)
  cache.setQueryData(['private', person.id, 'different-event', 'team-capacity'], { revision: 17, available: 2 })
  const view = render(<QueryClientProvider client={cache}><CommunicationSafety><IdentityContext.Provider value={{ person, csrf_token: 'synthetic-csrf', draft_scope: 'synthetic-survey-session', can_create_events: false }}><Theme><SurveyResponsePage /></Theme></IdentityContext.Provider></CommunicationSafety></QueryClientProvider>)
  return { ...view, cache }
}
test('actual private response owner never puts read answers into QueryClient and leaves unrelated owners intact', async () => {
  transport.get.mockResolvedValue(success('PRIVATE-NATIVE-ANSWER-CANARY'))
  const { cache } = mount(); await screen.findByText('PRIVATE-NATIVE-ANSWER-CANARY')
  expect(JSON.stringify(cache.getQueryCache().getAll().map(query => query.state.data))).not.toContain('PRIVATE-NATIVE-ANSWER-CANARY')
  expect(cache.getQueryData(['private', person.id, 'different-event', 'team-capacity'])).toEqual({ revision: 17, available: 2 })
  expect(localStorage.length).toBe(0); expect(sessionStorage.length).toBe(0)
})
test('a held older private GET cannot restore body or fields after the current owner is denied', async () => {
  let release!: (value: ReturnType<typeof success>) => void
  const held = new Promise<ReturnType<typeof success>>(resolve => { release = resolve })
  transport.get.mockResolvedValueOnce(success('PRIVATE-NATIVE-ANSWER-CANARY')).mockReturnValueOnce(held).mockResolvedValue({ error: { error: { code: 'forbidden', request_id: 'synthetic' } }, response: new Response(null, { status: 403 }) })
  mount(); await screen.findByText('PRIVATE-NATIVE-ANSWER-CANARY')
  fireEvent.click(screen.getByRole('button', { name: 'Refresh survey' })); await waitFor(() => expect(transport.get).toHaveBeenCalledTimes(2))
  fireEvent.click(screen.getByRole('button', { name: 'Refresh survey' })); await screen.findByText('You do not have access to this resource.')
  expect(screen.queryByText('PRIVATE-NATIVE-ANSWER-CANARY')).not.toBeInTheDocument(); expect(screen.queryByLabelText('Private answer (optional)')).not.toBeInTheDocument()
  await act(async () => { release(success('PRIVATE-NATIVE-ANSWER-CANARY')) })
  expect(screen.queryByText('PRIVATE-NATIVE-ANSWER-CANARY')).not.toBeInTheDocument(); expect(screen.queryByLabelText('Private answer (optional)')).not.toBeInTheDocument(); expect(transport.put).not.toHaveBeenCalled()
})

test('changing the actual actor and draft owner starts empty while its fresh read is held', async () => {
  let release!: (value: ReturnType<typeof success>) => void
  const held = new Promise<ReturnType<typeof success>>(resolve => { release = resolve })
  transport.get.mockResolvedValueOnce(success('FIRST-ACTOR-PRIVATE-CANARY')).mockReturnValueOnce(held)
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } }); caches.push(cache)
  function View({ other = false }: { other?: boolean }) { return <QueryClientProvider client={cache}><CommunicationSafety><IdentityContext.Provider value={{ person: other ? { ...person, id: 'de2fbe8b-d593-4b6c-9af2-73fb9ee8b55f' } : person, csrf_token: 'synthetic-csrf', draft_scope: other ? 'second-private-session' : 'first-private-session', can_create_events: false }}><Theme><SurveyResponsePage /></Theme></IdentityContext.Provider></CommunicationSafety></QueryClientProvider> }
  const view = render(<View />); await screen.findByText('FIRST-ACTOR-PRIVATE-CANARY')
  view.rerender(<View other />); await waitFor(() => expect(transport.get).toHaveBeenCalledTimes(2))
  expect(screen.queryByText('FIRST-ACTOR-PRIVATE-CANARY')).not.toBeInTheDocument(); expect(screen.queryByLabelText('Private answer (optional)')).not.toBeInTheDocument()
  await act(async () => { release(success('SECOND-ACTOR-PRIVATE-CANARY')) })
  await screen.findByText('SECOND-ACTOR-PRIVATE-CANARY'); expect(screen.queryByText('FIRST-ACTOR-PRIVATE-CANARY')).not.toBeInTheDocument()
  expect(JSON.stringify(cache.getQueryCache().getAll().map(query => query.state.data))).not.toContain('PRIVATE-CANARY'); expect(transport.put).not.toHaveBeenCalled()
})

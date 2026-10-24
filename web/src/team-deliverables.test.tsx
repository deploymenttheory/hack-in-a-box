import { afterEach, expect, test, vi } from 'vitest'
import { act, cleanup, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { IdentityContext } from './session'
import { TeamDeliverables } from './team-deliverables'
import { privateKey } from './api/client'
import { fixtureEvent, person } from '../e2e/fixtures'
import type { components } from './api/schema.gen'

const transport = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('./api/client', async original => ({ ...(await original<typeof import('./api/client')>()), api: { GET: transport.get } }))
vi.mock('@tanstack/react-router', () => ({ useBlocker: vi.fn() }))
afterEach(() => { cleanup(); vi.unstubAllGlobals(); transport.get.mockReset() })

for (const delayed of [false, true]) test(`current draft authority loss clears ${delayed ? 'late arriving' : 'previously loaded'} private evidence from the DOM and shared cache`, async () => {
  const event = fixtureEvent(), teamID = '00000000-0000-4000-8000-000000000011', definitionID = '00000000-0000-4000-8000-000000000022'
  const secret = 'SYNTHETIC-PRIVATE-DRAFT-CANARY', response = (data: unknown) => ({ data, response: new Response(null, { status: 200 }) })
  const page: components['schemas']['SubmissionPage'] = { event_revision: 1, revision: 0, entry_id: teamID, team_id: teamID, outcome: 'open', frozen_at: null, can_draft: true, can_read_draft: true, can_submit: false, can_review: false, live_start: event.configuration.live_start, cutoff: event.configuration.submission_cutoff, organisers: [], definitions: [{ id: definitionID, title: 'Repository', description: 'Published requirement', required: true, due_at: event.configuration.submission_cutoff, evidence_kinds: ['repository'] }], final: null }
  const draft = { revision: 1, updated_at: null, evidence: [{ deliverable_id: definitionID, kind: 'repository', label: secret, url: 'https://example.test/private', file_id: null }] }
  let finish!: () => void, draftSignal: AbortSignal | undefined
  const deferred = new Promise<ReturnType<typeof response>>(resolve => { finish = () => resolve(response(draft)) })
  transport.get.mockImplementation(async (path: string, options: { signal?: AbortSignal }) => {
    if (path.endsWith('/teams')) return response({ revision: 1, can_manage: false, teams: [{ id: teamID, name: 'Synthetic team', state: 'published', published: [{ id: person.id, name: person.display_name }] }] })
    if (path.endsWith('/preparation/tasks')) return response({ revision: 1, timezone: 'Europe/London', can_manage: false, items: [] })
    if (path.endsWith('/guidance')) return response({ items: [], warnings: [], can_manage: false })
    if (path.endsWith('/draft')) { draftSignal = options.signal; return delayed ? deferred : response(draft) }
    return response(page)
  })
  vi.stubGlobal('matchMedia', () => ({ matches: false, addEventListener() {}, removeEventListener() {} }))
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const view = render(<QueryClientProvider client={cache}><IdentityContext.Provider value={{ person, draft_scope: '22222222-2222-4222-8222-222222222222', csrf_token: 'synthetic-csrf-token', can_create_events: false }}><TeamDeliverables event={event} onDirty={() => undefined} /></IdentityContext.Provider></QueryClientProvider>)
  if (delayed) await waitFor(() => expect(draftSignal).toBeDefined())
  else expect(await screen.findByLabelText('Evidence label for Repository')).toHaveValue(secret)
  await act(async () => { cache.setQueryData(privateKey(person.id, event.id, `submission-${teamID}`), { ...page, can_read_draft: false, can_draft: false }) })
  await waitFor(() => expect(cache.getQueryData(privateKey(person.id, event.id, `submission-draft-${teamID}`))).toBeUndefined())
  if (delayed) { expect(draftSignal?.aborted).toBe(true); await act(async () => { finish(); await deferred }) }
  expect(screen.queryByLabelText('Evidence label for Repository')).not.toBeInTheDocument()
  expect(document.body.textContent).not.toContain(secret)
  expect(JSON.stringify(cache.getQueryCache().getAll().map(query => query.state.data))).not.toContain(secret)
  view.unmount(); cache.clear()
})

import type { ReactNode } from 'react'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, expect, test, vi } from 'vitest'
import { TemplatePage } from './templates'
import { defaultBranding } from './branding'
import { ObjectivesWorkspace } from './objective-workspace'
import { IdentityContext } from './session'
import { api, privateKey, type Session } from './api/client'
import { fixtureEvent, person, secondID } from '../e2e/fixtures'
import { objectiveID, objectiveVersion, plannedDefinition } from '../e2e/objective-fixtures'
vi.mock('@tanstack/react-router', () => ({ useBlocker: () => undefined, useParams: () => ({ templateId: '43333333-3333-4333-8333-333333333333' }), useNavigate: () => vi.fn(), Link: ({ children }: { children: ReactNode }) => <>{children}</> }))
afterEach(() => vi.restoreAllMocks())
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>(r => { resolve = r }); return { promise, resolve } }
const session: Session = { person, draft_scope: '22222222-2222-4222-8222-222222222222', csrf_token: 'synthetic-csrf-token-32-characters', can_create_events: true }
const result = (data: unknown) => ({ data, response: new Response(null, { status: 200 }) })
const deniedResult = () => ({ error: { error: { code: 'forbidden' } }, response: new Response(null, { status: 403 }) })

// The real mounted editor receives late native responses which deliberately
// ignore cancellation: abort cannot undo an already accepted native write.
for (const phase of ['preflight', 'put', 'outcome'] as const) {
  for (const leave of ['event', 'account', 'denial'] as const) {
    test(`${phase} response after ${leave} cannot start a late write or resurrect private cache`, async () => {
      const cache = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: Infinity } } })
      const held = deferred<ReturnType<typeof result>>(), started = deferred<void>()
      const event = fixtureEvent(), prefix = [...privateKey(person.id, event.id, 'objective-planning')]
      const initial = objectiveVersion(objectiveID, 1, plannedDefinition())
      const changed = { ...initial.definition, description: 'PRIVATE LATE ACCEPTED TEXT' }
      const accepted = objectiveVersion(objectiveID, 2, changed, 'Explicit change')
      let holding = false, deny = false, failWrite = phase === 'outcome'
      const readPaths: string[] = []
      vi.spyOn(api, 'GET').mockImplementation((async (path: string, options: { params?: { query?: { version?: number } } } = {}) => {
        readPaths.push(path)
        if (deny) return deniedResult()
        if (holding && ((phase === 'preflight' && path === '/api/v1/events/{event_id}') || (phase === 'outcome' && options.params?.query?.version === 2))) { holding = false; started.resolve(); return held.promise }
        if (path === '/api/v1/events/{event_id}') return result(event)
        if (path.endsWith('objective-owners')) return result({ items: [] })
        if (path.endsWith('/objectives')) return result({ items: [initial], next_cursor: null })
        return result(initial)
      }) as typeof api.GET)
      const put = vi.spyOn(api, 'PUT').mockImplementation((async () => {
        if (failWrite) { failWrite = false; throw new TypeError('response lost after native acceptance') }
        if (phase === 'put') { started.resolve(); return held.promise }
        return result(accepted)
      }) as typeof api.PUT)
      const dirty = vi.fn()
      const view = (identity = session, current = event) => <QueryClientProvider client={cache}><IdentityContext.Provider value={identity}><ObjectivesWorkspace key={`${identity.person.id}:${current.id}`} event={current} onDirty={dirty} /></IdentityContext.Provider></QueryClientProvider>
      const mounted = render(view())
      fireEvent.click(await screen.findByRole('button', { name: 'Open objective' }))
      await screen.findByLabelText('Objective description', { exact: true })
      fireEvent.change(screen.getByLabelText('Objective description', { exact: true }), { target: { value: changed.description } })
      fireEvent.change(screen.getByLabelText('Reason for this definition version'), { target: { value: 'Explicit change' } })
      holding = true
      fireEvent.click(screen.getByRole('button', { name: 'Save definition version' }))
      if (phase === 'outcome') {
        await screen.findByRole('button', { name: 'Check exact save outcome' })
        fireEvent.click(screen.getByRole('button', { name: 'Check exact save outcome' }))
      }
      await started.promise
      if (leave === 'denial') {
        deny = true
        await act(async () => { await cache.invalidateQueries({ queryKey: [...prefix, 'owners'] }) })
        await screen.findByRole('heading', { name: 'Objectives unavailable' })
      } else {
        mounted.rerender(leave === 'event' ? view(session, fixtureEvent(secondID)) : view({ ...session, person: { ...person, id: secondID }, draft_scope: secondID }))
        cache.removeQueries({ queryKey: prefix })
      }
      const readsAtDeparture = readPaths.length
      await act(async () => { held.resolve(result(phase === 'preflight' ? event : accepted)); await held.promise })
      await waitFor(() => expect(put).toHaveBeenCalledTimes(phase === 'preflight' ? 0 : 1))
      expect(cache.getQueryCache().findAll({ queryKey: prefix }).filter(query => query.state.data !== undefined)).toHaveLength(0)
      expect(readPaths.slice(readsAtDeparture).filter(path => path === '/api/v1/events/{event_id}/objectives/{objective_id}')).toHaveLength(0)
      expect(screen.queryByDisplayValue(changed.description)).not.toBeInTheDocument()
      mounted.unmount(); cache.clear()
    })
  }
}

for (const phase of ['preflight', 'response', 'review'] as const) {
  for (const leave of ['account', 'denial'] as const) {
    test(`template ${phase} after ${leave} cannot write late or restore cleared private data`, async () => {
      const templateID = '43333333-3333-4333-8333-333333333333'
      const cache = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: Infinity } } })
      const prefix = ['private', person.id, templateID]
      const initial = { id: templateID, name: 'Private template', revision: 1, version: 1, latest_version: 1, published_version: null, configuration: { event: fixtureEvent().configuration, definitions: { forms: [], tasks: [], deliverables: [], role_briefs: [], rubrics: [], messages: [], branding: defaultBranding, objectives: [] } } }
      const updated = { ...initial, revision: 2, version: 2, latest_version: 2, name: 'PRIVATE LATE TEMPLATE' }
      const held = deferred<ReturnType<typeof result>>(), started = deferred<void>()
      let holding = false, deny = false
      vi.spyOn(api, 'GET').mockImplementation((async () => {
        if (deny) return deniedResult()
        if (holding && phase !== 'response') { holding = false; started.resolve(); return held.promise }
        return result(initial)
      }) as typeof api.GET)
      const put = vi.spyOn(api, 'PUT').mockImplementation((async () => {
        if (phase === 'review') throw new TypeError('write outcome unknown')
        started.resolve(); return held.promise
      }) as typeof api.PUT)
      const post = vi.spyOn(api, 'POST')
      const view = (identity = session) => <QueryClientProvider client={cache}><IdentityContext.Provider value={identity}><TemplatePage key={identity.person.id} /></IdentityContext.Provider></QueryClientProvider>
      const mounted = render(view())
      await screen.findByLabelText('Template name')
      fireEvent.change(screen.getByLabelText('Template name'), { target: { value: updated.name } })
      holding = phase === 'preflight'
      fireEvent.click(screen.getByRole('button', { name: 'Save new draft version' }))
      if (phase === 'review') {
        await screen.findByRole('button', { name: 'Load current template for review' })
        holding = true
        fireEvent.click(screen.getByRole('button', { name: 'Load current template for review' }))
      }
      await started.promise
      if (leave === 'denial') {
        deny = true
        await act(async () => { await cache.invalidateQueries({ queryKey: privateKey(person.id, templateID, 'template') }) })
        await screen.findByRole('heading', { name: 'Template unavailable' })
      } else {
        mounted.rerender(view({ ...session, person: { ...person, id: secondID }, draft_scope: secondID }))
        cache.removeQueries({ queryKey: prefix })
      }
      await act(async () => { held.resolve(result(phase === 'response' ? updated : initial)); await held.promise })
      expect(put).toHaveBeenCalledTimes(phase === 'preflight' ? 0 : 1)
      expect(post).not.toHaveBeenCalled()
      expect(cache.getQueryCache().findAll({ queryKey: prefix }).filter(query => query.state.data !== undefined)).toHaveLength(0)
      expect(screen.queryByDisplayValue(updated.name)).not.toBeInTheDocument()
      mounted.unmount(); cache.clear()
    })
  }
}

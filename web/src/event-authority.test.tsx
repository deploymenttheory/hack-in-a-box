import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider, focusManager } from '@tanstack/react-query'
import { afterEach, expect, test, vi } from 'vitest'
import { useCallback, useLayoutEffect, useState } from 'react'
import { EventReadScopeProvider, useEventAuthority, useEventReadBoundary } from './event-authority'
import { useTodayRead } from './today-read'
import { APIError, api, privateKey, unwrap, type EventRecord } from './api/client'
import { IdentityContext } from './session'
import { fixtureEvent, person, secondID } from '../e2e/fixtures'
import type { components } from './api/schema.gen'
const transport = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('./api/client', async original => ({ ...(await original<typeof import('./api/client')>()), api: { GET: transport.get } }))
const caches: QueryClient[] = []
afterEach(() => { cleanup(); caches.splice(0).forEach(cache => cache.clear()); transport.get.mockReset(); vi.restoreAllMocks(); focusManager.setFocused(undefined) })
function event(canPresent = true): EventRecord { return { ...fixtureEvent(), stage: 'preparation', can_edit: false, can_read_staff: false, can_present: canPresent, can_manage_owner: false, can_transition: false, can_read_review_coverage: false, can_read_attendance: false, can_read_logistics: false, configuration: { ...fixtureEvent().configuration, description: 'PRIVATE PARENT EVENT CANARY' } } }
const response = <T,>(data: T) => ({ data, response: new Response(null, { status: 200 }) })
function today(current: EventRecord, role: 'mc' | 'judge'): components['schemas']['TodayPage'] { return { event: current, roles: [role], participation_state: '', program: { sequence: 0, server_now: '2027-06-12T12:00:00Z', current: null, next: null, mode: 'held', remaining_seconds: 0, timer_ends_at: null, override_active: false } } }
function held<T>() { let resolve!: (value: T) => void, reject!: (error: Error) => void; const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no }); return { promise, resolve, reject } }
function Draft() { const [value, change] = useState(''); return <input aria-label="Unsent draft" value={value} onChange={event => change(event.target.value)} /> }
function TodayProbe() {
 const boundary = useEventReadBoundary(fixtureEvent().id)!, observe = boundary.observeToday
 const load = useCallback(async (signal: AbortSignal, assert: () => void) => { const page = unwrap(await api.GET('/api/v1/events/{event_id}/today', { signal, params: { path: { event_id: fixtureEvent().id } } })); assert(); if (!observe(page.event)) throw new Error('Current parent read required'); return page }, [observe])
 const read = useTodayRead('actual-owned-today-read', load, boundary.scope)
 return <><button onClick={() => void read.refresh()}>Refresh Today</button><p>{read.data ? `Roles: ${read.data.roles.join(',')}` : 'Checking Today'}</p></>
}
function setup(withToday = true) {
 const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } }); caches.push(cache)
 const probe: { current?: ReturnType<typeof useEventAuthority> } = {}
 function Parent() { const authority = useEventAuthority(fixtureEvent().id); useLayoutEffect(() => { probe.current = authority }, [authority]); return <><button onClick={() => void authority.refresh()}>Read parent event</button><p>Parent revision: {authority.query.data?.revision ?? 'none'}</p>{authority.denial ? <p>Parent denied</p> : authority.verified && authority.query.data ? <EventReadScopeProvider value={authority.boundary}><p>{authority.query.data.configuration.description}</p><Draft key={authority.generation} />{withToday && <TodayProbe key={`today:${authority.generation}`} />}</EventReadScopeProvider> : <p>Checking parent</p>}</> }
 const view = render(<QueryClientProvider client={cache}><IdentityContext.Provider value={{ person, draft_scope: 'native-parent-session', csrf_token: 'synthetic', can_create_events: false }}><Parent /></IdentityContext.Provider></QueryClientProvider>)
 return { view, probe, cache }
}

test('equal-revision native parent capability loss fences an old Today read before teardown and cannot be overwritten by it', async () => {
 const initial = event(), current = event(false), parentRead = held<ReturnType<typeof response<EventRecord>>>(), oldToday = held<ReturnType<typeof response<components['schemas']['TodayPage']>>>(); let parentCalls = 0, todayCalls = 0
 transport.get.mockImplementation((path: string) => path.endsWith('/today') ? ++todayCalls === 2 ? oldToday.promise : Promise.resolve(response(today(todayCalls === 1 ? initial : current, todayCalls === 1 ? 'mc' : 'judge'))) : ++parentCalls === 1 ? Promise.resolve(response(initial)) : parentRead.promise)
 const { probe, cache } = setup(); await screen.findByText('Roles: mc')
 const ticket = probe.current!.scope.capture(), invalidate = probe.current!.scope.invalidate; let fencedBeforeTeardown = false
 vi.spyOn(probe.current!.scope, 'invalidate').mockImplementation(() => { const priorDOM = !!screen.queryByText('Roles: mc'); invalidate(); if (priorDOM && !ticket.current()) fencedBeforeTeardown = true })
 fireEvent.click(screen.getByRole('button', { name: 'Refresh Today' })); await waitFor(() => expect(todayCalls).toBe(2))
 fireEvent.click(screen.getByRole('button', { name: 'Read parent event' }))
 await act(async () => { parentRead.resolve(response(current)); await Promise.resolve(); await Promise.resolve(); oldToday.resolve(response(today(initial, 'mc'))) })
 await screen.findByText('Roles: judge')
 expect(fencedBeforeTeardown).toBe(true); expect(ticket.current()).toBe(false)
 expect(screen.queryByText('Roles: mc')).not.toBeInTheDocument()
 expect(cache.getQueryData<EventRecord>(privateKey(person.id, initial.id, 'event'))?.can_present).toBe(false)
 expect(parentCalls).toBe(2)
})

test('unchanged native authority and a metadata revision refresh preserve the actual unsent owner and its live scope', async () => {
 const initial = event(), current = { ...initial, revision: initial.revision + 1 }; let calls = 0
 transport.get.mockImplementation(() => Promise.resolve(response(++calls === 1 ? initial : current)))
 const { probe } = setup(false); await screen.findByRole('textbox', { name: 'Unsent draft' })
 const ticket = probe.current!.scope.capture()
 fireEvent.change(screen.getByRole('textbox', { name: 'Unsent draft' }), { target: { value: 'Keep genuine unsent work' } })
 fireEvent.click(screen.getByRole('button', { name: 'Read parent event' }))
 await screen.findByText('Parent revision: 2')
 expect(screen.getByRole('textbox', { name: 'Unsent draft' })).toHaveValue('Keep genuine unsent work')
 expect(ticket.current()).toBe(true); expect(probe.current!.generation).toBe(0)
})

test('known parent denial scrubs only its cache and remains latched against an old Today response until explicit fresh event success', async () => {
 const initial = event(), current = { ...event(false), configuration: { ...event(false).configuration, description: 'Current authorised event' } }, parentRead = held<ReturnType<typeof response<EventRecord>>>(), oldToday = held<ReturnType<typeof response<components['schemas']['TodayPage']>>>(); let parentCalls = 0, todayCalls = 0
 transport.get.mockImplementation((path: string) => path.endsWith('/today') ? ++todayCalls === 2 ? oldToday.promise : Promise.resolve(response(today(todayCalls === 1 ? initial : current, todayCalls === 1 ? 'mc' : 'judge'))) : ++parentCalls === 1 ? Promise.resolve(response(initial)) : parentCalls === 2 ? parentRead.promise : Promise.resolve(response(current)))
 const { cache } = setup(); await screen.findByText('Roles: mc')
 const other = { name: 'UNRELATED EVENT' }; cache.setQueryData(privateKey(person.id, secondID, 'event'), other)
 fireEvent.click(screen.getByRole('button', { name: 'Refresh Today' })); await waitFor(() => expect(todayCalls).toBe(2))
 fireEvent.click(screen.getByRole('button', { name: 'Read parent event' }))
 await act(async () => { parentRead.reject(new APIError(403, 'forbidden')); await Promise.resolve(); oldToday.resolve(response(today(initial, 'mc'))) })
 await screen.findByText('Parent denied')
 expect(cache.getQueryData(privateKey(person.id, initial.id, 'event'))).toBeNull()
 expect(cache.getQueryData(privateKey(person.id, secondID, 'event'))).toEqual(other)
 expect(screen.queryByText('PRIVATE PARENT EVENT CANARY')).not.toBeInTheDocument()
 act(() => { focusManager.setFocused(false); focusManager.setFocused(true) }); await act(async () => {})
 expect(parentCalls).toBe(2)
 fireEvent.click(screen.getByRole('button', { name: 'Read parent event' }))
 await screen.findByText('Current authorised event'); await screen.findByText('Roles: judge')
 expect(cache.getQueryData<EventRecord>(privateKey(person.id, initial.id, 'event'))?.can_present).toBe(false)
})

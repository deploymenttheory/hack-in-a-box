import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useEffect } from 'react'
import { expect, test } from 'vitest'
import { APIError, privateKey, type Session } from './api/client'
import { IdentityContext } from './session'
import { CommunicationSafety, useCommunicationCommands } from './communication-tests'
import { RepositoryReadAuthority, useRepositoryRead } from './repository-read'
import type { PrivateRequestTicket } from './private-request-scope'
const identity: Session = { person: { id: '11111111-1111-4111-8111-111111111111', display_name: 'Synthetic owner', platform_admin: false }, draft_scope: '22222222-2222-4222-8222-222222222222', csrf_token: 'synthetic', can_create_events: false }
const eventID = '33333333-3333-4333-8333-333333333333'
test('current parent event denial fences an already mounted child ticket synchronously before React teardown and preserves original safe metadata', async () => {
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } }), eventKey = privateKey(identity.person.id, eventID, 'event')
  cache.setQueryData(eventKey, { id: eventID })
  let childMounted = false, takeTicket!: () => PrivateRequestTicket
  function Child() {
    const owner = useRepositoryRead(eventID, 'access:native-team', async () => 'synthetic private projection')
    takeTicket = () => owner.scope.capture()
    useEffect(() => { childMounted = true; return () => { childMounted = false } }, [])
    return owner.query.data ? <p>Child authorised read</p> : <p>Child loading</p>
  }
  function Root() {
    const owner = useRepositoryRead(eventID, 'collection', async () => true)
    if (owner.denied) return <p>Current root denied</p>
    return owner.query.data ? <RepositoryReadAuthority.Provider value={{ scope: owner.scope, paused: owner.paused }}><Child /></RepositoryReadAuthority.Provider> : <p>Root loading</p>
  }
  function History() {
    const owner = useCommunicationCommands(`${identity.person.id}:${identity.draft_scope}:${eventID}:repositories:native-team`)
    return <><button onClick={() => owner.begin({ operation: 'Repository approval', revision: 4, command: { action: 'approve', resourceID: '44444444-4444-4444-8444-444444444444', expectedVersion: 3, intentFingerprint: 'a'.repeat(64) } })}>Retain original metadata</button><output>{JSON.stringify(owner.attempts)}</output></>
  }
  render(<QueryClientProvider client={cache}><IdentityContext.Provider value={identity}><CommunicationSafety><History /><Root /></CommunicationSafety></IdentityContext.Provider></QueryClientProvider>)
  await waitFor(() => expect(screen.getByText('Child authorised read')).toBeVisible())
  fireEvent.click(screen.getByText('Retain original metadata'))
  const ticket = takeTicket(); expect(ticket.current()).toBe(true); expect(childMounted).toBe(true)
  act(() => {
    cache.getQueryCache().find({ queryKey: eventKey, exact: true })!.setState({ status: 'error', error: new APIError(403, 'forbidden') })
    // This assertion is inside the cache notification's act callback: React
    // has not yet torn down Child, so unmount cleanup cannot hide the defect.
    expect(childMounted).toBe(true)
    expect(ticket.current()).toBe(false)
  })
  expect(screen.getByText('Current root denied')).toBeVisible(); expect(childMounted).toBe(false)
  expect(screen.getByRole('status')).toHaveTextContent('"state":"pending"')
  expect(screen.getByRole('status')).toHaveTextContent('"intentFingerprint":"' + 'a'.repeat(64) + '"')
  cache.clear()
})

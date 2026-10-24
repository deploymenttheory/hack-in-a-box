import { act, fireEvent, render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'
import { useState } from 'react'
import { CommunicationSafety, useCommunicationCommands, useCommunicationTests } from './communication-tests'
import { IdentityContext, useIdentity } from './session'
import type { Session } from './api/client'

const identity: Session = { person: { id: '11111111-1111-4111-8111-111111111111', display_name: 'Synthetic organiser', platform_admin: false }, draft_scope: '22222222-2222-4222-8222-222222222222', csrf_token: 'synthetic', can_create_events: false }
const known = { testID: 'native-original-test', campaignID: 'native-campaign', campaignVersion: 3, providerRevision: 'opaque-native-revision', state: 'accepted' as const, body: 'PRIVATE-KNOWN-TEST-CANARY' }

test('original TEST metadata survives presentation teardown and late effects stay in their original event and authenticated session', () => {
  let late: () => void = () => {}
  function Owner({ event }: { event: string }) {
    const tests = useCommunicationTests(event)
    return <><pre>{JSON.stringify(tests.items)}</pre><output>{tests.blocked() ? 'Replacement blocked' : 'No original TEST in this scope'}</output><button onClick={() => { late = () => tests.queued(known); late() }}>Queue original</button></>
  }
  function Workspace() {
    const [event, setEvent] = useState('first'), [visible, setVisible] = useState(true), [actor, setActor] = useState(identity)
    return <CommunicationSafety key={`${actor.person.id}:${actor.draft_scope}`}><IdentityContext.Provider value={actor}><button onClick={() => setEvent(value => value === 'first' ? 'second' : 'first')}>Switch event</button><button onClick={() => setVisible(value => !value)}>Switch sibling</button><button onClick={() => setActor(value => ({ ...value, person: { ...value.person, id: '33333333-3333-4333-8333-333333333333' } }))}>Switch actor</button>{visible && <Owner key={event} event={event} />}</IdentityContext.Provider></CommunicationSafety>
  }
  render(<Workspace />)
  fireEvent.click(screen.getByText('Queue original'))
  fireEvent.click(screen.getByText('Switch sibling')); fireEvent.click(screen.getByText('Switch sibling'))
  expect(screen.getByText('Replacement blocked')).toBeVisible()
  expect(screen.queryByText(/PRIVATE-KNOWN-TEST-CANARY/)).toBeNull()
  fireEvent.click(screen.getByText('Switch event'))
  act(() => late())
  expect(screen.getByText('No original TEST in this scope')).toBeVisible()
  fireEvent.click(screen.getByText('Switch event'))
  expect(screen.getByText('Replacement blocked')).toBeVisible()
  fireEvent.click(screen.getByText('Switch actor'))
  act(() => late())
  expect(screen.getByText('No original TEST in this scope')).toBeVisible()
})

test('pending and unknown original acknowledgements retain only whitelisted metadata across editor keys; current facts cannot settle them', () => {
  let original = '', complete: () => void = () => {}, uncertain: () => void = () => {}
  function Owner() {
    const actor = useIdentity(), history = useCommunicationCommands(`${actor.person.id}:${actor.draft_scope}:event:survey:invitation`)
    const command = { intentID: 'original-local-intent', action: 'save', endpoint: 'SECRET-ENDPOINT-CANARY', reason: 'PRIVATE-REASON-CANARY' }
    return <><output>{history.pending ? 'Awaiting original ACK' : history.unconfirmed ? 'Original ACK unknown' : 'No blocking acknowledgement'}</output><pre>{JSON.stringify(history.attempts)}</pre><button onClick={() => { original = history.begin({ operation: 'Invitation save', revision: 7, command }); complete = () => history.complete(original); uncertain = () => history.uncertain(original) }}>Start original</button><button onClick={history.selectNewBase}>Read current facts and deliberately select base</button></>
  }
  function Workspace() { const [generation, setGeneration] = useState(1); return <CommunicationSafety><IdentityContext.Provider value={identity}><button onClick={() => setGeneration(value => value + 1)}>Replace version and permission editor</button><Owner key={generation} /></IdentityContext.Provider></CommunicationSafety> }
  const view = render(<Workspace />)
  fireEvent.click(screen.getByText('Start original'))
  fireEvent.click(screen.getByText('Read current facts and deliberately select base'))
  expect(screen.getByText('Awaiting original ACK')).toBeVisible()
  fireEvent.click(screen.getByText('Replace version and permission editor'))
  act(() => uncertain())
  expect(screen.getByText('Original ACK unknown')).toBeVisible()
  expect(view.container).not.toHaveTextContent('SECRET-ENDPOINT-CANARY')
  expect(view.container).not.toHaveTextContent('PRIVATE-REASON-CANARY')
  expect(view.container).toHaveTextContent('"revision":7')
  fireEvent.click(screen.getByText('Read current facts and deliberately select base'))
  expect(screen.getByText('No blocking acknowledgement')).toBeVisible()
  expect(view.container).toHaveTextContent('original-local-intent')
  // Only the actual retained original acknowledgement callback removes it.
  act(() => complete())
  expect(view.container).not.toHaveTextContent('original-local-intent')
})

test('media resource metadata accepts only UUIDs and positive safe integer versions and drops private extra fields',()=>{
 function Owner(){const history=useCommunicationCommands('actor:session:media:event:collector');return <><pre>{JSON.stringify(history.attempts)}</pre><button onClick={()=>history.begin({operation:'Media upload',revision:7,command:{intentID:'local-intent',action:'upload',resourceID:'44444444-4444-4444-8444-444444444444',expectedVersion:1,caption:'SECRET-CAPTION',file:{content:'SECRET-BYTES'}} as never})}>Safe media metadata</button><button onClick={()=>history.begin({operation:'Media upload',revision:7,command:{action:'upload',resourceID:'SECRET-URL-OR-TEXT',expectedVersion:Infinity}})}>Invalid media metadata</button></>}
 const view=render(<CommunicationSafety><Owner/></CommunicationSafety>)
 fireEvent.click(screen.getByText('Safe media metadata'));expect(view.container).toHaveTextContent('44444444-4444-4444-8444-444444444444');expect(view.container).toHaveTextContent('"expectedVersion":1')
 fireEvent.click(screen.getByText('Invalid media metadata'));expect(view.container).not.toHaveTextContent('SECRET-');expect(view.container).not.toHaveTextContent('Infinity')
})

test('repository original identity digest survives sibling owners while malformed digest and raw command fields are discarded', () => {
  function Owner() {
    const current = useIdentity(), history = useCommunicationCommands(`${current.person.id}:${current.draft_scope}:repository-native-team`)
    return <><pre>{JSON.stringify(history.attempts)}</pre><button onClick={() => { const id = history.begin({ operation: 'Repository approval', revision: 7, command: { action: 'approve', resourceID: '44444444-4444-4444-8444-444444444444', expectedVersion: 3, intentFingerprint: 'a'.repeat(64), reason: 'RAW-PRIVATE-REASON', private_key: 'RAW-PRIVATE-KEY', members: ['RAW-PRIVATE-PERSON'] } as never }); history.uncertain(id) }}>Keep original repository metadata</button><button onClick={() => history.begin({ operation: 'Repository review', revision: 8, command: { action: 'review', intentFingerprint: 'RAW-PRIVATE-BODY' } })}>Reject malformed repository digest</button></>
  }
  function Workspace() { const [open, setOpen] = useState(true); return <CommunicationSafety><IdentityContext.Provider value={identity}><button onClick={() => setOpen(value => !value)}>Switch repository sibling</button>{open && <Owner />}</IdentityContext.Provider></CommunicationSafety> }
  const view = render(<Workspace />)
  fireEvent.click(screen.getByText('Keep original repository metadata'))
  fireEvent.click(screen.getByText('Reject malformed repository digest'))
  expect(view.container).not.toHaveTextContent('RAW-PRIVATE-')
  fireEvent.click(screen.getByText('Switch repository sibling')); fireEvent.click(screen.getByText('Switch repository sibling'))
  expect(view.container).toHaveTextContent('"intentFingerprint":"' + 'a'.repeat(64) + '"')
  expect(view.container).toHaveTextContent('"state":"unknown"')
  expect(view.container).not.toHaveTextContent('RAW-PRIVATE-')
})

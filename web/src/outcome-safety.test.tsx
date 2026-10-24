import { act, fireEvent, render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'
import { useState } from 'react'
import { OutcomeSafety, outcomeMetadata, useOutcomeAttempts } from './outcome-safety'
import { IdentityContext } from './session'
import type { Session } from './api/client'
import type { NativeIntent } from './outcome-api'

const actor: Session = { person: { id: '11111111-1111-4111-8111-111111111111', display_name: 'Synthetic owner', platform_admin: false }, draft_scope: '22222222-2222-4222-8222-222222222222', csrf_token: 'synthetic', can_create_events: false }
const id = '33333333-3333-4333-8333-333333333333'
const intent: Extract<NativeIntent, {kind: 'outcome'}> = { kind: 'outcome', outcomeID: id, expected: 2, command: { id: '44444444-4444-4444-8444-444444444444', action: 'progress', status: 'closed', reason: 'PRIVATE-REASON-CANARY' } }

test('outcome storage boundary copies only exact native metadata and rejects invalid identities', () => {
  const metadata = outcomeMetadata({ ...intent, privateSnapshot: 'PRIVATE-SNAPSHOT-CANARY' } as NativeIntent)
  expect(metadata).toEqual({ kind: 'outcome', outcomeID: id, expected: 2, commandID: intent.command.id, action: 'progress', state: 'pending' })
  expect(JSON.stringify(metadata)).not.toContain('PRIVATE')
  expect(() => outcomeMetadata({ ...intent, outcomeID: 'PRIVATE-RESOURCE-TEXT' })).toThrow()
  expect(() => outcomeMetadata({ ...intent, expected: NaN })).toThrow()
  expect(() => outcomeMetadata({ ...intent, command: { ...intent.command, action: 'PRIVATE-ACTION' } } as unknown as NativeIntent)).toThrow()
})

test('pending and unknown original effects survive route, source, capability and event association replacement without carrying drafts', () => {
  let unknown = () => {}, complete = () => {}, second = true
  function Owner({ source }: { source: number }) {
    const tracking = useOutcomeAttempts({ kind: 'existing', id, eventID: source > 1 ? 'new-event-association' : undefined })
    return <><output>{tracking.blocked() ? 'Blocked' : 'Clear'}</output><pre>{JSON.stringify(tracking.attempts)}</pre><button onClick={() => { tracking.begin(intent); unknown = () => tracking.uncertain(intent.command.id); complete = () => tracking.complete(intent.command.id) }}>Begin</button><button onClick={() => { second = tracking.begin({ ...intent, command: { ...intent.command, id: '55555555-5555-4555-8555-555555555555' } }) }}>Replacement</button></>
  }
  function Workspace() {
    const [source, setSource] = useState(1), [visible, setVisible] = useState(true), [identity, setIdentity] = useState(actor)
    return <OutcomeSafety key={`${identity.person.id}:${identity.draft_scope}`}><IdentityContext.Provider value={identity}><button onClick={() => setVisible(value => !value)}>Sibling route</button><button onClick={() => setSource(value => value + 1)}>Change source and capability</button><button onClick={() => setIdentity(value => ({ ...value, draft_scope: 'new-authenticated-session' }))}>New session</button>{visible && <Owner key={source} source={source} />}</IdentityContext.Provider></OutcomeSafety>
  }
  render(<Workspace />)
  fireEvent.click(screen.getByText('Begin'))
  fireEvent.click(screen.getByText('Sibling route')); fireEvent.click(screen.getByText('Sibling route'))
  expect(screen.getByText('Blocked')).toBeVisible()
  fireEvent.click(screen.getByText('Change source and capability'))
  act(() => unknown())
  expect(screen.getByText('Blocked')).toBeVisible()
  expect(screen.getByText(/"state":"unknown"/)).toBeVisible()
  fireEvent.click(screen.getByText('Replacement')); expect(second).toBe(false)
  expect(screen.queryByText(/PRIVATE-REASON-CANARY/)).toBeNull()
  act(() => complete()); expect(screen.getByText('Clear')).toBeVisible()
  fireEvent.click(screen.getByText('Begin')); fireEvent.click(screen.getByText('New session'))
  act(() => unknown()); expect(screen.getByText('Clear')).toBeVisible()
})

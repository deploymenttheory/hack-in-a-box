import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { useCallback } from 'react'
import { expect, test, vi } from 'vitest'
import type { Session } from './api/client'
import { CommunicationSafety, useCommunicationCommands } from './communication-tests'
import { Theme } from './components/theme'
import { ConfirmationDialog } from './components/ui/dialog'
import { useResultDecision } from './results-decision'
import { IdentityContext } from './session'
import { useTodayRead } from './today-read'
vi.mock('@tanstack/react-router', () => ({ useBlocker: vi.fn() }))
const identity: Session = { person: { id: '11111111-1111-4111-8111-111111111111', display_name: 'Synthetic judge', platform_admin: false }, draft_scope: '22222222-2222-4222-8222-222222222222', csrf_token: 'synthetic', can_create_events: false }
function held<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>(yes => { resolve = yes }); return { promise, resolve } }
const initial = { revision: 7, version: 0, binding: 'same-context' }
function setup() {
 const waiting = held<typeof initial>(), send = vi.fn(); let reads = 0; let refresh!: () => Promise<unknown>
 const load = async () => ++reads === 1 ? initial : waiting.promise
 function Editor({ reader }: { reader: ReturnType<typeof useTodayRead<typeof initial>> }) {
  const decision = useResultDecision({ owner: 'focus-vote', parent: reader.authority, paused: reader.pending, page: reader.data!, revision: p => p.revision, version: p => p.version, read: load, fingerprint: async () => 'a'.repeat(64), ready: () => {}, send, matches: () => false, refresh: reader.refresh })
  return <><fieldset disabled={decision.blocked} hidden={!!decision.review}><button onClick={event => void decision.prepare({ request_id: '33333333-3333-4333-8333-333333333333', action: 'vote', reason: 'Private current evidence' }, null, event.currentTarget)}>Review tie vote</button></fieldset><h3 ref={decision.headingRef} tabIndex={-1}>Decision status</h3><button>Another destination</button>{decision.review && <ConfirmationDialog open title="Review your tie vote" description="Synthetic independent focus regression" confirmLabel="Record reviewed tie vote" onConfirm={() => void decision.confirm()} onOpenChange={open => { if (!open) decision.cancel() }} finalFocus={decision.returnFocus}>Private current evidence</ConfirmationDialog>}</>
 }
 function Reader() {
  const signature = useCallback((p: typeof initial) => p.binding, []), reader = useTodayRead('focus-reader', load, undefined, signature)
  // Independent history consumer remains mounted across current reads.
  refresh = reader.refresh
  const history = useCommunicationCommands('focus-vote')
  return <><button onClick={() => void reader.refresh()}>Read current tie ballots</button><p>Attempts {history.attempts.length}</p>{reader.data && <Editor key={reader.generation} reader={reader} />}</>
 }
 render(<IdentityContext.Provider value={identity}><CommunicationSafety><Theme><Reader /></Theme></CommunicationSafety></IdentityContext.Provider>)
 return { waiting, send, refresh: () => refresh() }
}
test('cancel restores the reviewed opener after a same-context read temporarily disables it', async () => {
 const { waiting, send, refresh } = setup()
 const opener = await screen.findByRole('button', { name: 'Review tie vote' })
 fireEvent.click(opener)
 await screen.findByRole('alertdialog')
 act(() => { void refresh() })
 expect(opener).toBeDisabled()
 fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
 await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
 await act(async () => { waiting.resolve(initial) })
 await waitFor(() => expect(opener).toBeEnabled())
 await waitFor(() => expect(opener).toHaveFocus())
 expect(send).not.toHaveBeenCalled()
 expect(screen.getByText('Attempts 0')).toBeInTheDocument()
})

test('a deferred return never steals focus after the user chooses another destination', async () => {
 const { waiting, refresh } = setup()
 const opener = await screen.findByRole('button', { name: 'Review tie vote' })
 fireEvent.click(opener); await screen.findByRole('alertdialog')
 act(() => { void refresh() }); fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
 await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
 const other = screen.getByRole('button', { name: 'Another destination' }); other.focus()
 await act(async () => { waiting.resolve(initial) })
 await waitFor(() => expect(opener).toBeEnabled())
 expect(other).toHaveFocus()
})
test('a changed private source discards deferred focus instead of targeting the replacement editor', async () => {
 const { waiting, refresh } = setup()
 const original = await screen.findByRole('button', { name: 'Review tie vote' })
 fireEvent.click(original); await screen.findByRole('alertdialog')
 act(() => { void refresh() }); fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
 await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
 await act(async () => { waiting.resolve({ ...initial, binding: 'replacement-source' }) })
 await waitFor(() => expect(original.isConnected).toBe(false))
 expect(screen.getByRole('button', { name: 'Review tie vote' })).not.toHaveFocus()
})

test('ordinary cancel without a current-read overlap returns focus immediately', async () => {
 setup()
 const opener = await screen.findByRole('button', { name: 'Review tie vote' })
 fireEvent.click(opener); await screen.findByRole('alertdialog')
 fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
 await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
 await waitFor(() => expect(opener).toHaveFocus())
})

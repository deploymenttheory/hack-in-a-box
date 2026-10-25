import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { PlanningOperations } from './planning-operations'
import { PrivateRequestScope } from './private-request-scope'
import { useResourceOperation } from './resource-operation'
import { CommunicationSafety, CommunicationCommandHistory, useCommunicationCommandNamespace, useCommunicationCommands } from './communication-tests'
vi.mock('@tanstack/react-router', () => ({ useBlocker: () => undefined }))
afterEach(cleanup)
const owner = 'actor:session:event:preparation:task'
function held() { let resolve!: () => void, reject!: (error: Error) => void; const promise = new Promise<void>((yes, no) => { resolve = yes; reject = no }); return { promise, resolve, reject } }
function Owner({ parent, write, accept }: { parent: PrivateRequestScope; write: () => Promise<void>; accept: () => string }) {
 const safeCommands = useCommunicationCommands(owner), command = useResourceOperation(owner, undefined, { parentScope: parent, safeCommands })
 return <><button onClick={() => void command.run({ operation: 'Save personal preparation', revision: 7, command: { reason: 'PRIVATE REASON CANARY', evidence: 'PRIVATE EVIDENCE CANARY' } }, write, accept)}>Send original</button><button onClick={() => void command.read(async () => 'Current state', () => undefined)}>Read current</button></>
}
function Metadata() { const attempts = useCommunicationCommandNamespace('actor:session:event:preparation'); return <><CommunicationCommandHistory attempts={attempts} /><pre>{JSON.stringify(attempts)}</pre></> }
function Wrapper({ visible, parent, write, accept }: { visible: boolean; parent: PrivateRequestScope; write: () => Promise<void>; accept: () => string }) { return <CommunicationSafety><Metadata /><PlanningOperations key={visible ? 'organiser' : 'judge'}>{visible ? <Owner parent={parent} write={write} accept={accept} /> : <p>Organiser capability revoked</p>}</PlanningOperations></CommunicationSafety> }

test('original direct acknowledgement settles safe metadata after capability-keyed private teardown without accepting private output', async () => {
 const parent = new PrivateRequestScope('actor:session:event:role'), pending = held(), accept = vi.fn(() => 'Private saved output'), props = { parent, write: () => pending.promise, accept }
 const view = render(<Wrapper visible {...props} />)
 fireEvent.click(screen.getByRole('button', { name: 'Send original' }))
 await screen.findByText(/Awaiting original acknowledgement/)
 expect(document.body).not.toHaveTextContent('PRIVATE REASON CANARY'); expect(document.body).not.toHaveTextContent('PRIVATE EVIDENCE CANARY')
 act(() => parent.invalidate())
 view.rerender(<Wrapper visible={false} {...props} />)
 expect(screen.getByRole('complementary', { name: 'Unconfirmed command history' })).toHaveTextContent('Awaiting original acknowledgement')
 await act(async () => pending.resolve())
 expect(accept).not.toHaveBeenCalled()
 expect(screen.queryByRole('complementary', { name: 'Unconfirmed command history' })).not.toBeInTheDocument()
})

test('fenced lost response retains safe original revision after capability teardown and current reads cannot settle it', async () => {
 const parent = new PrivateRequestScope('actor:session:event:role'), pending = held(), accept = vi.fn(() => 'Private output'), props = { parent, write: () => pending.promise, accept }
 const view = render(<Wrapper visible {...props} />)
 fireEvent.click(screen.getByRole('button', { name: 'Send original' }))
 act(() => parent.invalidate())
 view.rerender(<Wrapper visible={false} {...props} />)
 await act(async () => pending.reject(new Error('Response lost')))
 expect(accept).not.toHaveBeenCalled()
 const history = screen.getByRole('complementary', { name: 'Unconfirmed command history' })
 expect(history).toHaveTextContent('Expected revision 7'); expect(history).toHaveTextContent('Original outcome remains unconfirmed')
 expect(document.body).not.toHaveTextContent('PRIVATE REASON CANARY')
 act(() => parent.renew())
 view.rerender(<Wrapper visible {...props} />)
 fireEvent.click(screen.getByRole('button', { name: 'Read current' }))
 await act(async () => {})
 expect(screen.getByRole('complementary', { name: 'Unconfirmed command history' })).toHaveTextContent('Original outcome remains unconfirmed')
})

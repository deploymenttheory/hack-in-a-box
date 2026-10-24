import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { expect, test, vi } from 'vitest'
import { PlanningOperations, planningDepartureAllowed, usePlanningDepartureBarrier, usePlanningUnknownCommands, usePlanningUnknownRedaction } from './planning-operations'
vi.mock('@tanstack/react-router', () => ({ useBlocker: vi.fn() }))

test('unknown command snapshots remain immutable across explicit new bases and editor replacement', async () => {
  const user = userEvent.setup(), command = { name: 'Original private command' }
  function Owner() {
    const operations = usePlanningUnknownCommands('setup:event-one')
    return <><p>{operations.unconfirmed ? 'Comparison required' : 'New command permitted'}</p><p>{JSON.stringify(operations.attempts.map(attempt => attempt.command))}</p><button onClick={() => { operations.record({ operation: 'Event configuration save', revision: 1, command }); command.name = 'Mutated later draft' }}>Uncertain response</button><button onClick={operations.selectNewBase}>Select fresh base</button><button onClick={operations.clear}>Definite denial</button></>
  }
  function Workspace() { const [open, setOpen] = useState(true); return <PlanningOperations><button onClick={() => setOpen(value => !value)}>Toggle editor</button>{open && <Owner />}</PlanningOperations> }
  render(<Workspace />)
  await user.click(screen.getByRole('button', { name: 'Uncertain response' }))
  expect(screen.getByText('Comparison required')).toBeVisible()
  expect(screen.getByText('[{"name":"Original private command"}]')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Select fresh base' }))
  await user.click(screen.getByRole('button', { name: 'Toggle editor' })); await user.click(screen.getByRole('button', { name: 'Toggle editor' }))
  expect(screen.getByText('New command permitted')).toBeVisible()
  expect(screen.getByText('[{"name":"Original private command"}]')).toBeVisible()
  expect(screen.getByLabelText('Unconfirmed command history')).toHaveTextContent('Original outcome remains unconfirmed')
  await user.click(screen.getByRole('button', { name: 'Definite denial' }))
  expect(screen.queryByLabelText('Unconfirmed command history')).not.toBeInTheDocument()
})

test('pending departure barrier follows the mounted private owner and releases on disposal', () => {
  function Owner({ pending }: { pending: boolean }) { usePlanningDepartureBarrier(pending); return null }
  const view = render(<Owner pending />)
  expect(planningDepartureAllowed()).toBe(false)
  view.rerender(<Owner pending={false} />); expect(planningDepartureAllowed()).toBe(true)
  view.rerender(<Owner pending />); view.unmount(); expect(planningDepartureAllowed()).toBe(true)
})


test('resource scope denial redacts only exact acknowledgement owners while retaining unknown revisions and barriers', async () => {
  const user = userEvent.setup(), prefix = 'person:session:event:'
  const owners = [prefix + 'page:acknowledgement', prefix + 'team:evidence', prefix + 'application:response', 'other:session:event:page:acknowledgement', 'person:other:event:page:acknowledgement', 'person:session:other:page:acknowledgement']
  function Owner({ owner, index }: { owner: string; index: number }) {
    const history = usePlanningUnknownCommands(owner)
    return <section aria-label={'Owner ' + index}><output aria-label={'Attempt ' + index}>{JSON.stringify(history.attempts)}</output><p>{history.unconfirmed ? 'Original outcome unknown; comparison required' : 'New action allowed'}</p><button onClick={() => history.record({ operation: 'PRIVATE TITLE ' + index, revision: index + 7, command: { private: 'PRIVATE PAYLOAD ' + index } })}>Record owner {index}</button></section>
  }
  function Denial() {
    const redact = usePlanningUnknownRedaction(owner => owner.startsWith(prefix) && owner.endsWith(':acknowledgement'))
    return <button onClick={() => redact(attempt => ({ ...attempt, operation: 'Resource acknowledgement', command: { redacted: 'Resource access revoked' } }))}>Deny library</button>
  }
  const view = render(<PlanningOperations>{owners.map((owner, index) => <Owner key={owner} owner={owner} index={index} />)}<Denial /></PlanningOperations>)
  for (let index = 0; index < owners.length; index++) await user.click(screen.getByRole('button', { name: 'Record owner ' + index }))
  const unaffected = owners.slice(1).map((_, index) => screen.getByLabelText('Attempt ' + (index + 1)).textContent)
  await user.click(screen.getByRole('button', { name: 'Deny library' }))
  expect(screen.getByLabelText('Attempt 0')).toHaveTextContent('"revision":7')
  expect(screen.getByLabelText('Attempt 0')).toHaveTextContent('Resource acknowledgement')
  expect(screen.getByLabelText('Attempt 0')).not.toHaveTextContent('PRIVATE')
  expect(screen.getByRole('region', { name: 'Owner 0' })).toHaveTextContent('Original outcome unknown; comparison required')
  expect(screen.getByLabelText('Unconfirmed command history')).toHaveTextContent('Expected revision 7 · Original outcome remains unconfirmed')
  owners.slice(1).forEach((_, index) => expect(screen.getByLabelText('Attempt ' + (index + 1)).textContent).toBe(unaffected[index]))
  view.unmount()
})

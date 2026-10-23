import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { expect, test, vi } from 'vitest'
import { PlanningOperations, planningDepartureAllowed, usePlanningDepartureBarrier, usePlanningUnknownCommands } from './planning-operations'
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

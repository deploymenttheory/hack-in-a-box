import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { expect, test, vi } from 'vitest'
import { PlanningOperations, usePlanningUnknownCommands, usePlanningUnknownRedaction } from './planning-operations'

const blocker = vi.hoisted(() => vi.fn())
vi.mock('@tanstack/react-router', () => ({ useBlocker: blocker }))

test('revoking the last private owner removes its payload but preserves departure protection after disposal and return', async () => {
  const user = userEvent.setup(), owner = 'person:session:event:page:acknowledgement'
  function Reader() {
    const history = usePlanningUnknownCommands(owner)
    return <section aria-label="Private reader">
      <output aria-label="Reader comparison">{history.unconfirmed ? 'Comparison required' : 'Ready'}</output>
      <output aria-label="Reader attempts">{JSON.stringify(history.attempts)}</output>
      <button onClick={() => history.record({ operation: 'PRIVATE PAGE TITLE', revision: 12, command: { body: 'PRIVATE PAGE PAYLOAD' } })}>Lose acknowledgement response</button>
    </section>
  }
  function Library() {
    const [visible, setVisible] = useState(true)
    const redact = usePlanningUnknownRedaction(candidate => candidate === owner)
    return <>
      {visible && <Reader />}
      <button onClick={() => {
        redact(attempt => ({ ...attempt, operation: 'Resource acknowledgement', command: { redacted: 'Resource access revoked' } }))
        setVisible(false)
      }}>Revoke library access</button>
      <button onClick={() => setVisible(true)}>Return with current access</button>
    </>
  }
  render(<PlanningOperations><Library /></PlanningOperations>)
  await user.click(screen.getByRole('button', { name: 'Lose acknowledgement response' }))
  await user.click(screen.getByRole('button', { name: 'Revoke library access' }))
  expect(screen.queryByRole('region', { name: 'Private reader' })).not.toBeInTheDocument()
  expect(document.body).not.toHaveTextContent('PRIVATE PAGE')
  expect(screen.getByLabelText('Unconfirmed command history')).toHaveTextContent('Expected revision 12')

  // Exercise the provider's actual route/unload contract after its last private
  // consumer is gone; a visible status message alone cannot prove this barrier.
  const departure = blocker.mock.calls.at(-1)?.[0] as { enableBeforeUnload: boolean; shouldBlockFn: () => boolean }
  expect(departure.enableBeforeUnload).toBe(true)
  const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
  expect(departure.shouldBlockFn()).toBe(true)
  expect(confirm).toHaveBeenCalledOnce()
  confirm.mockRestore()

  await user.click(screen.getByRole('button', { name: 'Return with current access' }))
  expect(screen.getByLabelText('Reader comparison')).toHaveTextContent('Comparison required')
  expect(screen.getByLabelText('Reader attempts')).toHaveTextContent('"revision":12')
  expect(screen.getByLabelText('Reader attempts')).not.toHaveTextContent('PRIVATE PAGE')
})

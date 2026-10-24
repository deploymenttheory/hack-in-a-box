import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { expect, test } from 'vitest'
import { OutcomeDecision, useOutcomeReview } from './outcome-decision'
import type { NativeIntent } from './outcome-api'

const intent: Extract<NativeIntent, { kind: 'outcome' }> = { kind: 'outcome', outcomeID: '33333333-3333-4333-8333-333333333333', expected: 4, command: { id: '44444444-4444-4444-8444-444444444444', action: 'evidence', reason: 'Synthetic review decision', evidence: { id: '55555555-5555-4555-8555-555555555555', basis: 'estimate', claim: 'A proposed synthetic benefit', value: '0', denominator: null, source: '', observed_at: null, measures: [] } } }

test('review freezes the exact private intent, distinguishes zero from unknown and returns focus to the original initiating action', async () => {
  const source = structuredClone(intent)
  let confirmed: NativeIntent | undefined
  function Example() {
    const owner = useOutcomeReview()
    return <><button onClick={() => owner.setReviewed(source)}>Prepare synthetic evidence</button>{owner.reviewed && <OutcomeDecision intent={owner.reviewed} title="Evidence" confirmLabel="Save exact evidence" pending={false} blocked={false} confirm={() => { confirmed = owner.reviewed }} correct={owner.correctReview} />}</>
  }
  render(<Example />)
  const opener = screen.getByRole('button', { name: 'Prepare synthetic evidence' })
  opener.focus(); fireEvent.click(opener)
  expect(screen.getByRole('heading', { name: 'Review evidence' })).toHaveFocus()
  expect(screen.getByText('Exact value').nextSibling).toHaveTextContent('0')
  expect(screen.getByText('Denominator').nextSibling).toHaveTextContent('Unknown')
  source.command.evidence!.value = '900'
  fireEvent.click(screen.getByRole('button', { name: 'Save exact evidence' }))
  expect(confirmed).toEqual(intent)
  fireEvent.click(screen.getByRole('button', { name: 'Correct details' }))
  await waitFor(() => expect(opener).toHaveFocus())
})

test.each([{ pending: true, blocked: false }, { pending: false, blocked: true }, { pending: false, blocked: false, stale: true }])('pending, unknown and stale review cannot confirm a replacement: %j', state => {
  let confirmations = 0
  render(<OutcomeDecision intent={intent} title="Evidence" confirmLabel="Confirm reviewed evidence" {...state} confirm={() => { confirmations++ }} correct={() => {}} />)
  const confirm = screen.getByRole('button', { name: 'Confirm reviewed evidence' })
  expect(confirm).toBeDisabled(); fireEvent.click(confirm); expect(confirmations).toBe(0)
})

import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, expect, test, vi } from 'vitest'
import { MentorDraftRecovery } from './mentor-draft-recovery'
import { invalidateMentorDrafts, MentorDraftStore, retainMentorDraftScope } from './mentor-drafts'
const target = { personID: '11111111-1111-4111-8111-111111111111', sessionScope: '22222222-2222-4222-8222-222222222222', eventID: '33333333-3333-4333-8333-333333333333', teamID: '44444444-4444-4444-8444-444444444444', noteID: '55555555-5555-4555-8555-555555555555' }
beforeEach(() => { sessionStorage.clear(); invalidateMentorDrafts(); retainMentorDraftScope(target.personID, target.sessionScope) })
test('requires explicit current checks and recovery without submitting or rendering private evidence', async () => {
  const store = new MentorDraftStore()
  store.save(target, 7, 0, { category: 'technical', evidence: 'PRIVATE-EVIDENCE', outcome: 'Synthetic outcome' })
  const read = vi.fn(async () => ({ eventID: target.eventID, teamID: target.teamID, noteID: target.noteID, revision: 8, noteVersion: 1, canWrite: true, reviewOpen: true }))
  const recover = vi.fn()
  render(<MentorDraftRecovery store={store} target={target} readCurrent={read} onRecover={recover} onInvalidate={vi.fn()} />)
  expect(read).not.toHaveBeenCalled(); expect(screen.queryByText('PRIVATE-EVIDENCE')).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Check for local text' }))
  await screen.findByText(/The server changed/)
  expect(recover).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole('button', { name: 'Restore text to the editor' }))
  await waitFor(() => expect(recover).toHaveBeenCalledOnce())
  expect(read).toHaveBeenCalledTimes(2)
  expect(recover.mock.calls[0][0]).toMatchObject({ changed: true, baseRevision: 7, text: { evidence: 'PRIVATE-EVIDENCE' } })
  expect(screen.queryByText('PRIVATE-EVIDENCE')).not.toBeInTheDocument()
})

test('remote scope invalidation immediately calls the actual editor clear boundary', async () => {
  const store = new MentorDraftStore()
  store.save(target, 7, 0, { category: 'technical', evidence: 'PRIVATE-RESTORED-TEXT', outcome: '' })
  const read = async () => ({ eventID: target.eventID, teamID: target.teamID, noteID: target.noteID, revision: 7, noteVersion: 0, canWrite: true, reviewOpen: true })
  let editorText = ''
  const clear = vi.fn(() => { editorText = '' })
  render(<MentorDraftRecovery store={store} target={target} readCurrent={read} onRecover={review => { editorText = review.text.evidence }} onInvalidate={clear} />)
  fireEvent.click(screen.getByRole('button', { name: 'Check for local text' }))
  await screen.findByText(/Unsent text is available/)
  fireEvent.click(screen.getByRole('button', { name: 'Restore text to the editor' }))
  await waitFor(() => expect(editorText).toBe('PRIVATE-RESTORED-TEXT'))
  invalidateMentorDrafts()
  expect(clear).toHaveBeenCalledOnce()
  expect(editorText).toBe('')
  expect(sessionStorage.length).toBe(0)
})

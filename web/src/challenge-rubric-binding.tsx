import { useCallback, useEffect, useImperativeHandle, useMemo, type Ref } from 'react'
import { api, APIError, unwrap } from './api/client'
import { useIdentity } from './session'
import { useEventReadBoundary } from './event-authority'
import { usePrivateRequestScope, type PrivateRequestScope } from './private-request-scope'
import { useTodayRead } from './today-read'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { ChallengeRubricChoice } from './challenge-rubric-choice'
import { readChallengeRubric, sameRubric, type RubricReference } from './challenge-rubric-model'
import { rubricReferenceKey } from './rubric-planning-model'

export type ChallengeRubricReadiness = { paused: boolean; approved: boolean }
export type ChallengeRubricCheck = { check: (captured: RubricReference | undefined, requireApproval: boolean) => Promise<void> }
export function ChallengeRubricBinding({ eventID, contentID, value, disabled, parent, onState, choose, configure, deny, ref }: { eventID: string; contentID: string; value?: RubricReference; disabled: boolean; parent: PrivateRequestScope; onState: (value: ChallengeRubricReadiness) => void; choose: (value?: RubricReference) => void; configure: () => void; deny: (error: unknown) => void; ref: Ref<ChallengeRubricCheck> }) {
  const identity = useIdentity(), boundary = useEventReadBoundary(eventID)
  const key = `${identity.person.id}:${identity.draft_scope}:${eventID}:${contentID}:challenge-rubric`
  const scope = usePrivateRequestScope(key, parent)
  const selectedKey = value ? rubricReferenceKey(value) : ''
  // A document-body keystroke does not restart private reads or steal focus.
  const selected = useMemo(() => {
    if (!selectedKey) return undefined
    const [id, version, digest] = JSON.parse(selectedKey) as [string, number, string]
    return { id, version, digest }
  }, [selectedKey])
  const load = useCallback((signal: AbortSignal, assert: () => void) => readChallengeRubric(eventID, selected, signal, assert), [eventID, selected])
  const read = useTodayRead(key + ':' + selectedKey, load, scope)
  const paused = read.pending || !!read.error || !read.data || !!boundary?.paused
  const approved = !!value && !!read.data?.approved && sameRubric(value, read.data.approved.reference)
  useEffect(() => { onState({ paused, approved }) }, [paused, approved, onState])
  useEffect(() => { if (read.denied) deny(read.error) }, [read.denied, read.error, deny])
  useImperativeHandle(ref, () => ({ check: async (captured, requireApproval) => {
    const ticket = scope.capture(); ticket.assert()
    try {
      const authority = unwrap(await api.GET('/api/v1/events/{event_id}', { signal: ticket.signal, cache: 'no-store', params: { path: { event_id: eventID } } }))
      ticket.assert()
      if (authority.id !== eventID || !authority.can_edit) throw new APIError(403, 'forbidden')
      if (boundary && !boundary.observeToday(authority)) throw new APIError(409, 'revision_conflict')
      const current = await readChallengeRubric(eventID, captured, ticket.signal, ticket.assert)
      ticket.assert()
      if (requireApproval && (!captured || !current.approved || !sameRubric(captured, current.approved.reference))) {
        void read.refresh()
        throw new APIError(409, 'rubric_changed')
      }
    } catch (error) {
      if (ticket.current() && error instanceof APIError && [401, 403, 404].includes(error.status)) { scope.invalidate(); deny(error) }
      if (ticket.current() && !(error instanceof APIError && [401, 403, 404].includes(error.status))) void read.refresh()
      throw error
    }
  } }))
  if (read.denied) return <Failure error={read.error} />
  return <div className="grid gap-4">
    {!read.data && !read.error && <Loading />}
    {!!read.error && <Failure error={read.error} retry={() => void read.refresh()} />}
    {read.data && <ChallengeRubricChoice data={read.data} value={value} paused={paused} disabled={disabled} choose={choose} configure={configure} />}
    <Button type="button" variant="outline" icon="refresh" disabled={read.pending || disabled} onClick={() => void read.refresh()}>Check current challenge criteria</Button>
  </div>
}

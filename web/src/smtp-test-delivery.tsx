import { Button } from './components/ui/button'
import { useEffect } from 'react'
import type { components } from './api/schema.gen'
import { CommunicationStep } from './communication-layout'
import { StatusBadge } from './components/ui/feedback'
import { usePrivateRequestScope } from './private-request-scope'
import { communicationRead } from './communication-private'
import { useQuery } from '@tanstack/react-query'
import { api, APIError, privateKey, unwrap } from './api/client'
import { useIdentity } from './session'
import { Failure } from './components/states'

export function SMTPTestDelivery({ eventID, testID, observed, denied }: { eventID: string, testID: string, denied: () => void, observed: (testID: string, state: components['schemas']['SMTPTestDeliveryStatus']['state']) => void }) {
  const identity = useIdentity(), scope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${eventID}:${testID}:smtp-test`)
  const query = useQuery({ queryKey: [...privateKey(identity.person.id, eventID, `smtp-test:${testID}`), identity.draft_scope], gcTime: 0, retry: false, queryFn: () => communicationRead(scope, async signal => { try { return unwrap(await api.GET('/api/v1/events/{event_id}/communications/tests/{test_id}/delivery', { signal, params: { path: { event_id: eventID, test_id: testID } } })) } catch (error) { if (scope.capture().current() && error instanceof APIError && [401,403,404].includes(error.status)) denied(); throw error } }), refetchInterval: query => query.state.data?.state === 'pending' || query.state.data?.state === 'retryable' ? 2000 : false, refetchOnWindowFocus: 'always' })
  const state = query.data?.state
  useEffect(() => { if (state && !query.error && scope.capture().current()) observed(testID, state) }, [state, query.error, scope, testID, observed])
  if (query.isPending) return <p aria-live="polite">Checking SMTP submission…</p>
  if (query.error) return <Failure error={query.error} retry={() => void query.refetch()} />
  const text = {
    pending: 'Test queued for SMTP submission.',
    accepted: 'The email relay accepted the test message. Inbox delivery and reading are unconfirmed.',
    retryable: 'The relay did not accept the message. A bounded retry is scheduled while this request remains eligible.',
    permanent: 'The test could not be submitted. Review the current message, email setup and permissions before creating another test.',
    ambiguous: 'Submission is uncertain. Review this attempt before sending another test; it will not be retried automatically.',
    cancelled: 'Test request cancelled.',
  }[query.data.state]
  return <CommunicationStep title="SMTP test submission" icon="send" description="This is the native submission state for the original organiser-only TEST request."><StatusBadge tone={query.data.state === 'ambiguous' ? 'warning' : 'neutral'}>{query.data.state.replaceAll('_', ' ')}</StatusBadge><p aria-live="polite">{text}</p><Button icon="refresh" variant="outline" disabled={query.isFetching} onClick={() => void query.refetch()}>Check original TEST</Button></CommunicationStep>
}

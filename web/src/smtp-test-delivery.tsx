import { useQuery } from '@tanstack/react-query'
import { api, privateKey, unwrap } from './api/client'
import { useIdentity } from './session'
import { Failure } from './components/states'

export function SMTPTestDelivery({ eventID, testID }: { eventID: string, testID: string }) {
  const identity = useIdentity()
  const query = useQuery({ queryKey: privateKey(identity.person.id, eventID, `smtp-test:${testID}`), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/communications/tests/{test_id}/delivery', { params: { path: { event_id: eventID, test_id: testID } } })), refetchInterval: query => query.state.data?.state === 'pending' || query.state.data?.state === 'retryable' ? 2000 : false, refetchOnWindowFocus: 'always' })
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
  return <section aria-label="SMTP test submission" aria-live="polite"><p>{text}</p></section>
}

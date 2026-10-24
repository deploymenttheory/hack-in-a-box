import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { api, unwrap } from './api/client'
import { usePrivateRequestScope, type PrivateRequestScope } from './private-request-scope'
import { Button } from './components/ui/button'
import { ConfirmationDialog } from './components/ui/dialog'
import { Failure } from './components/states'
import { nativeDenied } from './survey-context'
import { useIdentity } from './session'

export function SurveyResponseExport({ eventId, surveyId, disabled, authority, deny }: { eventId: string; surveyId: string; disabled: boolean; authority: PrivateRequestScope; deny: (error: unknown) => void }) {
  const identity = useIdentity(), scope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${eventId}:${surveyId}:export`, authority), opener = useRef<HTMLButtonElement>(null), lock = useRef(false), objectURL = useRef<string | undefined>(undefined)
  const [review, setReview] = useState(false), [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState('')
  useLayoutEffect(() => { if (disabled) scope.invalidate() }, [disabled, scope])
  useEffect(() => {
    function stop() { scope.invalidate(); if (objectURL.current) { URL.revokeObjectURL(objectURL.current); objectURL.current = undefined }; setReview(false) }
    window.addEventListener('beforeprint', stop); window.addEventListener('pagehide', stop)
    return () => { window.removeEventListener('beforeprint', stop); window.removeEventListener('pagehide', stop); if (objectURL.current) URL.revokeObjectURL(objectURL.current) }
  }, [scope])
  async function download() {
    if (lock.current || disabled) return
    lock.current = true; setPending(true); setFailure(undefined); setNotice('')
    const ticket = scope.capture()
    try {
      ticket.assert()
      const csv = unwrap(await api.GET('/api/v1/events/{event_id}/surveys/{survey_id}/responses/export.csv', { parseAs: 'blob', cache: 'no-store', signal: ticket.signal, params: { path: { event_id: eventId, survey_id: surveyId } } })); ticket.assert()
      objectURL.current = URL.createObjectURL(csv)
      const link = document.createElement('a'); link.href = objectURL.current; link.download = 'restricted-survey-responses.csv'; link.click()
      URL.revokeObjectURL(objectURL.current); objectURL.current = undefined
      setReview(false); setNotice('Restricted CSV download prepared. The native export rechecked current authority; downloaded copies remain private and cannot be recalled by this app.')
    } catch (error) { if (ticket.current()) { if (nativeDenied(error)) deny(error); else setFailure(error) } }
    finally { lock.current = false; setPending(false) }
  }
  return <div className="fleet-survey-export"><Button ref={opener} icon="download" variant="outline" disabled={disabled || pending} onClick={() => { if (!lock.current && authority.capture().current()) { scope.renew(); setReview(true) } }}>Review restricted CSV export</Button><p>This explicit export is audited and current-authority checked. The native CSV protects formula-like content. It contains identifiable names and answers, never a published report.</p>{notice && <p role="status">{notice}</p>}{!!failure && <Failure error={failure} />}<ConfirmationDialog open={review} onOpenChange={open => { if (!open && !pending) setReview(false) }} title="Review private response export" description="Download all retained identifiable responses authorised by the server. Store the file privately. Exported copies cannot be recalled after role loss or privacy deletion; private responses are excluded from printed app pages." confirmLabel="Download restricted responses CSV" pending={pending} disabled={disabled} finalFocus={opener} onConfirm={() => void download()}><p>Survey {surveyId} · Current event {eventId}</p></ConfirmationDialog></div>
}

import { useBlocker } from '@tanstack/react-router'
import { useEffect, useRef, useState } from 'react'
import { api, APIError, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { usePrivateRequestScope } from './private-request-scope'
import { usePlanningDepartureBarrier } from './planning-operations'
import { Button } from './components/ui/button'
import { Input, Label } from './components/ui/fields'
import { Alert } from './components/ui/surfaces'
import { Failure } from './components/states'
import { resourceDenied } from './resource-operation'

type UploadBody = { filename: string; media_type: 'application/pdf' | 'image/png' | 'image/jpeg' | 'text/plain'; content: string; team_id?: string; reason?: string }
type Attempt = { id: string; body: UploadBody; revision: number; file: File }
type Props = { eventID: string; revision?: number; teamID?: string; reason?: string; id: string; label: string; inputLabel?: string; action: string; disabled: boolean; busy: (value: boolean) => void; accepted: (value: components['schemas']['File'], file: File) => void; failed?: (error: unknown) => void }
export function ProtectedUpload(props: Props) {
  const identity = useIdentity()
  return <UploadLifetime key={`${identity.person.id}:${identity.draft_scope}:${props.eventID}:${props.teamID ?? ''}:${props.id}`} {...props} />
}
// The only replay path repeats the native actor/event/key fingerprint contract.
// Captured bytes, metadata, team, reason and expected revision never follow later edits.
function UploadLifetime({ eventID, revision, teamID, reason, id, label, inputLabel, action, disabled, busy, accepted, failed }: Props) {
  const identity = useIdentity(), scope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${eventID}:${teamID ?? ''}:${id}:upload`)
  const [file, setFile] = useState<File>(), [pending, setPending] = useState(false), [unknown, setUnknown] = useState(false), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState(''), [denied, setDenied] = useState(false)
  const input = useRef<HTMLInputElement>(null), captured = useRef<Attempt | undefined>(undefined), running = useRef(false)
  usePlanningDepartureBarrier(pending || unknown)
  useBlocker({ shouldBlockFn: () => running.current || unknown, enableBeforeUnload: pending || unknown })
  useEffect(() => { busy(pending || unknown); return () => busy(false) }, [pending, unknown, busy])
  async function upload() {
    if (running.current || disabled || denied) return
    const selected = captured.current?.file ?? file
    if (!selected) return
    if (!['application/pdf', 'image/png', 'image/jpeg', 'text/plain'].includes(selected.type) || selected.size > 8 * 1024 * 1024) { setNotice('Choose a PDF, PNG, JPEG or text file up to 8 MiB.'); return }
    const ticket = scope.capture(); if (!ticket.current()) return
    running.current = true; setPending(true); setFailure(undefined); setNotice('')
    let sent = false
    try {
      let attempt = captured.current
      if (!attempt) {
        const content = await new Promise<string>((resolve, reject) => { const reader = new FileReader(); reader.onload = () => resolve(String(reader.result).split(',')[1]); reader.onerror = reject; reader.readAsDataURL(selected) })
        ticket.assert()
        const expected = revision ?? unwrap(await api.GET('/api/v1/events/{event_id}', { signal: ticket.signal, params: { path: { event_id: eventID } } })).revision
        ticket.assert()
        attempt = { id: crypto.randomUUID(), revision: expected, file: selected, body: { filename: selected.name, media_type: selected.type as UploadBody['media_type'], content, ...(teamID ? { team_id: teamID, reason: reason ?? '' } : {}) } }
        captured.current = attempt
      }
      ticket.assert(); sent = true
      const result = unwrap(await api.POST('/api/v1/events/{event_id}/files', { params: { path: { event_id: eventID }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${attempt.revision}"`, 'Idempotency-Key': attempt.id } }, body: attempt.body }))
      ticket.assert()
      if (!result.id || result.filename !== attempt.body.filename || result.media_type !== attempt.body.media_type || result.size_bytes !== attempt.file.size || result.event_id !== eventID || result.owner_id !== identity.person.id) throw new Error('Unexpected upload response')
      accepted(result, attempt.file); captured.current = undefined; setUnknown(false); setFile(undefined); if (input.current) input.current.value = ''
      setNotice(`File ${result.id} uploaded. Save the owning page or evidence to attach it; this is not a formal submission.`)
    } catch (error) {
      if (!ticket.current()) return
      setFailure(error); failed?.(error)
      const uncertain = sent && (!(error instanceof APIError) || error.status >= 500 || error.status === 408)
      setUnknown(uncertain)
      if (!uncertain) captured.current = undefined
      if (resourceDenied(error)) { setDenied(true); setFile(undefined); captured.current = undefined; if (input.current) input.current.value = '' }
    } finally { running.current = false; if (ticket.current()) setPending(false) }
  }
  if (denied) return <Failure error={failure} />
  return <section className="fleet-resource-section" aria-label={label}><fieldset className="fleet-resource-fields" disabled={disabled || pending || unknown}><legend>{label}</legend><Label htmlFor={id}>{inputLabel ?? `Choose ${label.toLowerCase()}`}</Label><Input ref={input} id={id} type="file" accept="application/pdf,image/png,image/jpeg,text/plain" onChange={event => { setFile(event.target.files?.[0]); setFailure(undefined); setNotice('') }} />{file && <p>{file.name} · {Math.ceil(file.size / 1024)} KiB · {file.type}</p>}</fieldset>{unknown && <Alert tone="warning" role="status"><h4>Upload outcome unknown</h4><p>The original file may have been stored. A deliberate retry repeats the exact key, bytes, metadata, team and reason. Current authorisation is checked again; no retry is automatic.</p></Alert>}<Button type="button" icon="upload" pending={pending} disabled={disabled || (!file && !unknown)} onClick={() => void upload()}>{action}</Button>{notice && <p role="status">{notice}</p>}{!!failure && <Failure error={failure} />}</section>
}

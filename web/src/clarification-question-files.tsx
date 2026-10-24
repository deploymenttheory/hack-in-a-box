import { useEffect, useRef, useState } from 'react'
import { api, unwrap } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Field, Input } from './components/ui/fields'
import { Failure } from './components/states'
import { uncertain, useClarifications, useCurrentPage } from './clarification-context'
import { usePlanningUnknownCommands } from './planning-operations'

export type UploadedQuestionFile = { id: string; filename: string }
type UploadAttempt = { key: string; filename: string; media_type: 'application/pdf' | 'image/png' | 'image/jpeg' | 'text/plain'; content: string }
export function QuestionAttachmentUpload({ teamID, files, change, disabled, onBusy }: { teamID: string; files: UploadedQuestionFile[]; change: (files: UploadedQuestionFile[]) => void; disabled: boolean; onBusy: (busy: boolean) => void }) {
  const identity = useIdentity(), { event, deny, observe, refresh } = useClarifications(), active = useCurrentPage(), input = useRef<HTMLInputElement>(null), lock = useRef(false)
  const history = usePlanningUnknownCommands(`${identity.person.id}:${identity.draft_scope}:${event.id}:${teamID}:question-file`)
  const [file, setFile] = useState<File>(), [attempt, setAttempt] = useState<UploadAttempt>(), [busy, setBusy] = useState(false), [uncertainUpload, setUnknown] = useState(false), [notice, setNotice] = useState(''), [failure, setFailure] = useState<unknown>()
  const unknown = uncertainUpload || (!busy && history.unconfirmed)
  useEffect(() => { onBusy(busy || unknown); return () => onBusy(false) }, [busy, unknown, onBusy])
  async function upload() {
    if (!active.current() || disabled || lock.current || files.length >= 10 || (!file && !attempt)) return
    const mediaType = file?.type as UploadAttempt['media_type']
    if (!attempt && (!file || !['application/pdf', 'image/png', 'image/jpeg', 'text/plain'].includes(mediaType) || file.size > 8 * 1024 * 1024)) { setNotice('Choose a PDF, PNG, JPEG or text file up to 8 MiB.'); return }
    lock.current = true; setBusy(true); setFailure(undefined); setNotice('')
    let payload = attempt, attempted = false
    try {
      if (!payload && file) { const content = await new Promise<string>((resolve, reject) => { const reader = new FileReader(); reader.onload = () => resolve(String(reader.result).split(',')[1]); reader.onerror = reject; reader.readAsDataURL(file) }); if (!active.current()) return; payload = { key: crypto.randomUUID(), filename: file.name, media_type: mediaType, content }; setAttempt(payload) }
      if (!payload || !active.current() || !await refresh('ask') || !active.current()) return
      const access = unwrap(await api.GET('/api/v1/events/{event_id}/clarification-workspace', { cache: 'no-store', params: { path: { event_id: event.id } } }))
      if (!active.current()) return
      observe(access)
      if (!active.current() || !access.can_ask || access.archived || access.team_id !== teamID) return
      const latest = unwrap(await api.GET('/api/v1/events/{event_id}', { cache: 'no-store', params: { path: { event_id: event.id } } }))
      if (!active.current()) return
      attempted = true
      if (!unknown) history.record({ operation: 'Private question attachment upload', revision: 0, command: { intentID: payload.key } })
      const result = unwrap(await api.POST('/api/v1/events/{event_id}/files', { params: { path: { event_id: event.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${latest.revision}"`, 'Idempotency-Key': payload.key } }, body: { team_id: teamID, filename: payload.filename, media_type: payload.media_type, content: payload.content } }))
      if (!active.current()) return
      history.resolve(payload.key)
      change([...files, { id: result.id, filename: result.filename }]); setFile(undefined); setAttempt(undefined); setUnknown(false); setNotice('Uploaded to your team. Send the private question to attach it.'); if (input.current) input.current.value = ''
    } catch (error) {
      if (!active.current()) return
      setFailure(error); deny(error)
      const unresolved = attempted && uncertain(error) && !!payload
      setUnknown(unresolved)
      if (attempted && !unresolved && payload && !unknown) history.resolve(payload.key)
      setNotice(unresolved ? 'Upload outcome unknown. Retry this exact file explicitly before sending the question, or deliberately leave it unattached.' : attempted ? 'No attached file was confirmed. Your question remains unsent.' : 'Current file authority could not be verified. No upload was sent; your question remains unsent.')
    } finally { lock.current = false; if (active.current()) setBusy(false) }
  }
  return <fieldset className="grid gap-4 py-4 border-t border-border" disabled={disabled || busy}><legend>Private team attachments</legend><p>Up to 10 files. Original attachments never enter common official answers. Uploading a file does not send the question.</p><ul>{files.map(file => <li className="flex flex-wrap items-center gap-4 py-2 border-b border-border" key={file.id}><span className="break-all">{file.filename}</span><Button type="button" icon="remove" variant="ghost" onClick={() => change(files.filter(item => item.id !== file.id))}>Remove {file.filename}</Button></li>)}</ul>
    <Field id="question-upload" label="Choose a private attachment" description="PDF, PNG, JPEG or text file up to 8 MiB. The server rechecks current team ownership."><Input ref={input} type="file" disabled={unknown} accept="application/pdf,image/png,image/jpeg,text/plain" onChange={e => { setFile(e.target.files?.[0]); setAttempt(undefined); setFailure(undefined); setNotice('') }} /></Field><Button type="button" icon="upload" variant="outline" disabled={(!file && !attempt) || files.length >= 10} onClick={() => void upload()}>{unknown ? 'Retry the same file upload' : 'Upload private attachment'}</Button>
    {unknown && <Button type="button" icon="close" variant="ghost" onClick={() => { if (!window.confirm('Leave this upload outcome unresolved and keep the file unattached? This does not prove the upload failed.')) return; history.selectNewBase(); setAttempt(undefined); setUnknown(false); setFile(undefined); if (input.current) input.current.value = ''; setNotice('The uncertain file is left unattached. This does not claim that its upload failed.') }}>Continue without this uncertain attachment</Button>}{busy && <p role="status">Checking authority and uploading private file…</p>}{notice && <p role="status">{notice}</p>}{!!failure && <Failure error={failure} />}
  </fieldset>
}

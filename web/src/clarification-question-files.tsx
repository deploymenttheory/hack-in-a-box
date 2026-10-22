import { useEffect, useRef, useState } from 'react'
import { api, unwrap } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure } from './components/states'
import { uncertain, useClarifications, useCurrentPage } from './clarification-context'

export type UploadedQuestionFile = { id: string; filename: string }
type UploadAttempt = { key: string; filename: string; media_type: 'application/pdf' | 'image/png' | 'image/jpeg' | 'text/plain'; content: string }

export function QuestionAttachmentUpload({ teamID, files, change, disabled, onBusy }: { teamID: string; files: UploadedQuestionFile[]; change: (files: UploadedQuestionFile[]) => void; disabled: boolean; onBusy: (busy: boolean) => void }) {
  const identity = useIdentity(), { event, deny } = useClarifications(), active = useCurrentPage(), input = useRef<HTMLInputElement>(null)
  const [file, setFile] = useState<File>(), [attempt, setAttempt] = useState<UploadAttempt>(), [busy, setBusy] = useState(false), [unknown, setUnknown] = useState(false), [notice, setNotice] = useState(''), [failure, setFailure] = useState<unknown>()
  useEffect(() => { onBusy(busy || unknown); return () => onBusy(false) }, [busy, unknown, onBusy])
  async function upload() {
    if (disabled || busy || files.length >= 10 || (!file && !attempt)) return
    const mediaType = file?.type as UploadAttempt['media_type']
    if (!attempt && (!file || !['application/pdf', 'image/png', 'image/jpeg', 'text/plain'].includes(mediaType) || file.size > 8 * 1024 * 1024)) { setNotice('Choose a PDF, PNG, JPEG or text file up to 8 MiB.'); return }
    setBusy(true); setFailure(undefined); setNotice('')
    let payload = attempt
    try {
      if (!payload && file) {
        const content = await new Promise<string>((resolve, reject) => { const reader = new FileReader(); reader.onload = () => resolve(String(reader.result).split(',')[1]); reader.onerror = reject; reader.readAsDataURL(file) })
        payload = { key: crypto.randomUUID(), filename: file.name, media_type: mediaType, content }
        if (!active.current) return
        setAttempt(payload)
      }
      if (!payload) return
      const latest = unwrap(await api.GET('/api/v1/events/{event_id}', { cache: 'no-store', params: { path: { event_id: event.id } } }))
      const result = unwrap(await api.POST('/api/v1/events/{event_id}/files', { params: { path: { event_id: event.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${latest.revision}"`, 'Idempotency-Key': payload.key } }, body: { team_id: teamID, filename: payload.filename, media_type: payload.media_type, content: payload.content } }))
      if (!active.current) return
      change([...files, { id: result.id, filename: result.filename }]); setFile(undefined); setAttempt(undefined); setUnknown(false); setNotice('Uploaded to your team. Send the private question to attach it.'); if (input.current) input.current.value = ''
    } catch (error) {
      if (!active.current) return
      setFailure(error); deny(error); setUnknown(uncertain(error) && !!payload)
      setNotice(uncertain(error) ? 'Upload outcome unknown. Retry this exact file explicitly before sending the question, or leave it unattached.' : 'No attached file was confirmed. Your question remains unsent.')
    } finally { if (active.current) setBusy(false) }
  }
  return <fieldset disabled={disabled || busy}><legend>Private team attachments</legend><p>Up to 10 files. Original question attachments are not included in common official answers.</p><ul className="item-list">{files.map(file => <li key={file.id}><span>{file.filename}</span><Button type="button" variant="outline" onClick={() => change(files.filter(item => item.id !== file.id))}>Remove {file.filename}</Button></li>)}</ul>
    <label htmlFor="question-upload">Choose a private attachment</label><input ref={input} id="question-upload" type="file" disabled={unknown} accept="application/pdf,image/png,image/jpeg,text/plain" onChange={e => { setFile(e.target.files?.[0]); setAttempt(undefined); setFailure(undefined); setNotice('') }} /><Button type="button" variant="outline" disabled={(!file && !attempt) || files.length >= 10} onClick={() => void upload()}>{unknown ? 'Retry the same file upload' : 'Upload private attachment'}</Button>
    {unknown && <Button type="button" variant="outline" onClick={() => { setAttempt(undefined); setUnknown(false); setFile(undefined); if (input.current) input.current.value = ''; setNotice('The uncertain file is left unattached. This does not claim that its upload failed.') }}>Continue without this uncertain attachment</Button>}{busy && <p role="status">Uploading private file…</p>}{notice && <p role="status">{notice}</p>}{!!failure && <Failure error={failure} />}
  </fieldset>
}

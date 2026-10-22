import { useRef, useState } from 'react'
import { api, APIError, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure } from './components/states'
import type { Attachment } from './content-model'

export function ContentAttachments({ event, ids, details, change, disabled, busy, setBusy }: { event: EventRecord; ids: string[]; details: Attachment[]; change: (ids: string[], details: Attachment[]) => void; disabled: boolean; busy: boolean; setBusy: (value: boolean) => void }) {
  const identity = useIdentity(), keys = useRef(new WeakMap<File, string>())
  const [file, setFile] = useState<File>(), [failure, setFailure] = useState<unknown>(), [message, setMessage] = useState('')
  async function upload() {
    if (!file) return
    const mediaType = file.type as 'application/pdf' | 'image/png' | 'image/jpeg' | 'text/plain'
    if (!['application/pdf', 'image/png', 'image/jpeg', 'text/plain'].includes(mediaType) || file.size > 8 * 1024 * 1024) { setMessage('Choose a PDF, PNG, JPEG or text file up to 8 MiB.'); return }
    setBusy(true); setFailure(undefined); setMessage('')
    try {
      const content = await new Promise<string>((resolve, reject) => { const reader = new FileReader(); reader.onload = () => resolve(String(reader.result).split(',')[1]); reader.onerror = reject; reader.readAsDataURL(file) })
      const key = keys.current.get(file) ?? crypto.randomUUID(); keys.current.set(file, key)
      const latest = unwrap(await api.GET('/api/v1/events/{event_id}', { params: { path: { event_id: event.id } } }))
      const result = unwrap(await api.POST('/api/v1/events/{event_id}/files', { params: { path: { event_id: event.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${latest.revision}"`, 'Idempotency-Key': key } }, body: { filename: file.name, media_type: mediaType, content } }))
      change([...ids, result.id], [...details, { id: result.id, filename: file.name, media_type: mediaType, size: file.size, can_download: true }])
      setFile(undefined); setMessage('Uploaded. Save the page to attach this file.')
    } catch (error) { setFailure(error) } finally { setBusy(false) }
  }
  return <fieldset disabled={disabled || busy}><legend>Attachments</legend><p>Files follow this page’s release audience. Upload a separate copy when reusing a file on another page.</p><ul className="item-list">{ids.map((id, index) => {
    const detail = details.find(d => d.id === id)
    return <li key={id}><div>{detail?.can_download ? <a className="content-link" href={`/api/v1/events/${event.id}/files/${id}`}>{detail.filename}</a> : <strong>{detail?.filename ?? `Unavailable attachment ${index + 1}`}</strong>}<p>{detail ? `${detail.media_type} · ${Math.ceil(detail.size / 1024)} KiB` : 'Remove this missing attachment and upload it again.'}</p></div><Button type="button" variant="outline" onClick={() => change(ids.filter(i => i !== id), details)}>Remove {detail?.filename ?? `attachment ${index + 1}`}</Button></li>
  })}</ul><label htmlFor="content-upload">Choose an attachment</label><input id="content-upload" type="file" accept="application/pdf,image/png,image/jpeg,text/plain" onChange={e => setFile(e.target.files?.[0])} /><Button type="button" variant="outline" disabled={!file || ids.length >= 30} onClick={() => void upload()}>Upload attachment</Button>{busy && <p role="status">Uploading attachment…</p>}{message && <p role="status">{message}</p>}{!!failure && <Failure error={failure} />}{failure instanceof APIError && failure.status === 409 && <p>Retry with the latest event revision. Your page draft is retained.</p>}</fieldset>
}

import { useRef, useState } from 'react'
import { api, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure } from './components/states'
import type { Evidence } from './deliverable-model'

export function DeliverableUpload({ event, teamID, item, reason, disabled, busy, setBusy, change }: { event: EventRecord; teamID: string; item: Evidence; reason: string; disabled: boolean; busy: boolean; setBusy: (value: boolean) => void; change: (item: Evidence) => void }) {
  const identity = useIdentity(), keys = useRef(new WeakMap<File, string>())
  const [file, setFile] = useState<File>(), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState('')
  async function upload() {
    if (!file) return
    if (!['application/pdf', 'image/png', 'image/jpeg', 'text/plain'].includes(file.type) || file.size > 8 * 1024 * 1024) { setNotice('Choose a PDF, PNG, JPEG or text file up to 8 MiB.'); return }
    setBusy(true); setFailure(undefined); setNotice('')
    try {
      const content = await new Promise<string>((resolve, reject) => { const r = new FileReader(); r.onload = () => resolve(String(r.result).split(',')[1]); r.onerror = reject; r.readAsDataURL(file) })
      const key = keys.current.get(file) ?? crypto.randomUUID(); keys.current.set(file, key)
      const latest = unwrap(await api.GET('/api/v1/events/{event_id}', { params: { path: { event_id: event.id } } }))
      const result = unwrap(await api.POST('/api/v1/events/{event_id}/files', { params: { path: { event_id: event.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${latest.revision}"`, 'Idempotency-Key': key } }, body: { team_id: teamID, filename: file.name, media_type: file.type as 'application/pdf' | 'image/png' | 'image/jpeg' | 'text/plain', content, reason } }))
      change({ ...item, file_id: result.id, url: '' }); setFile(undefined); setNotice(`${file.name} uploaded. It is available in your working evidence.`)
    } catch (error) { setFailure(error) } finally { setBusy(false) }
  }
  return <div className="deliverable-upload"><label htmlFor={`deliverable-file-${item.deliverable_id}`}>File for {item.label}</label><input id={`deliverable-file-${item.deliverable_id}`} type="file" accept="application/pdf,image/png,image/jpeg,text/plain" disabled={disabled || busy} onChange={e => { setFile(e.target.files?.[0]); setFailure(undefined); setNotice('') }} /><Button type="button" variant="outline" disabled={disabled || busy || !file} onClick={() => void upload()}>Upload {item.label}</Button>{busy && <p role="status">Uploading evidence…</p>}{!!failure && <><Failure error={failure} /><p>Your other fields are retained. Retry this upload with the same file.</p></>}{notice && <p role="status">{notice}</p>}</div>
}

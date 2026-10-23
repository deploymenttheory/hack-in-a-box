import { Input, Select, Label } from './components/ui/fields'
import { useRef, useState } from 'react'
import { useFieldArray, type UseFormReturn } from 'react-hook-form'
import { api, APIError, unwrap } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure } from './components/states'
import type { TaskValues } from './preparation-editor'

export function TaskEvidence({ form, eventID, revision, reportFailure, disabled, uploading, setUploading }: { form: UseFormReturn<TaskValues>; eventID: string; revision: number; reportFailure: (error: unknown) => void; disabled: boolean; uploading: boolean; setUploading: (value: boolean) => void }) {
  const identity = useIdentity(), keys = useRef(new WeakMap<File, string>())
  const [file, setFile] = useState<File>(), [failure, setFailure] = useState<unknown>(), [message, setMessage] = useState('')
  const evidence = useFieldArray({ control: form.control, name: 'definition.evidence', keyName: 'key' })
  async function upload() {
    if (!file) return
    const mediaType = file.type as 'application/pdf' | 'image/png' | 'image/jpeg' | 'text/plain'
    if (!['application/pdf', 'image/png', 'image/jpeg', 'text/plain'].includes(mediaType) || file.size > 8 * 1024 * 1024) { setMessage('Choose a PDF, PNG, JPEG or text file up to 8 MiB.'); return }
    setUploading(true); setFailure(undefined); setMessage('')
    try {
      const content = await new Promise<string>((resolve, reject) => { const reader = new FileReader(); reader.onload = () => resolve(String(reader.result).split(',')[1]); reader.onerror = reject; reader.readAsDataURL(file) })
      const key = keys.current.get(file) ?? crypto.randomUUID(); keys.current.set(file, key)
      const result = unwrap(await api.POST('/api/v1/events/{event_id}/files', { params: { path: { event_id: eventID }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${revision}"`, 'Idempotency-Key': key } }, body: { filename: file.name, media_type: mediaType, content } }))
      evidence.append({ id: crypto.randomUUID(), kind: 'file', label: file.name, url: '', file_id: result.id, visibility: 'organisers' }); setFile(undefined); setMessage('File uploaded. Save the task to attach it.')
    } catch (error) { setFailure(error); if (error instanceof APIError && error.code === 'revision_conflict') reportFailure(error) } finally { setUploading(false) }
  }
  return <fieldset disabled={disabled || uploading}><legend>Evidence</legend><p>Links open directly. File downloads check current access. New evidence starts as organisers only.</p>{evidence.fields.map((item, i) => <div className="evidence-row" key={item.key}><div className="form-grid"><div><Label htmlFor={`evidence-label-${item.id}`}>Evidence label {i + 1}</Label><Input id={`evidence-label-${item.id}`} maxLength={300} {...form.register(`definition.evidence.${i}.label`)} /></div><div><Label htmlFor={`evidence-visibility-${item.id}`}>Evidence audience {i + 1}</Label><Select id={`evidence-visibility-${item.id}`} {...form.register(`definition.evidence.${i}.visibility`)}><option value="organisers">Organisers only</option><option value="assignees">Task assignees (file permissions still apply)</option></Select></div>{item.kind !== 'file' && <><div><Label htmlFor={`evidence-kind-${item.id}`}>Reference type {i + 1}</Label><Select id={`evidence-kind-${item.id}`} {...form.register(`definition.evidence.${i}.kind`)}><option value="url">Web link</option><option value="repository">Repository</option></Select></div><div><Label htmlFor={`evidence-url-${item.id}`}>Evidence URL {i + 1}</Label><Input id={`evidence-url-${item.id}`} type="url" maxLength={2048} {...form.register(`definition.evidence.${i}.url`)} /></div></>}</div><div className="actions">{item.kind === 'file' && <a href={`/api/v1/events/${eventID}/files/${item.file_id}`} target="_blank" rel="noopener noreferrer">Open {item.label}</a>}<Button type="button" variant="outline" onClick={() => evidence.remove(i)} aria-label={`Remove evidence ${i + 1}`}>Remove evidence</Button></div></div>)}
    <Button type="button" variant="outline" disabled={evidence.fields.length >= 30} onClick={() => evidence.append({ id: crypto.randomUUID(), kind: 'url', label: '', url: '', file_id: null, visibility: 'organisers' })}>Add evidence link</Button>
    <Label htmlFor="task-file">Upload evidence file</Label><Input id="task-file" type="file" accept="application/pdf,image/png,image/jpeg,text/plain" onChange={e => { setFile(e.target.files?.[0]); setMessage(''); setFailure(undefined) }} /><Button type="button" variant="outline" disabled={!file || evidence.fields.length >= 30} onClick={() => void upload()}>Upload selected file</Button>{message && <p role="status">{message}</p>}{!!failure && <Failure error={failure} />}
  </fieldset>
}

import { useEffect, useRef, type Ref } from 'react'
import { Button } from './components/ui/button'
import { Input, Label } from './components/ui/fields'
import { Alert } from './components/ui/surfaces'
import { Failure } from './components/states'

export type ProtectedUploadControlsProps = {
  id: string; label: string; inputLabel?: string; action: string
  file?: Pick<File, 'name' | 'size' | 'type'>
  state: 'idle' | 'pending' | 'unknown' | 'denied'; disabled: boolean
  notice?: string; failure?: unknown; inputRef?: Ref<HTMLInputElement>
  select: (file: File | undefined) => void; upload: () => void
}
// Shared controlled presentation. The owner owns private bytes, exact captured
// payload/key/revision, authority, request scope, outcomes and attachment effects.
export function ProtectedUploadControls({ id, label, inputLabel, action, file, state, disabled, notice, failure, inputRef, select, upload }: ProtectedUploadControlsProps) {
  const result = useRef<HTMLElement>(null), pending = state === 'pending', unknown = state === 'unknown'
  useEffect(() => { if (unknown || state === 'denied' || notice) result.current?.focus() }, [unknown, state, notice])
  if (state === 'denied') return <section ref={result} tabIndex={-1} aria-label="Upload access denied"><Failure error={failure} /></section>
  return <section className="fleet-resource-section" aria-label={label}><fieldset className="fleet-resource-fields" disabled={disabled || pending || unknown}><legend>{label}</legend><Label htmlFor={id}>{inputLabel ?? `Choose ${label.toLowerCase()}`}</Label><Input ref={inputRef} id={id} type="file" accept="application/pdf,image/png,image/jpeg,text/plain" onChange={event => select(event.target.files?.[0])} />{file && <p>{file.name} · {Math.ceil(file.size / 1024)} KiB · {file.type}</p>}</fieldset>{unknown && <section ref={result} tabIndex={-1} aria-label="Unconfirmed upload"><Alert tone="warning" role="status"><h4>Upload outcome unknown</h4><p>The original file may have been stored. A deliberate retry repeats the exact key, bytes, metadata, team and reason. Current authorisation is checked again; no retry is automatic.</p></Alert></section>}<Button type="button" icon="upload" pending={pending} disabled={disabled || (!file && !unknown)} onClick={upload}>{action}</Button>{notice && <section ref={result} tabIndex={-1} role="status">{notice}</section>}{!!failure && <Failure error={failure} />}</section>
}

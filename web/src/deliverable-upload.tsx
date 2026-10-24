import { ProtectedUpload } from './protected-upload'
import type { EventRecord } from './api/client'
import type { Evidence } from './deliverable-model'

export function DeliverableUpload({ event, teamID, item, reason, disabled, setBusy, change, onDeny }: { event: EventRecord; teamID: string; item: Evidence; reason: string; disabled: boolean; busy: boolean; setBusy: (value: boolean) => void; change: (item: Evidence) => void; onDeny?: (error: unknown) => void }) {
  return <ProtectedUpload eventID={event.id} teamID={teamID} reason={reason} id={`deliverable-file-${item.deliverable_id}`} label={`Evidence file for ${item.label}`} inputLabel={`File for ${item.label}`} action={`Upload ${item.label}`} disabled={disabled} busy={setBusy} failed={onDeny} accepted={file => change({ ...item, file_id: file.id, url: '' })} />
}

import { useEffect, useRef, useState } from 'react'
import { useInfiniteQuery } from '@tanstack/react-query'
import { api, APIError, getSession, privateKey, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure } from './components/states'
import { mediaDenied, type MediaContext } from './media-workspace'

type Metadata = components['schemas']['MediaMetadata']
type Item = components['schemas']['MediaItem']
type Command = components['schemas']['MediaCommand']
type Operation = { id: string; revision: number; command: Command } | { upload: components['schemas']['MediaPhotoUpload']; revision: number; key: string }
const empty = (): Metadata => ({ kind: 'photo', title: '', caption: '', captured_at: null, recording_url: '', people: [], challenge_related: false })

function useMediaWrite(context: MediaContext) {
  const identity = useIdentity(), [pending, setPending] = useState(false), [uncertain, setUncertain] = useState(false), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState('')
  const alive = useRef(true), inFlight = useRef(false)
  const retained = useRef<Operation | null>(null), generation = useRef(0), request = useRef<AbortController | null>(null)
  useEffect(() => () => { alive.current = false; generation.current++; request.current?.abort(); retained.current = null }, [])
  async function send(operation?: Operation) {
    if (!alive.current || inFlight.current || pending || !context.writable) return
    if (uncertain && operation) return
    const intent = operation ?? retained.current
    if (!intent) return
    inFlight.current = true; retained.current = intent; const ticket = generation.current, abort = new AbortController(); request.current = abort
    setPending(true); setFailure(undefined); setNotice('Checking current access…')
    try {
      const session = await getSession({ signal: abort.signal })
      if (ticket !== generation.current || abort.signal.aborted || session.person.id !== identity.person.id || session.draft_scope !== identity.draft_scope) return
      // This current-authority read must finish before an unsent write. It never
      // retimes/revises the retained intent after an unknown response.
      unwrap(await api.GET('/api/v1/events/{event_id}/media', { signal: abort.signal, cache: 'no-store', params: { path: { event_id: context.eventID }, query: { limit: 1 } } }))
      if (ticket !== generation.current || abort.signal.aborted) return
      const header = { 'X-CSRF-Token': session.csrf_token, 'If-Match': `"${intent.revision}"` }
      if ('upload' in intent) unwrap(await api.POST('/api/v1/events/{event_id}/media/upload', { signal: abort.signal, params: { path: { event_id: context.eventID }, header: { ...header, 'Idempotency-Key': intent.key } }, body: intent.upload }))
      else unwrap(await api.PUT('/api/v1/events/{event_id}/media/{media_id}', { signal: abort.signal, params: { path: { event_id: context.eventID, media_id: intent.id }, header }, body: intent.command }))
      if (ticket !== generation.current) return
      retained.current = null; setUncertain(false); setNotice('Saved. Current media has been refreshed.'); await context.refresh()
    } catch (error) {
      if (ticket !== generation.current) return
      setFailure(error); context.deny(error)
      if (error instanceof APIError && error.status < 500) { retained.current = null; setUncertain(false); setNotice('The request was rejected. Refresh and review before a new intent.') }
      else { setUncertain(true); setNotice('The outcome is uncertain. Check current media before deliberately retrying this identical intent. No automatic retry will run.') }
      await context.refresh()
    } finally { inFlight.current = false; if (ticket === generation.current) setPending(false) }
  }
  return { pending, uncertain, send, notice, controls: <>{!!failure && <Failure error={failure} />}<p role="status">{notice}</p>{uncertain && <div className="actions"><Button variant="outline" disabled={pending} onClick={() => void context.refresh()}>Check current media</Button><Button variant="outline" disabled={pending || !context.writable} onClick={() => void send()}>Retry identical media intent</Button><Button variant="outline" disabled={pending} onClick={() => { retained.current = null; setUncertain(false); setNotice('Retained intent discarded. Check current media before starting again.'); setFailure(undefined) }}>Discard retained intent</Button></div>}</> }
}
export function MediaCollector({ context }: { context: MediaContext }) {
  const preparing = useRef(false)
  const [metadata, setMetadata] = useState(empty), [file, setFile] = useState<File | null>(null), [reason, setReason] = useState(''), [localError, setLocalError] = useState<unknown>(), write = useMediaWrite(context)
  async function submit() {
    if (preparing.current) return
    preparing.current = true
    setLocalError(undefined)
    try {
      if (metadata.kind === 'photo') {
        if (!file || !['image/png', 'image/jpeg'].includes(file.type) || file.size > 8 * 1024 * 1024) throw new Error('Choose a PNG or JPEG photograph up to 8 MiB.')
        const bytes = new Uint8Array(await file.arrayBuffer()); let binary = ''; for (let i = 0; i < bytes.length; i += 8192) binary += String.fromCharCode(...bytes.subarray(i, i + 8192))
        await write.send({ revision: context.revision, key: crypto.randomUUID(), upload: { id: crypto.randomUUID(), metadata, reason, filename: file.name, media_type: file.type as 'image/png' | 'image/jpeg', content: btoa(binary) } })
      } else await write.send({ id: crypto.randomUUID(), revision: context.revision, command: { action: 'save', version: 0, command_id: crypto.randomUUID(), metadata, reason } })
    } catch (error) { setLocalError(error) } finally { preparing.current = false }
  }
  return <section className="panel media-private"><h2>Submit media for organiser review</h2><p>Uploads and unsent metadata are private to you and organisers. Tag every identifiable participant. Unknown or refused consent blocks release; untagged media still needs an explicit identification review. Keep other sensitive details out of captions.</p><form onSubmit={event => { event.preventDefault(); void submit() }}><fieldset disabled={write.pending || write.uncertain || !context.writable}>
    <label htmlFor="media-kind">Media type</label><select id="media-kind" value={metadata.kind} onChange={e => { setMetadata({ ...metadata, kind: e.target.value as Metadata['kind'], recording_url: '' }); setFile(null) }}><option value="photo">Photograph</option><option value="recording">External recording link</option></select>
    <MetadataFields metadata={metadata} change={setMetadata} context={context} prefix="new-media" />
    {metadata.kind === 'photo' && <><label htmlFor="media-file">Photograph (PNG or JPEG, up to 8 MiB)</label><input id="media-file" type="file" accept="image/png,image/jpeg" onChange={e => setFile(e.target.files?.[0] ?? null)} /></>}
    <label htmlFor="media-upload-reason">Submission reason</label><input id="media-upload-reason" maxLength={2000} required value={reason} onChange={e => setReason(e.target.value)} /><Button type="submit" disabled={!metadata.title.trim() || !reason.trim() || (metadata.kind === 'photo' && !file)}>Submit private media draft</Button>
  </fieldset></form>{!!localError && <Failure error={localError} />}{write.controls}</section>
}
export function MediaEditor({ item, context }: { item: Item; context: MediaContext }) {
  const [open, setOpen] = useState(false), [metadata, setMetadata] = useState<Metadata>({ kind: item.kind, title: item.title, caption: item.caption, captured_at: item.captured_at, recording_url: item.recording_url, people: item.people.map(p => p.id), challenge_related: item.challenge_related }), [reason, setReason] = useState(''), [identified, setIdentified] = useState(false), [none, setNone] = useState(false), [content, setContent] = useState(false), write = useMediaWrite(context)
  const change = (action: Command['action']) => write.send({ id: item.id, revision: context.revision, command: { action, version: item.version, command_id: crypto.randomUUID(), reason, ...(action === 'save' ? { metadata } : action === 'review' ? { review: { identification_reviewed: identified, no_identifiable_people: none, content_reviewed: content } } : {}) } })
  return <section className="media-private"><Button variant="outline" onClick={() => setOpen(!open)}>{open ? 'Close private media review' : 'Open private media review'}</Button>{open && <><h4>Private submission and review</h4><p>Consent: {item.consent_state}. Metadata edits discard the previous review and release. Publication is a separate explicit action.</p>{item.people.length > 0 && <ul>{item.people.map(p => <li key={p.id}>{p.name || 'Unavailable participant'} · {p.consent}{!p.eligible && ' · No current event eligibility'}</li>)}</ul>}<fieldset disabled={write.pending || write.uncertain || !context.writable}><MetadataFields metadata={metadata} change={setMetadata} context={context} prefix={`edit-${item.id}`} /><label htmlFor={`media-reason-${item.id}`}>Reason for this action</label><input id={`media-reason-${item.id}`} maxLength={2000} value={reason} onChange={e => setReason(e.target.value)} /><div className="actions"><Button disabled={!reason.trim()} onClick={() => void change('save')}>Save metadata draft</Button>{item.can_withdraw && <Button variant="outline" disabled={!reason.trim()} onClick={() => void change('withdraw')}>Withdraw media</Button>}</div>
    {item.can_review && <><label><input type="checkbox" checked={identified} onChange={e => setIdentified(e.target.checked)} />I checked the media and identified every recognisable person.</label>{item.people.length === 0 && <label><input type="checkbox" checked={none} onChange={e => setNone(e.target.checked)} />No identifiable people are present.</label>}<label><input type="checkbox" checked={content} onChange={e => setContent(e.target.checked)} />I reviewed the content, caption, recording destination and challenge classification.</label><Button disabled={!reason.trim() || !identified || !content || (item.people.length === 0 && !none)} onClick={() => void change('review')}>Record organiser review</Button></>}
    {item.can_publish && <Button disabled={!reason.trim()} onClick={() => void change('publish')}>Release reviewed media to gallery</Button>}
  </fieldset></>}{write.controls}</section>
}
function MetadataFields({ metadata, change, context, prefix }: { metadata: Metadata; change: (value: Metadata) => void; context: MediaContext; prefix: string }) {
  const identity = useIdentity()
  const guide = useInfiniteQuery({ queryKey: [...privateKey(identity.person.id, context.eventID, 'media-guidance'), identity.draft_scope], gcTime: 0, retry: false, refetchOnWindowFocus: 'always', refetchInterval: 10_000, initialPageParam: undefined as string | undefined, queryFn: async ({ pageParam, signal }) => unwrap(await api.GET('/api/v1/events/{event_id}/media/consent-guidance', { signal, cache: 'no-store', params: { path: { event_id: context.eventID }, query: { after: pageParam, limit: 50 } } })), getNextPageParam: page => page.next_cursor ?? undefined })
  useEffect(() => { if (mediaDenied(guide.error)) context.deny(guide.error) }, [guide.error, context])
  return <><label htmlFor={`${prefix}-title`}>Media title</label><input id={`${prefix}-title`} maxLength={200} required value={metadata.title} onChange={e => change({ ...metadata, title: e.target.value })} /><label htmlFor={`${prefix}-caption`}>Caption</label><textarea id={`${prefix}-caption`} rows={3} maxLength={4000} value={metadata.caption} onChange={e => change({ ...metadata, caption: e.target.value })} /><label htmlFor={`${prefix}-captured`}>Captured at (local time, optional)</label><input id={`${prefix}-captured`} type="datetime-local" value={metadata.captured_at ? localInput(metadata.captured_at) : ''} onChange={e => change({ ...metadata, captured_at: e.target.value ? new Date(e.target.value).toISOString() : null })} /><label><input type="checkbox" checked={metadata.challenge_related} onChange={e => change({ ...metadata, challenge_related: e.target.checked })} />Contains challenge or brief content (blocked until actual common release)</label>
    {metadata.kind === 'recording' && <><label htmlFor={`${prefix}-url`}>Recording HTTPS link</label><input id={`${prefix}-url`} type="url" maxLength={2048} required value={metadata.recording_url} onChange={e => change({ ...metadata, recording_url: e.target.value })} /><p>Provide an approved destination. The app stores the link and does not fetch, embed or host its content.</p></>}
    <fieldset><legend>Identifiable participants</legend><p>Only event names and photo-consent indicators are shown. Unknown and “no” are not permission to release.</p>{guide.error ? <Failure error={guide.error} retry={() => void guide.refetch()} /> : guide.data?.pages.flatMap(p => p.items).map(person => <label key={person.person_id}><input type="checkbox" checked={metadata.people.includes(person.person_id)} onChange={e => change({ ...metadata, people: e.target.checked ? [...metadata.people, person.person_id] : metadata.people.filter(id => id !== person.person_id) })} />{person.display_name || 'Unavailable participant'} · {person.choice}</label>)}{guide.hasNextPage && <Button type="button" variant="outline" disabled={guide.isFetchingNextPage} onClick={() => void guide.fetchNextPage()}>More consent guidance</Button>}</fieldset>
  </>
}
function localInput(value: string) { const d = new Date(value); return new Date(d.getTime() - d.getTimezoneOffset() * 60_000).toISOString().slice(0, 16) }

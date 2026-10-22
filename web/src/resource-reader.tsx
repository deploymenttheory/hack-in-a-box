import { useEffect, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, APIError, privateKey, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { ContentContacts } from './content-contacts'
import { SafeMarkdown } from './components/safe-markdown'
import { localDate } from './event-time'

export function ResourceReader({ id, event, back }: { id: string; event: EventRecord; back: () => void }) {
  const identity = useIdentity(), cache = useQueryClient(), heading = useRef<HTMLHeadingElement>(null)
  const [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>(), [reloadRequired, setReloadRequired] = useState(false)
  const key = [...privateKey(identity.person.id, event.id, 'resource-reader'), id]
  const resource = useQuery({ queryKey: key, queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/content/{content_id}', { params: { path: { event_id: event.id, content_id: id } } })), refetchOnWindowFocus: 'always', refetchInterval: 15_000 })
  useEffect(() => { heading.current?.focus() }, [resource.data?.id])
  async function reload() { const result = await resource.refetch(); if (!result.error) { setFailure(undefined); setReloadRequired(false) } }
  async function acknowledge() {
    if (!resource.data) return
    setPending(true); setFailure(undefined)
    try {
      const result = unwrap(await api.POST('/api/v1/events/{event_id}/content/{content_id}/acknowledgements', { params: { path: { event_id: event.id, content_id: id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${resource.data.version}"` } } }))
      cache.setQueryData(key, result)
      await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, event.id, 'resource-library') })
      await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, event.id, 'event-guidance') })
    } catch (error) { setFailure(error); if (error instanceof APIError && [403, 404, 409].includes(error.status)) setReloadRequired(true) } finally { setPending(false) }
  }
  const page = resource.data
  return <section className="resource-reader"><Button variant="outline" onClick={back}>Back to resources</Button>
    {resource.isPending ? <Loading /> : resource.error ? <Failure error={resource.error} retry={() => void reload()} /> : reloadRequired ? <div className="panel"><Failure error={failure} /><p>This resource or your access changed. Load the current released version before continuing.</p><Button onClick={() => void reload()}>Reload latest resource</Button></div> : page && <article className="panel"><p className="eyebrow">Released version {page.version}</p><h2 ref={heading} tabIndex={-1}>{page.document.title}</h2><p className="muted">Released {localDate(page.released_at, event.configuration.timezone)} · {event.configuration.timezone}</p>
      {page.kind === 'challenge' && <p className="notice">The common challenge is released. Your team may start building now. Formal submissions and mentor or judge review open at live start: {localDate(event.configuration.live_start, event.configuration.timezone)}.</p>}
      <SafeMarkdown>{page.document.markdown}</SafeMarkdown><ContentContacts document={page.document} />
      {page.attachment_details.length > 0 && <section aria-label="Resource attachments"><h3>Attachments</h3><ul className="item-list">{page.attachment_details.map(a => <li key={a.id}><div>{a.can_download ? <a className="content-link" href={`/api/v1/events/${event.id}/files/${a.id}`}>{a.filename}</a> : <strong>{a.filename} · Unavailable</strong>}<p>{a.media_type} · {Math.ceil(a.size / 1024)} KiB</p></div></li>)}</ul><p className="muted">Downloads check your current access. Previously downloaded copies cannot be recalled.</p></section>}
      {page.previous_acknowledgement && !page.acknowledged_at && <p className="notice">You acknowledged version {page.previous_acknowledgement.version} on {localDate(page.previous_acknowledgement.at, event.configuration.timezone)}. This released version has changed; that receipt does not acknowledge version {page.version}.</p>}
      {page.document.require_acknowledgement && <section className="resource-acknowledgement" aria-label="Required acknowledgement"><h3>Required reading</h3>{page.acknowledged_at ? <p role="status">You acknowledged version {page.version} on {localDate(page.acknowledged_at, event.configuration.timezone)}.</p> : <><p>Confirm that you have read this version and its attachments. A material update can require a new acknowledgement.</p><Button disabled={pending || ['archived', 'cancelled'].includes(event.stage)} onClick={() => void acknowledge()}>I have read version {page.version}</Button>{['archived', 'cancelled'].includes(event.stage) && <p>This event no longer accepts acknowledgements.</p>}</>}{!!failure && <Failure error={failure} />}</section>}
    </article>}
  </section>
}

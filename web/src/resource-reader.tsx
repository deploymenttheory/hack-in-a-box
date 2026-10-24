import { useEffect, useId, useRef, useState } from 'react'
import { useQuery, useQueryClient, type InfiniteData } from '@tanstack/react-query'
import { api, privateKey, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { usePlanningUnknownRedaction } from './planning-operations'
import { Button } from './components/ui/button'
import { ConfirmationDialog } from './components/ui/dialog'
import { CollectionHeading } from './components/patterns'
import { Failure, Loading } from './components/states'
import { ContentContacts } from './content-contacts'
import { ObjectiveDisclosure } from './objective-disclosure'
import { SafeMarkdown } from './components/safe-markdown'
import { localDate } from './event-time'
import { usePrivateRequestScope } from './private-request-scope'
import { resourceDenied, ResourceOperationResult, useResourceOperation } from './resource-operation'

type Page = import('./content-model').ContentPage
export function ResourceReader({ id, event, back }: { id: string; event: EventRecord; back: () => void }) {
  const identity = useIdentity()
  return <ReaderLifetime key={`${identity.person.id}:${identity.draft_scope}:${event.id}:${id}`} id={id} event={event} back={back} />
}
function ReaderLifetime({ id, event, back }: { id: string; event: EventRecord; back: () => void }) {
  const identity = useIdentity(), mount = useId(), [denied, setDenied] = useState<unknown>(), scope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${event.id}:${id}:reader`)
  const cache = useQueryClient(), cleared = useRef(false), redactAcknowledgement = usePlanningUnknownRedaction(owner => owner === `${identity.person.id}:${identity.draft_scope}:${event.id}:${id}:acknowledgement`)
  // Cached private content is not authority for a new mount: every parent
  // path must finish this lifetime's current GET before rendering the document.
  const key = [...privateKey(identity.person.id, event.id, 'resource-reader'), id, mount]
  const resource = useQuery({ queryKey: key, enabled: !denied, retry: false, queryFn: async ({ signal }) => { const ticket = scope.capture(); const value = unwrap(await api.GET('/api/v1/events/{event_id}/content/{content_id}', { signal, params: { path: { event_id: event.id, content_id: id } } })); ticket.assert(); return value }, refetchOnWindowFocus: 'always', refetchInterval: 15_000 })
  useEffect(() => () => {
    const owned = [...privateKey(identity.person.id, event.id, 'resource-reader'), id, mount]
    void cache.cancelQueries({ queryKey: owned, exact: true })
    cache.removeQueries({ queryKey: owned, exact: true })
  }, [cache, identity.person.id, event.id, id, mount])
  const revoked = !!denied || resourceDenied(resource.error)
  useEffect(() => {
    if (!revoked || cleared.current) return
    cleared.current = true
    setDenied(denied ?? resource.error)
    redactAcknowledgement(attempt => ({ ...attempt, operation: 'Resource acknowledgement', command: { redacted: 'Resource access revoked' } }))
    scope.invalidate()
    const affected = [...privateKey(identity.person.id, event.id, 'resource-reader'), id]
    void cache.cancelQueries({ queryKey: affected })
    cache.removeQueries({ queryKey: affected })
    // A per-page denial revokes this page only, not unrelated event resources.
    cache.setQueriesData<InfiniteData<{ items: Page[]; next_cursor: string | null }>>({ queryKey: privateKey(identity.person.id, event.id, 'resource-library') }, value => value && ({ ...value, pages: value.pages.map(page => ({ ...page, items: page.items.filter(item => item.id !== id) })) }))
    cache.setQueriesData<import('./api/schema.gen').components['schemas']['EventGuidance']>({ queryKey: privateKey(identity.person.id, event.id, 'event-guidance') }, value => value && ({ ...value, items: value.items.filter(item => item.id !== id) }))
    void cache.invalidateQueries({ queryKey: privateKey(identity.person.id, event.id, 'resource-library') })
    void cache.invalidateQueries({ queryKey: privateKey(identity.person.id, event.id, 'event-guidance') })
  }, [revoked, denied, resource.error, redactAcknowledgement, cache, identity.person.id, event.id, id, scope])
  if (revoked) return <section className="fleet-resource-reader"><Button icon="back" variant="ghost" onClick={back}>Back to resources</Button><Failure error={denied ?? resource.error} /></section>
  if (resource.isPending) return <section className="fleet-resource-reader"><Button icon="back" variant="ghost" onClick={back}>Back to resources</Button><Loading /></section>
  if (!resource.data) return <Failure error={resource.error} retry={() => void resource.refetch()} />
  return <ReleasedDocument key={`${identity.person.id}:${identity.draft_scope}:${event.id}:${id}`} event={event} id={id} page={resource.data} acceptPage={page => cache.setQueryData(key, page)} paused={!!resource.error} onDeny={setDenied} back={back} read={async () => { const result = await resource.refetch(); if (result.error) throw result.error; return result.data! }} />
}
function ReleasedDocument({ event, id, page, acceptPage, paused, onDeny, back, read }: { event: EventRecord; id: string; page: Page; acceptPage: (page: Page) => void; paused: boolean; onDeny: (error: unknown) => void; back: () => void; read: () => Promise<Page> }) {
  const identity = useIdentity(), cache = useQueryClient(), heading = useRef<HTMLDivElement>(null), opener = useRef<HTMLButtonElement>(null)
  const operation = useResourceOperation(`${identity.person.id}:${identity.draft_scope}:${event.id}:${id}:acknowledgement`, onDeny, { preserveUnknownOnDenial: true })
  const [review, setReview] = useState<{ version: number; title: string }>()
  useEffect(() => { const target = heading.current?.querySelector('h1'); if (target) { target.tabIndex = -1; target.focus() } }, [id, page.version])
  async function acknowledge() {
    if (!review || paused) return
    const intent = review
    const ticket = operation.capture()
    await operation.run({ operation: `Acknowledge ${intent.title}`, revision: intent.version, command: { content_id: id, version: intent.version } }, async () => {
      const result = unwrap(await api.POST('/api/v1/events/{event_id}/content/{content_id}/acknowledgements', { params: { path: { event_id: event.id, content_id: id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${intent.version}"` } } }))
      if (result.id !== id || result.version !== intent.version || !result.acknowledged_at) throw new Error('Unexpected acknowledgement response')
      return result
    }, async (result, ticket) => { acceptPage(result); setReview(undefined); await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, event.id, 'resource-library') }); ticket.assert(); await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, event.id, 'event-guidance') }); return `Acknowledgement recorded for version ${result.version}.` })
    if (ticket.current()) setReview(undefined)
  }
  function readCurrent() { void operation.read(() => read(), () => { setReview(undefined) }) }
  return <section className="fleet-resource-reader"><Button icon="back" variant="ghost" disabled={operation.blocked} onClick={() => { if (operation.canLeave()) back() }}>Back to resources</Button><div ref={heading} tabIndex={-1}><CollectionHeading title={page.document.title} scope={{ event: event.name, entity: `Released version ${page.version}` }} description={<>Released {localDate(page.released_at, event.configuration.timezone)} · {event.configuration.timezone}</>} /></div>{paused && <p role="status">Current access could not be refreshed. Reading is retained; acknowledgement is paused.</p>}
    {page.kind === 'challenge' && <p className="fleet-resource-section">The common challenge is released. Your team may start building now. Formal submissions and mentor or judge review open at live start: {localDate(event.configuration.live_start, event.configuration.timezone)}.</p>}
    <article className="fleet-resource-prose" aria-label="Released resource text"><SafeMarkdown>{page.document.markdown}</SafeMarkdown><ContentContacts document={page.document} /></article>
    {page.document.objectives?.map(reference => <ObjectiveDisclosure key={`${event.id}:${id}:${reference.objective.id}:${reference.objective.version}`} eventID={event.id} contentID={id} reference={reference} />)}
    {page.attachment_details.length > 0 && <section className="fleet-resource-section" aria-label="Resource attachments"><h3>Attachments</h3><ul className="fleet-resource-records">{page.attachment_details.map(file => <li key={file.id}><div>{file.can_download ? <a href={`/api/v1/events/${event.id}/files/${file.id}`}>{file.filename}</a> : <strong>{file.filename} · Unavailable</strong>}<p>{file.media_type} · {Math.ceil(file.size / 1024)} KiB</p></div></li>)}</ul><p>Downloads check your current access. Previously downloaded copies cannot be recalled.</p></section>}
    {page.previous_acknowledgement && !page.acknowledged_at && <p className="fleet-resource-section">You acknowledged version {page.previous_acknowledgement.version} on {localDate(page.previous_acknowledgement.at, event.configuration.timezone)}. This released version has changed; that receipt does not acknowledge version {page.version}.</p>}
    {page.document.require_acknowledgement && <section className="fleet-resource-section" aria-label="Required acknowledgement"><h3>Required reading</h3>{page.acknowledged_at ? <p role="status">You acknowledged version {page.version} on {localDate(page.acknowledged_at, event.configuration.timezone)}.</p> : <><p>Confirm that you have read this version and its attachments. A material update can require a new acknowledgement.</p><Button ref={opener} icon="check" disabled={paused || operation.blocked || ['archived', 'cancelled'].includes(event.stage)} onClick={() => setReview({ version: page.version, title: page.document.title })}>I have read version {page.version}</Button>{['archived', 'cancelled'].includes(event.stage) && <p>This event no longer accepts acknowledgements.</p>}</>}</section>}
    <ResourceOperationResult operation={operation} read={readCurrent} /><ConfirmationDialog open={!!review} onOpenChange={open => { if (!open && !operation.pending) setReview(undefined) }} title="Review reading acknowledgement" description="This records your own confirmation for the named released version and its attachments. It does not acknowledge a future material update." confirmLabel="Confirm acknowledgement" pending={operation.pending} disabled={paused || operation.blocked} finalFocus={opener} onConfirm={() => void acknowledge()}><p>{review?.title} · Released version {review?.version}</p></ConfirmationDialog>
  </section>
}

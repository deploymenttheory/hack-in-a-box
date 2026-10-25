import { useId, useLayoutEffect, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, APIError, privateKey, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Icon } from './components/ui/icon'
import { StatusBadge } from './components/ui/feedback'
import { Failure, Loading } from './components/states'
import { usePrivateRequestScope, type PrivateRequestScope } from './private-request-scope'
import { useResourceReturnFocus } from './resource-focus'
import { safeContactURL } from './content-model'
import { localDate } from './event-time'
import { ResourceReader } from './resource-reader'
import { usePlanningUnknownRedaction } from './planning-operations'

export function EventGuidance({ event, open, readiness = false, parentScope, paused = false }: { paused?: boolean; parentScope?: PrivateRequestScope; event: EventRecord; open?: (id: string, opener?: HTMLElement) => void; readiness?: boolean }) {
  const identity = useIdentity()
  return <GuidanceLifetime key={`${identity.person.id}:${identity.draft_scope}:${event.id}`} event={event} open={open} readiness={readiness} parentScope={parentScope} paused={paused} />
}
function GuidanceLifetime({ event, open, readiness, parentScope, paused }: { paused: boolean; parentScope?: PrivateRequestScope; event: EventRecord; open?: (id: string, opener?: HTMLElement) => void; readiness: boolean }) {
  const identity = useIdentity(), mount = useId(), cache = useQueryClient(), [selected, setSelected] = useState<string>(), root = useRef<HTMLElement>(null), focus = useResourceReturnFocus(root)
  const [generation, setGeneration] = useState(0), [denial, setDenial] = useState<unknown>(), revoked = useRef(false)
  const scope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${event.id}:guidance:${generation}`, parentScope)
  const prefix = `${identity.person.id}:${identity.draft_scope}:${event.id}:`
  const redactAcknowledgements = usePlanningUnknownRedaction(owner => owner.startsWith(prefix) && owner.endsWith(':acknowledgement'))
  const query = useQuery({ queryKey: [...privateKey(identity.person.id, event.id, 'event-guidance'), generation, ...(parentScope ? [mount] : [])], gcTime: parentScope ? 0 : undefined, enabled: !denial, retry: false, queryFn: async ({ signal }) => { const ticket = scope.capture(); const value = unwrap(await api.GET('/api/v1/events/{event_id}/guidance', { signal, params: { path: { event_id: event.id } } })); ticket.assert(); return value }, refetchOnWindowFocus: 'always', refetchInterval: 15_000 })
  const denied = !!denial || (query.error instanceof APIError && [401, 403, 404].includes(query.error.status))
  useLayoutEffect(() => {
    if (!denied) { revoked.current = false; return }
    if (revoked.current) return
    revoked.current = true
    setDenial(denial ?? query.error); setSelected(undefined); scope.invalidate()
    // The standalone guidance parent must scrub access before disposing a child
    // whose own pending denial may not have arrived yet.
    for (const name of ['resource-library', 'resource-reader', 'event-guidance', 'released-objective']) {
      const affected = privateKey(identity.person.id, event.id, name)
      void cache.cancelQueries({ queryKey: affected })
      cache.removeQueries({ queryKey: affected })
    }
    redactAcknowledgements(attempt => ({ ...attempt, operation: 'Resource acknowledgement', command: { redacted: 'Resource access revoked' } }))
  }, [denied, denial, query.error, scope, cache, identity.person.id, event.id, redactAcknowledgements])
  function retry() { if (denied) { setGeneration(value => value + 1); setDenial(undefined) } else void query.refetch() }
  if (selected && !denied) return <section ref={root}><ResourceReader paused={paused} parentScope={parentScope ? scope : undefined} key={selected} id={selected} event={event} back={() => { setSelected(undefined); focus.restore() }} /></section>
  return <section ref={root} className="fleet-resource-section" aria-label={readiness ? 'Guidance readiness' : 'Rules and contacts'}><h3>{readiness ? 'Guidance readiness' : 'Rules and contacts'}</h3>
    {query.isPending ? <Loading /> : denied ? <><p>Released guidance is available after your event access is confirmed.</p><Button icon="refresh" variant="outline" onClick={retry}>Check guidance access</Button></> : !query.data ? <Failure error={query.error} retry={retry} /> : <>
      {!!query.error && <Failure error={query.error} retry={retry} />}
      {readiness && query.data.warnings.length > 0 && <section><p>Organiser preparation reminders. These do not block the event lifecycle.</p><ul>{query.data.warnings.map(warning => <li key={warning.code}>{warning.message}</li>)}</ul></section>}
      <ul className="fleet-resource-records">{query.data.items.map(item => <li key={item.id}><div><p className="fleet-scope-label"><Icon name={item.purpose === 'rules' ? 'book' : 'help'} /> {item.purpose === 'conduct' ? 'Conduct reporting' : item.purpose === 'support' ? 'Support' : 'Rules'}</p><h4>{item.title}</h4><p>Released version {item.version}</p>
        {item.require_acknowledgement && <p><StatusBadge tone={item.acknowledged_at ? 'success' : 'warning'}>{item.acknowledged_at ? `You acknowledged version ${item.version} on ${localDate(item.acknowledged_at, event.configuration.timezone)}.` : item.previous_acknowledgement ? `You acknowledged version ${item.previous_acknowledgement.version}. Read and acknowledge the updated version ${item.version}.` : 'Read and acknowledge this version.'}</StatusBadge></p>}
        {item.contacts.length > 0 && <ul>{item.contacts.map((contact, index) => <li key={index}>{safeContactURL(contact.url) ? <a href={contact.url} target="_blank" rel="noopener noreferrer">{contact.label}</a> : <span>Contact unavailable</span>}</li>)}</ul>}
        {item.purpose === 'conduct' && <p>Use the published contact route for concerns. This app does not collect reports; external services have their own privacy arrangements.</p>}
      </div><Button data-resource-opener={`guidance:${item.id}`} icon="book" variant="outline" disabled={!!query.error || paused} onClick={event => { if (open) open(item.id, event.currentTarget); else { focus.capture(event.currentTarget); setSelected(item.id) } }}>Read {item.title}</Button></li>)}</ul>
      {!query.data.items.length && <p>No rules or contact pages have been released to your current audience yet.</p>}
    </>}
  </section>
}

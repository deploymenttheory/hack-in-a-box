import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, APIError, privateKey, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { safeContactURL } from './content-model'
import { localDate } from './event-time'
import { ResourceReader } from './resource-reader'

export function EventGuidance({ event, open, readiness = false }: { event: EventRecord; open?: (id: string) => void; readiness?: boolean }) {
  const identity = useIdentity(), [selected, setSelected] = useState<string>()
  const query = useQuery({ queryKey: privateKey(identity.person.id, event.id, 'event-guidance'), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/guidance', { params: { path: { event_id: event.id } } })), refetchOnWindowFocus: 'always', refetchInterval: 15_000 })
  if (selected) return <ResourceReader key={selected} id={selected} event={event} back={() => setSelected(undefined)} />
  return <section className="event-guidance" aria-label={readiness ? 'Guidance readiness' : 'Rules and contacts'}><h3>{readiness ? 'Guidance readiness' : 'Rules and contacts'}</h3>
    {query.isPending ? <Loading /> : query.error ? query.error instanceof APIError && query.error.status === 403 ? <p>Released guidance is available after your event access is confirmed.</p> : <Failure error={query.error} retry={() => void query.refetch()} /> : <>
      {readiness && query.data.warnings.length > 0 && <div className="notice"><p>Organiser preparation reminders. These do not block the event lifecycle.</p><ul>{query.data.warnings.map(w => <li key={w.code}>{w.message}</li>)}</ul></div>}
      <div className="event-grid">{query.data.items.map(item => <article className="panel guidance-card" key={item.id}><span className="badge">{item.purpose === 'conduct' ? 'Conduct reporting' : item.purpose === 'support' ? 'Support' : 'Rules'}</span><h4>{item.title}</h4><p>Released version {item.version}</p>
        {item.require_acknowledgement && <p>{item.acknowledged_at ? `You acknowledged version ${item.version} on ${localDate(item.acknowledged_at, event.configuration.timezone)}.` : item.previous_acknowledgement ? `You acknowledged version ${item.previous_acknowledgement.version}. Read and acknowledge the updated version ${item.version}.` : 'Read and acknowledge this version.'}</p>}
        {item.contacts.length > 0 && <ul>{item.contacts.map((contact, index) => <li key={index}>{safeContactURL(contact.url) ? <a href={contact.url} target="_blank" rel="noopener noreferrer">{contact.label}</a> : <span>Contact unavailable</span>}</li>)}</ul>}
        {item.purpose === 'conduct' && <p className="muted">Use the published contact route for concerns. This app does not collect reports; external services have their own privacy arrangements.</p>}
        <Button variant="outline" onClick={() => open ? open(item.id) : setSelected(item.id)}>Read {item.title}</Button></article>)}</div>
      {!query.data.items.length && <p className="empty-state">No rules or contact pages have been released to your current audience yet.</p>}
    </>}
  </section>
}

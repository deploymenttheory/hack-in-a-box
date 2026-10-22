import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, privateKey, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { localDate } from './event-time'

export function LogisticsWorkspace({ event, back }: { event: EventRecord; back: () => void }) {
  const identity = useIdentity(), [cursors, setCursors] = useState<(string | undefined)[]>([undefined]), [search, setSearch] = useState(''), [exporting, setExporting] = useState(false), [failure, setFailure] = useState<unknown>()
  const after = cursors.at(-1)
  const query = useQuery({ queryKey: privateKey(identity.person.id, event.id, `logistics:${after ?? ''}`), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/logistics', { params: { path: { event_id: event.id }, query: { after, limit: 50 } } })), refetchOnWindowFocus: 'always', staleTime: 0, gcTime: 0 })
  async function download() {
    setExporting(true); setFailure(undefined)
    try {
      const data = unwrap(await api.GET('/api/v1/events/{event_id}/logistics/export', { params: { path: { event_id: event.id }, query: { after, limit: 50 } } }))
      const url = URL.createObjectURL(new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })), link = document.createElement('a')
      link.href = url; link.download = `logistics-page-${cursors.length}.json`; link.click(); setTimeout(() => URL.revokeObjectURL(url), 1000)
    } catch (error) { setFailure(error); await query.refetch() } finally { setExporting(false) }
  }
  return <section className="logistics-workspace"><h2>Restricted logistics</h2><p className="notice">Operational details for event delivery. Use the separate arrivals screen on shared check-in devices.</p><Button variant="outline" onClick={back}>Return to shared check-in</Button>
    {query.isPending ? <Loading /> : query.error ? <Failure error={query.error} retry={() => void query.refetch()} /> : <><p className="section-heading">Page {cursors.length} · Roster as of {localDate(query.data.roster_as_of, event.configuration.timezone)} ({event.configuration.timezone})</p><label htmlFor="logistics-search">Search this logistics page</label><input id="logistics-search" type="search" value={search} onChange={e => setSearch(e.target.value)} autoComplete="off" /><div className="actions"><Button variant="outline" disabled={query.isFetching} onClick={() => void query.refetch()}>Refresh operational details</Button><Button variant="outline" disabled={exporting} onClick={() => void download()}>Download page {cursors.length}</Button></div><p className="muted">Downloads include the complete page and record the exporting actor and event. They recheck current permissions and roster membership.</p>
      <div className="logistics-grid">{query.data.items.filter(p => p.display_name.toLocaleLowerCase().includes(search.trim().toLocaleLowerCase())).map(p => <article className="panel" key={p.person_id}><h3>{p.display_name}</h3><p>{p.competing ? 'Confirmed competitor' : 'Active staff'}</p><dl className="dates">{[
        ['Contact email', p.contact_email], ['Contact phone', p.contact_phone], ['Dietary arrangements', p.dietary], ['Accessibility arrangements', p.accessibility], ['Travel details', p.travel_details], ['Social attendance', p.social_attendance === 'yes' ? 'Attending' : p.social_attendance === 'no' ? 'Not attending' : 'Unknown — not confirmed'],
      ].map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value || 'Not provided'}</dd></div>)}</dl></article>)}</div>
      {!query.data.items.length && <p>No current competitors or staff on this page.</p>}
      <div className="actions"><Button variant="outline" disabled={cursors.length === 1 || query.isFetching} onClick={() => { setCursors(c => c.slice(0, -1)); setSearch('') }}>Previous page</Button><Button variant="outline" disabled={!query.data.next_cursor || query.isFetching} onClick={() => { setCursors(c => [...c, query.data.next_cursor ?? undefined]); setSearch('') }}>Next page</Button></div></>}{!!failure && <Failure error={failure} />}
  </section>
}

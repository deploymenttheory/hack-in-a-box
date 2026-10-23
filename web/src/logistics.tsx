import { CollectionRecords, CollectionPagination, DetailWorkspace, Inspector } from './components/patterns'
import { Input, Label } from './components/ui/fields'
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, privateKey, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { localDate } from './event-time'

export function LogisticsWorkspace({ event, back }: { event: EventRecord; back: () => void }) {
  const identity = useIdentity(), [cursors, setCursors] = useState<(string | undefined)[]>([undefined]), [search, setSearch] = useState(''), [exporting, setExporting] = useState(false), [failure, setFailure] = useState<unknown>(), [selected,setSelected] = useState<string>()
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
  const selectedPerson = query.data?.items.find(person=>person.person_id===selected)
  return <section className="logistics-workspace fleet-planning-private"><h2>Restricted logistics</h2><p className="notice">Operational details for event delivery. Use the separate arrivals screen on shared check-in devices.</p><Button variant="outline" onClick={back}>Return to shared check-in</Button>
    {query.isPending ? <Loading /> : query.error ? <Failure error={query.error} retry={() => void query.refetch()} /> : <><p className="section-heading">Page {cursors.length} · Roster as of {localDate(query.data.roster_as_of, event.configuration.timezone)} ({event.configuration.timezone})</p><Label htmlFor="logistics-search">Search this logistics page</Label><Input id="logistics-search" type="search" value={search} onChange={e => setSearch(e.target.value)} autoComplete="off" /><div className="actions"><Button variant="outline" disabled={query.isFetching} onClick={() => void query.refetch()}>Refresh operational details</Button><Button variant="outline" disabled={exporting} onClick={() => void download()}>Download page {cursors.length}</Button></div><p className="muted">Downloads include the complete page and record the exporting actor and event. They recheck current permissions and roster membership.</p>
      <DetailWorkspace collection={<CollectionRecords label="Current logistics page" rows={query.data.items.filter(p=>p.display_name.toLocaleLowerCase().includes(search.trim().toLocaleLowerCase()))} rowKey={p=>p.person_id} rowLabel={p=>p.display_name} destination={p=>({label:p.display_name,href:'#logistics-person',onNavigate:()=>setSelected(p.person_id)})} columns={[{id:'role',label:'Roster membership',essential:true,cell:p=>p.competing?'Confirmed competitor':'Active staff'},{id:'email',label:'Contact email',cell:p=>p.contact_email||'Not provided'},{id:'phone',label:'Contact phone',cell:p=>p.contact_phone||'Not provided'}]} actions={p=><Button variant="ghost" icon="shield" onClick={()=>setSelected(p.person_id)}>Review {p.display_name}</Button>} />} detail={selectedPerson&&<Inspector identity={selectedPerson.person_id} title={selectedPerson.display_name} scope={{event:event.name,entity:'Restricted logistics · Page '+cursors.length}} back={{label:'Back to logistics page',href:'#logistics',onNavigate:()=>setSelected(undefined)}}><dl id="logistics-person" className="dates">{[['Contact email',selectedPerson.contact_email],['Contact phone',selectedPerson.contact_phone],['Dietary arrangements',selectedPerson.dietary],['Accessibility arrangements',selectedPerson.accessibility],['Travel details',selectedPerson.travel_details],['Social attendance',selectedPerson.social_attendance==='yes'?'Attending':selectedPerson.social_attendance==='no'?'Not attending':'Unknown — not confirmed']].map(([label,value])=><div key={label}><dt>{label}</dt><dd>{value||'Not provided'}</dd></div>)}</dl></Inspector>} />
      {!query.data.items.length && <p>No current competitors or staff on this page.</p>}
      <CollectionPagination label="Logistics pages" source={{kind:'cursor',loaded:query.data.items.length,busy:query.isFetching,previous:cursors.length>1?()=>{setCursors(c=>c.slice(0,-1));setSearch('');setSelected(undefined)}:undefined,next:query.data.next_cursor?()=>{setCursors(c=>[...c,query.data.next_cursor??undefined]);setSearch('');setSelected(undefined)}:undefined}} /></>}{!!failure && <Failure error={failure} />}
  </section>
}

import { useState } from 'react'
import { CollectionRecords, type SortControl } from '../../src/components/patterns'
import { createRoot } from 'react-dom/client'
import { PatternExamples } from '../../src/components/patterns/examples'
import { exampleRecords } from '../../src/components/patterns/scenario'
import { Theme } from '../../src/components/theme'
import '../../src/styles.css'

const query = new URLSearchParams(window.location.search), portal = query.get('portal') === 'participant' ? 'participant' : 'admin'
// A retained synthetic baseline of the former event-grid/card composition,
// using the same records. It is test-only and never a production route or build.
function Before() {
 return <Theme><main className="fleet-pattern-example legacy-example"><p>Synthetic baseline · {portal}</p><header className="page-heading"><div><p className="eyebrow">Northstar 2027</p><h1>Review requests</h1><p>Card composition before the shared collection patterns.</p></div></header><div className="event-grid">{exampleRecords.slice(0,2).map(row=><article className="panel" key={row.id}><h2>{row.name}</h2><p>{row.owner}</p><p>{row.status}</p><button type="button">Review {row.name}</button></article>)}</div></main></Theme>
}
// Complete synthetic owner; cursor examples deliberately expose no sort.
function Sorting() {
 const [sort, setSort] = useState<Pick<SortControl, 'key' | 'direction'>>({ key: 'name', direction: 'ascending' })
 const [selection, setSelection] = useState<ReadonlySet<string>>(new Set())
 const records = query.has('focus-keys') ? exampleRecords.map((row,index)=>({...row,id:index===0?'page':index===1?'sort:name':row.id})) : exampleRecords
 const rows = [...records].sort((a,b) => (sort.key === 'name' ? a.name.localeCompare(b.name) : a.owner.localeCompare(b.owner)) * (sort.direction === 'ascending' ? 1 : -1))
 return <Theme><main className="fleet-pattern-example"><h1>Complete synthetic sorting</h1><CollectionRecords label="Complete synthetic requests" rows={rows} selection={{selected:selection,onChange:setSelection,eligible:()=>true}} rowKey={row=>row.id} rowLabel={row=>row.name} destination={row=>({label:row.name,href:'#'+row.id})} columns={[{id:'name',label:'Title',sortable:true,cell:row=>row.name},{id:'owner',label:'Contact',sortable:true,cell:row=>row.owner},{id:'status',label:'Status',cell:row=>row.status}]} sort={{...sort,onChange:(key,direction)=>setSort({key,direction})}} /><p role="status">Owner sort: {sort.key} {sort.direction}</p></main></Theme>
}
createRoot(document.getElementById('root')!).render(query.has('sorting') ? <Sorting /> : query.has('baseline') ? <Before /> : <PatternExamples portal={portal} />)

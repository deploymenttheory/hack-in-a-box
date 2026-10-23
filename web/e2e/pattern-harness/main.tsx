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
createRoot(document.getElementById('root')!).render(query.has('baseline') ? <Before /> : <PatternExamples portal={portal} />)

import { useRef, useState } from 'react'
import { CollectionRecords, DetailWorkspace, Inspector } from '../components/patterns'
import { Button } from '../components/ui/button'
import { Snippet } from './snippet'

const records = [{ id: 'draft-one', name: 'Synthetic local draft', version: 2 }, { id: 'draft-two', name: 'Synthetic local question', version: 1 }]
export function LocalRecordExamples() {
  const [selected, setSelected] = useState<typeof records[number]>()
  const actions = useRef<Record<string, HTMLButtonElement | null>>({})
  function back() {
    const id = selected?.id
    setSelected(undefined)
    requestAnimationFrame(() => { if (id) actions.current[id]?.focus() })
  }
  return <section aria-label="Local record actions" className="guide-section"><h2>Records with local actions</h2><p>When a record has no addressable route, keep its name as text and provide a named action button. This example opens local detail without inventing a bookmark or writing anything.</p><DetailWorkspace collection={<div id="local-records"><CollectionRecords label="Synthetic local drafts" rows={records} rowKey={row => row.id} rowLabel={row => row.name} columns={[{ id: 'version', label: 'Version', cell: row => row.version }]} actions={row => <Button ref={element => { actions.current[row.id] = element }} data-record-focus={row.id} icon="next" variant="outline" onClick={() => setSelected(row)}>Review {row.name}</Button>} /></div>} detail={selected && <Inspector identity={selected.id} title={selected.name} scope={{ entity: 'Synthetic local detail' }} back={{ label: 'Back to local drafts', href: '#local-records', onNavigate: back }}><p>Version {selected.version}. This is a local selection, not an API receipt or admission result.</p></Inspector>} /><Snippet>{'<CollectionRecords rows={records} rowLabel={row => row.name}\n  actions={row => <Button onClick={() => open(row)}>Review {row.name}</Button>}\n  /* supply the normal label, columns and rowKey props */\n/>'}</Snippet></section>
}

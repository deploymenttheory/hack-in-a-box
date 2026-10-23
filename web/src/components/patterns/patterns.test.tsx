import { act, fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test, vi } from 'vitest'
import { useState } from 'react'
import { Theme } from '../theme'
import { Input } from '../ui/fields'
import { ActionQueue, Breadcrumbs, CollectionColumns, CollectionFrame, CollectionHeading, CollectionPagination, CollectionRecords, CollectionToolbar, CommandDestinations, CommandState, Inspector, ReviewFlow, type CollectionColumn, type CommandView } from './index'
import { createExampleTransport, examplePage, exampleReceiptMatches, exampleRecords, type ExampleIntent, type ExampleMode, type ExampleReceipt, type ExampleRecord } from './scenario'

let narrow = false
const listeners = new Set<() => void>()
beforeEach(() => {
 narrow = false; listeners.clear()
 vi.stubGlobal('matchMedia', () => ({ get matches() { return narrow }, addEventListener: (_: string, listener: () => void) => listeners.add(listener), removeEventListener: (_: string, listener: () => void) => listeners.delete(listener) }))
})
const themed = (node: React.ReactNode) => <Theme>{node}</Theme>
const columns: CollectionColumn<ExampleRecord>[] = [ { id: 'status', label: 'Status', essential: true, sortable: true, cell: row => row.status }, { id: 'owner', label: 'Contact', cell: row => row.owner } ]
const base = { label: 'Current requests', rows: exampleRecords, columns, rowKey: (row: ExampleRecord) => row.id, rowLabel: (row: ExampleRecord) => row.name, destination: (row: ExampleRecord) => ({ label: row.name, href: '/requests/' + row.id }) }

test('cursor pagination reports loaded records without inventing a total, pages or download', async () => {
 const next = vi.fn(), user = userEvent.setup()
 render(themed(<CollectionPagination source={{ kind: 'cursor', loaded: 2, next }} />))
 expect(screen.getByRole('status')).toHaveTextContent('2 loaded')
 expect(screen.queryByText(/of|page 1|download/i)).not.toBeInTheDocument()
 expect(screen.queryByRole('button', { name: 'Previous' })).not.toBeInTheDocument()
 await user.click(screen.getByRole('button', { name: 'More records' })); expect(next).toHaveBeenCalledOnce()
})
test('authoritative totals and complete collections are explicit owner inputs', () => {
 const { rerender } = render(themed(<CollectionPagination source={{ kind: 'cursor', loaded: 2, total: 8 }} />))
 expect(screen.getByRole('status')).toHaveTextContent('2 loaded of 8')
 rerender(themed(<CollectionPagination source={{ kind: 'complete', total: 3 }} />))
 expect(screen.getByRole('status')).toHaveTextContent('3 records')
})
test('toolbar has no implicit unsupported search, sort, export or bulk controls', () => {
 render(themed(<CollectionToolbar source="cursor" summary="Current scope" />))
 expect(screen.queryByRole('search')).not.toBeInTheDocument()
 expect(screen.queryByRole('button')).not.toBeInTheDocument()
})
test('server search submits to its owner and preserves the typed query', async () => {
 const search = vi.fn(), user = userEvent.setup()
 function Search() { const [value,onChange] = useState(''); return <CollectionToolbar source="cursor" search={{ kind:'server',label:'Search permitted titles',value,onChange,onSearch:()=>search(value),onClear:()=>onChange('') }} /> }
 render(themed(<Search />)); const input = screen.getByRole('searchbox')
 await user.type(input,'Cedar{Enter}'); expect(search).toHaveBeenCalledExactlyOnceWith('Cedar')
 await user.click(screen.getByRole('button',{name:'Clear'})); expect(input).toHaveValue('')
})
test('table keeps native row links and sort direction is controlled by its owner', async () => {
 const sort = vi.fn(), navigate = vi.fn(), user = userEvent.setup()
 render(themed(<CollectionRecords {...base} destination={row=>({...base.destination(row),onNavigate:navigate})} sort={{key:'status',direction:'ascending',onChange:sort}} />))
 const table = screen.getByRole('table',{name:'Current requests'})
 expect(within(table).getByRole('columnheader',{name:'Status'})).toHaveAttribute('aria-sort','ascending')
 await user.click(within(table).getByRole('button',{name:'Status'})); expect(sort).toHaveBeenCalledWith('status','descending')
 const link = screen.getByRole('link',{name:'Project Atlas'}); expect(link).toHaveAttribute('href','/requests/atlas')
 link.addEventListener('click',event=>event.preventDefault(),{once:true}); fireEvent.click(link,{ctrlKey:true}); expect(navigate).not.toHaveBeenCalled()
 await user.click(link); expect(navigate).toHaveBeenCalledOnce()
})
test('narrow records retain labels, required columns and focus across a breakpoint', () => {
 render(themed(<CollectionRecords {...base} visibleColumns={new Set()} />))
 const link = screen.getByRole('link',{name:'Project Atlas'}); link.focus()
 act(()=>{narrow=true;listeners.forEach(listener=>listener())})
 expect(screen.queryByRole('table')).not.toBeInTheDocument()
 expect(screen.getByRole('list',{name:'Current requests'})).toBeInTheDocument()
 expect(screen.getAllByText('Status')).toHaveLength(3)
 expect(screen.queryByText('Morgan Lee')).not.toBeInTheDocument()
 expect(screen.getByRole('link',{name:'Project Atlas'})).toHaveFocus()
})
test('page selection selects eligible visible IDs only and exposes mixed state', async () => {
 const user = userEvent.setup(), changed = vi.fn()
 const { rerender } = render(themed(<CollectionRecords {...base} selection={{selected:new Set(['atlas']),onChange:changed,eligible:row=>row.id!=='beacon'}} />))
 const all = screen.getByRole('checkbox',{name:'Select eligible records on this page'})
 expect(all).toHaveAttribute('aria-checked','mixed'); expect((all as HTMLInputElement).indeterminate).toBe(true)
 expect(screen.getByRole('checkbox',{name:'Select Project Beacon'})).toBeDisabled()
 rerender(themed(<CollectionRecords {...base} selection={{selected:new Set(['off-page-id']),onChange:changed,eligible:row=>row.id!=='beacon'}} />))
 await user.click(screen.getByRole('checkbox',{name:'Select eligible records on this page'}))
 expect(changed).toHaveBeenLastCalledWith(new Set(['atlas','cedar']))
})
test('column controls keep required fields and own no record mutation', async () => {
 const changed = vi.fn(), user = userEvent.setup()
 render(themed(<CollectionColumns columns={columns} visible={new Set(['status','owner'])} onChange={changed} />))
 await user.click(screen.getByRole('button',{name:'Columns'}))
 expect(screen.getByRole('checkbox',{name:'Status (required)'})).toBeDisabled()
 await user.click(screen.getByRole('checkbox',{name:'Contact'})); expect(changed).toHaveBeenCalledWith(new Set(['status']))
})
test('definite denial clears private heading, toolbar and children even with stale supplied props', () => {
 const { rerender } = render(themed(<CollectionFrame scopeKey="one" state={{kind:'ready'}} heading={<h1>Private event title</h1>} toolbar={<span>Private filters</span>}><Input defaultValue="PRIVATE-DRAFT" /></CollectionFrame>))
 expect(screen.getByDisplayValue('PRIVATE-DRAFT')).toBeInTheDocument()
 rerender(themed(<CollectionFrame scopeKey="one" state={{kind:'denied',message:'Current access ended'}} heading={<h1>Private event title</h1>} toolbar={<span>Private filters</span>}><Input defaultValue="LATE-PRIVATE-DATA" /></CollectionFrame>))
 expect(screen.getByRole('alert')).toHaveTextContent('Current access ended')
 expect(screen.queryByText('Private event title')).not.toBeInTheDocument(); expect(screen.queryByText('Private filters')).not.toBeInTheDocument(); expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
})
test('temporary authority failure pauses activation while preserving the mounted draft', async () => {
 const user = userEvent.setup(), read = vi.fn()
 const header=<CollectionHeading title="Requests" scope={{entity:'Current event'}} primary={{label:'Record request',onSelect:vi.fn()}} />
 const { rerender } = render(themed(<CollectionFrame scopeKey="one" state={{kind:'ready'}} heading={header}><Input aria-label="Draft" /></CollectionFrame>))
 await user.type(screen.getByRole('textbox'),'Keep this draft')
 rerender(themed(<CollectionFrame scopeKey="one" state={{kind:'paused',message:'Read unavailable',read:{label:'Check current access',onSelect:read}}} heading={header}><Input aria-label="Draft" /></CollectionFrame>))
 expect(screen.getByRole('textbox')).toHaveValue('Keep this draft'); expect(screen.getByRole('button',{name:'Record request'})).toBeDisabled()
 await user.click(screen.getByRole('button',{name:'Check current access'})); expect(read).toHaveBeenCalledOnce()
 rerender(themed(<CollectionFrame scopeKey="two" state={{kind:'ready'}} heading={header}><Input aria-label="Draft" /></CollectionFrame>))
 expect(screen.getByRole('textbox')).toHaveValue('')
})
test.each(['offline','stale'] as const)('%s view retains records and pauses controls', kind=>{
 render(themed(<CollectionFrame scopeKey="one" state={{kind,message:'Review current source'}} heading={<h1>Requests</h1>}><Input defaultValue="Retained input" /></CollectionFrame>))
 expect(screen.getByDisplayValue('Retained input')).toBeDisabled()
 expect(screen.getByRole('status')).toHaveTextContent('Review current source')
})
test('empty and no-match states retain distinct useful next actions', async () => {
 const clear = vi.fn(), user=userEvent.setup()
 const { rerender }=render(themed(<CollectionFrame scopeKey="one" state={{kind:'empty',message:'No current records'}} heading={<h1>Requests</h1>}>{null}</CollectionFrame>))
 expect(screen.getByRole('heading',{name:'Nothing here yet'})).toBeInTheDocument()
 rerender(themed(<CollectionFrame scopeKey="one" state={{kind:'no-match',message:'No server matches',clear}} heading={<h1>Requests</h1>}>{null}</CollectionFrame>))
 await user.click(screen.getByRole('button',{name:'Clear filters'})); expect(clear).toHaveBeenCalledOnce()
})
test('unknown command exposes only a read callback and exact intent identity', async () => {
 const read=vi.fn(),user=userEvent.setup()
 render(themed(<CommandState value={{kind:'uncertain',intentID:'command-1',message:'Provider outcome unknown',readStatus:read}} />))
 expect(screen.getByRole('status')).toHaveTextContent('command-1')
 expect(screen.getAllByRole('button')).toHaveLength(1)
 await user.click(screen.getByRole('button',{name:'Check exact command status'}));expect(read).toHaveBeenCalledOnce()
 expect(screen.queryByRole('button',{name:/retry|replay|confirm/i})).not.toBeInTheDocument()
})
test('review supports correction and cancel while pending prevents another activation', async()=>{
 const user=userEvent.setup(),correct=vi.fn(),confirm=vi.fn(),cancel=vi.fn()
 const props={title:'Review request',scope:{entity:'Project Atlas',event:'Northstar'},stage:'review' as const,editor:<Input defaultValue="Private note" />,summary:<p>Named request: Project Atlas</p>,onReview:vi.fn(),onCorrect:correct,onConfirm:confirm,onCancel:cancel,confirmLabel:'Record request'}
 const {rerender}=render(themed(<ReviewFlow {...props} command={{kind:'idle'}} />))
 await user.click(screen.getByRole('button',{name:'Correct details'}));expect(correct).toHaveBeenCalledOnce()
 await user.click(screen.getByRole('button',{name:'Cancel'}));expect(cancel).toHaveBeenCalledOnce()
 rerender(themed(<ReviewFlow {...props} command={{kind:'pending',intentID:'one',message:'Sending request'}} />))
 expect(screen.getByRole('button',{name:'Cancel'})).toBeDisabled();expect(screen.getByRole('button',{name:'Record request'})).toBeDisabled()
 await user.click(screen.getByRole('button',{name:'Record request'}));expect(confirm).not.toHaveBeenCalled()
})
test('denied review removes entity, summary and draft while exact receipts use owner wording',()=>{
 const props={title:'Private review',scope:{entity:'Private entity'},stage:'review' as const,editor:<Input defaultValue="Private draft" />,summary:<p>Private summary</p>,onReview:vi.fn(),onCorrect:vi.fn(),onConfirm:vi.fn(),onCancel:vi.fn(),confirmLabel:'Confirm'}
 const {rerender}=render(themed(<ReviewFlow {...props} command={{kind:'denied',message:'Access ended'}} />))
 expect(screen.queryByText('Private entity')).not.toBeInTheDocument();expect(screen.queryByText('Private review')).not.toBeInTheDocument();expect(screen.queryByText('Private summary')).not.toBeInTheDocument()
 const value:CommandView={kind:'receipt',intentID:'request-1',receiptID:'receipt-1',message:'Request queued; external outcome pending'}
 rerender(themed(<CommandState value={value}/>));expect(screen.getByRole('region',{name:'Exact command receipt'})).toHaveTextContent('Request queued; external outcome pending')
 expect(screen.queryByText(/completed|provider accepted/i)).not.toBeInTheDocument()
})
test('inspector focuses its named heading and breadcrumbs retain owned route context',()=>{
 render(themed(<><Breadcrumbs items={[{label:'Filtered requests',href:'/requests?state=pending&cursor=abc'}]}/><Inspector identity="one" title="Project Atlas" scope={{entity:'Request review'}} back={{label:'Back to requests',href:'/requests?state=pending&cursor=abc'}}><p>Current evidence</p></Inspector></>))
 expect(screen.getByRole('heading',{name:'Project Atlas'})).toHaveFocus()
 expect(screen.getByRole('link',{name:'Back to requests'})).toHaveAttribute('href','/requests?state=pending&cursor=abc')
})
test('action queue preserves owner states without manufacturing completion and navigation searches permitted destinations only',()=>{
 render(themed(<><ActionQueue title="Next actions" items={[{id:'one',title:'Check request',description:'Read its exact status',scope:{entity:'Project Atlas'},icon:'refresh',status:{label:'External outcome unknown',tone:'warning'},action:{label:'View request',href:'/requests/atlas'}}]} emptyMessage="No current actions"/><CommandDestinations query="team" destinations={[{id:'teams',label:'Your team',href:'/team'},{id:'account',label:'Your account',href:'/account'}]}/></>))
 expect(screen.getByText('External outcome unknown')).toBeInTheDocument();expect(screen.queryByRole('button',{name:/complete|retry/i})).not.toBeInTheDocument()
 const nav=screen.getByRole('navigation',{name:'Permitted destinations'});expect(within(nav).getByRole('link',{name:'Your team'})).toHaveAttribute('href','/team');expect(within(nav).queryByText('Your account')).not.toBeInTheDocument()
})

const intent:ExampleIntent={id:'command-one',scopeKey:'one:northstar:0',recordID:'atlas',expected:3,note:'Synthetic note'}
const receipt:ExampleReceipt={id:'receipt-one',commandID:intent.id,scopeKey:intent.scopeKey,recordID:intent.recordID,expected:intent.expected}
test('synthetic owner matches every exact receipt field and rejects an empty proof',()=>{
 expect(exampleReceiptMatches(intent,receipt)).toBe(true)
 for(const changed of [{id:''},{commandID:'other'},{scopeKey:'two'},{recordID:'beacon'},{expected:4}])expect(exampleReceiptMatches(intent,{...receipt,...changed})).toBe(false)
})
test('synthetic server search includes a permitted record beyond the loaded page',()=>{
 expect(examplePage(exampleRecords,'',0).rows.map(row=>row.id)).toEqual(['atlas','beacon'])
 expect(examplePage(exampleRecords,'Cedar',0).rows.map(row=>row.id)).toEqual(['cedar'])
})
test('synthetic unknown and latest-only reads never replay and scopes retain separate server data',async()=>{
 let mode:ExampleMode='uncertain';const transport=createExampleTransport(()=>mode,0)
 expect((await transport.send(intent)).kind).toBe('uncertain');expect(transport.stats.writes).toBe(1)
 mode='latest-only';expect((await transport.status(intent)).kind).toBe('latest');expect(transport.stats.writes).toBe(1)
 mode='confirmed';const found=await transport.status(intent);expect(found.kind).toBe('receipt');expect(transport.stats.writes).toBe(1);expect(transport.stats.reads).toBe(2)
 const other=await transport.list('two:northstar:1','',0);expect(other.rows[0].name).toBe('Project Iris');expect(other.rows[0].status).toBe('Awaiting review')
})

test('stale synthetic commands cannot silently rebase or accept an old expected revision',async()=>{
 let mode:ExampleMode='stale';const transport=createExampleTransport(()=>mode,0)
 expect((await transport.send(intent)).kind).toBe('stale')
 mode='confirmed';expect((await transport.send({...intent,id:'new-old-version'})).kind).toBe('stale')
 const current=await transport.list(intent.scopeKey,'',0);expect(current.rows[0].revision).toBe(4)
 expect((await transport.send({...intent,id:'new-reviewed-version',expected:4})).kind).toBe('receipt')
 expect(transport.stats.writes).toBe(3)
})
test('cursor toolbar rejects complete-array search at the type boundary',()=>{
 // @ts-expect-error Cursor sources cannot claim complete-array search capability.
 const invalid:Parameters<typeof CollectionToolbar>[0]={source:'cursor',search:{kind:'complete',completeCollection:true,label:'Wrong scope',value:'',onChange:vi.fn(),onSearch:vi.fn(),onClear:vi.fn()}}
 expect(invalid.source).toBe('cursor')
})

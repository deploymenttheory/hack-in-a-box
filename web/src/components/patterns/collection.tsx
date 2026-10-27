import { useContext, useId, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { Button } from '../ui/button'
import { Checkbox, Input, Label } from '../ui/fields'
import { Popover, PopoverContent, PopoverTrigger } from '../ui/popover'
import { PatternActionControl } from './actions'
import { CollectionStateMessage, collectionRetainsRecords } from './state'
import { WorkspaceFeedbackContext } from './workspace-context'
import type { CollectionColumn, CollectionSource, CollectionState, NavigationTarget, PatternAction, ScopeLabel, SearchControl, SelectionControl, SortControl } from './model'

export function CollectionHeading({ title, scope, description, primary, secondary = [], status }: { title: string; scope: ScopeLabel; description?: ReactNode; primary?: PatternAction; secondary?: readonly PatternAction[]; status?: ReactNode }) {
  return <header className="fleet-collection-heading"><div><p className="fleet-scope-label">{scope.event && <>{scope.event} · </>}{scope.entity}</p><h1>{title}</h1>{description && <div className="fleet-description">{description}</div>}</div><div className="fleet-pattern-actions">{secondary.map(action => <PatternActionControl key={action.label} action={action} />)}{primary && <PatternActionControl action={primary} primary />}</div>{status && <div className="fleet-collection-status">{status}</div>}</header>
}

type ToolbarProps = { filters?: ReactNode; columns?: ReactNode; summary?: ReactNode; disabled?: boolean } & ({ source: 'complete'; search?: SearchControl } | { source: 'cursor'; search?: Extract<SearchControl, { kind: 'server' }> })
export function CollectionToolbar({ search, filters, columns, summary, disabled = false }: ToolbarProps) {
  const id = useId()
  return <div className="fleet-collection-toolbar">{summary && <div className="fleet-collection-summary">{summary}</div>}<div className="fleet-pattern-actions">{columns}{filters}{search && <form role="search" aria-label={search.label} onSubmit={event => { event.preventDefault(); if (!disabled) search.onSearch() }}><Label htmlFor={id}>{search.label}</Label><div className="fleet-pattern-actions"><Input id={id} type="search" maxLength={search.maxLength} value={search.value} onChange={event => search.onChange(event.target.value)} disabled={disabled} /><Button type="submit" icon="search" variant="outline" disabled={disabled}>Search</Button>{search.value && <Button type="button" variant="ghost" disabled={disabled} onClick={search.onClear}>Clear</Button>}</div></form>}</div></div>
}

export function CollectionColumns({ columns, visible, onChange }: { columns: readonly { id: string; label: string; essential?: boolean }[]; visible: ReadonlySet<string>; onChange: (ids: ReadonlySet<string>) => void }) {
  return <Popover><PopoverTrigger render={<Button icon="columns" variant="ghost" />}>Columns</PopoverTrigger><PopoverContent title="Visible columns"><fieldset><legend>Choose optional columns</legend>{columns.map(column => <label className="fleet-column-option" key={column.id}><Checkbox checked={column.essential || visible.has(column.id)} disabled={column.essential} onChange={event => { const next = new Set(visible); if (event.target.checked) next.add(column.id); else next.delete(column.id); onChange(next) }} />{column.label}{column.essential && ' (required)'}</label>)}</fieldset></PopoverContent></Popover>
}

export function CollectionPagination({ source, label = 'Collection pages' }: { source: CollectionSource; label?: string }) {
  return <nav className="fleet-collection-pagination" aria-label={label}><p role="status">{source.kind === 'complete' ? source.total + ' records' : source.loaded + ' loaded' + (source.total === undefined ? '' : ' of ' + source.total)}</p>{source.kind === 'cursor' && <div className="fleet-pattern-actions">{source.previous && <Button icon="previousPage" variant="outline" disabled={source.busy} onClick={source.previous}>Previous</Button>}{source.next && <Button icon="nextPage" variant="outline" disabled={source.busy} onClick={source.next}>More records</Button>}</div>}</nav>
}

const narrowQuery = '(max-width: 48rem)'

type CollectionFocus = { attribute: 'data-record-focus' | 'data-collection-focus'; key: string }

type RecordsProps<T> = { label: string; rows: readonly T[]; columns: readonly CollectionColumn<T>[]; rowKey: (row: T) => string; rowLabel: (row: T) => string; destination?: (row: T) => NavigationTarget; sort?: SortControl; selection?: SelectionControl<T>; actions?: (row: T) => ReactNode; visibleColumns?: ReadonlySet<string>; disabled?: boolean }
function RecordName<T>({ row, destination, rowKey, rowLabel }: Pick<RecordsProps<T>, 'destination' | 'rowKey' | 'rowLabel'> & { row: T }) {
  const link = destination?.(row)
  if (!link) return <span>{rowLabel(row)}</span>
  return <a href={link.href} data-record-focus={rowKey(row)} onClick={event => { if (link.onNavigate && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey && event.button === 0) { event.preventDefault(); link.onNavigate() } }}>{link.label}</a>
}
function RowSelection<T>({ row, selection, rowKey, rowLabel, disabled }: Pick<RecordsProps<T>, 'selection' | 'rowKey' | 'rowLabel' | 'disabled'> & { row: T }) {
  if (!selection) return null
  const id = rowKey(row)
  return <Checkbox data-collection-focus={JSON.stringify(['selection', id])} aria-label={'Select ' + rowLabel(row)} checked={selection.selected.has(id)} disabled={disabled || !selection.eligible(row)} onChange={event => { const next = new Set(selection.selected); if (event.target.checked) next.add(id); else next.delete(id); selection.onChange(next) }} />
}
export function CollectionRecords<T>(props: RecordsProps<T>) {
  const [narrow, setNarrow] = useState(false), root = useRef<HTMLDivElement>(null), focused = useRef<CollectionFocus | undefined>(undefined), layout = useRef(false), restoreFocus = useRef(false)
  const columns = props.columns.filter(column => column.essential || !props.visibleColumns || props.visibleColumns.has(column.id))
  useLayoutEffect(() => {
    const container = root.current
    if (!container) return
    const media = window.matchMedia(narrowQuery)
    const measure = () => {
      const width = container.getBoundingClientRect().width
      const next = width > 0 ? width <= 48 * parseFloat(getComputedStyle(document.documentElement).fontSize) : media.matches
      if (next === layout.current) return
      restoreFocus.current = container.contains(document.activeElement)
      layout.current = next
      setNarrow(next)
    }
    const observer = new ResizeObserver(measure)
    observer.observe(container)
    media.addEventListener('change', measure)
    measure()
    return () => { observer.disconnect(); media.removeEventListener('change', measure) }
  }, [])
  useLayoutEffect(() => {
    const target = focused.current
    if (restoreFocus.current && target) [...(root.current?.querySelectorAll<HTMLElement>(`[${target.attribute}]`) ?? [])].find(control => control.getAttribute(target.attribute) === target.key)?.focus()
    restoreFocus.current = false
  }, [narrow])
  function rememberFocus(target: HTMLElement) {
    const control = target.closest<HTMLElement>('[data-collection-focus],[data-record-focus]')
    if (!control) { focused.current = undefined; return }
    const attribute = control.hasAttribute('data-collection-focus') ? 'data-collection-focus' : 'data-record-focus'
    focused.current = { attribute, key: control.getAttribute(attribute) ?? '' }
  }
  const links = (row: T) => <RecordName row={row} destination={props.destination} rowKey={props.rowKey} rowLabel={props.rowLabel} />
  return <div className="fleet-collection-records" ref={root} onFocusCapture={event => rememberFocus(event.target as HTMLElement)}>{narrow ? <>{props.selection && <label className="fleet-column-option"><PageSelection {...props} />Select eligible records on this page</label>}{props.sort && columns.some(column => column.sortable) && <fieldset className="fleet-record-sort" disabled={props.disabled}><legend>Sort {props.label}</legend><div className="fleet-pattern-actions">{columns.filter(column => column.sortable).map(column => <Button key={column.id} variant="ghost" data-collection-focus={JSON.stringify(['sort', column.id])} aria-label={'Sort by ' + column.label} aria-description={props.sort?.key === column.id ? props.sort.direction : undefined} aria-pressed={props.sort?.key === column.id} icon={props.sort?.key === column.id && props.sort.direction === 'descending' ? 'sortDescending' : 'sortAscending'} onClick={() => props.sort?.onChange(column.id, props.sort.key === column.id && props.sort.direction === 'ascending' ? 'descending' : 'ascending')}>{column.label}{props.sort?.key === column.id && <span className="fleet-visually-hidden"> · {props.sort.direction}</span>}</Button>)}</div></fieldset>}<ul className="fleet-record-list" aria-label={props.label}>{props.rows.map(row => <li key={props.rowKey(row)}><div className="fleet-record-heading"><RowSelection {...props} row={row} /><h2>{links(row)}</h2>{props.actions?.(row)}</div><dl>{columns.map(column => <div key={column.id}><dt>{column.label}</dt><dd>{column.cell(row)}</dd></div>)}</dl></li>)}</ul></> : <table className="fleet-collection-table"><caption className="fleet-visually-hidden">{props.label}</caption><thead><tr>{props.selection && <th scope="col"><PageSelection {...props} /></th>}<th scope="col">Record</th>{columns.map(column => <th key={column.id} scope="col" aria-sort={props.sort?.key === column.id ? props.sort.direction : undefined}>{column.sortable && props.sort ? <Button variant="ghost" data-collection-focus={JSON.stringify(['sort', column.id])} disabled={props.disabled} icon={props.sort.key === column.id && props.sort.direction === 'descending' ? 'sortDescending' : 'sortAscending'} onClick={() => props.sort?.onChange(column.id, props.sort.key === column.id && props.sort.direction === 'ascending' ? 'descending' : 'ascending')}>{column.label}</Button> : column.label}</th>)}{props.actions && <th scope="col">Actions</th>}</tr></thead><tbody>{props.rows.map(row => <tr key={props.rowKey(row)}>{props.selection && <td><RowSelection {...props} row={row} /></td>}<th scope="row">{links(row)}</th>{columns.map(column => <td key={column.id}>{column.cell(row)}</td>)}{props.actions && <td>{props.actions(row)}</td>}</tr>)}</tbody></table>}</div>
}
function PageSelection<T>({ rows, selection, rowKey, disabled }: RecordsProps<T>) {
  const eligible = rows.filter(row => selection?.eligible(row)), selected = eligible.filter(row => selection?.selected.has(rowKey(row))).length
  return <Checkbox data-collection-focus={JSON.stringify(['page'])} aria-label="Select eligible records on this page" aria-checked={selected > 0 && selected < eligible.length ? 'mixed' : selected > 0} ref={element => { if (element) element.indeterminate = selected > 0 && selected < eligible.length }} checked={eligible.length > 0 && selected === eligible.length} disabled={disabled || eligible.length === 0} onChange={event => selection?.onChange(new Set(event.target.checked ? eligible.map(rowKey) : []))} />
}

// Keying the frame resets presentation state when the owner replaces identity,
// session or event. Owners still fence asynchronous reads with PrivateRequestScope.
export function CollectionFrame({ scopeKey, state, children, heading, toolbar }: { scopeKey: string; state: CollectionState; heading: ReactNode; toolbar?: ReactNode; children: ReactNode }) {
  const workspaceFeedback = useContext(WorkspaceFeedbackContext)
  return <CollectionLifetime key={scopeKey} state={state} heading={heading} toolbar={toolbar} announce={!workspaceFeedback}>{children}</CollectionLifetime>
}
function CollectionLifetime({ state, heading, children, toolbar, announce }: { state: CollectionState; heading: ReactNode; children: ReactNode; toolbar?: ReactNode; announce: boolean }) {
  return <section className="fleet-collection" data-collection-state={state.kind}>{state.kind !== 'denied' && <fieldset className="fleet-collection-fieldset" disabled={state.kind !== 'ready' && state.kind !== 'empty' && state.kind !== 'no-match'}>{heading}{toolbar}</fieldset>}{announce && <CollectionStateMessage state={state} />}{collectionRetainsRecords(state) && <div aria-busy={state.kind === 'paused' || undefined}><fieldset className="fleet-collection-fieldset" disabled={state.kind !== 'ready'}>{children}</fieldset></div>}</section>
}

// Optional presentation memory is local to a mounted scope; private query text,
// selection, cursor and route context stay controlled by the owning collection.
export function useCollectionColumns(defaults: readonly string[]) { return useState<ReadonlySet<string>>(() => new Set(defaults)) }

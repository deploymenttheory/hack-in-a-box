import { Alert } from '../ui/surfaces'
import { LiveMessage } from '../ui/feedback'
import { Icon } from '../ui/icon'
import { Button } from '../ui/button'
import { PatternActionControl } from './actions'
import type { CollectionState } from './model'

export function CollectionStateMessage({ state }: { state: CollectionState }) {
  if (state.kind === 'ready') return null
  if (state.kind === 'loading') return <LiveMessage className="fleet-collection-state"><Icon name="pending" />{state.message ?? 'Loading records…'}</LiveMessage>
  if (state.kind === 'empty') return <div className="fleet-collection-state"><h2>Nothing here yet</h2><p>{state.message}</p>{state.action && <PatternActionControl action={state.action} />}</div>
  if (state.kind === 'no-match') return <div className="fleet-collection-state"><h2>No matching records</h2><p>{state.message}</p><Button variant="outline" onClick={state.clear}>Clear filters</Button></div>
  return <Alert role={state.kind === 'denied' || state.kind === 'failed' ? 'alert' : 'status'} tone={state.kind === 'denied' || state.kind === 'failed' ? 'destructive' : 'warning'} className="fleet-collection-state"><p>{state.message}</p>{'read' in state && state.read && <PatternActionControl action={state.read} />}</Alert>
}
export const collectionRetainsRecords = (state: CollectionState) => state.kind === 'ready' || state.kind === 'paused' || state.kind === 'offline' || state.kind === 'stale'

import { StatusBadge, type Tone } from '../ui/feedback'
import { Icon, type IconName } from '../ui/icon'
import { PatternActionControl } from './actions'
import type { PatternAction, ScopeLabel } from './model'

export type QueueItem = { id: string; title: string; description: string; scope: ScopeLabel; status?: { label: string; tone?: Tone }; icon: IconName; action?: PatternAction }
export function ActionQueue({ title, items, emptyMessage }: { title: string; items: readonly QueueItem[]; emptyMessage: string }) {
  return <section className="fleet-action-queue" aria-label={title}><h2>{title}</h2>{items.length === 0 ? <p>{emptyMessage}</p> : <ul>{items.map(item => <li key={item.id}><Icon name={item.icon} /><div><p className="fleet-scope-label">{item.scope.event && <>{item.scope.event} · </>}{item.scope.entity}</p><h3>{item.title}</h3><p>{item.description}</p>{item.status && <StatusBadge tone={item.status.tone}>{item.status.label}</StatusBadge>}{item.action && <PatternActionControl action={item.action} />}</div></li>)}</ul>}</section>
}

// Shells supply only their current permitted navigation registry. No API cache,
// private record index, hidden destination or global keyboard binding is read.
export function CommandDestinations({ query, destinations, emptyMessage = 'No permitted destinations match.' }: { query: string; destinations: readonly { id: string; label: string; href: string; keywords?: string }[]; emptyMessage?: string }) {
  const needle = query.trim().toLocaleLowerCase(), visible = destinations.filter(item => (item.label + ' ' + (item.keywords ?? '')).toLocaleLowerCase().includes(needle))
  return <nav aria-label="Permitted destinations">{visible.length ? <ul className="fleet-command-destinations">{visible.map(item => <li key={item.id}><a href={item.href}>{item.label}</a></li>)}</ul> : <p role="status">{emptyMessage}</p>}</nav>
}

import type { ReactNode } from 'react'
import type { components } from './api/schema.gen'
import { CollectionToolbar } from './components/patterns'
import { StatusBadge } from './components/ui/feedback'

type Item = components['schemas']['MediaItem']
// The owner supplies only current-authorised native projections. Thumbnails and
// downloads remain deliberate protected-byte reads, never direct file URLs.
export function MediaGallery({ eventID, title, items, renderContent }: { eventID: string; title: string; items: readonly Item[]; renderContent: (item: Item) => ReactNode }) {
  return <section aria-label={title} className="py-6 min-w-0"><h2>{title}</h2><CollectionToolbar source="cursor" summary={`${items.length} loaded media records · current authorised view`} /><p className="fleet-scope-label">Gallery release is separate from private submission and organiser review. Consent and current audience are checked again for protected media access.</p>{!items.length ? <p>No media is available in this view.</p> : <ul className="grid grid-cols-1 gap-x-8 md:grid-cols-2 list-none p-0 m-0" aria-label="Current media records">{items.map(item => <li key={item.id} className="min-w-0"><article data-media-record={item.id} className={`py-6 border-b border-border min-w-0${item.state !== 'released' || item.consent_state !== 'reviewed' ? ' media-private' : ''}`}><header className="grid gap-2"><div className="fleet-pattern-actions"><StatusBadge>{item.kind === 'photo' ? 'Photograph' : 'External recording'}</StatusBadge><StatusBadge tone={item.state === 'released' && item.consent_state === 'reviewed' ? 'success' : 'neutral'}>{item.state.replaceAll('_', ' ')}</StatusBadge><span className="fleet-scope-label">Version {item.version} · Consent {item.consent_state.replaceAll('_', ' ')}</span></div><h3><a href={`/events/${encodeURIComponent(eventID)}/media/${encodeURIComponent(item.id)}`}>{item.title}</a></h3></header>{item.caption && <p className="whitespace-pre-wrap break-words py-2">{item.caption}</p>}{item.captured_at && <p className="fleet-scope-label">Captured {new Date(item.captured_at).toLocaleString()}</p>}{renderContent(item)}</article></li>)}</ul>}</section>
}

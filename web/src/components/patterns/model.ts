import type { ReactNode } from 'react'
import type { IconName } from '../ui/icon'

export type PatternAction = { label: string; icon?: IconName; disabled?: boolean } & (
  | { href: string; onSelect?: never }
  | { href?: never; onSelect: () => void }
)
export type NavigationTarget = { label: string; href: string; onNavigate?: () => void }
export type ScopeLabel = { entity: string; event?: string }
export type CollectionState =
  | { kind: 'ready' }
  | { kind: 'loading'; message?: string }
  | { kind: 'empty'; message: string; action?: PatternAction }
  | { kind: 'no-match'; message: string; clear: () => void }
  | { kind: 'paused' | 'offline' | 'stale'; message: string; read?: PatternAction }
  | { kind: 'denied'; message: string }
  | { kind: 'failed'; message: string; read?: PatternAction }

// Capabilities describe the owning API, never capabilities inferred from a page
// length. A cursor source cannot accidentally opt into complete-array searching.
export type CollectionSource =
  | { kind: 'complete'; total: number }
  | { kind: 'cursor'; loaded: number; total?: number; next?: () => void; previous?: () => void; busy?: boolean }
export type SearchControl = { label: string; maxLength?: number; value: string; onChange: (value: string) => void; onSearch: () => void; onClear: () => void } & (
  | { kind: 'server' }
  | { kind: 'complete'; completeCollection: true }
)
export type SortControl = { key: string; direction: 'ascending' | 'descending'; onChange: (key: string, direction: 'ascending' | 'descending') => void }
export type CollectionColumn<T> = {
  id: string
  label: string
  cell: (row: T) => ReactNode
  essential?: boolean
  sortable?: boolean
}
export type SelectionControl<T> = { selected: ReadonlySet<string>; onChange: (ids: ReadonlySet<string>) => void; eligible: (row: T) => boolean }
export type CommandView =
  | { kind: 'idle' }
  | { kind: 'pending'; intentID: string; message: string }
  | { kind: 'not-sent' | 'rejected' | 'stale'; message: string; correct?: () => void }
  | { kind: 'uncertain'; intentID: string; message: string; readStatus?: () => void; checking?: boolean }
  | { kind: 'receipt'; intentID: string; receiptID: string; message: string; details?: ReactNode; next?: PatternAction }
  | { kind: 'denied'; message: string }

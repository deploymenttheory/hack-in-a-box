import { useState } from 'react'
import type { components } from './api/schema.gen'
import { localDate } from './event-time'
import { ActionQueue, CollectionToolbar } from './components/patterns'
import { Checkbox, Field, Label, Select } from './components/ui/fields'

type Queue = components['schemas']['HelpQueue']
// HelpQueue is the complete current-authority summary projection. It contains no
// request prose; filters never inspect the detail cache or an assessment source.
export function HelpAttentionQueue({ eventID, timezone, value, actorID }: { eventID: string; timezone: string; value: Queue; actorID: string }) {
  const [status, setStatus] = useState(''), [category, setCategory] = useState(''), [owner, setOwner] = useState(''), [overdue, setOverdue] = useState(false)
  const items = value.items.filter(r => (!status || r.status === status) && (!category || r.category === category) && (!owner || (owner === 'unassigned' ? !r.assignee_id : r.assignee_id === owner)) && (!overdue || r.overdue))
  const owners = new Map(value.items.filter(i => i.assignee_id).map(i => [i.assignee_id!, i.assignee_name || i.assignee_id!]))
  return <section className="py-6 border-t border-border" aria-label="Help attention workspace">
    <p>Current authorised requests: {value.waiting} waiting · {value.claimed} claimed · {value.resolved} resolved · {value.cancelled} cancelled. Reopened requests return to Waiting with an explicit history entry.</p>
    <CollectionToolbar source="complete" summary={`Showing ${items.length} of ${value.items.length} currently authorised requests.`} filters={<div className="flex flex-wrap items-end gap-4">
      <Field id="help-filter-status" label="Status"><Select value={status} onChange={e => setStatus(e.target.value)}><option value="">All statuses</option>{['waiting', 'claimed', 'resolved', 'cancelled'].map(s => <option key={s}>{s}</option>)}</Select></Field>
      <Field id="help-filter-category" label="Category"><Select value={category} onChange={e => setCategory(e.target.value)}><option value="">All categories</option>{['setup', 'logistics', 'competition_output'].map(s => <option key={s}>{s}</option>)}</Select></Field>
      <Field id="help-filter-owner" label="Current owner"><Select value={owner} onChange={e => setOwner(e.target.value)}><option value="">All visible owners</option><option value="unassigned">Unassigned</option><option value={actorID}>My claims</option>{[...owners].filter(([id]) => id !== actorID).map(([id, name]) => <option key={id} value={id}>{name} · {id.slice(0, 8)}</option>)}</Select></Field>
      <Label className="flex items-center gap-2"><Checkbox checked={overdue} onChange={e => setOverdue(e.target.checked)} />Overdue waiting requests only</Label>
    </div>} />
    <ActionQueue title={value.can_manage ? 'Organiser help queue' : 'Your authorised help queue'} emptyMessage={value.items.length ? 'No requests match these filters.' : 'No authorised help requests are waiting for attention.'} items={items.map(r => ({
      id: r.id, title: r.category.replaceAll('_', ' '), icon: r.overdue ? 'warning' : 'help',
      scope: { entity: `Team ${r.team_id.slice(0, 8)}`, event: `Owner ${r.assignee_name || r.assignee_id || 'Unassigned'}` },
      status: { label: `${r.status}${r.overdue ? ' · Overdue' : ''}`, tone: r.overdue ? 'warning' : 'neutral' },
      description: `${r.status === 'waiting' ? `Waiting ${Math.floor(r.waiting_seconds / 60)} minutes at the last server check.` : 'See the request for its operational history.'} Authoritative update: ${localDate(r.updated_at, timezone)}.`,
      action: { label: `Open request · ${r.id.slice(0, 8)}`, icon: 'next', href: `/events/${encodeURIComponent(eventID)}/help/${encodeURIComponent(r.id)}` },
    }))} />
  </section>
}

import { ActionQueue, CollectionFrame, CollectionHeading, type CollectionState, type QueueItem } from './components/patterns'
import { Field, Select } from './components/ui/fields'
import { StatusBadge } from './components/ui/feedback'
import { localDate } from './event-time'
import type { components } from './api/schema.gen'

type Page = components['schemas']['TodayPage']
export type TodayContext = 'personal' | components['schemas']['StaffRole']
export const todayContextNames: Record<TodayContext, string> = { personal: 'Your participation', organiser: 'Organiser', mentor: 'Mentor', judge: 'Judge', mc: 'Master of ceremonies', logistics: 'Logistics', photographer: 'Photographer', early_brief_access: 'Privileged event access' }
export function TodayView({ page, context, changeContext, items, paused = false, actionsState = { kind: 'ready' }, contextDisabled = false }: { page: Page; context: TodayContext; changeContext: (value: TodayContext) => void; items: readonly QueueItem[]; paused?: boolean; actionsState?: CollectionState; contextDisabled?: boolean }) {
  const { event, program } = page
  return <section className="fleet-today" aria-label="Your event Today"><CollectionHeading title="Today" scope={{ event: event.name, entity: todayContextNames[context] }} description="One next action from current current responsibilities. Preparation, team readiness and formal review remain separate." />
    <Field label="Today context" description="Only your current event grants are offered. Switching context does not grant permissions."><Select value={context} disabled={contextDisabled} onChange={event => changeContext(event.target.value as TodayContext)}><option value="personal">Your participation</option>{page.roles.map(role => <option key={role} value={role}>{todayContextNames[role]}</option>)}</Select></Field>
    <p className="fleet-scope-label">Observed {localDate(program.server_now, event.configuration.timezone)} · {event.configuration.timezone} · Event revision {event.revision}</p>
    <section className="fleet-today-section" aria-label="Published room program"><h2>In the room</h2><p>The MC’s currently published room program, shared by every authorised event reader. This is not a personal agenda or an unreleased staff schedule.</p><dl className="fleet-today-program"><div><dt>Current</dt><dd>{program.current ? <><strong>{program.current.title}</strong><p>{program.current.text}</p>{program.current.content_title && <p>Released resource: {program.current.content_title}</p>}</> : 'No current room activity is available.'}</dd></div><div><dt>Next</dt><dd>{program.next ? <><strong>{program.next.title}</strong><p>{program.next.text}</p></> : 'No next room activity is available.'}</dd></div></dl><StatusBadge>{program.override_active ? 'MC override active' : program.mode === 'running' ? 'Room timer running' : 'Room timer held'}</StatusBadge><p>Room program revision {program.sequence}. Timer: {program.remaining_seconds} seconds at the server observation above; this display does not infer a later state from your device clock.</p></section>
    <CollectionFrame scopeKey={`${page.event.id}:${context}`} heading={null} state={actionsState}><ActionQueue title="Next relevant actions" items={items.map(item => ({ ...item, action: item.action && { ...item.action, disabled: paused || item.action.disabled } }))} emptyMessage="No next action is available from the current observations in this context." /></CollectionFrame>
  </section>
}

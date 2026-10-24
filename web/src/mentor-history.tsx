import { useState } from 'react'
import { useInfiniteQuery } from '@tanstack/react-query'
import { api, privateKey, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { localDate } from './event-time'
export function MentorHistory({ event, teamID, noteID }: { event: EventRecord; teamID: string; noteID?: string }) {
  const identity = useIdentity(), [open, setOpen] = useState(false)
  const query = useInfiniteQuery({ queryKey: [...privateKey(identity.person.id, event.id, 'mentor-history'), teamID, noteID ?? 'status'], enabled: open, initialPageParam: undefined as number | undefined, queryFn: async ({ pageParam }) => unwrap(await api.GET('/api/v1/events/{event_id}/teams/{team_id}/mentor-observations/history', { params: { path: { event_id: event.id, team_id: teamID }, query: { observation_id: noteID, observation_cursor: noteID ? pageParam : undefined, status_cursor: noteID ? undefined : pageParam, limit: 50 } } })), getNextPageParam: page => (noteID ? page.next_observation_cursor : page.next_status_cursor) ?? undefined })
  return <section className="py-4 border-t border-border"><Button icon="list" variant="outline" onClick={() => setOpen(!open)}>{open ? 'Hide' : 'Show'} {noteID ? 'observation' : 'check-in'} history</Button>{open && (query.isPending ? <Loading /> : query.error ? <Failure error={query.error} /> : <><ol>{query.data.pages.flatMap(p => noteID ? p.observations.map(n => <li key={n.version}>Version {n.version} · {localDate(n.changed_at, event.configuration.timezone)}<p className="whitespace-pre-wrap">{n.evidence.evidence}</p><p className="whitespace-pre-wrap">{n.evidence.outcome}</p><p>Category: {n.evidence.category} · Feedback reference: {n.evidence.feedback_reference || 'None'}</p></li>) : p.statuses.map(s => <li key={s.version}>{s.state.replaceAll('_', ' ')} · {localDate(s.changed_at, event.configuration.timezone)}<p className="whitespace-pre-wrap">{s.reason}</p><p className="whitespace-pre-wrap">{s.next_action}</p></li>))}</ol>{query.hasNextPage && <Button variant="outline" onClick={() => void query.fetchNextPage()}>Older history</Button>}</>)}</section>
}

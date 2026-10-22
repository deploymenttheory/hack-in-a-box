import { useInfiniteQuery } from '@tanstack/react-query'
import { api, privateKey, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { localDate } from './event-time'
import { referenceURL, words, type Evidence, type SubmissionPage, type Team } from './deliverable-model'

export function EvidenceList({ items, eventID }: { items: Evidence[]; eventID: string }) {
  return items.length ? <ul className="evidence-links">{items.map(e => { const href = e.kind === 'file' && e.file_id ? `/api/v1/events/${eventID}/files/${e.file_id}` : referenceURL(e.url); return <li key={e.deliverable_id}>{href ? <a href={href} target="_blank" rel="noopener noreferrer">{e.label}</a> : <span>{e.label}</span>} · {words(e.kind)}</li> })}</ul> : <p>No evidence recorded.</p>
}
export function OrganiserHelp({ page, team, event }: { page: SubmissionPage; team: Team; event: EventRecord }) {
  return <details className="panel organiser-help"><summary>Contact an organiser about an exception</summary><p>Ask {page.organisers.length ? page.organisers.map(p => p.name).join(' or ') : 'the event organising team'} at the event help desk or through your event’s usual contact channel. Give them the team reference and deadline below.</p><p>Team: <strong>{team.name}</strong> · {team.id}<br />Cutoff: {localDate(page.cutoff, event.configuration.timezone)} · {event.configuration.timezone}<br />Recorded outcome: {words(page.outcome)}</p><p>Explain what needs changing and why. An organiser must record the reason for any late amendment. Only an organiser can approve an exception; formal evidence is never accepted or reviewed before live start.</p></details>
}
export function SubmissionHistory({ event, teamID }: { event: EventRecord; teamID: string }) {
  const identity = useIdentity()
  const history = useInfiniteQuery({ queryKey: privateKey(identity.person.id, event.id, `submission-history-${teamID}`), initialPageParam: undefined as number | undefined, queryFn: async ({ pageParam }) => unwrap(await api.GET('/api/v1/events/{event_id}/teams/{team_id}/submission/history', { params: { path: { event_id: event.id, team_id: teamID }, query: { before: pageParam, limit: 20 } } })), getNextPageParam: p => p.next_cursor ?? undefined, refetchOnWindowFocus: 'always' })
  return <section className="submission-history" aria-label="Submission history and feedback"><h3>History and organiser feedback</h3><p>Completion feedback is separate from mentor checkpoint scores and final judging.</p>{history.isPending ? <Loading /> : history.error ? <Failure error={history.error} retry={() => void history.refetch()} /> : <><ol className="history">{history.data.pages.flatMap(p => p.items).map(h => <li key={h.revision}><strong>{words(h.action)}</strong>{h.review_status && ` · ${words(h.review_status)}`} · {localDate(h.at, event.configuration.timezone)}{h.active_revision && ` · Formal version ${h.active_revision}`}<p>{h.reason}</p>{h.submission && <EvidenceList eventID={event.id} items={h.submission.evidence} />}</li>)}</ol>{!history.data.pages[0].items.length && <p>No formal submissions or reviews yet.</p>}{history.hasNextPage && <Button variant="outline" disabled={history.isFetchingNextPage} onClick={() => void history.fetchNextPage()}>Earlier submission history</Button>}</>}</section>
}

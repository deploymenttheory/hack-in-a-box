import { useState } from 'react'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { api, APIError, privateKey, unwrap } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { readObservations } from './mentor-observation-api'
import { submissionPage } from './scorecard-api'
import type { Evidence } from './scorecard-model'

export function EvidencePicker({ eventID, teamID, add }: { eventID: string; teamID: string; add: (ref: Evidence) => void }) {
  const identity = useIdentity(), [open, setOpen] = useState(false)
  const observations = useInfiniteQuery({ queryKey: [...privateKey(identity.person.id, eventID, 'scorecard-observations'), teamID], enabled: open, initialPageParam: undefined as string | undefined, queryFn: ({ pageParam }) => readObservations(eventID, teamID, pageParam), getNextPageParam: p => p.next_cursor ?? undefined, refetchInterval: 15_000 })
  const submission = useQuery({ queryKey: [...privateKey(identity.person.id, eventID, 'scorecard-submission'), teamID], enabled: open, queryFn: () => submissionPage(eventID, teamID), refetchInterval: 15_000 })
  return <div><Button type="button" variant="outline" onClick={() => setOpen(!open)}>{open ? 'Hide' : 'Choose'} saved evidence references</Button>{open && <section aria-label="Available saved evidence"><h5>Currently authorised observations</h5>{observations.isPending ? <Loading /> : observations.error ? <><p>Observation references are unavailable under your current observation permissions. A scoring role does not grant access to private mentor notes.</p><Failure error={observations.error} /></> : <>{observations.data.pages.flatMap(p => p.observations).map(n => <div key={n.id}><p>{n.evidence.category} · Version {n.version}</p><Button type="button" variant="outline" onClick={() => add({ kind: 'observation', id: n.id, version: n.version, note: '' })}>Attach observation {n.id.slice(0, 8)} version {n.version}</Button></div>)}{observations.hasNextPage && <Button type="button" variant="outline" onClick={() => void observations.fetchNextPage()}>More saved observations</Button>}</>}<h5>Formal submission</h5>{submission.isPending ? <Loading /> : submission.error ? <Failure error={submission.error} /> : submission.data.final ? <><p>{submission.data.outcome === 'frozen' ? 'Frozen' : 'Current formal'} submission revision {submission.data.final.revision}. Working drafts are not evidence here.</p><Button type="button" variant="outline" onClick={() => add({ kind: 'submission', id: teamID, version: submission.data.final!.revision, note: '' })}>Attach submission revision {submission.data.final.revision}</Button></> : <p>No formal submission is available.</p>}</section>}</div>
}
function safeURL(url: string) { try { const value = new URL(url); return ['https:', 'http:'].includes(value.protocol) && !value.username && !value.password ? value.href : undefined } catch { return undefined } }
export function EvidencePreview({ eventID, teamID, reference }: { eventID: string; teamID: string; reference: Evidence }) {
  const identity = useIdentity(), [open, setOpen] = useState(false)
  const query = useQuery({ queryKey: [...privateKey(identity.person.id, eventID, 'scorecard-exact-evidence'), teamID, reference.kind, reference.id, reference.version], enabled: open, refetchInterval: 15_000, queryFn: async () => {
    if (reference.kind === 'observation') {
      const history = unwrap(await api.GET('/api/v1/events/{event_id}/teams/{team_id}/mentor-observations/history', { params: { path: { event_id: eventID, team_id: teamID }, query: { observation_id: reference.id!, observation_cursor: reference.version! + 1, limit: 1 } } }))
      const exact = history.observations[0]
      if (!exact || exact.id !== reference.id || exact.version !== reference.version || exact.team_id !== teamID) throw new APIError(404, 'not_found')
      return { observation: exact, submission: null }
    }
    if (reference.kind !== 'submission' || reference.id !== teamID) throw new APIError(404, 'not_found')
    const current = await submissionPage(eventID, teamID)
    if (current.final?.revision === reference.version) return { observation: null, submission: current.final }
    let cursor: number | undefined
    do {
      const page = unwrap(await api.GET('/api/v1/events/{event_id}/teams/{team_id}/submission/history', { params: { path: { event_id: eventID, team_id: teamID }, query: { before: cursor, limit: 50 } } }))
      const exact = page.items.find(item => item.submission?.revision === reference.version)?.submission
      if (exact) return { observation: null, submission: exact }
      cursor = page.next_cursor ?? undefined
    } while (cursor)
    throw new APIError(404, 'not_found')
  } })
  if (reference.kind === 'note') return <p className="mentor-prose">{reference.note}</p>
  return <div><p>{reference.kind} {reference.id} · Exact version {reference.version}</p><Button type="button" variant="outline" onClick={() => setOpen(!open)}>{open ? 'Hide' : 'Read'} exact evidence version</Button>{open && (query.isPending ? <Loading /> : query.error ? <Failure error={query.error} /> : query.data.observation ? <><p className="mentor-prose">{query.data.observation.evidence.evidence}</p><p className="mentor-prose">Outcome: {query.data.observation.evidence.outcome || 'Not recorded'}</p></> : <><p>Formal submission revision {query.data.submission!.revision}</p><ul>{query.data.submission!.evidence.map((e, i) => <li key={i}>{e.label} · {e.kind}{e.url && safeURL(e.url) && <> · <a href={safeURL(e.url)} target="_blank" rel="noopener noreferrer">Open submitted reference</a></>}{e.file_id && <> · <a href={`/api/v1/files/${encodeURIComponent(e.file_id)}/download`}>Download submitted file</a></>}</li>)}</ul></>)}</div>
}

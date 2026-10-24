import { EventGuidance } from './event-guidance'
import { useCallback, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { api, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { Failure, Loading } from './components/states'
import { Button } from './components/ui/button'
import type { CollectionState } from './components/patterns'
import { TodayView, type TodayContext } from './today-view'
import { useTodayRead } from './today-read'
import { loadTodayFacts, todayActions, type TodayPage } from './today-facts'
import { TodayLifecycle, TodayReadiness } from './today-lifecycle'
import type { PrivateRequestScope } from './private-request-scope'

function authoritySignature(page: TodayPage) { return `${page.event.id}:${page.roles.join(':')}:${page.participation_state}:${page.event.stage}:${page.event.can_read_staff}:${page.event.can_edit}:${page.event.can_present}:${page.event.can_read_review_coverage}` }

export function EventToday({ event, openSection, availableSections }: { availableSections: readonly string[]; event: EventRecord; openSection: (section: string) => void }) {
  const identity = useIdentity()
  return <TodayLifetime key={`${identity.person.id}:${identity.draft_scope}:${event.id}`} eventID={event.id} openSection={openSection} availableSections={availableSections} />
}
function TodayLifetime({ eventID, openSection, availableSections }: { availableSections: readonly string[]; eventID: string; openSection: (section: string) => void }) {
  const identity = useIdentity(), [selected, setSelected] = useState<TodayContext>('personal')
  const load = useCallback(async (signal: AbortSignal) => { const value = unwrap(await api.GET('/api/v1/events/{event_id}/today', { signal, cache: 'no-store', params: { path: { event_id: eventID } } })); if (value.event.id !== eventID) throw new Error('The current Today event identity changed.'); return value }, [eventID])
  const query = useTodayRead(`${identity.person.id}:${identity.draft_scope}:${eventID}:today`, load, undefined, authoritySignature), page = query.data
  if (query.denied || !page && query.error) return <Failure error={query.error} retry={() => void query.refresh()} />
  if (!page) return <Loading />
  const context = selected === 'personal' || page.roles.includes(selected) ? selected : 'personal'
  const changeContext = (value: TodayContext) => { if (value === 'personal' || page.roles.includes(value)) setSelected(value) }
  // Actual role/stage/participation changes dispose the local private projection;
  // a selected UI context is never used as a permission or an API capability.
  const owner = `${identity.person.id}:${identity.draft_scope}:${eventID}:${context}:${page.roles.join(':')}:${page.participation_state}:${page.event.stage}:${page.event.can_read_staff}:${page.event.can_edit}`
  return <><div className="fleet-pattern-actions"><Button icon="refresh" variant="outline" onClick={() => void query.refresh()}>Refresh Today facts</Button></div>{query.error && <p role="alert">Current Today authority could not be checked. Previously observed facts remain visible; Today queue actions are paused.</p>}<TodayContextFacts owner={owner} page={page} context={context} changeContext={changeContext} openSection={openSection} availableSections={availableSections} parentScope={query.authority} paused={!!query.error || query.pending} /></>
}
function TodayContextFacts({ owner, page, context, changeContext, openSection, availableSections, parentScope, paused }: { availableSections: readonly string[]; owner: string; page: TodayPage; context: TodayContext; changeContext: (value: TodayContext) => void; openSection: (section: string) => void; parentScope: PrivateRequestScope; paused: boolean }) {
  const identity = useIdentity(), navigate = useNavigate(), [cursors, setCursors] = useState<{ owner: string; review?: string; document?: string }>({ owner })
  const reviewCursor = cursors.owner === owner ? cursors.review : undefined, documentCursor = cursors.owner === owner ? cursors.document : undefined
  const setReviewCursor = (review?: string) => setCursors({ owner, review, document: documentCursor }), setDocumentCursor = (document?: string) => setCursors({ owner, review: reviewCursor, document })
  const load = useCallback((signal: AbortSignal, assertCurrent: () => void) => loadTodayFacts(page, context, identity.person.id, signal, reviewCursor, documentCursor, assertCurrent), [page, context, identity.person.id, reviewCursor, documentCursor])
  const query = useTodayRead(`${owner}:${reviewCursor ?? 'first'}:${documentCursor ?? 'first'}:facts`, load, parentScope)
  const items = query.data && !query.denied ? todayActions(page, context, identity.person.id, query.data, {
    section: openSection, availableSections,
    registration: () => { void navigate({ to: '/register/$eventId', params: { eventId: page.event.id } }) },
    resource: id => { void navigate({ to: '/events/$eventId/resources/$resourceId', params: { eventId: page.event.id, resourceId: id } }) },
    help: id => { if (id) void navigate({ to: '/events/$eventId/help/$requestId', params: { eventId: page.event.id, requestId: id } }); else void navigate({ to: '/events/$eventId/help', params: { eventId: page.event.id } }) },
    clarifications: () => { void navigate({ to: '/events/$eventId/clarifications', params: { eventId: page.event.id } }) },
    reviews: teamID => { if (teamID) void navigate({ to: '/events/$eventId/review/$teamId', params: { eventId: page.event.id, teamId: teamID } }); else void navigate({ to: '/events/$eventId/review', params: { eventId: page.event.id } }) },
  }) : []
  const actionsState: CollectionState = query.denied ? { kind: 'denied', message: 'Current task/readiness access is unavailable. No prior private tasks are retained.' } : query.error ? { kind: query.data ? 'paused' : 'failed', message: 'Current task/readiness facts could not be checked.', read: { label: 'Check current task access', onSelect: () => { void query.refresh() } } } : query.pending && !query.data ? { kind: 'loading' } : { kind: 'ready' }
  return <section className="fleet-today-private"><TodayView page={page} context={context} changeContext={changeContext} items={items} actionsState={actionsState} contextDisabled={paused} paused={paused || !!query.error || query.pending} /><EventGuidance key={authoritySignature(page)} event={page.event} parentScope={parentScope} /><TodayLifecycle page={page} facts={query.denied ? undefined : query.data} />{query.pending && !query.data ? <Loading /> : query.error ? <Failure error={query.error} retry={() => void query.refresh()} /> : query.data && <><TodayReadiness facts={query.data} />{query.data.guidance.warnings.length > 0 && <section className="fleet-today-section"><h2>Guidance readiness warnings</h2><ul>{query.data.guidance.warnings.map((warning, index) => <li key={`${warning.code}:${index}`}>{warning.message}</li>)}</ul></section>}{(documentCursor || query.data.nextDocumentCursor) && <div className="fleet-pattern-actions"><p>Only the current released-document page is represented; no all-documents completion is inferred.</p>{documentCursor && <Button variant="outline" onClick={() => setDocumentCursor(undefined)}>First released-document page</Button>}{query.data.nextDocumentCursor && <Button variant="outline" disabled={paused || query.pending} onClick={() => setDocumentCursor(query.data!.nextDocumentCursor!)}>Next released-document page</Button>}</div>}{query.data.review && <div className="fleet-pattern-actions"><p>Only the current review page is shown; no global counts are inferred.</p>{reviewCursor && <Button variant="outline" onClick={() => setReviewCursor(undefined)}>First review coverage page</Button>}{query.data.review.next_cursor && <Button variant="outline" disabled={paused || query.pending} onClick={() => setReviewCursor(query.data!.review!.next_cursor!)}>Next current review coverage page</Button>}</div>}</>}</section>
}

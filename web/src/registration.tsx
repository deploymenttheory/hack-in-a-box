import { CollectionFrame, CollectionHeading, CollectionRecords, CollectionPagination, ContextLink } from './components/patterns'
import { StatusBadge } from './components/ui/feedback'
import { Field, Input, Select, Checkbox, Label } from './components/ui/fields'
import { Alert } from './components/ui/surfaces'
import { usePrivateRequestScope } from './private-request-scope'
import { TeamWorkspace } from './teams'
import { RegistrationActions } from './registration-actions'
import { useEffect, useRef, useState } from 'react'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from '@tanstack/react-router'
import { Controller, useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { api, APIError, privateKey, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Theme } from './components/theme'
import { Failure, Loading } from './components/states'
import { brandStyle, brandAsset } from './branding'
import { FormQuestions, visibleFields } from './form-questions'

export type Registration = components['schemas']['Registration']
export type RegistrationEvent = components['schemas']['RegistrationEvent']
export function eventTime(value: string, timezone: string) { return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short', timeZone: timezone }).format(new Date(value)) }
export function RegistrationDirectory() {
  const identity = useIdentity()
  const query = useInfiniteQuery({ queryKey: privateKey(identity.person.id, '', 'registration-directory'), initialPageParam: undefined as string | undefined, queryFn: async ({ pageParam }) => unwrap(await api.GET('/api/v1/registration/events', { params: { query: { after: pageParam } } })), getNextPageParam: last => last.next_cursor ?? undefined })
  const items = query.data?.pages.flatMap(page => page.items) ?? []
  const state = query.isPending ? { kind: 'loading' as const } : query.error ? { kind: 'failed' as const, message: 'Available events could not be loaded.', read: { label: 'Try again', onSelect: () => void query.refetch() } } : !items.length ? { kind: 'empty' as const, message: 'No open events or previous applications are available.' } : { kind: 'ready' as const }
  return <CollectionFrame scopeKey={identity.person.id} state={state} heading={<CollectionHeading title="Find your next hack" scope={{ entity: 'Event registration' }} description="Each person registers using their organisation account. Team invitations do not reserve places." />}><CollectionRecords label="Available events and applications" rows={items} rowKey={event => event.id} rowLabel={event => event.name} destination={event => ({ label: event.name, href: `/register/${event.id}` })} columns={[
    { id: 'applications', label: 'Applications', essential: true, cell: event => <StatusBadge tone={event.open ? 'success' : 'neutral'}>{event.open ? 'Applications open' : 'Applications closed'}</StatusBadge> },
    { id: 'start', label: 'Live start', cell: event => eventTime(event.live_start, event.timezone) },
    { id: 'venue', label: 'Venue', cell: event => event.venue },
    { id: 'timezone', label: 'Timezone', cell: event => event.timezone },
  ]} /><CollectionPagination source={{ kind: 'cursor', loaded: items.length, next: query.hasNextPage ? () => void query.fetchNextPage() : undefined, busy: query.isFetchingNextPage }} label="Registration directory pages" /></CollectionFrame>
}
export function RegistrationPage() {
  const { eventId } = useParams({ strict: false }), identity = useIdentity()
  const query = useQuery({ queryKey: privateKey(identity.person.id, eventId!, 'registration-page'), queryFn: async () => unwrap(await api.GET('/api/v1/registration/events/{event_id}', { params: { path: { event_id: eventId! } } })), refetchOnWindowFocus: 'always' })
  if (query.isPending) return <Loading />
  if (!query.data || (query.error instanceof APIError && [401, 403].includes(query.error.status))) return <Failure error={query.error} retry={() => void query.refetch()} />
  const { event, branding, forms, registrations } = query.data, logo = brandAsset(branding, 'logo')
  const available = forms.filter(f => !registrations.some(r => r.kind === (f.definition.audience === 'attendee' ? 'attendee' : 'facilitator') && !['withdrawn', 'declined'].includes(r.state)))
  return <Theme style={brandStyle(branding)}><section className="fleet-registration-workspace">{!!query.error && <Failure error={query.error} retry={() => void query.refetch()} />}<ContextLink back target={{ href: '/register', label: 'Find events' }} />{logo && <img className="event-logo" src={logo} alt="Event logo" />}<CollectionHeading title={event.name} scope={{ entity: 'Registration', event: event.stage }} description={event.description} /><dl className="fleet-admission-facts"><div><dt>Live hack</dt><dd>{eventTime(event.live_start, event.timezone)} — {eventTime(event.live_end, event.timezone)}</dd></div><div><dt>Venue</dt><dd>{event.venue || 'Not yet announced'}</dd></div><div><dt>Timezone</dt><dd>{event.timezone}</dd></div><div><dt>Final admission</dt><dd>{eventTime(event.admission_cutoff, event.timezone)}</dd></div></dl>
    {registrations.length > 0 && <section className="fleet-admission-section"><h2>Your applications</h2>{registrations.map(r => <RegistrationStatus key={r.id} registration={r} event={event} />)}</section>}
    {!event.open && <Alert role="status">New applications are closed. Existing registrations remain valid.</Alert>}
    {event.open && <section className="fleet-admission-section"><fieldset className="fleet-admission-fields" disabled={!!query.error}><p>Attendee places are allocated in order. Facilitator applications are reviewed separately and do not reserve a competitor place.</p>{available.length ? <ApplicationChooser key={available.map(f => f.definition.id + ':' + f.version).join(',')} event={event} forms={available} /> : !registrations.length && <p>Registration forms will appear once the organiser publishes them.</p>}</fieldset></section>}
    {registrations.some(r => r.kind === 'attendee' && ['confirmed', 'waitlisted', 'offered'].includes(r.state)) && <TeamWorkspace eventID={event.id} timezone={event.timezone} />}
  </section></Theme>
}
const states: Record<string, string> = { confirmed: 'Your competitor place is confirmed.', waitlisted: 'You are on the waitlist. You do not yet hold a place.', received: 'Your facilitator application has been received for review.', selected: 'You have been selected as a facilitator.', backup: 'You are a backup facilitator.', declined: 'This application was declined.', withdrawn: 'This application has been withdrawn.', offered: 'A competitor place is reserved for you. Respond before the deadline.' }
export function RegistrationStatus({ registration: r, event }: { registration: Registration; event: RegistrationEvent }) {
  const expired = r.offer?.state === 'expired'
  const tone = r.state === 'confirmed' || r.state === 'selected' ? 'success' : r.state === 'offered' || r.state === 'waitlisted' ? 'warning' : 'neutral'
  return <section className="fleet-admission-section" aria-label={r.audience + ' application status'}><div className="fleet-admission-heading"><h3>{r.audience === 'attendee' ? 'Competitor registration' : r.audience + ' application'}</h3><StatusBadge tone={tone}>{expired ? 'Offer expired' : r.state}</StatusBadge></div><p role="status">{states[r.state]}</p>{expired && <p>The offer deadline passed. This expired offer cannot confirm a competitor place.</p>}{r.waitlist_position != null && <p>Waitlist position: {r.waitlist_position}</p>}{r.state === 'confirmed' && <Link className="fleet-back-link" to="/events/$eventId" params={{ eventId: event.id }}>Open event workspace</Link>}<dl className="fleet-admission-facts"><div><dt>Application reference</dt><dd>{r.id}</dd></div><div><dt>Form version</dt><dd>{r.form_version}</dd></div><div><dt>Current revision</dt><dd>{r.revision}</dd></div></dl>{r.offer && <p>{r.offer.state === 'pending' ? 'Respond by' : 'Offer ' + r.offer.state} · {eventTime(r.offer.expires_at, event.timezone)} ({event.timezone})</p>}<RegistrationActions registration={r} frozen={event.team_frozen} /></section>
}
function ApplicationChooser({ event, forms }: { event: RegistrationEvent; forms: components['schemas']['PublishedRegistrationForm'][] }) {
  const [selected, setSelected] = useState(forms[0].definition.id), [locked, setLocked] = useState(false)
  return <section><h2>Apply to take part</h2><Field id="application-type" label="Application type" description="Each published form has its own audience and version."><Select disabled={locked} value={selected} onChange={e => setSelected(e.target.value)}>{forms.map(f => <option key={f.definition.id} value={f.definition.id}>{f.definition.name}</option>)}</Select></Field><ApplicationForm key={selected} event={event} published={forms.find(f => f.definition.id === selected)!} onLocked={setLocked} /></section>
}
const applicationSchema = z.object({ answers: z.record(z.string(), z.unknown()), team_preference: z.enum(['captain', 'join', 'solo']), invitation_reference: z.string().max(160), fallback_to_attendee: z.boolean() })
function ApplicationForm({ event, published, onLocked }: { onLocked: (locked: boolean) => void; event: RegistrationEvent; published: components['schemas']['PublishedRegistrationForm'] }) {
  const identity = useIdentity(), cache = useQueryClient(), [id] = useState(() => crypto.randomUUID()), busy = useRef(false), [failure, setFailure] = useState<unknown>(), [pending, setPending] = useState(false)
  const form = useForm<z.infer<typeof applicationSchema>>({ resolver: zodResolver(applicationSchema), defaultValues: { answers: {}, team_preference: 'solo', invitation_reference: '', fallback_to_attendee: false } })
  const scope = usePrivateRequestScope(identity.person.id + ':' + identity.draft_scope + ':' + event.id + ':' + id)
  const captured = useRef<z.infer<typeof applicationSchema> | undefined>(undefined)
  const [uncertain, setUncertain] = useState(false), [review, setReview] = useState<z.infer<typeof applicationSchema>>()
  useEffect(() => { onLocked(pending || uncertain || !!review); return () => onLocked(false) }, [pending, uncertain, review, onLocked])
  async function submit(values: z.infer<typeof applicationSchema>) {
    if (busy.current) return
    const ticket = scope.capture(); if (!ticket.current()) return
    const input = captured.current ?? structuredClone(values)
    captured.current = input
    busy.current = true; setPending(true); setFailure(undefined)
    try {
      const result = unwrap(await api.POST('/api/v1/events/{event_id}/registrations', { params: { path: { event_id: event.id }, header: { 'X-CSRF-Token': identity.csrf_token } }, body: { ...input, id, form_id: published.definition.id, form_version: published.version } }))
      if (!ticket.current()) return
      if (result.id !== id || result.event_id !== event.id || result.person_id !== identity.person.id || result.form_id !== published.definition.id || result.form_version !== published.version) throw new Error('Unverified application response')
      setUncertain(false)
      await cache.invalidateQueries({ queryKey: ['private', identity.person.id] })
    } catch (error) { if (ticket.current()) { setFailure(error); const unknown = !(error instanceof APIError) || error.status >= 500 || error.status === 408; setUncertain(unknown); if (!unknown) { captured.current = undefined; setReview(undefined) } await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, event.id, 'registration-page') }) } } finally { busy.current = false; if (ticket.current()) setPending(false) }
  }
  return <form onSubmit={e => void form.handleSubmit(values => { setReview({ ...structuredClone(values), answers: Object.fromEntries(visibleFields(published.definition, values.answers).filter(field => field.id in values.answers).map(field => [field.id, values.answers[field.id]])) }) })(e)} className="fleet-admission-form"><p>Application for {identity.person.display_name}. Your answers are visible to authorised event organisers and retained under the event’s privacy policy.</p><p className="fleet-reference">Published form version {published.version} · Application reference {id}</p><fieldset className="fleet-admission-fields" disabled={pending || uncertain || !!review}><legend>Application details</legend><Controller control={form.control} name="answers" render={({ field }) => <FormQuestions definition={published.definition} answers={field.value} change={field.onChange} issues={failure instanceof APIError ? failure.blockers : []} />} />{published.definition.audience === 'attendee' ? <><Field id="team-preference" label="Team preference"><Select {...form.register('team_preference')}><option value="solo">Place me in a team</option><option value="captain">I would like to captain a team</option><option value="join">I would like to join a team</option></Select></Field><Field id="invitation-reference" label="Team invitation reference (optional)" description="Every teammate must register individually. This preference does not reserve a team or an event place."><Input maxLength={160} {...form.register('invitation_reference')} /></Field></> : <Label className="fleet-admission-check"><Checkbox {...form.register('fallback_to_attendee')} />If I am not selected, invite me to apply separately as a competitor. This does not register me or guarantee eligibility.</Label>}</fieldset>{uncertain && <Alert tone="warning" role="status"><h3>Application outcome unknown</h3><p>Keep this exact application reference. Your next deliberate submission repeats the same UUID and captured answers; the server checks its original fingerprint and cannot allocate a second application for that request. No retry occurs automatically.</p><Button type="button" icon="refresh" variant="outline" onClick={() => void cache.invalidateQueries({ queryKey: privateKey(identity.person.id, event.id, 'registration-page') })}>Check current application status</Button></Alert>}{review && !uncertain && <section className="fleet-admission-review"><h3>Review your application</h3><p>{published.definition.name} · version {published.version} · reference {id}. Confirm your answers and team preference before sending.</p><dl className="fleet-admission-summary">{visibleFields(published.definition, review.answers).filter(field => field.id in review.answers).map(field => <div key={field.id}><dt>{field.label}</dt><dd>{Array.isArray(review.answers[field.id]) ? (review.answers[field.id] as unknown[]).map(String).join(', ') : String(review.answers[field.id])}</dd></div>)}{published.definition.audience === 'attendee' && <div><dt>Team preference</dt><dd>{review.team_preference}{review.invitation_reference && ` · ${review.invitation_reference}`}</dd></div>}</dl><Button type="button" variant="outline" disabled={pending} onClick={() => setReview(undefined)}>Correct application details</Button></section>}<div className="fleet-pattern-actions">{review || uncertain ? <Button type="button" icon="send" pending={pending} onClick={() => void submit(uncertain ? captured.current! : review!)}>Submit application</Button> : <Button type="submit" icon="next">Review application</Button>}</div>{!!failure && <Failure error={failure} />}</form>
}

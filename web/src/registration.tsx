import { useRef, useState } from 'react'
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
import { FormQuestions } from './form-questions'

export type Registration = components['schemas']['Registration']
export type RegistrationEvent = components['schemas']['RegistrationEvent']
export function eventTime(value: string, timezone: string) { return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short', timeZone: timezone }).format(new Date(value)) }
export function RegistrationDirectory() {
  const identity = useIdentity()
  const query = useInfiniteQuery({ queryKey: privateKey(identity.person.id, '', 'registration-directory'), initialPageParam: undefined as string | undefined, queryFn: async ({ pageParam }) => unwrap(await api.GET('/api/v1/registration/events', { params: { query: { after: pageParam } } })), getNextPageParam: last => last.next_cursor ?? undefined })
  return <section className="workspace"><h1>Find your next hack</h1><p>Each person registers using their organisation account. Team invitations do not reserve places.</p>{query.isPending ? <Loading /> : query.error ? <Failure error={query.error} /> : <>{query.data.pages.flatMap(p => p.items).map(event => <article className="panel section-heading" key={event.id}><h2><Link to="/register/$eventId" params={{ eventId: event.id }}>{event.name}</Link></h2><p>{event.description}</p><p>{eventTime(event.live_start, event.timezone)} · {event.venue} · {event.timezone}</p><p>{event.open ? 'Applications open' : 'Applications closed'}</p></article>)}{!query.data.pages[0].items.length && <p>No events are accepting applications.</p>}{query.hasNextPage && <Button disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>More events</Button>}</>}</section>
}
export function RegistrationPage() {
  const { eventId } = useParams({ strict: false }), identity = useIdentity()
  const query = useQuery({ queryKey: privateKey(identity.person.id, eventId!, 'registration-page'), queryFn: async () => unwrap(await api.GET('/api/v1/registration/events/{event_id}', { params: { path: { event_id: eventId! } } })), refetchOnWindowFocus: 'always' })
  if (query.isPending) return <Loading />
  if (query.error) return <Failure error={query.error} retry={() => void query.refetch()} />
  const { event, branding, forms, registrations } = query.data, logo = brandAsset(branding, 'logo')
  const available = forms.filter(f => !registrations.some(r => r.kind === (f.definition.audience === 'attendee' ? 'attendee' : 'facilitator') && !['withdrawn', 'declined'].includes(r.state)))
  return <Theme style={brandStyle(branding)}><section className="workspace"><Link to="/register">← Find events</Link>{logo && <img className="event-logo" src={logo} alt="Event logo" />}<h1>{event.name}</h1><p>{event.description}</p><p>{eventTime(event.live_start, event.timezone)} — {eventTime(event.live_end, event.timezone)} · {event.timezone}</p><p>{event.venue}</p>{registrations.map(r => <RegistrationStatus key={r.id} registration={r} event={event} />)}{!event.open && <p role="status">New applications are closed. Existing registrations remain valid.</p>}{event.open && <><p>Attendee places are allocated in order. Facilitator applications are reviewed separately and do not reserve a competitor place.</p>{available.length ? <ApplicationChooser key={available.map(f => `${f.definition.id}:${f.version}`).join(',')} event={event} forms={available} /> : !registrations.length && <p>Registration forms will appear once the organiser publishes them.</p>}</>}</section></Theme>
}
const states: Record<string, string> = { confirmed: 'Your competitor place is confirmed.', waitlisted: 'You are on the waitlist. You do not yet hold a place.', received: 'Your facilitator application has been received for review.', selected: 'You have been selected as a facilitator.', backup: 'You are a backup facilitator.', declined: 'This application was declined.', withdrawn: 'This application has been withdrawn.', offered: 'A competitor place is reserved for you. Respond before the deadline.' }
export function RegistrationStatus({ registration: r, event }: { registration: Registration; event: RegistrationEvent }) {
  return <section className="panel section-heading" aria-label={`${r.audience} application status`}><h2>{r.audience === 'attendee' ? 'Competitor registration' : `${r.audience} application`}</h2><p role="status">{states[r.state]}</p>{r.waitlist_position != null && <p>Waitlist position: {r.waitlist_position}</p>}{r.state === 'confirmed' && <Link to="/events/$eventId" params={{ eventId: event.id }}>Open event workspace</Link>}<p className="muted">Reference: {r.id}</p></section>
}
function ApplicationChooser({ event, forms }: { event: RegistrationEvent; forms: components['schemas']['PublishedRegistrationForm'][] }) {
  const [selected, setSelected] = useState(forms[0].definition.id)
  return <section className="panel section-heading"><h2>Apply to take part</h2><label htmlFor="application-type">Application type</label><select id="application-type" value={selected} onChange={e => setSelected(e.target.value)}>{forms.map(f => <option key={f.definition.id} value={f.definition.id}>{f.definition.name}</option>)}</select><ApplicationForm key={selected} event={event} published={forms.find(f => f.definition.id === selected)!} /></section>
}
const applicationSchema = z.object({ answers: z.record(z.string(), z.unknown()), team_preference: z.enum(['captain', 'join', 'solo']), invitation_reference: z.string().max(160), fallback_to_attendee: z.boolean() })
function ApplicationForm({ event, published }: { event: RegistrationEvent; published: components['schemas']['PublishedRegistrationForm'] }) {
  const identity = useIdentity(), cache = useQueryClient(), [id] = useState(() => crypto.randomUUID()), busy = useRef(false), [failure, setFailure] = useState<unknown>(), [pending, setPending] = useState(false)
  const form = useForm<z.infer<typeof applicationSchema>>({ resolver: zodResolver(applicationSchema), defaultValues: { answers: {}, team_preference: 'solo', invitation_reference: '', fallback_to_attendee: false } })
  async function submit(values: z.infer<typeof applicationSchema>) {
    if (busy.current) return
    busy.current = true; setPending(true); setFailure(undefined)
    try {
      unwrap(await api.POST('/api/v1/events/{event_id}/registrations', { params: { path: { event_id: event.id }, header: { 'X-CSRF-Token': identity.csrf_token } }, body: { ...values, id, form_id: published.definition.id, form_version: published.version } }))
      await cache.invalidateQueries({ queryKey: ['private', identity.person.id] })
    } catch (error) { setFailure(error); await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, event.id, 'registration-page') }) } finally { busy.current = false; setPending(false) }
  }
  return <form onSubmit={e => void form.handleSubmit(submit)(e)} className="setup-form section-heading"><p>Application for {identity.person.display_name}. Your answers are visible to authorised event organisers and retained under the event’s privacy policy.</p><Controller control={form.control} name="answers" render={({ field }) => <FormQuestions definition={published.definition} answers={field.value} change={field.onChange} issues={failure instanceof APIError ? failure.blockers : []} />} />{published.definition.audience === 'attendee' ? <><label htmlFor="team-preference">Team preference</label><select id="team-preference" {...form.register('team_preference')}><option value="solo">Place me in a team</option><option value="captain">I would like to captain a team</option><option value="join">I would like to join a team</option></select><label htmlFor="invitation-reference">Team invitation reference (optional)</label><input id="invitation-reference" maxLength={160} {...form.register('invitation_reference')} /><p>Every teammate must register individually. This preference does not reserve a team or an event place.</p></> : <label className="check"><input type="checkbox" {...form.register('fallback_to_attendee')} />If I am not selected, invite me to apply separately as a competitor. This does not register me or guarantee eligibility.</label>}<Button type="submit" disabled={pending}>{pending ? 'Submitting…' : 'Submit application'}</Button>{!!failure && <Failure error={failure} />}</form>
}

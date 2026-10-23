import { useEffect, useRef, useState } from 'react'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, APIError, privateKey, unwrap, type EventRecord } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { eventTime } from './registration'
import { CollectionHeading, CollectionRecords, CollectionPagination, DetailWorkspace } from './components/patterns'
import { Input, Select, Textarea, Label, Checkbox } from './components/ui/fields'
import { StatusBadge } from './components/ui/feedback'
import { ConfirmationDialog } from './components/ui/dialog'
import { AdmissionFeedback, useAdmissionCommand } from './registration-command'

export function ApplicantReview({ event, openSetup, onDirty }: { event: EventRecord; openSetup: () => void; onDirty?: (dirty: boolean) => void }) {
  const identity = useIdentity(), [selected, setSelected] = useState<string>(), [filter, setFilter] = useState('all'), [search, setSearch] = useState(''), [decisionLocked, setDecisionLocked] = useState(false), [policyLocked, setPolicyLocked] = useState(false)
  const locked = decisionLocked || policyLocked
  useEffect(() => { onDirty?.(locked); return () => onDirty?.(false) }, [locked, onDirty])
  const trigger = useRef<HTMLButtonElement | null>(null)
  const query = useInfiniteQuery({ queryKey: privateKey(identity.person.id, event.id, 'applicants'), initialPageParam: 0, queryFn: async ({ pageParam, signal }) => unwrap(await api.GET('/api/v1/events/{event_id}/registrations/review', { signal, params: { path: { event_id: event.id }, query: { after: pageParam } } })), getNextPageParam: page => page.next_cursor ?? undefined })
  const rows = query.data?.pages.flatMap(p => p.items) ?? [], visible = rows.filter(a => (filter === 'all' || a.registration.kind === filter) && a.display_name.toLowerCase().includes(search.toLowerCase()))
  const denied = query.error instanceof APIError && [401, 403].includes(query.error.status)
  return <section><CollectionHeading title="Applicant review" scope={{ entity: 'Registration operations', event: event.name }} description={<>{event.confirmed_attendees} confirmed competitors · capacity {event.configuration.capacity} · minimum {event.configuration.minimum_attendance}. Reserved offers do not count as confirmed competitors.</>} />{event.can_edit && <><Button variant="outline" icon="list" disabled={locked} onClick={openSetup}>Change capacity in event setup</Button><OfferPolicy event={event} onLocked={setPolicyLocked} /></>}
    <DetailWorkspace collection={<><div className="fleet-admission-filter"><div><Label htmlFor="applicant-filter">Applications shown</Label><Select id="applicant-filter" value={filter} disabled={locked} onChange={e => setFilter(e.target.value)}><option value="all">All applicants</option><option value="attendee">Competitors and waitlist</option><option value="facilitator">Facilitator applications</option></Select></div><div><Label htmlFor="applicant-search">Find in loaded applicants</Label><Input id="applicant-search" type="search" value={search} disabled={locked} onChange={e => setSearch(e.target.value)} /></div></div><p>{visible.length} matching in {rows.length} loaded applicants. Load more to inspect later records.</p>{query.isPending ? <Loading /> : denied || !query.data ? <Failure error={query.error} /> : <>{!!query.error && <Failure error={query.error} />}<CollectionRecords label="Applicants" rows={visible} rowKey={a => a.registration.id} rowLabel={a => a.display_name} columns={[{ id: 'audience', label: 'Audience', cell: a => a.registration.audience }, { id: 'state', label: 'Status', cell: a => <StatusBadge tone="neutral">{a.registration.state}</StatusBadge> }, { id: 'position', label: 'Waitlist', cell: a => a.registration.waitlist_position ?? '—' }]} actions={a => <Button icon="search" variant="ghost" data-record-focus={a.registration.id} disabled={locked || !!query.error} onClick={e => { trigger.current = e.currentTarget; setSelected(a.registration.id) }}>Review {a.display_name}</Button>} /><CollectionPagination label="Applicant pages" source={{ kind: 'cursor', loaded: rows.length, busy: query.isFetchingNextPage, next: query.hasNextPage ? () => { void query.fetchNextPage() } : undefined }} /></>}</>} detail={selected && !denied ? <section className="fleet-inspector" aria-label="Applicant details"><Button icon="back" variant="ghost" disabled={locked} onClick={() => { setSelected(undefined); requestAnimationFrame(() => trigger.current?.focus()) }}>Back to applicants</Button><ApplicantDetails key={selected} event={event} id={selected} onLocked={setDecisionLocked} /></section> : undefined} />
  </section>
}
function OfferPolicy({ event, onLocked }: { event: EventRecord; onLocked: (locked: boolean) => void }) {
  const identity = useIdentity()
  const query = useQuery({ queryKey: privateKey(identity.person.id, event.id, 'offer-policy'), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/registrations/offer-policy', { params: { path: { event_id: event.id } } })) })
  if (query.isPending) return <Loading />
  if (!query.data || (query.error instanceof APIError && [401, 403].includes(query.error.status))) return <Failure error={query.error} />
  return <PolicyEditor event={event} onLocked={onLocked} value={query.data} paused={!!query.error} read={async () => { const result = await query.refetch(); if (result.error) throw result.error }} />
}
function PolicyEditor({ event, value, read, paused, onLocked }: { onLocked: (locked: boolean) => void; paused: boolean; event: EventRecord; value: components['schemas']['OfferSettings']; read: () => Promise<void> }) {
  const identity = useIdentity(), cache = useQueryClient(), [automated, setAutomated] = useState(value.automated), [days, setDays] = useState(value.working_days), [review, setReview] = useState<{ automated: boolean; working_days: number; revision: number }>()
  const command = useAdmissionCommand(`${identity.person.id}:${identity.draft_scope}:${event.id}:offer-policy`)
  const dirty = automated !== value.automated || days !== value.working_days
  useEffect(() => { onLocked(command.blocked || !!review || dirty); return () => onLocked(false) }, [command.blocked, review, dirty, onLocked])
  async function save() {
    if (!review) return
    const captured = review
    await command.run({ label: 'Save offer policy', resource: event.id, revision: captured.revision }, async () => {
      const result = unwrap(await api.PUT('/api/v1/events/{event_id}/registrations/offer-policy', { params: { path: { event_id: event.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${captured.revision}"` } }, body: { automated: captured.automated, working_days: captured.working_days } }))
      if (result.revision <= captured.revision) throw new Error('Unexpected policy response')
      return result
    }, async result => { setReview(undefined); await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, event.id, 'offer-policy') }); return `Offer policy saved at revision ${result.revision}.` })
  }
  return <details className="fleet-admission-section"><summary>Waitlist automation and offer policy</summary><p>Approved automation offers available places in FIFO order, including during preparation, until final admission. Existing offers still expire when automation is disabled.</p><form className="fleet-admission-form" onSubmit={e => { e.preventDefault(); setReview({ automated, working_days: days, revision: value.revision }) }}><fieldset className="fleet-admission-fields" disabled={command.blocked || paused}><legend>Offer policy · revision {value.revision}</legend><Label htmlFor="offer-days">Working days to respond</Label><Input id="offer-days" type="number" min={1} max={30} value={days} onChange={e => setDays(Number(e.target.value))} /><Label className="fleet-admission-check"><Checkbox checked={automated} onChange={e => setAutomated(e.target.checked)} />Approve automatic FIFO offers</Label></fieldset><Button type="submit" icon="list" disabled={command.blocked || paused}>Review offer policy</Button><Button type="button" variant="outline" disabled={command.blocked || !dirty} onClick={() => { setAutomated(value.automated); setDays(value.working_days) }}>Reset policy edits</Button><AdmissionFeedback command={command} read={read} /></form><ConfirmationDialog open={!!review} onOpenChange={open => { if (!open && !command.blocked) setReview(undefined) }} title="Review offer policy" description="Automation offers places in FIFO order. It does not confirm attendance. Review the response window before saving." confirmLabel="Save offer policy" pending={command.pending} disabled={command.blocked || paused} onConfirm={() => void save()}><p>{review?.automated ? 'Automatic FIFO offers approved' : 'Automatic offers disabled'} · {review?.working_days} working days to respond · revision {review?.revision}</p>{review && <AdmissionFeedback command={command} read={async () => { await read(); setReview(undefined) }} />}</ConfirmationDialog></details>
}
function ApplicantDetails({ event, id, onLocked }: { event: EventRecord; id: string; onLocked: (locked: boolean) => void }) {
  const identity = useIdentity(), [denied, setDenied] = useState<APIError>()
  const query = useQuery({ queryKey: privateKey(identity.person.id, event.id, `applicant:${id}`), queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/events/{event_id}/registrations/{registration_id}/review', { signal, params: { path: { event_id: event.id, registration_id: id } } })) })
  if (denied) return <Failure error={denied} />
  if (query.isPending) return <Loading />
  if (!query.data || (query.error instanceof APIError && [401, 403].includes(query.error.status))) return <Failure error={query.error} />
  return <>{!!query.error && <Failure error={query.error} />}<ApplicantDecision event={event} id={id} data={query.data} paused={!!query.error} onLocked={onLocked} onDenied={setDenied} read={async () => { const result = await query.refetch(); if (result.error) throw result.error }} /></>
}
type ReviewDetail = components['schemas']['ApplicantDetail']
function ApplicantDecision({ event, id, data, paused, onLocked, read, onDenied }: { onDenied: (error: APIError) => void; event: EventRecord; id: string; data: ReviewDetail; paused: boolean; onLocked: (locked: boolean) => void; read: () => Promise<void> }) {
  const heading = useRef<HTMLHeadingElement>(null)
  useEffect(() => { heading.current?.focus() }, [id])
  const identity = useIdentity(), cache = useQueryClient(), [reason, setReason] = useState(''), [ack, setAck] = useState(false)
  const command = useAdmissionCommand(`${identity.person.id}:${identity.draft_scope}:${event.id}:${id}:decision`)
  useEffect(() => { if (command.failure instanceof APIError && [401, 403].includes(command.failure.status)) onDenied(command.failure) }, [command.failure, onDenied])
  const [review, setReview] = useState<{ action: 'selected' | 'backup' | 'declined' | 'invite' | 'promote'; reason: string; ack: boolean; revision: number }>()
  const { applicant, response, history, fallback_available } = data, r = applicant.registration
  useEffect(() => { onLocked(command.blocked || !!reason.trim()); return () => onLocked(false) }, [command.blocked, reason, onLocked])
  async function decide() {
    if (!review) return
    const intent = review
    await command.run({ label: `Applicant decision: ${intent.action}`, resource: id, revision: intent.revision }, async () => {
      const params = { path: { event_id: event.id, registration_id: id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${intent.revision}"` } }
      return intent.action === 'promote' ? unwrap(await api.POST('/api/v1/events/{event_id}/registrations/{registration_id}/promote', { params, body: { reason: intent.reason } })) : unwrap(await api.POST('/api/v1/events/{event_id}/registrations/{registration_id}/decision', { params, body: { action: intent.action, reason: intent.reason, acknowledge_competition_restriction: intent.ack } }))
    }, async result => {
      if (result.id !== id || result.event_id !== event.id || result.revision <= intent.revision) throw new Error('Unexpected decision response')
      setReview(undefined); setReason(''); setAck(false)
      await cache.invalidateQueries({ queryKey: ['private', identity.person.id, event.id] })
      return intent.action === 'invite' ? `Invitation queued. The person must apply separately; no place has been allocated. Reference ${result.id} · revision ${result.revision}.` : `Decision recorded. Reference ${result.id} · revision ${result.revision}.`
    })
  }
  function prepare(action: NonNullable<typeof review>['action']) { setReview({ action, reason, ack, revision: r.revision }) }
  return <><h2 tabIndex={-1} ref={heading}>Reviewing {applicant.display_name}</h2><p>{r.audience} · <StatusBadge tone="neutral">{r.state}</StatusBadge> · revision {r.revision}</p>{r.offer && <p>Offer {r.offer.state}: {eventTime(r.offer.expires_at, event.configuration.timezone)}</p>}{response ? <dl className="fleet-admission-summary">{response.definition.fields.filter(f => f.id in response.answers).map(f => <div key={f.id}><dt>{f.label}</dt><dd>{Array.isArray(response.answers[f.id]) ? (response.answers[f.id] as unknown[]).map(String).join(', ') : String(response.answers[f.id])}</dd></div>)}</dl> : <p>Answers have been removed under the event retention policy.</p>}<p>{r.fallback_to_attendee ? 'Prefers an invitation to apply separately as a competitor if not selected.' : 'No fallback invitation requested.'}</p>{event.can_edit && <fieldset className="fleet-admission-fields" disabled={command.blocked || paused}><legend>Record a decision</legend><Label htmlFor="review-reason">Decision or exception reason</Label><Textarea id="review-reason" maxLength={2000} value={reason} onChange={e => setReason(e.target.value)} /><p>Selection as organiser permanently prevents competition in this event. Organiser, mentor and judge selection cannot silently withdraw an active competitor application.</p>{r.audience === 'organiser' && <Label className="fleet-admission-check"><Checkbox checked={ack} onChange={e => setAck(e.target.checked)} />I acknowledge the permanent competition restriction.</Label>}<div className="actions">{r.kind === 'facilitator' && r.state !== 'withdrawn' && <><Button icon="check" disabled={!reason.trim() || (r.audience === 'organiser' && !ack)} onClick={() => prepare('selected')}>Select facilitator</Button><Button icon="list" variant="outline" disabled={!reason.trim()} onClick={() => prepare('backup')}>Keep as backup</Button><Button icon="close" variant="outline" disabled={!reason.trim()} onClick={() => prepare('declined')}>Decline application</Button>{fallback_available && <Button icon="send" variant="outline" disabled={!reason.trim()} onClick={() => prepare('invite')}>Invite separate attendee application</Button>}</>}{r.kind === 'attendee' && r.state === 'waitlisted' && <Button icon="check" disabled={!reason.trim()} onClick={() => prepare('promote')}>Promote with recorded exception</Button>}</div></fieldset>}
    <AdmissionFeedback command={command} read={read} /><h3>Decision history</h3>{!history.length ? <p>No decisions recorded.</p> : <ol>{history.map((h, i) => <li key={i}>{eventTime(h.at, event.configuration.timezone)} · {h.action} · {h.from || 'new'} → {h.to}{h.reason && ` · ${h.reason}`}</li>)}</ol>}
    <ConfirmationDialog open={!!review} onOpenChange={open => { if (!open && !command.blocked) setReview(undefined) }} title="Review applicant decision" description="Confirm the captured decision and reason against this registration version. No place or role changes before confirmation." confirmLabel="Confirm decision" pending={command.pending} disabled={command.blocked || paused} onConfirm={() => void decide()}><dl className="fleet-admission-summary"><div><dt>Action</dt><dd>{review?.action}</dd></div><div><dt>Reason</dt><dd>{review?.reason}</dd></div><div><dt>Registration revision</dt><dd>{review?.revision}</dd></div></dl>{review && <AdmissionFeedback command={command} read={async () => { await read(); setReview(undefined) }} />}</ConfirmationDialog>
  </>
}

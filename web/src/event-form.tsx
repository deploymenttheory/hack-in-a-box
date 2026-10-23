import { PlanningEditorSections, PlanningStep, revealPlanningField } from './planning-layout'
import { Input, Textarea, Select, Label } from './components/ui/fields'
import { wallTime } from './event-time'
import { useEffect, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useBlocker, useNavigate } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { api, APIError, unwrap, type EventRecord } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { safeDraftKey } from './auth'
import { Button } from './components/ui/button'
import { Failure } from './components/states'

const schema = z.object({
  name: z.string().trim().min(1, 'Enter an event name.').max(160), slug: z.string().regex(/^[a-z0-9]+(?:-[a-z0-9]+)*$/, 'Use lowercase letters, numbers and hyphens.'),
  description: z.string().max(4000), venue: z.string().max(500), timezone: z.string().min(1), capacity: z.number().int().min(1).max(10000), minimum_attendance: z.number().int().min(1),
  live_start: z.string(), live_end: z.string(), admission_cutoff: z.string(), team_freeze: z.string(), submission_cutoff: z.string(), dst_choice: z.enum(['', 'earlier', 'later']),
  working_days: z.array(z.string()).min(1, 'Select at least one working day.'), excluded_dates: z.string(), acknowledge: z.boolean(), reason: z.string().max(2000), approve_postponement: z.boolean(),
})
type Values = z.infer<typeof schema>
const dateFields = [['admission_cutoff', 'Final admission'], ['team_freeze', 'Team freeze'], ['live_start', 'Live start'], ['submission_cutoff', 'Submission cutoff'], ['live_end', 'Live end']] as const

function defaults(event?: EventRecord): Values {
  const c = event?.configuration
  return { name: event?.name ?? '', slug: event?.slug ?? '', description: c?.description ?? '', venue: c?.venue ?? '', timezone: c?.timezone ?? 'Europe/London', capacity: c?.capacity ?? 200, minimum_attendance: c?.minimum_attendance ?? 30,
    live_start: wallTime(c?.live_start, c?.timezone), live_end: wallTime(c?.live_end, c?.timezone), admission_cutoff: wallTime(c?.admission_cutoff, c?.timezone), team_freeze: wallTime(c?.team_freeze, c?.timezone), submission_cutoff: wallTime(c?.submission_cutoff, c?.timezone),
    dst_choice: c?.dst_choice ?? '', working_days: (c?.calendar.working_days ?? [1, 2, 3, 4, 5]).map(String), excluded_dates: (c?.calendar.excluded_dates ?? []).join('\n'), acknowledge: !!event, reason: '', approve_postponement: false }
}

export function EventForm({ event, onDirty }: { event?: EventRecord; onDirty?: (dirty: boolean) => void }) {
  const identity = useIdentity(); const cache = useQueryClient(); const navigate = useNavigate()
  const draftKey = safeDraftKey(identity.person.id, event?.id ?? 'new-event')
  const [id] = useState(() => event?.id ?? crypto.randomUUID())
  const root = useRef<HTMLFormElement>(null)
  const [reviewed, setReviewed] = useState<Values>()
  const [savedRevision, setSavedRevision] = useState<number>()
  const [failure, setFailure] = useState<unknown>(); const [saved, setSaved] = useState(false)
  const [preview, setPreview] = useState<components['schemas']['EventPreview']>()
  const [initial] = useState(() => {
    const base = defaults(event)
    try { const draft = JSON.parse(sessionStorage.getItem(draftKey) ?? 'null'); if (draft && draft.revision === (event?.revision ?? 0)) return { ...base, ...draft.values } as Values } catch { /* A malformed local draft must not prevent setup. */ }
    return base
  })
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: initial })
  const dirty = form.formState.isDirty
  useBlocker({ shouldBlockFn: () => dirty && !window.confirm('Leave this page with unsaved changes? Safe setup fields remain in your draft.'), enableBeforeUnload: dirty })
  useEffect(() => { onDirty?.(dirty); return () => onDirty?.(false) }, [dirty, onDirty])
  useEffect(() => {
    return form.subscribe({ formState: { values: true }, callback: ({ values }) => {
      setReviewed(undefined)
      const safe = { name: values.name, slug: values.slug, timezone: values.timezone, capacity: values.capacity, minimum_attendance: values.minimum_attendance, live_start: values.live_start, live_end: values.live_end, admission_cutoff: values.admission_cutoff, team_freeze: values.team_freeze, submission_cutoff: values.submission_cutoff }
      sessionStorage.setItem(draftKey, JSON.stringify({ revision: event?.revision ?? 0, values: safe }))
    } })
  }, [form, draftKey, event?.revision])
  async function input(values: Values): Promise<components['schemas']['EventInput']> {
    const dates = await Promise.all(dateFields.map(async ([field]) => {
      if (!values[field]) return [field, '0001-01-01T00:00:00Z'] as const
      try {
        const resolved = unwrap(await api.POST('/api/v1/time/resolve', { params: { header: { 'X-CSRF-Token': identity.csrf_token } }, body: { wall_time: values[field].length === 16 ? values[field] + ':00' : values[field], timezone: values.timezone, dst_choice: values.dst_choice } }))
        return [field, resolved.instant] as const
      } catch (error) { if (error instanceof APIError) error.blockers = error.blockers.map(item => ({ ...item, field })); throw error }
    }))
    const times = Object.fromEntries(dates) as Pick<components['schemas']['EventConfiguration'], typeof dateFields[number][0]>
    return { id, name: values.name, slug: values.slug, acknowledge_competition_restriction: values.acknowledge, reason: values.reason, approve_postponement: values.approve_postponement,
      configuration: { description: values.description, venue: values.venue, timezone: values.timezone, capacity: values.capacity, minimum_attendance: values.minimum_attendance, ...times, dst_choice: values.dst_choice,
        calendar: { working_days: values.working_days.map(Number), excluded_dates: values.excluded_dates.split(/[\s,]+/).filter(Boolean) }, schedule: event?.configuration.schedule ?? [] } }
  }
  async function submit(values: Values, previewOnly = false, confirmed = false) {
    setFailure(undefined); setSaved(false)
    try {
      const body = await input(values)
      const planned = unwrap(await api.POST('/api/v1/event-preview', { params: { header: { 'X-CSRF-Token': identity.csrf_token } }, body }))
      setPreview(planned)
      if (previewOnly) return
      if (!event && !values.acknowledge) { form.setError('acknowledge', { message: 'Acknowledge the lasting competition restriction before becoming the organiser.' }); revealPlanningField(root.current,'acknowledge'); return }
      if (!confirmed) { setReviewed(structuredClone(values)); return }
      const savedEvent = event ? unwrap(await api.PUT('/api/v1/events/{event_id}', { params: { path: { event_id: event.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${event.revision}"` } }, body }))
      : unwrap(await api.POST('/api/v1/events', { params: { header: { 'X-CSRF-Token': identity.csrf_token } }, body }))
      setSavedRevision(savedEvent.revision); setReviewed(undefined); form.reset(values); sessionStorage.removeItem(draftKey); setSaved(true)
      await cache.invalidateQueries({ queryKey: ['private', identity.person.id] })
      if (!event) await navigate({ to: '/events/$eventId', params: { eventId: id }, ignoreBlocker: true })
    } catch (error) { setFailure(error) }
  }
  const errorProps = (name: keyof Values) => ({ 'aria-invalid': !!form.formState.errors[name] || undefined, 'aria-describedby': form.formState.errors[name] ? `${name}-error` : undefined })
  const fieldError = (name: keyof Values) => form.formState.errors[name]?.message && <p className="field-error" role="alert" id={`${name}-error`}>{form.formState.errors[name]?.message}</p>
  const invalid = (errors: typeof form.formState.errors) => revealPlanningField(root.current, Object.keys(errors)[0])
  return <form ref={root} className="fleet-planning-form" onSubmit={event => void form.handleSubmit(values => submit(values), invalid)(event)}>
    <PlanningEditorSections label="Event setup sections">
    <PlanningStep title="Event details"><fieldset><legend>1. Event details</legend><div className="form-grid">
      <div><Label htmlFor="name">Event name</Label><Input id="name" {...form.register('name', { onChange: event => { if (!form.getFieldState('slug').isDirty) form.setValue('slug', String(event.target.value).toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '')) } })} {...errorProps('name')} />{fieldError('name')}</div>
      <div><Label htmlFor="slug">Event slug</Label><Input id="slug" {...form.register('slug')} {...errorProps('slug')} />{fieldError('slug')}</div>
      <div className="span-all"><Label htmlFor="description">Description</Label><Textarea id="description" {...form.register('description')} /></div>
      <div><Label htmlFor="venue">Venue</Label><Input id="venue" {...form.register('venue')} /></div>
      <div><Label htmlFor="timezone">Event timezone</Label><Input id="timezone" {...form.register('timezone')} list="timezones" {...errorProps('timezone')} /><datalist id="timezones">{['Europe/London', 'Europe/Paris', 'America/New_York', 'America/Los_Angeles', 'Asia/Singapore', 'Australia/Sydney', 'UTC'].map(zone => <option key={zone}>{zone}</option>)}</datalist>{fieldError('timezone')}</div>
    </div></fieldset></PlanningStep>
    <PlanningStep title="Attendance and dates"><fieldset><legend>2. Attendance and dates</legend><p>Times use the event timezone. Challenge publication is calculated seven calendar days before live start; teams must be frozen before publication.</p><div className="form-grid">
      <div><Label htmlFor="capacity">Event capacity</Label><Input id="capacity" type="number" min={1} {...form.register('capacity', { valueAsNumber: true })} {...errorProps('capacity')} />{fieldError('capacity')}</div>
      <div><Label htmlFor="minimum_attendance">Minimum confirmed competitors</Label><Input id="minimum_attendance" type="number" min={1} {...form.register('minimum_attendance', { valueAsNumber: true })} {...errorProps('minimum_attendance')} />{fieldError('minimum_attendance')}</div>
      {dateFields.map(([name, label]) => <div key={name}><Label htmlFor={name}>{label}</Label><Input id={name} type="datetime-local" step="1" {...form.register(name)} /></div>)}
      <div><Label htmlFor="dst_choice">When a local time occurs twice</Label><Select id="dst_choice" {...form.register('dst_choice')}><option value="">Ask me to choose</option><option value="earlier">Use the earlier occurrence</option><option value="later">Use the later occurrence</option></Select></div>
    </div></fieldset></PlanningStep>
    <PlanningStep title="Business calendar"><fieldset><legend>3. Business calendar</legend><p>Used for business-day deadlines. It does not change the challenge’s seven-calendar-day preparation week.</p><div className="weekdays">{['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'].map((day, index) => <Label className="check" key={day}><Input type="checkbox" value={String(index)} {...form.register('working_days')} {...errorProps('working_days')} />{day}</Label>)}</div>{fieldError('working_days')}<Label htmlFor="excluded_dates">Excluded dates, one YYYY-MM-DD date per line</Label><Textarea id="excluded_dates" {...form.register('excluded_dates')} /></fieldset></PlanningStep>
    {!event && <PlanningStep title="Organiser responsibility"><fieldset><legend>4. Organiser responsibility</legend><Label className="check"><Input type="checkbox" {...form.register('acknowledge')} {...errorProps('acknowledge')} />I understand that becoming this event’s organiser permanently prevents me from competing in it, even if my role is later removed.</Label>{fieldError('acknowledge')}</fieldset></PlanningStep>}
    {event?.published_at && <PlanningStep title="Exceptional postponement"><fieldset><legend>Exceptional postponement</legend><p>The challenge remains available and teams may continue working. A later live start requires explicit approval and a reason.</p><Label htmlFor="postpone-reason">Postponement reason</Label><Textarea id="postpone-reason" {...form.register('reason')} /><Label className="check"><Input type="checkbox" {...form.register('approve_postponement')} />Approve the exceptional postponement and revised deadlines</Label></fieldset></PlanningStep>}
    </PlanningEditorSections>
    <div className="actions"><Button type="button" icon="search" variant="outline" pending={form.formState.isSubmitting} onClick={event => void form.handleSubmit(values => submit(values, true), invalid)(event)}>Preview dates and blockers</Button><Button type="submit" icon="save" pending={form.formState.isSubmitting}>{event ? 'Save event' : 'Save draft event'}</Button></div>
    {preview && <section className="fleet-planning-section" aria-label="Event preview"><h2>Before you proceed</h2><p>Challenge publication: <strong>{preview.challenge_publication ? new Intl.DateTimeFormat('en-GB', { timeZone: form.getValues('timezone'), dateStyle: 'full', timeStyle: 'short' }).format(new Date(preview.challenge_publication)) : 'Set a valid live start'}</strong></p>{preview.blockers.length ? <ul className="blockers">{preview.blockers.map((item, index) => <li key={index}><a href={`#${item.field}`}>{item.message}</a></li>)}</ul> : <p>The date configuration is ready.</p>}</section>}
    {reviewed && <section className="fleet-planning-section" aria-label="Review event configuration"><h2>Review before saving</h2><dl className="dates"><div><dt>Event</dt><dd>{reviewed.name}</dd></div><div><dt>Timezone</dt><dd>{reviewed.timezone}</dd></div><div><dt>Attendance gate</dt><dd>{reviewed.minimum_attendance} confirmed competitors · Capacity {reviewed.capacity}</dd></div><div><dt>Expected revision</dt><dd>{event?.revision ?? 'New draft'}</dd></div></dl><p>Saving retains a draft configuration. Opening registration and the go decision remain separate organiser actions.</p><Button type="button" icon="save" pending={form.formState.isSubmitting} onClick={() => void submit(reviewed, false, true)}>Confirm {event ? 'save event' : 'save draft event'}</Button><Button type="button" variant="ghost" onClick={() => setReviewed(undefined)}>Continue editing</Button></section>}
    {saved && <p role="status" className="notice">Event saved. Revision {savedRevision}.</p>}{!!failure && <Failure error={failure} />}
  </form>
}

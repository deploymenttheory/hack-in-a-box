import { useEffect, useState } from 'react'
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
function wallTime(value?: string, timezone = 'Europe/London') {
  if (!value || value.startsWith('0001-')) return ''
  const parts = new Intl.DateTimeFormat('en-GB', { timeZone: timezone, year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23' }).formatToParts(new Date(value))
  const get = (key: string) => parts.find(p => p.type === key)?.value
  return `${get('year')}-${get('month')}-${get('day')}T${get('hour')}:${get('minute')}:${get('second')}`
}
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
  async function submit(values: Values, previewOnly = false) {
    setFailure(undefined); setSaved(false)
    try {
      const body = await input(values)
      const planned = unwrap(await api.POST('/api/v1/event-preview', { params: { header: { 'X-CSRF-Token': identity.csrf_token } }, body }))
      setPreview(planned)
      if (previewOnly) return
      if (!event && !values.acknowledge) { form.setError('acknowledge', { message: 'Acknowledge the lasting competition restriction before becoming the organiser.' }); return }
      if (event) unwrap(await api.PUT('/api/v1/events/{event_id}', { params: { path: { event_id: event.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${event.revision}"` } }, body }))
      else unwrap(await api.POST('/api/v1/events', { params: { header: { 'X-CSRF-Token': identity.csrf_token } }, body }))
      form.reset(values); sessionStorage.removeItem(draftKey); setSaved(true)
      await cache.invalidateQueries({ queryKey: ['private', identity.person.id] })
      if (!event) await navigate({ to: '/events/$eventId', params: { eventId: id }, ignoreBlocker: true })
    } catch (error) { setFailure(error) }
  }
  const fieldError = (name: keyof Values) => form.formState.errors[name]?.message && <p className="field-error" role="alert" id={`${name}-error`}>{form.formState.errors[name]?.message}</p>
  return <form className="setup-form" onSubmit={form.handleSubmit(values => submit(values))}>
    <fieldset><legend>1. Event details</legend><div className="form-grid">
      <div><label htmlFor="name">Event name</label><input id="name" {...form.register('name', { onChange: event => { if (!form.getFieldState('slug').isDirty) form.setValue('slug', String(event.target.value).toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '')) } })} aria-invalid={!!form.formState.errors.name} />{fieldError('name')}</div>
      <div><label htmlFor="slug">Event slug</label><input id="slug" {...form.register('slug')} aria-invalid={!!form.formState.errors.slug} />{fieldError('slug')}</div>
      <div className="span-all"><label htmlFor="description">Description</label><textarea id="description" {...form.register('description')} /></div>
      <div><label htmlFor="venue">Venue</label><input id="venue" {...form.register('venue')} /></div>
      <div><label htmlFor="timezone">Event timezone</label><input id="timezone" {...form.register('timezone')} list="timezones" /><datalist id="timezones">{['Europe/London', 'Europe/Paris', 'America/New_York', 'America/Los_Angeles', 'Asia/Singapore', 'Australia/Sydney', 'UTC'].map(zone => <option key={zone}>{zone}</option>)}</datalist>{fieldError('timezone')}</div>
    </div></fieldset>
    <fieldset><legend>2. Attendance and dates</legend><p>Times use the event timezone. Challenge publication is calculated seven calendar days before live start; teams must be frozen before publication.</p><div className="form-grid">
      <div><label htmlFor="capacity">Event capacity</label><input id="capacity" type="number" min={1} {...form.register('capacity', { valueAsNumber: true })} />{fieldError('capacity')}</div>
      <div><label htmlFor="minimum_attendance">Minimum confirmed competitors</label><input id="minimum_attendance" type="number" min={1} {...form.register('minimum_attendance', { valueAsNumber: true })} />{fieldError('minimum_attendance')}</div>
      {dateFields.map(([name, label]) => <div key={name}><label htmlFor={name}>{label}</label><input id={name} type="datetime-local" step="1" {...form.register(name)} /></div>)}
      <div><label htmlFor="dst_choice">When a local time occurs twice</label><select id="dst_choice" {...form.register('dst_choice')}><option value="">Ask me to choose</option><option value="earlier">Use the earlier occurrence</option><option value="later">Use the later occurrence</option></select></div>
    </div></fieldset>
    <fieldset><legend>3. Business calendar</legend><p>Used for business-day deadlines. It does not change the challenge’s seven-calendar-day preparation week.</p><div className="weekdays">{['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'].map((day, index) => <label className="check" key={day}><input type="checkbox" value={String(index)} {...form.register('working_days')} />{day}</label>)}</div>{fieldError('working_days')}<label htmlFor="excluded_dates">Excluded dates, one YYYY-MM-DD date per line</label><textarea id="excluded_dates" {...form.register('excluded_dates')} /></fieldset>
    {!event && <fieldset><legend>4. Organiser responsibility</legend><label className="check"><input type="checkbox" {...form.register('acknowledge')} />I understand that becoming this event’s organiser permanently prevents me from competing in it, even if my role is later removed.</label>{fieldError('acknowledge')}</fieldset>}
    {event?.published_at && <fieldset><legend>Exceptional postponement</legend><p>The challenge remains available and teams may continue working. A later live start requires explicit approval and a reason.</p><label htmlFor="postpone-reason">Postponement reason</label><textarea id="postpone-reason" {...form.register('reason')} /><label className="check"><input type="checkbox" {...form.register('approve_postponement')} />Approve the exceptional postponement and revised deadlines</label></fieldset>}
    <div className="actions"><Button type="button" variant="outline" disabled={form.formState.isSubmitting} onClick={form.handleSubmit(values => submit(values, true))}>Preview dates and blockers</Button><Button type="submit" disabled={form.formState.isSubmitting}>{event ? 'Save event' : 'Save draft event'}</Button></div>
    {preview && <section className="panel" aria-label="Event preview"><h2>Before you proceed</h2><p>Challenge publication: <strong>{preview.challenge_publication ? new Intl.DateTimeFormat('en-GB', { timeZone: form.getValues('timezone'), dateStyle: 'full', timeStyle: 'short' }).format(new Date(preview.challenge_publication)) : 'Set a valid live start'}</strong></p>{preview.blockers.length ? <ul className="blockers">{preview.blockers.map((item, index) => <li key={index}><a href={`#${item.field}`}>{item.message}</a></li>)}</ul> : <p>The date configuration is ready.</p>}</section>}
    {saved && <p role="status" className="notice">Event saved.</p>}{!!failure && <Failure error={failure} />}
  </form>
}

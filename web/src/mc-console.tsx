import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, APIError, privateKey, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { localDate } from './event-time'
import { CuePreview, Rehearsal } from './cue-rehearsal'
import { DisplaySessions } from './presentation-pairings'
import { countdown, cueIssue, emptyCue, type ConsolePage, type Cue, type CueCommand } from './presentation-model'

export function MCWorkspace({ event, onDirty }: { event: EventRecord; onDirty: (value: boolean) => void }) {
  const identity = useIdentity(), cache = useQueryClient(), key = privateKey(identity.person.id, event.id, 'presentation')
  const query = useQuery({ queryKey: key, queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/presentation', { params: { path: { event_id: event.id } } })), refetchInterval: 2000, refetchOnWindowFocus: 'always', retry: false })
  if (!query.data) return query.isPending ? <Loading /> : <Failure error={query.error} retry={() => void query.refetch()} />
  return <><MCConsole event={event} blocked={!!query.error} initial={query.data} latest={query.data} onDirty={onDirty} receive={value => cache.setQueryData(key, value)} reload={async () => { const value = unwrap(await api.GET('/api/v1/events/{event_id}/presentation', { params: { path: { event_id: event.id } } })); cache.setQueryData(key, value); return value }} />{query.error && <Failure error={query.error} retry={() => void query.refetch()} />}</>
}
function MCConsole({ event, blocked, initial, latest, onDirty, receive, reload }: { event: EventRecord; blocked: boolean; initial: ConsolePage; latest: ConsolePage; onDirty: (value: boolean) => void; receive: (value: ConsolePage) => void; reload: () => Promise<ConsolePage> }) {
  const identity = useIdentity(), [reviewed, setReviewed] = useState(initial), [draft, setDraft] = useState<Cue[]>(), [selected, setSelected] = useState(0), [reason, setReason] = useState(''), [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>(), [conflict, setConflict] = useState(false), [notice, setNotice] = useState(''), [seconds, setSeconds] = useState(120), [urgent, setUrgent] = useState(emptyCue), [elapsed, setElapsed] = useState(0)
  const stale = conflict || reviewed.revision !== latest.revision, cues = draft ?? reviewed.draft, dirty = draft !== undefined
  const permitted = !blocked && latest.can_control && event.can_present
  const disabled = pending || stale || !permitted || !reason.trim()
  const current = latest.override ?? latest.published[latest.current_index], next = latest.override ? latest.published[Math.max(0, latest.current_index)] : latest.published[latest.current_index + 1]
  const mode = latest.override ? latest.override_mode : latest.mode, remaining = latest.override ? latest.override_remaining_seconds : latest.remaining_seconds
  useEffect(() => { onDirty(dirty); return () => onDirty(false) }, [dirty, onDirty])
  useEffect(() => { const start = performance.now(), timer = window.setInterval(() => setElapsed((performance.now() - start) / 1000), 250); return () => window.clearInterval(timer) }, [latest.server_now])
  async function command(value: Omit<CueCommand, 'reason'>, saved = false) {
    setPending(true); setFailure(undefined); setNotice('')
    try {
      const page = unwrap(await api.POST('/api/v1/events/{event_id}/presentation', { params: { path: { event_id: event.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${reviewed.revision}"` } }, body: { ...value, reason } }))
      receive(page); setReviewed(page); if (saved) setDraft(undefined); setNotice(value.action === 'save' ? 'Rehearsal saved. The room display is unchanged.' : value.action === 'return_to_schedule' ? 'Returned to the planned schedule.' : 'Room control updated.')
    } catch (error) { setFailure(error); if (error instanceof APIError && error.status === 409) { setConflict(true); void reload().catch(() => undefined) } } finally { setPending(false) }
  }
  function accept(page: ConsolePage) { setReviewed(page); receive(page); setConflict(false); setFailure(undefined) }
  return <section className="mc-console"><div className="page-heading"><div><p className="eyebrow">Live event operation</p><h2>MC console</h2><p>Review the room cues, then choose each live change deliberately.</p></div><p>Server snapshot {localDate(latest.server_now, latest.timezone)}</p></div>
    {!permitted && <p role="alert">Room controls are unavailable. Your current role or the event's operating state has changed. Your rehearsal is preserved.</p>}
    {stale && <section role="alert" className="panel"><h3>Another console changed the program</h3><p>Your rehearsal is preserved. Review the latest current cue and saved rehearsal before choosing another action. Nothing will be retried automatically.</p><p>Latest current cue: {current?.title || 'None'}. Latest saved rehearsal: {latest.draft.map(c => c.title).join(' · ') || 'Empty'}.</p><Button disabled={pending} onClick={() => accept(latest)}>Use latest revision and keep my rehearsal</Button></section>}
    <label htmlFor="mc-reason">Reason for room changes</label><input id="mc-reason" value={reason} maxLength={2000} onChange={e => setReason(e.target.value)} placeholder="Why are you changing the room program?" />
    <section className="panel" aria-label="Live room controls"><div className="mc-previews"><CuePreview cue={current} label={latest.override ? 'Urgent live cue' : 'Current live cue'} /><CuePreview cue={next} label={latest.override ? 'Return to planned cue' : 'Next live cue'} /></div><p className="mc-countdown" role="timer" aria-label="Room countdown">{countdown(remaining - (mode === 'running' ? elapsed : 0))}</p><p>{mode === 'held' ? 'Countdown held' : 'Countdown running'} · Reaching zero never advances the program automatically.</p>
      {latest.override && <p className="notice">An urgent announcement is active. The planned cue and remaining time are preserved.</p>}
      <div className="actions"><Button disabled={disabled || !!latest.override || latest.current_index < 1} onClick={() => void command({ action: 'back' })}>Back</Button><Button disabled={disabled || !!latest.override || !latest.published.length || latest.current_index >= latest.published.length} onClick={() => void command({ action: 'advance' })}>{latest.current_index === latest.published.length - 1 ? 'Finish program' : 'Advance'}</Button><Button disabled={disabled || !current || mode === 'held'} onClick={() => void command({ action: 'hold' })}>Hold countdown</Button><Button disabled={disabled || !current || mode === 'running'} onClick={() => void command({ action: 'resume' })}>Resume countdown</Button>{latest.override && <Button disabled={disabled} onClick={() => void command({ action: 'return_to_schedule' })}>Return to schedule</Button>}</div>
      <label htmlFor="mc-timer">Set remaining seconds</label><input id="mc-timer" type="number" min={0} max={86400} step={1} value={seconds} onChange={e => setSeconds(e.target.valueAsNumber)} /><Button variant="outline" disabled={disabled || !current || !Number.isInteger(seconds) || seconds < 0 || seconds > 86400} onClick={() => void command({ action: 'timer', seconds })}>Set countdown</Button>
      {!!latest.published.length && <details><summary>Approved cue list</summary><ol className="cue-list">{latest.published.map((cue, index) => <li key={cue.id}><span>{index + 1}. {cue.title}</span><Button variant="outline" disabled={disabled || !!latest.override} onClick={() => void command({ action: 'activate', index })}>Activate approved cue {index + 1}</Button></li>)}</ol></details>}
    </section>
    <Rehearsal cues={cues} selected={selected} setSelected={setSelected} change={setDraft} disabled={pending || !permitted || stale} saveDisabled={disabled} activateDisabled={disabled || !!latest.override} saved={!dirty && reviewed.revision === latest.revision} save={() => { if (!disabled) void command({ action: 'save', cues }, true) }} activate={() => { if (!disabled && !latest.override) void command({ action: 'activate', index: selected, from_draft: true }) }} />
    {!reason.trim() && <p className="muted">Record a reason to save or activate a rehearsal or change the live room.</p>}{latest.override && <p className="notice">Return to schedule before activating a rehearsal.</p>}
    <section className="panel" aria-label="Urgent announcement"><h3>Urgent announcement</h3><p>The planned sequence and remaining time are preserved until you explicitly return to schedule.</p><fieldset disabled={pending || !permitted}><legend>Public room announcement</legend><label htmlFor="urgent-title">Urgent title</label><input id="urgent-title" value={urgent.title} maxLength={200} onChange={e => setUrgent({ ...urgent, title: e.target.value })} /><label htmlFor="urgent-text">Urgent room text</label><textarea id="urgent-text" value={urgent.text} maxLength={4000} onChange={e => setUrgent({ ...urgent, text: e.target.value })} /><label htmlFor="urgent-seconds">Urgent countdown in seconds</label><input id="urgent-seconds" type="number" min={0} max={86400} step={1} value={urgent.duration_seconds} onChange={e => setUrgent({ ...urgent, duration_seconds: e.target.valueAsNumber })} /></fieldset><Button disabled={disabled || !!cueIssue(urgent)} onClick={() => void command({ action: 'override', override: urgent })}>{latest.override ? 'Replace urgent announcement' : 'Activate urgent announcement'}</Button></section>
    <DisplaySessions event={event} page={latest} revision={reviewed.revision} reason={reason} disabled={disabled} receive={accept} reload={reload} onConflict={() => setConflict(true)} />
    {pending && <p role="status">Updating room controls…</p>}{notice && <p role="status">{notice}</p>}{!!failure && <Failure error={failure} />}
  </section>
}

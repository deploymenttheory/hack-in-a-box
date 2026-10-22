import { useState } from 'react'
import { api, APIError, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure } from './components/states'
import { localDate } from './event-time'
import type { ConsolePage } from './presentation-model'

export function DisplaySessions({ event, page, revision, reason, disabled, receive, reload, onConflict }: { event: EventRecord; page: ConsolePage; revision: number; reason: string; disabled: boolean; receive: (page: ConsolePage) => void; reload: () => Promise<ConsolePage>; onConflict: () => void }) {
  const identity = useIdentity(), [label, setLabel] = useState(''), [minutes, setMinutes] = useState(120), [secret, setSecret] = useState(''), [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>()
  const params = { path: { event_id: event.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${revision}"` } }
  async function change(id?: string) {
    setPending(true); setFailure(undefined)
    try {
      if (id) receive(unwrap(await api.POST('/api/v1/events/{event_id}/presentation/pairings/{pairing_id}/revoke', { params: { ...params, path: { ...params.path, pairing_id: id } }, body: { reason } })))
      else { const paired = unwrap(await api.POST('/api/v1/events/{event_id}/presentation/pairings', { params, body: { label, minutes, reason } })); setSecret(paired.token); setLabel(''); const fresh = await reload(); if (fresh.revision === paired.revision) receive(fresh); else onConflict() }
    } catch (error) { setFailure(error); if (error instanceof APIError && error.status === 409) { onConflict(); void reload().catch(() => undefined) } } finally { setPending(false) }
  }
  return <section className="panel" aria-label="Display sessions"><h3>Display sessions</h3><p>Recent contact records successful authorized snapshots. It does not confirm a screen remains connected or has rendered the cue.</p>
    <ul className="item-list">{page.pairings.map(pairing => <li key={pairing.id}><div><strong>{pairing.label}</strong><p>{pairing.revoked_at ? 'Revoked' : Date.parse(pairing.expires_at) <= Date.parse(page.server_now) ? 'Expired' : pairing.last_seen_at ? pairing.stale ? 'Contact stale' : 'Recent contact' : 'No contact yet'}</p><p>Last seen: {pairing.last_seen_at ? localDate(pairing.last_seen_at, page.timezone) : 'Never'} · Expires {localDate(pairing.expires_at, page.timezone)}</p></div>{!pairing.revoked_at && <Button variant="outline" disabled={disabled || pending} onClick={() => void change(pairing.id)}>Revoke {pairing.label}</Button>}</li>)}</ul>{!page.pairings.length && <p>No displays paired yet.</p>}
    <fieldset disabled={pending || disabled}><legend>Pair an independent room display</legend><label htmlFor="display-label">Display label</label><input id="display-label" value={label} maxLength={100} onChange={e => setLabel(e.target.value)} /><label htmlFor="display-lifetime">Display lifetime in minutes</label><input id="display-lifetime" type="number" min={5} max={1440} step={1} value={minutes} onChange={e => setMinutes(e.target.valueAsNumber)} /><Button disabled={!label.trim() || !Number.isInteger(minutes) || minutes < 5 || minutes > 1440} onClick={() => void change()}>Pair display</Button></fieldset>
    {secret && <section className="notice" aria-label="New display secret"><h4>Display secret — shown once</h4><p>Enter this secret in the room display. Keep it out of links and screenshots. It disappears when you leave this console.</p><label htmlFor="display-secret">New display secret</label><input id="display-secret" readOnly value={secret} autoComplete="off" spellCheck={false} /><Button variant="outline" onClick={() => setSecret('')}>Dismiss display secret</Button></section>}
    {!!failure && <Failure error={failure} />}
  </section>
}

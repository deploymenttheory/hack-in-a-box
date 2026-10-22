import { useCallback, useEffect, useRef, useState } from 'react'
import { api, APIError, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { Failure } from './components/states'
import { Button } from './components/ui/button'
import { localDate, wallTime } from './event-time'
import { resourceTypes, type Resource, type ResourceCommand, type InventoryTeam } from './resource-inventory-model'

type Save = (command: ResourceCommand) => Promise<boolean>
type LocalTime = { wall: string; dst: '' | 'earlier' | 'later' }
export const providerFollowUp = 'Changing app access does not revoke the provider credential or erase copies already revealed. Revoke or rotate the credential at the provider separately when needed.'

// Actual secret text lives only in the password element and the in-flight POST.
// It never enters React state, query/mutation caches or persisted form drafts.
function useSecretInput() {
  const input = useRef<HTMLInputElement>(null), [hasValue, setHasValue] = useState(false)
  const clear = useCallback(() => { if (input.current) input.current.value = ''; setHasValue(false) }, [])
  useEffect(() => {
    const element = input.current
    const hidden = () => { if (document.hidden) clear() }
    window.addEventListener('pagehide', clear); document.addEventListener('visibilitychange', hidden)
    return () => { window.removeEventListener('pagehide', clear); document.removeEventListener('visibilitychange', hidden); if (element) element.value = '' }
  }, [clear])
  return { input, clear, hasValue, changed: () => setHasValue(!!input.current?.value) }
}
function LocalTimeField({ label, id, value, timezone, change }: { label: string; id: string; value: LocalTime; timezone: string; change: (value: LocalTime) => void }) {
  return <div><label htmlFor={id}>{label} ({timezone}, optional)</label><input id={id} type="datetime-local" step="1" value={value.wall} onChange={e => change({ ...value, wall: e.target.value })} /><label htmlFor={`${id}-dst`}>Repeated {label.toLowerCase()} time</label><select id={`${id}-dst`} value={value.dst} onChange={e => change({ ...value, dst: e.target.value as LocalTime['dst'] })}><option value="">Not ambiguous</option><option value="earlier">Earlier occurrence</option><option value="later">Later occurrence</option></select></div>
}
async function resolveTime(value: LocalTime, timezone: string, csrf: string) {
  if (!value.wall) return null
  return unwrap(await api.POST('/api/v1/time/resolve', { params: { header: { 'X-CSRF-Token': csrf } }, body: { timezone, wall_time: value.wall.length === 16 ? value.wall + ':00' : value.wall, dst_choice: value.dst } })).instant
}
export function ResourceEditor({ event, resource, mode, disabled, stale, save, cancel }: { event: EventRecord; resource?: Resource; mode: 'create' | 'metadata' | 'replace'; disabled: boolean; stale: boolean; save: Save; cancel: () => void }) {
  const identity = useIdentity()
  const { input: secretInput, clear: clearSecret, hasValue: hasSecret, changed: secretChanged } = useSecretInput()
  const [label, setLabel] = useState(resource?.label ?? ''), [kind, setKind] = useState<Resource['resource_type']>(resource?.resource_type ?? 'credential')
  const [available, setAvailable] = useState<LocalTime>({ wall: wallTime(resource?.available_at ?? undefined, event.configuration.timezone), dst: '' }), [expires, setExpires] = useState<LocalTime>({ wall: wallTime(resource?.expires_at ?? undefined, event.configuration.timezone), dst: '' })
  const [acknowledged, setAcknowledged] = useState(false), [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>(), [cleared, setCleared] = useState(false)
  const needsSecret = mode !== 'metadata'
  async function submit() {
    setPending(true); setFailure(undefined); setCleared(false)
    try {
      const metadata = mode === 'replace' ? {} : { resource_type: kind, available_at: await resolveTime(available, event.configuration.timezone, identity.csrf_token), expires_at: await resolveTime(expires, event.configuration.timezone, identity.csrf_token) }
      if (metadata.available_at && metadata.expires_at && new Date(metadata.expires_at) <= new Date(metadata.available_at)) throw new Error('Expiry must be after availability.')
      const value = needsSecret ? secretInput.current?.value ?? '' : ''
      clearSecret()
      if (needsSecret && !value) throw new Error('The secret field was cleared. Re-enter the new value before saving.')
      if (!await save({ action: mode, ...(resource ? { id: resource.id } : {}), label: mode === 'replace' ? resource!.label : label.trim(), ...metadata, ...(needsSecret ? { value } : {}) })) setCleared(needsSecret)
    } catch (error) { clearSecret(); setFailure(error); setCleared(needsSecret) } finally { setPending(false) }
  }
  return <form className="panel section-heading credential-editor" aria-label={`${mode === 'create' ? 'New' : mode === 'replace' ? 'Replacement' : 'Edit'} resource`} autoComplete="off" onSubmit={e => { e.preventDefault(); void submit() }}>
    <h3>{mode === 'create' ? 'Add a supplied resource' : mode === 'replace' ? `Replace ${resource!.label}` : `Edit ${resource!.label}`}</h3>
    <fieldset disabled={disabled || stale || pending}>
      {mode !== 'replace' && <><label htmlFor="resource-label">Resource label</label><input id="resource-label" maxLength={120} required value={label} onChange={e => setLabel(e.target.value)} /><label htmlFor="resource-type">Resource type</label><select id="resource-type" value={kind} onChange={e => setKind(e.target.value as Resource['resource_type'])}>{Object.entries(resourceTypes).map(([value, text]) => <option key={value} value={value}>{text}</option>)}</select><div className="form-grid"><LocalTimeField label="Available from" id="resource-available" value={available} timezone={event.configuration.timezone} change={setAvailable} /><LocalTimeField label="Expiry" id="resource-expiry" value={expires} timezone={event.configuration.timezone} change={setExpires} /></div><p>Empty availability means immediately. Empty expiry means no recorded expiry. Expiry gates app access; it does not change the provider’s expiry. Resolve repeated local times using the earlier or later occurrence.</p></>}
      {needsSecret && <><label htmlFor="resource-secret">{mode === 'replace' ? 'New secret value' : 'Secret value'}</label><input id="resource-secret" ref={secretInput} type="password" autoComplete="new-password" required maxLength={16384} onChange={() => secretChanged()} aria-describedby="resource-secret-help" /><p id="resource-secret-help">Write-only: the saved value is never loaded into this form. It is cleared on submission, when leaving this page or when the browser tab is hidden. Secret text is excluded from form drafts.</p></>}
      {mode === 'create' && <p>The resource starts unassigned. Save it, then explicitly review its team audience.</p>}
      {mode === 'replace' && <><p className="notice">{providerFollowUp}</p><p>Replacement preserves the existing teams, availability, expiry and revoked state. The old value is not shown.</p><label className="check"><input type="checkbox" checked={acknowledged} onChange={e => setAcknowledged(e.target.checked)} />I understand the provider follow-up</label></>}
      <Button type="submit" disabled={!label.trim() || (needsSecret && !hasSecret) || (mode === 'replace' && !acknowledged)}>{mode === 'create' ? 'Save new resource' : mode === 'replace' ? 'Save replacement value' : 'Save resource metadata'}</Button>
    </fieldset>
    <Button type="button" variant="outline" disabled={pending} onClick={() => { clearSecret(); cancel() }}>Cancel resource edit</Button>
    {stale && <p role="alert">The inventory changed while editing. Cancel and review the current inventory before saving again.</p>}
    {cleared && <p role="status">The new secret field was cleared. Re-enter it to retry; no secret draft was stored.</p>}
    {!!failure && (failure instanceof APIError ? <Failure error={failure} /> : <p role="alert">{failure instanceof Error ? failure.message : 'Check the resource fields and try again.'}</p>)}
  </form>
}
export function ResourceAssignment({ resource, teams, disabled, stale, save, cancel }: { resource: Resource; teams: InventoryTeam[]; disabled: boolean; stale: boolean; save: Save; cancel: () => void }) {
  const [selected, setSelected] = useState(resource.team_ids), [review, setReview] = useState(false), [pending, setPending] = useState(false)
  const active = teams.filter(t => t.state !== 'disbanded'), unknown = selected.filter(id => !active.some(t => t.id === id))
  const name = (id: string) => teams.find(t => t.id === id)?.name ?? `Inactive team ${id.slice(0, 8)}`
  async function submit() { setPending(true); try { await save({ action: 'assign', id: resource.id, team_ids: selected }) } finally { setPending(false) } }
  return <section className="panel section-heading" aria-label="Resource team assignment"><h3>Assign {resource.label}</h3><p>Team assignments change access immediately when availability, published membership and confirmation permit. Competitor teams are not moved.</p><fieldset disabled={disabled || stale || pending}><legend>Teams allowed to reveal this resource</legend>{active.map(t => <label className="check" key={t.id}><input type="checkbox" checked={selected.includes(t.id)} onChange={e => { setSelected(e.target.checked ? [...selected, t.id] : selected.filter(id => id !== t.id)); setReview(false) }} />{t.name} · {t.id.slice(0, 8)} · {t.state}</label>)}{unknown.map(id => <label className="check" key={id}><input type="checkbox" checked onChange={() => { setSelected(selected.filter(value => value !== id)); setReview(false) }} />Remove inactive assignment: {name(id)}</label>)}{!active.length && <p>No active teams are available. An empty audience leaves the resource unassigned.</p>}<Button variant="outline" disabled={unknown.length > 0} onClick={() => setReview(true)}>Preview resource assignment</Button>{review && <section aria-label="Resource assignment preview"><h4>Review team access</h4><p>Before: {resource.team_ids.map(name).join(', ') || 'Unassigned'}</p><p>After: {selected.map(name).join(', ') || 'Unassigned'}</p><p>{providerFollowUp}</p><Button onClick={() => void submit()}>Confirm resource assignment</Button></section>}</fieldset><Button variant="outline" disabled={pending} onClick={cancel}>Cancel resource assignment</Button>{stale && <p role="alert">The inventory changed. Cancel and review the latest team assignments.</p>}</section>
}
export function ResourceRevocation({ resource, timezone, disabled, stale, save, cancel }: { resource: Resource; timezone: string; disabled: boolean; stale: boolean; save: Save; cancel: () => void }) {
  const [acknowledged, setAcknowledged] = useState(false), [pending, setPending] = useState(false)
  async function submit() { setPending(true); try { await save({ action: 'revoke', id: resource.id }) } finally { setPending(false) } }
  return <section className="panel section-heading" aria-label="Resource revocation review"><h3>Revoke app access to {resource.label}</h3><p>{providerFollowUp}</p><p>This immediately blocks future app reveals. No provider action is performed. Resource expiry: {resource.expires_at ? localDate(resource.expires_at, timezone) : 'Not recorded'}.</p><label className="check"><input type="checkbox" disabled={disabled || stale || pending} checked={acknowledged} onChange={e => setAcknowledged(e.target.checked)} />I understand the provider follow-up</label><div className="actions"><Button disabled={disabled || stale || pending || !acknowledged} onClick={() => void submit()}>Revoke app access</Button><Button variant="outline" disabled={pending} onClick={cancel}>Cancel revocation</Button></div>{stale && <p role="alert">The inventory changed. Cancel and review the resource again.</p>}</section>
}

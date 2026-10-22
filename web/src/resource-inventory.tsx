import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, privateKey, unwrap, type EventRecord } from './api/client'
import { useIdentity } from './session'
import { Failure, Loading } from './components/states'
import { Button } from './components/ui/button'
import { localDate } from './event-time'
import { useCurrentTime } from './use-current-time'
import { ResourceEditor, ResourceAssignment, ResourceRevocation } from './resource-inventory-editor'
import { resourceTypes, readinessLabels, type Inventory, type Resource, type ResourceCommand } from './resource-inventory-model'

type Edit = { mode: 'create'; revision: number } | { mode: 'metadata' | 'replace' | 'assign' | 'revoke'; revision: number; resource: Resource }
export function ResourceInventory({ event, onDirty }: { event: EventRecord; onDirty: (dirty: boolean) => void }) {
  const identity = useIdentity()
  const query = useQuery({ queryKey: privateKey(identity.person.id, event.id, 'credential-inventory'), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/credentials', { params: { path: { event_id: event.id } } })), refetchOnWindowFocus: 'always', refetchOnMount: 'always', refetchInterval: 15_000 })
  if (query.isPending) return <Loading />
  if (query.error) return <Failure error={query.error} retry={() => void query.refetch()} />
  if (!query.data.can_manage) return <p role="alert">Current organiser authority is required to manage the resource inventory.</p>
  return <InventoryControls event={event} inventory={query.data} onDirty={onDirty} refresh={() => void query.refetch()} />
}
function InventoryControls({ event, inventory, onDirty, refresh }: { event: EventRecord; inventory: Inventory; onDirty: (dirty: boolean) => void; refresh: () => void }) {
  const identity = useIdentity(), cache = useQueryClient(), now = useCurrentTime()
  const [edit, setEdit] = useState<Edit>(), [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState('')
  const teams = useQuery({ queryKey: privateKey(identity.person.id, event.id, 'teams'), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/teams', { params: { path: { event_id: event.id } } })), refetchOnWindowFocus: 'always' })
  useEffect(() => { onDirty(!!edit); return () => onDirty(false) }, [edit, onDirty])
  const teamList = teams.data?.teams ?? [], activeTeams = teamList.filter(t => t.state !== 'disbanded')
  const teamName = (id: string) => teamList.find(t => t.id === id)?.name ?? `Unknown team ${id.slice(0, 8)}`
  const missing = activeTeams.filter(t => !inventory.items.some(r => r.active && r.team_ids.includes(t.id) && (!r.expires_at || new Date(r.expires_at).getTime() > now)))
  const disabled = pending || !event.can_edit, stale = !!edit && edit.revision !== inventory.revision
  function begin(mode: Exclude<Edit['mode'], 'create'>, resource: Resource) { setFailure(undefined); setNotice(''); setEdit({ mode, resource, revision: inventory.revision }) }
  async function save(command: ResourceCommand) {
    if (!edit || stale) return false
    setPending(true); setFailure(undefined); setNotice('')
    try {
      // Plain async POST: a mutation cache must never retain secret variables.
      const updated = unwrap(await api.POST('/api/v1/events/{event_id}/credentials', { params: { path: { event_id: event.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${edit.revision}"` } }, body: command }))
      cache.setQueryData(privateKey(identity.person.id, event.id, 'credential-inventory'), updated)
      setEdit(undefined); setNotice(command.action === 'create' ? 'Resource stored without displaying its value. Review its team assignment next.' : command.action === 'replace' ? 'Replacement value stored. Review the provider-side follow-up separately.' : command.action === 'revoke' ? 'App disclosure access revoked. Provider credentials and previously revealed copies remain outside this app’s control.' : command.action === 'assign' ? 'Resource audience updated. Current confirmed published-team membership is checked at every reveal.' : 'Resource metadata updated. Availability and expiry are enforced by the server.')
      await cache.invalidateQueries({ queryKey: ['private', identity.person.id, event.id] })
      return true
    } catch (error) { setFailure(error); await cache.invalidateQueries({ queryKey: ['private', identity.person.id, event.id] }); return false } finally { setPending(false) }
  }
  return <section className="section-heading credential-inventory"><div className="preparation-heading"><div><p className="eyebrow">Organiser administration</p><h2>Resource inventory</h2><p>Prepare supplied credentials and assign teams without displaying existing values. Labels, types and timing are safe metadata; never put a secret in those fields.</p></div><div className="actions"><Button variant="outline" onClick={() => { refresh(); void teams.refetch() }}>Refresh inventory</Button><Button disabled={disabled || !!edit} onClick={() => { setFailure(undefined); setNotice(''); setEdit({ mode: 'create', revision: inventory.revision }) }}>Add resource</Button></div></div>
    {!event.can_edit && <p className="notice">This event is read-only. Inventory remains visible; resource changes are unavailable.</p>}
    <p>Readiness reflects assignment, availability, expiry and revocation. A reveal still requires current confirmed membership of a published active team. This inventory has no reveal or export action.</p>
    <div className="detail-grid"><section className="panel"><h3>Unassigned resources</h3><p>{inventory.items.filter(r => r.active && r.team_ids.length === 0).length} active resources have no team audience.</p></section><section className="panel"><h3>Teams missing an active resource</h3>{teams.isPending ? <Loading /> : teams.error ? <Failure error={teams.error} retry={() => void teams.refetch()} /> : missing.length ? <ul>{missing.map(t => <li key={t.id}>{t.name} · {t.id.slice(0, 8)}</li>)}</ul> : <p>Every active team has at least one assigned resource that has not expired.</p>}<p className="muted">Scheduled resources count as prepared. Check each resource’s availability before the event.</p></section></div>
    <div className="table-scroll" role="region" tabIndex={0} aria-label="Masked resource inventory"><table><caption>Values are always masked; mask length does not reveal the value length.</caption><thead><tr><th scope="col">Resource</th><th scope="col">Assigned teams</th><th scope="col">Readiness</th><th scope="col">Availability and expiry</th><th scope="col">Actions</th></tr></thead><tbody>{inventory.items.map(r => <tr key={r.id}><th scope="row"><strong>{r.label}</strong><p>{resourceTypes[r.resource_type]} · version {r.version}</p><span aria-label="Stored secret masked">••••••••</span><p className="muted">Reference {r.id.slice(0, 8)}</p></th><td>{r.team_ids.length ? r.team_ids.map(teamName).join(', ') : 'Unassigned'}</td><td><span className="badge">{readinessLabels[r.readiness]}</span>{r.active && r.expires_at && new Date(r.expires_at).getTime() <= now && r.readiness !== 'expired' && <p>Expiry reached; refresh current server status.</p>}</td><td><p>From: {r.available_at ? localDate(r.available_at, event.configuration.timezone) : 'Immediately'}</p><p>Expiry: {r.expires_at ? localDate(r.expires_at, event.configuration.timezone) : 'Not recorded'}</p><p className="muted">{event.configuration.timezone}</p></td><td><div className="actions"><Button variant="outline" disabled={disabled || !!edit} onClick={() => begin('metadata', r)}>Edit metadata for {r.label}</Button><Button variant="outline" disabled={disabled || !!edit || !teams.isSuccess} onClick={() => begin('assign', r)}>Assign {r.label}</Button><Button variant="outline" disabled={disabled || !!edit} onClick={() => begin('replace', r)}>Replace {r.label}</Button><Button variant="outline" disabled={disabled || !!edit || !r.active} onClick={() => begin('revoke', r)}>Revoke {r.label}</Button></div></td></tr>)}</tbody></table></div>
    {!inventory.items.length && <p className="empty-state">No resources have been stored. Add a supplied credential, then review its team audience.</p>}
    {edit && (edit.mode === 'assign' ? <ResourceAssignment key={`${edit.resource.id}-assign`} resource={edit.resource} teams={teamList} disabled={disabled || !teams.isSuccess} stale={stale} save={save} cancel={() => setEdit(undefined)} /> : edit.mode === 'revoke' ? <ResourceRevocation key={`${edit.resource.id}-revoke`} resource={edit.resource} timezone={event.configuration.timezone} disabled={disabled} stale={stale} save={save} cancel={() => setEdit(undefined)} /> : <ResourceEditor key={edit.mode === 'create' ? 'create' : `${edit.resource.id}-${edit.mode}`} event={event} resource={edit.mode === 'create' ? undefined : edit.resource} mode={edit.mode} disabled={disabled} stale={stale} save={save} cancel={() => setEdit(undefined)} />)}
    {notice && <p role="status">{notice}</p>}{!!failure && <Failure error={failure} />}
  </section>
}

import { useBlocker } from '@tanstack/react-router'
import { usePrivateRequestScope } from './private-request-scope'
import { usePlanningDepartureBarrier } from './planning-operations'
import { usePlanningActionFocus } from './planning-layout'
import { CollectionRecords, DetailWorkspace, Inspector } from './components/patterns'
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
  const identity = useIdentity(), cache = useQueryClient(), now = useCurrentTime(), actionFocus=usePlanningActionFocus()
  const scope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${event.id}:inventory`)
  const [childPending, setChildPending] = useState(false)
  const [edit, setEdit] = useState<Edit>(), [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState('')
  const teams = useQuery({ queryKey: privateKey(identity.person.id, event.id, 'teams'), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/teams', { params: { path: { event_id: event.id } } })), refetchOnWindowFocus: 'always' })
  useEffect(() => { onDirty(!!edit); return () => onDirty(false) }, [edit, onDirty])
  const busy = pending || childPending
  usePlanningDepartureBarrier(busy)
  useBlocker({ shouldBlockFn: () => busy, enableBeforeUnload: busy })
  const teamList = teams.data?.teams ?? [], activeTeams = teamList.filter(t => t.state !== 'disbanded')
  const teamName = (id: string) => teamList.find(t => t.id === id)?.name ?? `Unknown team ${id.slice(0, 8)}`
  const missing = activeTeams.filter(t => !inventory.items.some(r => r.active && r.team_ids.includes(t.id) && (!r.expires_at || new Date(r.expires_at).getTime() > now)))
  const disabled = busy || !event.can_edit, stale = !!edit && edit.revision !== inventory.revision
  function closeEditor(){if(!edit||busy)return;const target=edit.mode==='create'?'create':edit.resource.id+'-'+edit.mode;setEdit(undefined);actionFocus.restore(target)}
  function begin(mode: Exclude<Edit['mode'], 'create'>, resource: Resource) { if(edit||busy)return;setFailure(undefined); setNotice(''); setEdit({ mode, resource, revision: inventory.revision }) }
  async function save(command: ResourceCommand) {
    if (!edit || stale || pending) return false
    const ticket = scope.capture(); if (!ticket.current()) return false
    setPending(true); setFailure(undefined); setNotice('')
    try {
      // Plain async POST: a mutation cache must never retain secret variables.
      const updated = unwrap(await api.POST('/api/v1/events/{event_id}/credentials', { params: { path: { event_id: event.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${edit.revision}"` } }, body: command }))
      ticket.assert()
      cache.setQueryData(privateKey(identity.person.id, event.id, 'credential-inventory'), updated)
      setEdit(undefined); setNotice(command.action === 'create' ? 'Resource stored without displaying its value. Review its team assignment next.' : command.action === 'replace' ? 'Replacement value stored. Review the provider-side follow-up separately.' : command.action === 'revoke' ? 'App disclosure access revoked. Provider credentials and previously revealed copies remain outside this app’s control.' : command.action === 'assign' ? 'Resource audience updated. Current confirmed published-team membership is checked at every reveal.' : 'Resource metadata updated. Availability and expiry are enforced by the server.')
      await cache.invalidateQueries({ queryKey: ['private', identity.person.id, event.id] })
      return true
    } catch (error) { if (ticket.current()) { setFailure(error); await cache.invalidateQueries({ queryKey: ['private', identity.person.id, event.id] }) } return false } finally { if (ticket.current()) setPending(false) }
  }
  return <section id="inventory" className="section-heading credential-inventory"><div className="preparation-heading"><div><p className="eyebrow">Organiser administration</p><h2>Resource inventory</h2><p>Prepare supplied credentials and assign teams without displaying existing values. Labels, types and timing are safe metadata; never put a secret in those fields.</p></div><div className="actions"><Button variant="outline" onClick={() => { refresh(); void teams.refetch() }}>Refresh inventory</Button><Button ref={actionFocus.bind('create')} data-record-focus="create" disabled={disabled || !!edit} onClick={() => { setFailure(undefined); setNotice(''); setEdit({ mode: 'create', revision: inventory.revision }) }}>Add resource</Button></div></div>
    {!event.can_edit && <p className="notice">This event is read-only. Inventory remains visible; resource changes are unavailable.</p>}
    <p>Readiness reflects assignment, availability, expiry and revocation. A reveal still requires current confirmed membership of a published active team. This inventory has no reveal or export action.</p>
    <div className="fleet-planning-summary"><section className="fleet-planning-section"><h3>Unassigned resources</h3><p>{inventory.items.filter(r => r.active && r.team_ids.length === 0).length} active resources have no team audience.</p></section><section className="fleet-planning-section"><h3>Teams missing an active resource</h3>{teams.isPending ? <Loading /> : teams.error ? <Failure error={teams.error} retry={() => void teams.refetch()} /> : missing.length ? <ul>{missing.map(t => <li key={t.id}>{t.name} · {t.id.slice(0, 8)}</li>)}</ul> : <p>Every active team has at least one assigned resource that has not expired.</p>}<p className="muted">Scheduled resources count as prepared. Check each resource’s availability before the event.</p></section></div>
    <DetailWorkspace collection={<CollectionRecords label="Masked resource inventory" rows={inventory.items} rowKey={r=>r.id} rowLabel={r=>r.label} columns={[{id:'masked',label:'Stored value',essential:true,cell:r=><><span aria-label="Stored secret masked">••••••••</span><p>{resourceTypes[r.resource_type]} · version {r.version}</p><small>Reference {r.id.slice(0,8)}</small></>},{id:'teams',label:'Assigned teams',essential:true,cell:r=>r.team_ids.length?r.team_ids.map(teamName).join(', '):'Unassigned'},{id:'readiness',label:'Readiness',essential:true,cell:r=><><span className="badge">{readinessLabels[r.readiness]}</span>{r.active&&r.expires_at&&new Date(r.expires_at).getTime()<=now&&r.readiness!=='expired'&&<p>Expiry reached; refresh current server status.</p>}</>},{id:'timing',label:'Availability and expiry',cell:r=><><p>From: {r.available_at?localDate(r.available_at,event.configuration.timezone):'Immediately'}</p><p>Expiry: {r.expires_at?localDate(r.expires_at,event.configuration.timezone):'Not recorded'}</p><small>{event.configuration.timezone}</small></>}]} actions={r=><div className="fleet-pattern-actions"><Button ref={actionFocus.bind(r.id+'-metadata')} data-record-focus={r.id+'-metadata'} variant="ghost" disabled={disabled||!!edit} onClick={()=>begin('metadata',r)}>Edit metadata for {r.label}</Button><Button ref={actionFocus.bind(r.id+'-assign')} data-record-focus={r.id+'-assign'} variant="ghost" disabled={disabled||!!edit||!teams.isSuccess} onClick={()=>begin('assign',r)}>Assign {r.label}</Button><Button ref={actionFocus.bind(r.id+'-replace')} data-record-focus={r.id+'-replace'} variant="ghost" disabled={disabled||!!edit} onClick={()=>begin('replace',r)}>Replace {r.label}</Button><Button ref={actionFocus.bind(r.id+'-revoke')} data-record-focus={r.id+'-revoke'} variant="ghost" disabled={disabled||!!edit||!r.active} onClick={()=>begin('revoke',r)}>Revoke {r.label}</Button></div>} />} detail={edit && <Inspector identity={edit.mode==='create'?'create':edit.resource.id+'-'+edit.mode} title={edit.mode==='create'?'New supplied resource':edit.resource.label} scope={{event:event.name,entity:'Private resource configuration'}} back={{label:'Back to resource inventory',href:'#inventory',onNavigate:closeEditor}}>{edit.mode === 'assign' ? <ResourceAssignment key={`${edit.resource.id}-assign`} resource={edit.resource} teams={teamList} disabled={disabled || !teams.isSuccess} stale={stale} save={save} cancel={closeEditor} /> : edit.mode === 'revoke' ? <ResourceRevocation key={`${edit.resource.id}-revoke`} resource={edit.resource} timezone={event.configuration.timezone} disabled={disabled} stale={stale} save={save} cancel={closeEditor} /> : <ResourceEditor key={edit.mode === 'create' ? 'create' : `${edit.resource.id}-${edit.mode}`} event={event} resource={edit.mode === 'create' ? undefined : edit.resource} mode={edit.mode} disabled={disabled} stale={stale} save={save} cancel={closeEditor} onPending={setChildPending} />}</Inspector>} />
    {!inventory.items.length && <p className="empty-state">No resources have been stored. Add a supplied credential, then review its team audience.</p>}

    {notice && <p role="status">{notice}</p>}{!!failure && <Failure error={failure} />}
  </section>
}

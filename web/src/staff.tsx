import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, privateKey, unwrap, type EventRecord } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
type Role = components['schemas']['StaffRole']
const descriptions: Record<Role, string> = {
  organiser: 'Manages this event and its staff. Permanently ineligible to compete in this event.',
  mentor: 'Reviews assigned teams and approved assessment contexts. Cannot compete in the same event.',
  judge: 'Receives assigned judging data. Cannot compete in the same event.',
  mc: 'Receives presentation-safe information for the live event.',
  logistics: 'Receives the operational details needed to support attendees.',
  photographer: 'Receives consent information and media tasks.',
  early_brief_access: 'Can access the unpublished challenge. Permanently ineligible to compete in this event, even without opening it.',
}
export function Staff({ event }: { event: EventRecord }) {
  const identity = useIdentity(); const cache = useQueryClient()
  const [person, setPerson] = useState(''); const [role, setRole] = useState<Role>('mentor'); const [acknowledge, setAcknowledge] = useState(false)
  const [pending, setPending] = useState(false); const [failure, setFailure] = useState<unknown>(); const [success, setSuccess] = useState('')
  const restricted = role === 'organiser' || role === 'early_brief_access'
  const key = privateKey(identity.person.id, event.id, 'staff')
  const query = useQuery({ queryKey: key, queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/staff', { params: { path: { event_id: event.id } } })), refetchInterval: 15_000 })
  const history = useQuery({ queryKey: privateKey(identity.person.id, event.id, 'staff-audit'), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/audit', { params: { path: { event_id: event.id }, query: { limit: 100 } } })) })
  async function change(personId: string, selectedRole: Role, active: boolean, owner = false) {
    setPending(true); setFailure(undefined); setSuccess('')
    try {
      const params = { path: { event_id: event.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${event.revision}"` } }
      if (owner) unwrap(await api.PUT('/api/v1/events/{event_id}/owner', { params, body: { person_id: personId, acknowledge_competition_restriction: acknowledge } }))
      else unwrap(await api.PUT('/api/v1/events/{event_id}/staff', { params, body: { person_id: personId, role: selectedRole, active, acknowledge_competition_restriction: acknowledge } }))
      setSuccess(owner ? 'Ownership transferred. Existing organiser grants remain until explicitly revoked.' : active ? 'Role assigned.' : 'Role revoked. Any lasting competition restriction remains.')
      await cache.invalidateQueries({ queryKey: ['private', identity.person.id, event.id] }); setAcknowledge(false)
    } catch (error) { setFailure(error) } finally { setPending(false) }
  }
  return <div className="detail-grid"><section className="panel"><h2>{event.can_edit ? 'Assign event staff' : 'Event staff'}</h2><p>Ask the person for the account ID shown on their account page. They must have signed in at least once.</p>
    <fieldset disabled={!event.can_edit}><form onSubmit={e => { e.preventDefault(); void change(person, role, true) }}><label htmlFor="staff-person">Person account ID</label><input id="staff-person" value={person} onChange={e => setPerson(e.target.value)} required pattern="[a-fA-F0-9-]{36}" />
      <label htmlFor="staff-role">Role</label><select id="staff-role" value={role} onChange={e => { setRole(e.target.value as Role); setAcknowledge(false) }}>{Object.keys(descriptions).map(value => <option key={value} value={value}>{value.replaceAll('_', ' ')}</option>)}</select><p className="notice">{descriptions[role]}</p>
      {(restricted || event.can_manage_owner) && <label className="check"><input type="checkbox" checked={acknowledge} onChange={e => setAcknowledge(e.target.checked)} />Acknowledge the lasting competition restriction for organiser or early brief access. Active and waitlisted attendees must withdraw first.</label>}
      <div className="actions"><Button disabled={pending || (restricted && !acknowledge)} type="submit">Assign role</Button>{event.can_manage_owner && <Button variant="outline" type="button" disabled={pending || !person || !acknowledge} onClick={() => void change(person, 'organiser', true, true)}>Transfer ownership</Button>}</div>
    </form></fieldset>{success && <p role="status" className="notice">{success}</p>}{!!failure && <Failure error={failure} />}
  </section><section className="panel"><h2>Current and previous assignments</h2>{query.isPending ? <Loading /> : query.error ? <Failure error={query.error} /> : <ul className="item-list">{query.data.items.map(member => <li key={`${member.person_id}-${member.role}`}><div><strong>{member.display_name || member.person_id}</strong><p>{member.role.replaceAll('_', ' ')} · {member.active ? 'Active' : 'Revoked'}{event.owner_id === member.person_id && member.role === 'organiser' ? ' · Event owner' : ''}</p>{(member.role === 'organiser' || member.role === 'early_brief_access') && <small>Removing this role does not restore competition eligibility.</small>}</div>{member.active && <Button variant="outline" disabled={pending || !event.can_edit} onClick={() => void change(member.person_id, member.role, false)}>Revoke {member.role.replaceAll('_', ' ')} from {member.display_name || member.person_id}</Button>}</li>)}</ul>}
    <h3>Assignment history</h3>{history.error ? <Failure error={history.error} /> : <ul className="history">{history.data?.items.filter(item => ['role.granted', 'role.revoked', 'owner.transferred'].includes(item.action)).map(item => <li key={item.id}>{item.action.replaceAll('.', ' ')} · {item.metadata.role?.replaceAll('_', ' ')} · {new Date(item.occurred_at).toLocaleString()}</li>)}</ul>}
  </section></div>
}

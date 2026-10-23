import { CollectionRecords } from './components/patterns'
import { PlanningEditorSections, PlanningStep } from './planning-layout'
import { localDate } from './event-time'
import { Input, Select, Label } from './components/ui/fields'
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
  return <PlanningEditorSections label="Staff administration sections"><PlanningStep title="Assign staff"><section className="fleet-planning-section"><h2>{event.can_edit ? 'Assign event staff' : 'Event staff'}</h2><p>Ask the person for the account ID shown on their account page. They must have signed in at least once.</p>
    <fieldset disabled={!event.can_edit}><form onSubmit={e => { e.preventDefault(); void change(person, role, true) }}><Label htmlFor="staff-person">Person account ID</Label><Input id="staff-person" value={person} onChange={e => setPerson(e.target.value)} required pattern="[a-fA-F0-9-]{36}" />
      <Label htmlFor="staff-role">Role</Label><Select id="staff-role" value={role} onChange={e => { setRole(e.target.value as Role); setAcknowledge(false) }}>{Object.keys(descriptions).map(value => <option key={value} value={value}>{value.replaceAll('_', ' ')}</option>)}</Select><p className="notice">{descriptions[role]}</p>
      {(restricted || event.can_manage_owner) && <Label className="check"><Input type="checkbox" checked={acknowledge} onChange={e => setAcknowledge(e.target.checked)} />Acknowledge the lasting competition restriction for organiser or early brief access. Active and waitlisted attendees must withdraw first.</Label>}
      <div className="actions"><Button icon="user" disabled={pending || (restricted && !acknowledge)} type="submit">Assign role</Button>{event.can_manage_owner && <Button variant="outline" type="button" disabled={pending || !person || !acknowledge} onClick={() => void change(person, 'organiser', true, true)}>Transfer ownership</Button>}</div>
    </form></fieldset>{success && <p role="status" className="notice">{success}</p>}{!!failure && <Failure error={failure} />}
  </section></PlanningStep><PlanningStep title="Assignments"><section className="fleet-planning-section"><h2>Current and previous assignments</h2>{query.isPending ? <Loading /> : query.error ? <Failure error={query.error} /> : <CollectionRecords label="Current and previous staff assignments" rows={query.data.items} rowKey={member=>member.person_id+'-'+member.role} rowLabel={member=>member.display_name||member.person_id} destination={member=>({label:member.display_name||member.person_id,href:'#staff-person',onNavigate:()=>{setPerson(member.person_id);setRole(member.role);document.querySelector<HTMLButtonElement>('[aria-label="Staff administration sections"] button')?.click();requestAnimationFrame(()=>document.getElementById('staff-person')?.focus())}})} columns={[{id:'role',label:'Role',essential:true,cell:member=><>{member.role.replaceAll('_',' ')}{event.owner_id===member.person_id && member.role==='organiser'?' · Event owner':''}</>},{id:'state',label:'Assignment',essential:true,cell:member=>member.active?'Active':'Revoked'},{id:'restriction',label:'Competition restriction',cell:member=>member.role==='organiser'||member.role==='early_brief_access'?'Removing this role does not restore competition eligibility.':'Role eligibility rules apply'}]} actions={member=>member.active&&<Button variant="outline" icon="remove" disabled={pending||!event.can_edit} onClick={()=>void change(member.person_id,member.role,false)}>Revoke {member.role.replaceAll('_',' ')} from {member.display_name||member.person_id}</Button>} />}
</section></PlanningStep><PlanningStep title="Assignment history"><section className="fleet-planning-section"><h3>Assignment history</h3>{history.error ? <Failure error={history.error} /> : <ul className="history">{history.data?.items.filter(item => ['role.granted', 'role.revoked', 'owner.transferred'].includes(item.action)).map(item => <li key={item.id}>{item.action.replaceAll('.', ' ')} · {item.metadata.role?.replaceAll('_', ' ')} · {localDate(item.occurred_at,event.configuration.timezone)}</li>)}</ul>}
  </section></PlanningStep></PlanningEditorSections>
}

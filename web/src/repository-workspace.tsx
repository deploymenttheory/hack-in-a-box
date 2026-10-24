import { useRef, useState } from 'react'
import { api, unwrap, type EventRecord } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { useRepositoryRead } from './repository-read'
import { CollectionHeading, CollectionRecords, DetailWorkspace, Inspector } from './components/patterns'
import { Button } from './components/ui/button'
import { StatusBadge } from './components/ui/feedback'
import { Failure, Loading } from './components/states'
type Repository = components['schemas']['TeamRepository']
const stateLabel = (row: Repository) => row.unused ? 'Unused · unscored' : ({ pending: 'Queued · not confirmed', ready: 'Repository confirmed', review: 'Needs organiser review', ambiguous: 'Provider effect unknown' }[row.state])
export function RepositoryWorkspace({ event }: { event: EventRecord }) {
  const identity = useIdentity()
  return <RepositoryLifetime key={`${identity.person.id}:${identity.draft_scope}:${event.id}:${event.can_read_staff}`} event={event} />
}
function RepositoryLifetime({ event }: { event: EventRecord }) {
  const [selected, setSelected] = useState<string>(), opener = useRef<HTMLElement | null>(null)
  const { query, denied, check } = useRepositoryRead(event.id, 'collection', async signal => unwrap(await api.GET('/api/v1/events/{event_id}/repositories', { signal, cache: 'no-store', params: { path: { event_id: event.id } } })))
  const close = () => { setSelected(undefined); requestAnimationFrame(() => { if (opener.current?.isConnected) opener.current.focus() }) }
  if (denied) return <section><Failure error={denied} /><Button icon="refresh" onClick={check}>Check current repository access</Button></section>
  if (query.isPending) return <Loading />
  if (query.error) return <Failure error={query.error} retry={() => void query.refetch()} />
  const row = query.data.find(item => item.team_id === selected)
  const collection = <><CollectionHeading title="Repositories" scope={{ event: event.name, entity: event.can_read_staff ? 'Published team setup and readiness' : 'Your current team repository' }} description="Review repository setup and access before building. Repository activity never accepts a competition entry or changes its score." secondary={[{ label: 'Refresh current status', icon: 'refresh', onSelect: () => void query.refetch() }]} />
    <CollectionRecords label="Current authorised repositories" rows={query.data} rowKey={item => item.team_id} rowLabel={item => `${item.organization}/${item.name}`} columns={[
      { id: 'state', label: 'Readiness', essential: true, cell: item => <StatusBadge>{stateLabel(item)}</StatusBadge> },
      { id: 'team', label: 'Team reference', cell: item => <span className="fleet-reference">{item.team_id}</span> },
      { id: 'repository', label: 'Repository ID', cell: item => item.github_repository_id ?? 'Not confirmed' },
    ]} actions={item => <Button icon="next" variant="ghost" onClick={e => { opener.current = e.currentTarget; setSelected(item.team_id) }}>Inspect repository</Button>} />
    <p>{query.data.length} authorised mappings loaded.</p>{!query.data.length && <p>No repository mapping is currently available. A published team allocation and configured GitHub App are required.</p>}</>
  return <section className="fleet-repositories"><DetailWorkspace collection={collection} detail={row ? <Inspector identity={row.team_id} title={`${row.organization}/${row.name}`} scope={{ event: event.name, entity: 'Selected team repository' }} back={{ href: '#repositories', label: 'Back to repositories', onNavigate: close }}><dl className="fleet-repository-facts"><div><dt>Event</dt><dd>{event.id}</dd></div><div><dt>Team</dt><dd>{row.team_id}</dd></div><div><dt>Numeric repository</dt><dd>{row.github_repository_id ?? 'Not confirmed'}</dd></div><div><dt>Current readiness</dt><dd>{stateLabel(row)}</dd></div><div><dt>Provider diagnostic</dt><dd>{row.diagnostic || 'No diagnostic reported'}</dd></div></dl>{row.unused ? <p>This disbanded team’s repository is unused and unscored. Repository recovery cannot reassign competitors or reopen team allocation after freeze.</p> : row.github_repository_id && event.can_read_staff ? <RepositoryAccess eventID={event.id} teamID={row.team_id} /> : <p>Repository creation is separate from access invitations. A queued operation is not provider acceptance.</p>}</Inspector> : undefined} /></section>
}
function RepositoryAccess({ eventID, teamID }: { eventID: string; teamID: string }) {
  const { query, denied, check } = useRepositoryRead(eventID, `access:${teamID}`, async signal => unwrap(await api.GET('/api/v1/events/{event_id}/repositories/{team_id}/access', { signal, cache: 'no-store', params: { path: { event_id: eventID, team_id: teamID } } })))
  return <section className="fleet-repository-section"><h3>Repository access</h3>{denied ? <><Failure error={denied} /><Button icon="refresh" onClick={check}>Check current access review</Button></> : query.isPending ? <Loading /> : query.error ? <Failure error={query.error} /> : <><p>{query.data.grants_open ? 'Current published membership may receive reviewed access.' : 'New access grants are currently closed.'}</p><p>{query.data.grants.length} recorded grants · {query.data.execution.length} planned or attempted changes.</p><ul>{query.data.flags.map((flag, index) => <li key={index}>{flag.code.replaceAll('_', ' ')}{flag.person_id ? ` · Person ${flag.person_id}` : ''}</li>)}</ul><p>Current state is not an acknowledgement of an earlier command. Review exact people and external changes before approval.</p></>}</section>
}

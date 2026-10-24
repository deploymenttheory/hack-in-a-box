import { RepositoryDrafts, useRepositoryDeparture } from './repository-drafts'
import { RepositorySetup } from './repository-setup'
import { RepositoryProvision } from './repository-provision'
import { RepositoryInvitations } from './repository-invitations'
import { RepositoryAccess } from './repository-access'
import { useRef, useState } from 'react'
import { api, unwrap, type EventRecord } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { RepositoryReadAuthority, RepositoryProjectionMismatch, useRepositoryRead } from './repository-read'
import { CollectionHeading, CollectionRecords, DetailWorkspace, Inspector } from './components/patterns'
import { Button } from './components/ui/button'
import { StatusBadge } from './components/ui/feedback'
import { Failure, Loading } from './components/states'
type Repository = components['schemas']['TeamRepository'] & { teamName?: string }
const stateLabel = (row: Repository) => row.unused ? 'Unused · unscored' : ({ pending: 'Queued · not confirmed', ready: 'Repository confirmed', review: 'Needs organiser review', ambiguous: 'Provider effect unknown' }[row.state])
export function RepositoryWorkspace({ event, onDirty, administration = false }: { event: EventRecord; onDirty?: (dirty: boolean) => void; administration?: boolean }) {
  const identity = useIdentity()
  return <RepositoryDrafts key={`${identity.person.id}:${identity.draft_scope}:${event.id}`} onDirty={onDirty}><RepositoryLifetime key={`${administration}:${event.can_read_staff}`} event={event} administration={administration} /></RepositoryDrafts>
}
function RepositoryLifetime({ event, administration }: { event: EventRecord; administration: boolean }) {
  const identity = useIdentity(), staff = administration && event.can_read_staff
  const leave = useRepositoryDeparture()
  const [selected, setSelected] = useState<string>(), opener = useRef<HTMLElement | null>(null)
  const { query, denied, check, scope, paused } = useRepositoryRead(event.id, 'collection', async signal => {
    const params = { path: { event_id: event.id } }
    const rows = unwrap(await api.GET('/api/v1/events/{event_id}/repositories', { signal, cache: 'no-store', params }))
    const teams = unwrap(await api.GET('/api/v1/events/{event_id}/teams', { signal, cache: 'no-store', params }))
    const names = new Map(teams.teams.map(team => [team.id, team.name]))
    if (staff) return rows.map(row => ({ ...row, teamName: names.get(row.team_id) }))
    const own = rows.filter(row => teams.teams.some(team => team.id === row.team_id && team.state === 'published' && team.published.some(member => member.id === identity.person.id)))
    // A regular portal never turns organiser flags into administrative discovery.
    // The own-invitation endpoint proves active current identity for ready rows.
    const permitted: Repository[] = []
    for (const row of own) {
      if (row.github_repository_id) {
        const invitation = unwrap(await api.GET('/api/v1/events/{event_id}/repositories/{team_id}/invitations', { signal, cache: 'no-store', params: { path: { event_id: event.id, team_id: row.team_id } } }))
        if (invitation.team_id !== row.team_id || invitation.repository_id !== row.github_repository_id) throw new RepositoryProjectionMismatch('The current own-repository projection changed.')
        permitted.push({ ...row, teamName: names.get(row.team_id) })
      } else if (!event.can_read_staff) permitted.push({ ...row, teamName: names.get(row.team_id) })
    }
    return permitted
  })
  const close = () => { if (!leave()) return; setSelected(undefined); requestAnimationFrame(() => { if (opener.current?.isConnected) opener.current.focus() }) }
  if (denied) return <section><Failure error={denied} /><Button icon="refresh" onClick={check}>Check current repository access</Button></section>
  if (query.isPending) return <Loading />
  if (query.error && !query.data) return <Failure error={query.error} retry={() => void query.refetch()} />
  const row = query.data!.find(item => item.team_id === selected)
  const collection = <><CollectionHeading title="Repositories" scope={{ event: event.name, entity: staff ? 'Published team setup and readiness' : 'Your current team repository' }} description="Review repository setup and access before building. Repository activity never accepts a competition entry or changes its score." secondary={[{ label: 'Refresh current status', icon: 'refresh', onSelect: () => void query.refetch() }]} />
    <CollectionRecords label="Current authorised repositories" rows={query.data!} rowKey={item => item.team_id} rowLabel={item => item.teamName ?? `${item.organization}/${item.name}`} visibleColumns={row ? new Set(['state']) : undefined} columns={[
      { id: 'state', label: 'Readiness', essential: true, cell: item => <StatusBadge>{stateLabel(item)}</StatusBadge> },
      { id: 'repository-name', label: 'Repository', cell: item => <span>{item.organization}/{item.name}</span> },
      { id: 'repository', label: 'Repository ID', cell: item => item.github_repository_id ?? 'Not confirmed' },
    ]} actions={item => <Button icon="next" variant="ghost" onClick={e => { if (!leave()) return; opener.current = e.currentTarget; setSelected(item.team_id) }}>Inspect repository</Button>} />
    <p>{query.data!.length} authorised mappings loaded.</p>{!query.data!.length && <p>No repository mapping is currently available. A published team allocation and configured GitHub App are required.</p>}</>
  return <RepositoryReadAuthority.Provider value={{ scope, paused }}><section className="fleet-repositories">{query.error ? <Failure error={query.error} retry={() => void query.refetch()} /> : null}<DetailWorkspace collection={collection} detail={row ? <Inspector identity={row.team_id} title={`${row.organization}/${row.name}`} scope={{ event: event.name, entity: 'Selected team repository' }} back={{ href: '#repositories', label: 'Back to repositories', onNavigate: close }}><dl className="fleet-repository-facts"><div><dt>Event</dt><dd>{event.id}</dd></div><div><dt>Team</dt><dd>{row.teamName && <>{row.teamName}<br /></>}{row.team_id}</dd></div><div><dt>Numeric repository</dt><dd>{row.github_repository_id ?? 'Not confirmed'}</dd></div><div><dt>Current readiness</dt><dd>{stateLabel(row)}</dd></div><div><dt>Provider diagnostic</dt><dd>{row.diagnostic || 'No diagnostic reported'}</dd></div></dl>{row.unused ? <><p>This disbanded team’s repository is unused and unscored. Repository recovery cannot reassign competitors or reopen team allocation after freeze.</p>{row.github_repository_id && staff && <RepositoryAccess eventID={event.id} teamID={row.team_id} repositoryID={row.github_repository_id} writable={false} unused />}</> : row.github_repository_id && staff ? <RepositoryAccess eventID={event.id} teamID={row.team_id} repositoryID={row.github_repository_id} writable={event.can_edit} /> : row.github_repository_id ? <RepositoryInvitations eventID={event.id} teamID={row.team_id} repositoryID={row.github_repository_id} /> : <p>Repository creation is separate from access invitations. A queued operation is not provider acceptance.</p>}{administration && event.can_edit && !row.unused && <RepositoryProvision event={event} row={row} updated={() => void query.refetch()} />}</Inspector> : undefined} />{staff && <RepositorySetup event={event} />}{administration && event.can_edit && <RepositoryProvision event={event} updated={() => void query.refetch()} />}</section></RepositoryReadAuthority.Provider>
}

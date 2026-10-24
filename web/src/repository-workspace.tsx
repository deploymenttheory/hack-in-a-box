import { useQueryClient } from '@tanstack/react-query'
import { ActionLink } from './components/ui/button'
import { usePrivateRequestScope } from './private-request-scope'
import { repositoryDiagnostic } from './repository-diagnostics'
import { RepositoryDrafts, useRepositoryDeparture } from './repository-drafts'
import { RepositorySetup } from './repository-setup'
import { RepositoryProvision } from './repository-provision'
import { RepositoryInvitations } from './repository-invitations'
import { RepositoryAccess } from './repository-access'
import { useCallback, useRef, useState } from 'react'
import { api, privateKey, unwrap, type EventRecord } from './api/client'
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
  const identity = useIdentity(), cache = useQueryClient(), staff = administration && event.can_read_staff
  const writeScope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${event.id}:repository-write-authority`), wasWritable = useRef(administration && event.can_edit)
  const leave = useRepositoryDeparture()
  const [selected, setSelected] = useState<string>(), opener = useRef<HTMLElement | null>(null)
  const clearSelection = useCallback(() => setSelected(undefined), [])
  const { query, denied, check, scope, paused } = useRepositoryRead(event.id, 'collection', async signal => {
    const params = { path: { event_id: event.id } }
    const rows = unwrap(await api.GET('/api/v1/events/{event_id}/repositories', { signal, cache: 'no-store', params }))
    const teams = unwrap(await api.GET('/api/v1/events/{event_id}/teams', { signal, cache: 'no-store', params }))
    const names = new Map(teams.teams.map(team => [team.id, team.name]))
    // Teams.can_manage grants StaffRead, including read-only archive access.
    // A held older collection must also respect the latest native Event read
    // before it can renew any write lifetime from an older event closure.
    const writable = staff && event.can_edit && teams.can_manage && cache.getQueryData<EventRecord>(privateKey(identity.person.id, event.id, 'event'))?.can_edit !== false
    if (!writable) writeScope.invalidate(); else if (!wasWritable.current) writeScope.renew()
    wasWritable.current = writable
    if (staff) return { rows: rows.map(row => ({ ...row, teamName: names.get(row.team_id) })), writable }
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
    return { rows: permitted, writable: false }
  }, clearSelection, writeScope)
  const close = () => { if (!leave()) return; setSelected(undefined); requestAnimationFrame(() => { if (opener.current?.isConnected) opener.current.focus() }) }
  if (denied) return <section><Failure error={denied} /><Button icon="refresh" onClick={check}>Check current repository access</Button></section>
  if (query.isPending) return <Loading />
  if (query.error && !query.data) return <Failure error={query.error} retry={() => void query.refetch()} />
  const writable = staff && event.can_edit && query.data!.writable && cache.getQueryData<EventRecord>(privateKey(identity.person.id, event.id, 'event'))?.can_edit !== false
  const row = query.data!.rows.find(item => item.team_id === selected)
  const collection = <><CollectionHeading title="Repositories" scope={{ event: event.name, entity: staff ? 'Published team setup and readiness' : 'Your current team repository' }} description="Review repository setup and access before building. Repository activity never accepts a competition entry or changes its score." secondary={[{ label: 'Refresh current status', icon: 'refresh', onSelect: () => void query.refetch() }]} />
    <CollectionRecords label="Current authorised repositories" rows={query.data!.rows} rowKey={item => item.team_id} rowLabel={item => item.teamName ?? `${item.organization}/${item.name}`} visibleColumns={row ? new Set(['state']) : undefined} columns={[
      { id: 'state', label: 'Readiness', essential: true, cell: item => <StatusBadge>{stateLabel(item)}</StatusBadge> },
      { id: 'repository-name', label: 'Repository', cell: item => <span>{item.organization}/{item.name}</span> },
      { id: 'repository', label: 'Repository ID', cell: item => item.github_repository_id ?? 'Not confirmed' },
    ]} actions={item => <Button className="fleet-repository-inspect" icon="next" variant="ghost" onClick={e => { if (!leave()) return; opener.current = e.currentTarget; setSelected(item.team_id) }}>Inspect repository</Button>} />
    <p>{query.data!.rows.length} authorised mappings loaded.</p>{!query.data!.rows.length && <p>No repository mapping is currently available. A published team allocation and configured GitHub App are required.</p>}</>
  return <RepositoryReadAuthority.Provider value={{ scope, paused, writeScope, writable }}><section className="fleet-repositories">{query.error ? <Failure error={query.error} retry={() => void query.refetch()} /> : null}<DetailWorkspace collection={collection} detail={row ? <Inspector identity={row.team_id} title={`${row.organization}/${row.name}`} scope={{ event: event.name, entity: 'Selected team repository' }} back={{ href: '#repositories', label: 'Back to repositories', onNavigate: close }}><div className="fleet-pattern-actions"><Button icon="refresh" variant="outline" onClick={() => void query.refetch()}>Refresh current repository facts</Button>{row.github_repository_id && row.state === 'ready' && <ActionLink icon="external" href={`https://github.com/${encodeURIComponent(row.organization)}/${encodeURIComponent(row.name)}`} target="_blank" rel="noopener noreferrer">Open repository on GitHub</ActionLink>}</div><dl className="fleet-repository-facts"><div><dt>Event</dt><dd>{event.id}</dd></div><div><dt>Team</dt><dd>{row.teamName && <>{row.teamName}<br /></>}{row.team_id}</dd></div><div><dt>Numeric repository</dt><dd>{row.github_repository_id ?? 'Not confirmed'}</dd></div><div><dt>Current readiness</dt><dd>{stateLabel(row)}</dd></div><div><dt>Provider diagnostic</dt><dd>{repositoryDiagnostic(row.diagnostic)}</dd></div></dl>{row.unused ? <><p>This disbanded team’s repository is unused and unscored. Repository recovery cannot reassign competitors or reopen team allocation after freeze.</p>{row.github_repository_id && staff && <RepositoryAccess eventID={event.id} teamID={row.team_id} repositoryID={row.github_repository_id} labels={{ eventName: event.name, teamName: row.teamName, repositoryPath: `${row.organization}/${row.name}` }} writable={false} unused />}</> : row.github_repository_id && staff ? <RepositoryAccess eventID={event.id} teamID={row.team_id} repositoryID={row.github_repository_id} labels={{ eventName: event.name, teamName: row.teamName, repositoryPath: `${row.organization}/${row.name}` }} writable={writable} /> : row.github_repository_id ? <RepositoryInvitations eventID={event.id} teamID={row.team_id} repositoryID={row.github_repository_id} /> : <p>Repository creation is separate from access invitations. A queued operation is not provider acceptance.</p>}{writable && !row.unused && <RepositoryProvision event={event} row={row} updated={() => void query.refetch()} />}</Inspector> : undefined} />{staff && <RepositorySetup event={{ ...event, can_edit: writable }} />}{!row && writable && <RepositoryProvision event={event} updated={() => void query.refetch()} />}</section></RepositoryReadAuthority.Provider>
}

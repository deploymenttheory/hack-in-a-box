import { useCallback, useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, APIError, privateKey, unwrap, type EventRecord } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { usePrivateRequestScope } from './private-request-scope'
import { usePlanningUnknownCommands } from './planning-operations'
import { CollectionHeading, CollectionRecords } from './components/patterns'
import { StatusBadge } from './components/ui/feedback'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { localDate } from './event-time'

type Resource = components['schemas']['CredentialMetadata']
type ResourcePage = components['schemas']['CredentialPage']
type Team = components['schemas']['HackTeam']
type Visible = { epoch: number; id: string; version: number; value: string; revision: number; expiresAt: string | null }
const visibilityMilliseconds = 30_000
function expiredBeforeDisclosure(expiresAt: string | null) { return expiresAt !== null && Date.now() >= new Date(expiresAt).getTime() }

export function TeamAccess({ event }: { event: EventRecord }) {
  const identity = useIdentity()
  return <AccessLifetime key={`${identity.person.id}:${identity.draft_scope}:${event.id}`} event={event} />
}
function AccessLifetime({ event }: { event: EventRecord }) {
  const identity = useIdentity(), scope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${event.id}:access-teams`)
  const teams = useQuery({ queryKey: privateKey(identity.person.id, event.id, 'teams'), retry: false, queryFn: async ({ signal }) => { const ticket = scope.capture(); const value = unwrap(await api.GET('/api/v1/events/{event_id}/teams', { signal, cache: 'no-store', params: { path: { event_id: event.id } } })); ticket.assert(); return value }, refetchOnWindowFocus: 'always', refetchOnMount: 'always', refetchInterval: 5_000 })
  const team = teams.data?.teams.find(t => t.state === 'published' && t.published.some(p => p.id === identity.person.id))
  return <section className="private-team-access fleet-resource-workspace"><CollectionHeading title="Team access" scope={{ event: event.name, entity: "Private resources" }} description="Current confirmed membership and resource assignments control each audited reveal." />
    {teams.isPending ? <Loading /> : teams.error ? <Failure error={teams.error} retry={() => void teams.refetch()} /> : team ? <AssignedAccess key={`${identity.person.id}:${event.id}:${team.id}`} event={event} team={team} teamRevision={teams.data.revision} /> : <section className="fleet-resource-section"><h3>Waiting for your confirmed team</h3><p>Private access is available only to current confirmed members of a published team. Ask an organiser about your place or allocation.</p></section>}
  </section>
}

function AssignedAccess({ event, team, teamRevision }: { event: EventRecord; team: Team; teamRevision: number }) {
  const identity = useIdentity(), scope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${event.id}:${team.id}:access-metadata`)
  const metadata = useQuery({ queryKey: privateKey(identity.person.id, event.id, 'team-access'), retry: false, queryFn: async ({ signal }) => {
    const ticket = scope.capture()
    const data = unwrap(await api.GET('/api/v1/events/{event_id}/credentials', { signal, cache: 'no-store', params: { path: { event_id: event.id } } }))
    ticket.assert()
    // Keep an explicit metadata projection in the shared cache.
    return { revision: data.revision, can_manage: data.can_manage, items: data.items.map(item => ({ id: item.id, label: item.label, version: item.version, active: item.active, team_ids: item.team_ids, updated_at: item.updated_at, resource_type: item.resource_type, available_at: item.available_at, expires_at: item.expires_at, readiness: item.readiness })) }
  }, refetchOnWindowFocus: 'always', refetchOnMount: 'always', refetchInterval: 5_000 })
  const [failure, setFailure] = useState<unknown>()
  const page = metadata.data
  const items = page?.items.filter(item => !page.can_manage || item.team_ids.includes(team.id)) ?? []
  const fingerprint = `${teamRevision}:${page?.revision}:${items.map(item => `${item.id}:${item.version}:${item.active}:${item.updated_at}:${item.readiness}:${item.expires_at}`).join('|')}`
  return <>{metadata.isPending ? <Loading /> : metadata.error ? <Failure error={metadata.error} retry={() => void metadata.refetch()} /> : page && <AccessControls key={fingerprint} event={event} team={team} page={page} items={items} onRefresh={() => metadata.refetch()} onError={setFailure} />}{!!failure && <Failure error={failure} />}</>
}

function AccessControls({ event, team, page, items, onRefresh, onError }: { event: EventRecord; team: Team; page: ResourcePage; items: Resource[]; onRefresh: () => Promise<unknown>; onError: (error: unknown) => void }) {
  const identity = useIdentity()
  const [visible, setVisible] = useState<Visible>(), [pending, setPending] = useState<string>(), [notice, setNotice] = useState(''), [review, setReview] = useState<Resource>()
  const auditHistory = usePlanningUnknownCommands(`${identity.person.id}:${identity.draft_scope}:${event.id}:${team.id}:credential-audits`)
  const valueNode = useRef<HTMLElement>(null), request = useRef<AbortController | null>(null), epoch = useRef(0)
  const clearDOM = useCallback(() => { if (valueNode.current) valueNode.current.textContent = '' }, [])
  const stop = useCallback(() => {
    epoch.current++
    request.current?.abort(); request.current = null
    clearDOM()
  }, [clearDOM])
  const clear = useCallback(() => {
    stop(); setVisible(undefined); setPending(undefined); setNotice(''); setReview(undefined)
  }, [stop])
  useEffect(() => {
    const hidden = () => { if (document.visibilityState !== 'visible') clear() }
    for (const name of ['blur', 'pagehide', 'beforeprint', 'session-expired', 'session-ending']) window.addEventListener(name, clear)
    document.addEventListener('visibilitychange', hidden)
    return () => {
      for (const name of ['blur', 'pagehide', 'beforeprint', 'session-expired', 'session-ending']) window.removeEventListener(name, clear)
      document.removeEventListener('visibilitychange', hidden)
      stop()
    }
  }, [clear, stop])
  useEffect(() => {
    if (!visible) return
    const remaining = visible.expiresAt ? new Date(visible.expiresAt).getTime() - Date.now() : visibilityMilliseconds
    const timer = window.setTimeout(clear, Math.max(0, Math.min(visibilityMilliseconds, remaining)))
    return () => window.clearTimeout(timer)
  }, [visible, clear])

  async function disclose(item: Resource, copy: boolean) {
    if (request.current) return
    clear(); onError(undefined)
    const token = epoch.current, controller = new AbortController()
    request.current = controller; setPending(item.id)
    try {
      // Reveal responses never enter a shared query/mutation cache. Copy makes
      // another explicit audited reveal against current server authority.
      const revealed = unwrap(await api.POST('/api/v1/events/{event_id}/credentials/{credential_id}/reveal', { signal: controller.signal, cache: 'no-store', params: { path: { event_id: event.id, credential_id: item.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${page!.revision}"` } } }))
      if (token !== epoch.current || controller.signal.aborted || document.visibilityState !== 'visible') return
      if (revealed.id !== item.id || revealed.version !== item.version) throw new APIError(409, 'revision_conflict')
      // A response authorized before expiry may arrive after the resource closes.
      // Reject it before React state or the clipboard can receive its value.
      if (expiredBeforeDisclosure(item.expires_at)) throw new APIError(403, 'forbidden')
      setVisible({ epoch: token, id: revealed.id, version: revealed.version, value: revealed.value, revision: page!.revision, expiresAt: item.expires_at })
      if (copy) {
        try {
          if (!navigator.clipboard?.writeText) throw new Error('clipboard_unavailable')
          await navigator.clipboard.writeText(revealed.value)
          if (token === epoch.current) setNotice('Value copied to your clipboard.')
        } catch {
          if (token === epoch.current) setNotice('Copy failed. Try again or select the temporarily displayed value.')
        }
      } else setNotice('Value revealed temporarily. It will hide within 30 seconds.')
    } catch (error) {
      if (token !== epoch.current || controller.signal.aborted) return
      clear(); onError(error)
      if (!(error instanceof APIError) || error.status >= 500 || error.status === 408) {
        auditHistory.record({ operation: `${copy ? 'Copy' : 'Reveal'} private resource ${item.label}`, revision: page.revision, command: { credential_id: item.id, version: item.version, copy } })
        setNotice('Reveal outcome unknown. The audit may have been recorded. No reveal or copy retries automatically. Another deliberate reveal starts a separate audited request.')
      }
      if (error instanceof APIError && [401, 403, 404].includes(error.status)) auditHistory.clear()
      if (error instanceof APIError && [401, 403, 404, 409].includes(error.status)) await onRefresh()
    } finally {
      if (token === epoch.current) { request.current = null; setPending(undefined) }
    }
  }
  const validVisible = visible && visible.revision === page?.revision && items.some(item => item.id === visible.id && item.version === visible.version && item.active && item.readiness === 'ready') ? visible : undefined
  const shown = validVisible ? items.find(item => item.id === validVisible.id) : undefined
  return <><section className="fleet-resource-section"><div className="fleet-resource-heading"><h3>{team.name}</h3><Button icon="refresh" variant="outline" onClick={() => { clear(); void onRefresh() }}>Refresh team access</Button></div><p>Only resources available to your current team appear here. Keep revealed values private; this page is excluded from printing.</p><p>Hide a value when finished. Moving away, losing access or leaving this tab clears the display.</p></section>
    <CollectionRecords label="Private team resource metadata" rows={items} rowKey={item => item.id} rowLabel={item => item.label} columns={[{ id: 'kind', label: 'Type', cell: item => item.resource_type.replaceAll('_', ' ') }, { id: 'version', label: 'Version', cell: item => item.version }, { id: 'state', label: 'Current availability', cell: item => <StatusBadge tone={item.readiness === 'ready' ? 'success' : 'neutral'}>{item.readiness === 'ready' ? 'Ready for your team' : 'Unavailable'}</StatusBadge> }, { id: 'expiry', label: 'Available until', cell: item => item.expires_at ? `Available until ${localDate(item.expires_at, event.configuration.timezone)}` : 'No expiry recorded' }, { id: 'value', label: 'Private value', cell: () => <span aria-label="Value hidden">••••••••</span> }]} actions={item => <Button icon="shield" variant="outline" disabled={!!pending || !item.active || item.readiness !== 'ready'} onClick={() => { if (request.current) return; clear(); setReview(structuredClone(item)) }}>Reveal {item.label}</Button>} />
    {review && <section className="fleet-resource-section" aria-label="Review private reveal"><h3>Review private access</h3><p>{review.label} · Version {review.version} · {team.name}. Revealing is explicitly audited and rechecks your current membership, assignment, revision and expiry. Nothing has been revealed yet.</p><div className="fleet-pattern-actions"><Button icon="shield" disabled={!!pending} onClick={() => void disclose(review, false)}>Confirm reveal</Button><Button variant="outline" onClick={clear}>Cancel reveal</Button></div></section>}
    {shown && validVisible && <section className="fleet-resource-section" aria-label="Temporarily revealed resource"><h3>{shown.label} · Private value</h3><p>Temporarily visible for at most 30 seconds. Copy performs another explicitly audited reveal against current access.</p><div className="credential-value" aria-label={`Private value for ${shown.label}`}><code key={validVisible.epoch} ref={valueNode}>{validVisible.value}</code></div><div className="fleet-pattern-actions"><Button icon="code" variant="outline" disabled={!!pending} onClick={() => void disclose(shown, true)}>Copy {shown.label}</Button><Button icon="close" variant="outline" onClick={clear}>Hide value</Button></div></section>}
    {!items.length && <section className="fleet-resource-section"><h3>No private resources are available yet</h3><p>Your organiser supplies accounts, licences and access codes. Ask them to confirm your team's assignments and availability; this page cannot create or repair external access.</p></section>}
    {auditHistory.attempts.length > 0 && <section className="fleet-resource-section" aria-label="Unconfirmed private access audits"><p>Earlier audited request outcomes remain unknown. A later reveal does not settle an earlier audit.</p><ul>{auditHistory.attempts.map((attempt, index) => <li key={index}>{attempt.operation} · Expected revision {attempt.revision} · Original outcome remains unconfirmed</li>)}</ul></section>}
    {pending && <p role="status">Checking current access for the audited reveal…</p>}{notice && <p role="status">{notice}</p>}
  </>
}

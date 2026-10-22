import { useCallback, useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, APIError, privateKey, unwrap, type EventRecord } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { localDate } from './event-time'

type Resource = components['schemas']['CredentialMetadata']
type ResourcePage = components['schemas']['CredentialPage']
type Team = components['schemas']['HackTeam']
type Visible = { epoch: number; id: string; version: number; value: string; revision: number; expiresAt: string | null }
const visibilityMilliseconds = 30_000

export function TeamAccess({ event }: { event: EventRecord }) {
  const identity = useIdentity()
  const teams = useQuery({ queryKey: privateKey(identity.person.id, event.id, 'teams'), queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/events/{event_id}/teams', { signal, cache: 'no-store', params: { path: { event_id: event.id } } })), refetchOnWindowFocus: 'always', refetchOnMount: 'always', refetchInterval: 5_000 })
  const team = teams.data?.teams.find(t => t.state === 'published' && t.published.some(p => p.id === identity.person.id))
  return <section className="private-team-access"><p className="eyebrow">Private team resources</p><h2>Team access</h2>
    {teams.isPending ? <Loading /> : teams.error ? <Failure error={teams.error} retry={() => void teams.refetch()} /> : team ? <AssignedAccess key={`${identity.person.id}:${event.id}:${team.id}`} event={event} team={team} teamRevision={teams.data.revision} /> : <div className="panel"><h3>Waiting for your confirmed team</h3><p>Private access is available only to current confirmed members of a published team. Ask an organiser about your place or allocation.</p></div>}
  </section>
}

function AssignedAccess({ event, team, teamRevision }: { event: EventRecord; team: Team; teamRevision: number }) {
  const identity = useIdentity()
  const metadata = useQuery({ queryKey: privateKey(identity.person.id, event.id, 'team-access'), queryFn: async ({ signal }) => {
    const data = unwrap(await api.GET('/api/v1/events/{event_id}/credentials', { signal, cache: 'no-store', params: { path: { event_id: event.id } } }))
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
  const [visible, setVisible] = useState<Visible>(), [pending, setPending] = useState<string>(), [notice, setNotice] = useState('')
  const valueNode = useRef<HTMLElement>(null), request = useRef<AbortController | null>(null), epoch = useRef(0)
  const clearDOM = useCallback(() => { if (valueNode.current) valueNode.current.textContent = '' }, [])
  const stop = useCallback(() => {
    epoch.current++
    request.current?.abort(); request.current = null
    clearDOM()
  }, [clearDOM])
  const clear = useCallback(() => {
    stop(); setVisible(undefined); setPending(undefined); setNotice('')
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
    clear(); onError(undefined)
    const token = epoch.current, controller = new AbortController()
    request.current = controller; setPending(item.id)
    try {
      // Reveal responses never enter a shared query/mutation cache. Copy makes
      // another explicit audited reveal against current server authority.
      const revealed = unwrap(await api.POST('/api/v1/events/{event_id}/credentials/{credential_id}/reveal', { signal: controller.signal, cache: 'no-store', params: { path: { event_id: event.id, credential_id: item.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${page!.revision}"` } } }))
      if (token !== epoch.current || controller.signal.aborted || document.visibilityState !== 'visible') return
      if (revealed.id !== item.id || revealed.version !== item.version) throw new APIError(409, 'revision_conflict')
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
      if (error instanceof APIError && [401, 403, 404, 409].includes(error.status)) await onRefresh()
    } finally {
      if (token === epoch.current) { request.current = null; setPending(undefined) }
    }
  }
  const validVisible = visible && visible.revision === page?.revision && items.some(item => item.id === visible.id && item.version === visible.version && item.active && item.readiness === 'ready') ? visible : undefined
  return <><div className="panel"><h3>{team.name}</h3><p>Only resources available to your current team appear here. Keep revealed values private; this page is excluded from printing.</p><p>Hide a value when finished. Moving away, losing access or leaving this tab clears the display.</p><Button variant="outline" onClick={() => { clear(); void onRefresh() }}>Refresh team access</Button></div>
    <div className="team-access-grid">{items.map(item => <article key={item.id} className="panel"><span className="badge">{item.resource_type.replaceAll('_', ' ')}</span><h3>{item.label}</h3><p>Version {item.version} · {item.readiness === 'ready' ? 'Ready for your team' : 'Unavailable'}</p><p>{item.expires_at ? `Available until ${localDate(item.expires_at, event.configuration.timezone)}` : 'No expiry recorded. Ask your organiser if access stops working.'}</p>
      <div className="credential-value" aria-label={`Private value for ${item.label}`}>{validVisible?.id === item.id ? <code key={validVisible.epoch} ref={valueNode}>{validVisible.value}</code> : <span aria-label="Value hidden">••••••••</span>}</div>
      <div className="actions"><Button disabled={!!pending || !item.active || item.readiness !== 'ready'} onClick={() => void disclose(item, false)}>Reveal {item.label}</Button>{validVisible?.id === item.id && <><Button variant="outline" disabled={!!pending} onClick={() => void disclose(item, true)}>Copy {item.label}</Button><Button variant="outline" onClick={clear}>Hide value</Button></>}</div>
    </article>)}{!items.length && <div className="panel"><h3>No private resources are available yet</h3><p>Your organiser supplies accounts, licences and access codes. Ask them to confirm your team's assignments and availability; this page cannot create or repair external access.</p></div>}</div>
    {notice && <p role="status">{notice}</p>}
  </>
}

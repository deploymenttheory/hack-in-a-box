import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, privateKey, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'

type Page = components['schemas']['TeamsWorkflowPage']
type Command = components['schemas']['TeamsWorkflowCommand']
export function TeamsWorkflowSetup({ eventID, canEdit, changed }: { eventID: string; canEdit: boolean; changed: () => void }) {
  const identity = useIdentity()
  const query = useQuery({ queryKey: privateKey(identity.person.id, eventID, 'teams-workflow'), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/teams-workflow', { params: { path: { event_id: eventID } } })), retry: false })
  if (query.isPending) return <Loading />
  if (query.error) return <Failure error={query.error} retry={() => void query.refetch()} />
  return <Setup key={`${eventID}:${identity.person.id}`} eventID={eventID} canEdit={canEdit} initial={query.data!} changed={changed} />
}
function Setup({ eventID, canEdit, initial, changed }: { eventID: string; canEdit: boolean; initial: Page; changed: () => void }) {
  const identity = useIdentity(), [page, setPage] = useState(initial), [endpoint, setEndpoint] = useState(''), [label, setLabel] = useState(initial.connection?.label ?? ''), [owner, setOwner] = useState(initial.connection?.owner ?? ''), [coOwner, setCoOwner] = useState(initial.connection?.co_owner ?? ''), [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState('')
  const active = useRef(true), testID = useRef<string | undefined>(undefined)
  useEffect(() => {
    const clear = () => setEndpoint('')
    const hide = () => { if (document.visibilityState !== 'visible') clear() }
    active.current = true
    document.addEventListener('visibilitychange', hide); window.addEventListener('session-expired', clear); window.addEventListener('beforeprint', clear)
    return () => { active.current = false; document.removeEventListener('visibilitychange', hide); window.removeEventListener('session-expired', clear); window.removeEventListener('beforeprint', clear) }
  }, [])
  const path = { event_id: eventID }
  async function update(action: Command['action'] | 'test') {
    // The write-only URL stays in this component and this request, never in a
    // query/mutation cache, storage, receipt, notification or error message.
    const secret = endpoint
    setEndpoint(''); setPending(true); setFailure(undefined); setNotice('')
    try {
      const header = { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${page.revision}"` }
      const fresh = action === 'test'
        ? unwrap(await api.POST('/api/v1/events/{event_id}/teams-workflow/test', { params: { path, header }, body: { request_id: testID.current ??= crypto.randomUUID() } }))
        : unwrap(await api.POST('/api/v1/events/{event_id}/teams-workflow', { params: { path, header }, body: { action, label, owner, co_owner: coOwner, endpoint: action === 'save' ? secret : '' } }))
      if (!active.current) return
      setPage(fresh); testID.current = undefined
      setNotice(action === 'test' ? 'Shared-channel TEST queued. Check its receipt and the workflow run history.' : 'Connection updated. Existing campaign approvals require explicit review and reapproval.')
      changed()
    } catch (error) { if (active.current) setFailure(error) } finally { if (active.current) setPending(false) }
  }
  async function refresh() {
    setPending(true); setFailure(undefined); setEndpoint('')
    try { const fresh = unwrap(await api.GET('/api/v1/events/{event_id}/teams-workflow', { params: { path } })); if (active.current) setPage(fresh) } catch (error) { if (active.current) setFailure(error) } finally { if (active.current) setPending(false) }
  }
  const disabled = pending || !canEdit
  return <section id="teams-workflow-setup" aria-label="Teams workflow connection"><h4>Teams workflow connection</h4><p>Use an approved Teams Workflows incoming webhook configured for Anyone. Assign an owner and co-owner in Microsoft Workflows so the connection survives an owner leaving. Tenant authentication modes require a different integration.</p><p>The endpoint is a secret: it is write-only and encrypted. Replacing, rotating or revoking it invalidates existing approvals. Personal, team-specific and private messages cannot be broadcast.</p>
    {!page.encryption_ready && <p role="alert">Endpoint encryption is unavailable. Ask the deployment administrator to restore the configured encryption keys.</p>}
    {page.connection && <p>Current connection: {page.connection.label} · version {page.connection.version} · {page.connection.active ? page.connection.available ? 'Ready' : 'Unavailable: restore the encryption key or replace the endpoint' : 'Revoked'}. Owner: {page.connection.owner}; co-owner: {page.connection.co_owner}.</p>}
    <fieldset disabled={disabled || !page.encryption_ready}><legend>Save or replace the approved connection</legend><label htmlFor="teams-label">Connection label</label><input id="teams-label" maxLength={160} value={label} onChange={e => setLabel(e.target.value)} /><label htmlFor="teams-owner">Workflow owner</label><input id="teams-owner" maxLength={160} value={owner} onChange={e => setOwner(e.target.value)} /><label htmlFor="teams-co-owner">Workflow co-owner</label><input id="teams-co-owner" maxLength={160} value={coOwner} onChange={e => setCoOwner(e.target.value)} /><label htmlFor="teams-endpoint">New secret workflow endpoint</label><input id="teams-endpoint" type="password" autoComplete="off" maxLength={4096} value={endpoint} onChange={e => setEndpoint(e.target.value)} /><Button disabled={!endpoint || !label.trim() || !owner.trim() || !coOwner.trim()} onClick={() => void update('save')}>Save Teams connection</Button></fieldset>
    <div className="actions"><Button variant="outline" disabled={disabled || !page.connection?.active || !page.connection.available || !page.encryption_ready} onClick={() => void update('rotate')}>Rotate endpoint encryption</Button><Button variant="outline" disabled={disabled || !page.connection?.active} onClick={() => void update('revoke')}>Revoke Teams connection</Button></div>
    <p>A connection TEST posts a fixed, clearly labelled message to everyone in the configured channel. It is separate from a private self-test and does not send any campaign or recipient content.</p><Button disabled={disabled || !page.connection?.active || !page.connection.available} onClick={() => { if (window.confirm('Broadcast the fixed TEST message to the configured shared Teams channel?')) void update('test') }}>Queue shared-channel TEST</Button><Button variant="outline" disabled={pending} onClick={() => void refresh()}>Refresh Teams receipts</Button>
    <p>Provider accepted means the webhook accepted the request; it does not prove the workflow posted to Teams. Reconcile uncertain outcomes in Microsoft Workflows run history before explicitly queuing any new message. Uncertain sends are never replayed automatically.</p>
    <ul>{page.history.map(receipt => <li key={receipt.id}>{receipt.kind === 'connection_test' ? 'Connection TEST' : 'Approved broadcast'} · {receipt.status.replaceAll('_', ' ')} · {receipt.reason.replaceAll('_', ' ')} · {receipt.attempts} attempts{receipt.retry_at && ` · retry after ${new Date(receipt.retry_at).toLocaleString()}`}</li>)}</ul>{pending && <p role="status">Updating Teams connection…</p>}{notice && <p role="status">{notice}</p>}{!!failure && <Failure error={failure} />}
  </section>
}

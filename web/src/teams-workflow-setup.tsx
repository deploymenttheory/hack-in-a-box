import { useCommunicationCommands, CommunicationCommandHistory } from './communication-tests'
import { usePlanningDepartureBarrier } from './planning-operations'
import { useCallback, useMemo } from 'react'
import { APIError } from './api/client'
import { useQueryClient } from '@tanstack/react-query'
import { usePrivateRequestScope, type PrivateRequestScope } from './private-request-scope'
import { communicationRead } from './communication-private'
import { CommunicationStep } from './communication-layout'
import { Input, Label } from './components/ui/fields'
import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, privateKey, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { ConfirmationDialog } from './components/ui/dialog'
import { StatusBadge } from './components/ui/feedback'
import { Failure, Loading } from './components/states'

type Page = components['schemas']['TeamsWorkflowPage']
type Command = components['schemas']['TeamsWorkflowCommand']
type Intent = { action: Command['action'] | 'test'; revision: number; requestID?: string }
function rememberedIntent(value: unknown): Intent | undefined {
  if (!value || typeof value !== 'object' || !('action' in value) || !('revision' in value) || !['save','rotate','revoke','test'].includes(String(value.action)) || typeof value.revision !== 'number') return undefined
  const requestID = 'requestID' in value && typeof value.requestID === 'string' ? value.requestID : undefined
  if (value.action === 'test' && !requestID) return undefined
  return { action: value.action as Intent['action'], revision: value.revision, requestID }
}
export function TeamsWorkflowSetup({ eventID, canEdit, changed }: { eventID: string; canEdit: boolean; changed: () => void }) {
  const identity = useIdentity()
  return <ConnectionScope key={`${eventID}:${identity.person.id}:${identity.draft_scope}:${canEdit}`} eventID={eventID} canEdit={canEdit} changed={changed} />
}
function ConnectionScope({ eventID, canEdit, changed }: { eventID: string; canEdit: boolean; changed: () => void }) {
  const identity = useIdentity(), cache = useQueryClient(), [denied, setDenied] = useState(false)
  const key = useMemo(() => [...privateKey(identity.person.id, eventID, 'teams-workflow'), identity.draft_scope], [identity.person.id, identity.draft_scope, eventID])
  const scope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${eventID}:teams`)
  const deny = useCallback(() => { scope.invalidate(); setDenied(true); cache.removeQueries({ queryKey: key }) }, [scope, cache, key])
  useEffect(() => { const events = ['session-ending', 'session-expired', 'mentor-draft-invalidated']; for (const name of events) window.addEventListener(name, deny); return () => { for (const name of events) window.removeEventListener(name, deny) } }, [deny])
  const query = useQuery({ queryKey: key, gcTime: 0, queryFn: () => communicationRead(scope, async signal => { try { return unwrap(await api.GET('/api/v1/events/{event_id}/teams-workflow', { signal, params: { path: { event_id: eventID } } })) } catch (error) { if (scope.capture().current() && error instanceof APIError && [401,403,404].includes(error.status)) deny(); throw error } }), retry: false, refetchOnWindowFocus: 'always' })
  const revoked = query.error instanceof APIError && [401,403,404].includes(query.error.status)
  if (denied || revoked) return <p role="alert">The current Teams connection is no longer available. Private connection edits have been cleared.</p>
  if (query.isPending) return <Loading />
  if (!query.data) return <Failure error={query.error} retry={() => void query.refetch()} />
  return <>{!!query.error && <Failure error={query.error} retry={() => void query.refetch()} />}<Setup key={`${eventID}:${identity.person.id}:${query.data?.connection?.id}:${query.data?.connection?.version}`} eventID={eventID} canEdit={canEdit} initial={query.data!} changed={changed} parent={scope} deny={deny} readFresh={!query.error} /></>
}
function Setup({ eventID, canEdit, initial, changed, parent, deny, readFresh }: { readFresh: boolean; parent: PrivateRequestScope; deny: () => void; eventID: string; canEdit: boolean; initial: Page; changed: () => void }) {
  const scope = usePrivateRequestScope('connection-editor', parent)
  const identity = useIdentity(), [page, setPage] = useState(initial), [endpoint, setEndpoint] = useState(''), [label, setLabel] = useState(initial.connection?.label ?? ''), [owner, setOwner] = useState(initial.connection?.owner ?? ''), [coOwner, setCoOwner] = useState(initial.connection?.co_owner ?? ''), [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState('')
  const history = useCommunicationCommands(`${identity.person.id}:${identity.draft_scope}:${eventID}:teams-workflow`)
  const [testOpen, setTestOpen] = useState(false), [localUnknown, setUnknown] = useState<Intent>()
  const unknown = localUnknown ?? (history.unconfirmed ? rememberedIntent(history.attempts.at(-1)?.command) : undefined)
  usePlanningDepartureBarrier(pending)
  const writing = useRef(false), testTrigger = useRef<HTMLButtonElement>(null)
  const active = useRef(true), testID = useRef<string | undefined>(undefined)
  useEffect(() => {
    const clear = () => { setEndpoint(''); setUnknown(undefined); setTestOpen(false); scope.invalidate(); deny() }
    const erase = () => setEndpoint('')
    const hide = () => { if (document.visibilityState !== 'visible') erase() }
    active.current = true
    document.addEventListener('visibilitychange', hide); window.addEventListener('session-expired', clear); window.addEventListener('session-ending', clear); window.addEventListener('beforeprint', erase)
    return () => { active.current = false; document.removeEventListener('visibilitychange', hide); window.removeEventListener('session-expired', clear); window.removeEventListener('session-ending', clear); window.removeEventListener('beforeprint', erase) }
  }, [scope, deny])
  const path = { event_id: eventID }
  async function update(action: Command['action'] | 'test', retry = false) {
    if (writing.current || !canEdit || !readFresh || (unknown && !retry) || (retry && unknown?.action !== 'test')) return
    const ticket = scope.capture(); if (!ticket.current()) return
    const intent = retry && unknown ? unknown : { action, revision: page.revision, requestID: action === 'test' ? (testID.current ??= crypto.randomUUID()) : undefined }
    writing.current = true
    const prior = retry ? history.attempts.find(attempt => attempt.command.requestID === intent.requestID) : undefined
    const operation = history.begin({ operation: `Teams ${intent.action}`, revision: intent.revision, command: { intentID: intent.requestID ?? crypto.randomUUID(), action: intent.action, revision: intent.revision, requestID: intent.requestID } }, prior?.id)
    // The write-only URL stays in this component and this request, never in a
    // query/mutation cache, storage, receipt, notification or error message.
    const secret = endpoint
    setEndpoint(''); setPending(true); setFailure(undefined); setNotice('')
    try {
      const header = { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${intent.revision}"` }
      const fresh = action === 'test'
        ? unwrap(await api.POST('/api/v1/events/{event_id}/teams-workflow/test', { params: { path, header }, body: { request_id: intent.requestID! } }))
        : unwrap(await api.POST('/api/v1/events/{event_id}/teams-workflow', { params: { path, header }, body: { action, label, owner, co_owner: coOwner, endpoint: action === 'save' ? secret : '' } }))
      history.complete(operation)
      if (!active.current || !ticket.current()) return
      setUnknown(undefined); setPage(fresh); testID.current = undefined
      setNotice(action === 'test' ? 'Shared-channel TEST queued. Check its receipt and the workflow run history.' : 'Connection updated. Existing campaign approvals require explicit review and reapproval.')
      changed()
    } catch (error) {
      const unknown = !(error instanceof APIError) || error.status >= 500 || error.status === 408
      if (unknown) history.uncertain(operation); else history.complete(operation)
      if (active.current && ticket.current()) {
        if (error instanceof APIError && [401,403,404].includes(error.status)) { setEndpoint(''); setUnknown(undefined); deny(); return }
        setFailure(error); if (unknown) setUnknown(intent)
      }
    } finally { writing.current = false; if (active.current) setPending(false) }
  }
  async function refresh() {
    if (writing.current) return
    const ticket = scope.capture(); if (!ticket.current()) return
    setPending(true); setFailure(undefined); setEndpoint('')
    try { const fresh = unwrap(await api.GET('/api/v1/events/{event_id}/teams-workflow', { params: { path } })); if (active.current && ticket.current()) setPage(fresh) } catch (error) { if (active.current && ticket.current()) { if (error instanceof APIError && [401,403,404].includes(error.status)) deny(); else setFailure(error) } } finally { if (active.current && ticket.current()) setPending(false) }
  }
  const disabled = pending || history.pending || !!unknown || !canEdit || !readFresh
  return <CommunicationStep id="teams-workflow-setup" title="Teams workflow connection" icon="send" description="Configure an approved shared workflow connection with a current owner and co-owner."><CommunicationCommandHistory attempts={history.attempts} /><p>Use an approved Teams Workflows incoming webhook configured for Anyone. Assign an owner and co-owner in Microsoft Workflows so the connection survives an owner leaving. Tenant authentication modes require a different integration.</p><p>The endpoint is a secret: it is write-only and encrypted. Replacing, rotating or revoking it invalidates existing approvals. Personal, team-specific and private messages cannot be broadcast.</p>
    {!page.encryption_ready && <p role="alert">Endpoint encryption is unavailable. Ask the deployment administrator to restore the configured encryption keys.</p>}
    {page.connection && <p>Current connection: {page.connection.label} · version {page.connection.version} · {page.connection.active ? page.connection.available ? 'Ready' : 'Unavailable: restore the encryption key or replace the endpoint' : 'Revoked'}. Owner: {page.connection.owner}; co-owner: {page.connection.co_owner}.</p>}
    <fieldset disabled={disabled || !page.encryption_ready}><legend>Save or replace the approved connection</legend><Label htmlFor="teams-label">Connection label</Label><Input id="teams-label" maxLength={160} value={label} onChange={e => setLabel(e.target.value)} /><Label htmlFor="teams-owner">Workflow owner</Label><Input id="teams-owner" maxLength={160} value={owner} onChange={e => setOwner(e.target.value)} /><Label htmlFor="teams-co-owner">Workflow co-owner</Label><Input id="teams-co-owner" maxLength={160} value={coOwner} onChange={e => setCoOwner(e.target.value)} /><Label htmlFor="teams-endpoint">New secret workflow endpoint</Label><Input id="teams-endpoint" type="password" autoComplete="off" maxLength={4096} value={endpoint} onChange={e => setEndpoint(e.target.value)} /><Button icon="save" pending={pending} disabled={!endpoint || !label.trim() || !owner.trim() || !coOwner.trim()} onClick={() => void update('save')}>Save Teams connection</Button></fieldset>
    <div className="actions"><Button icon="refresh" variant="outline" disabled={disabled || !page.connection?.active || !page.connection.available || !page.encryption_ready} onClick={() => void update('rotate')}>Rotate endpoint encryption</Button><Button icon="remove" variant="outline" disabled={disabled || !page.connection?.active} onClick={() => void update('revoke')}>Revoke Teams connection</Button></div>
    <p>A connection TEST posts a fixed, clearly labelled message to everyone in the configured channel. It is separate from a private self-test and does not send any campaign or recipient content.</p><Button ref={testTrigger} icon="send" disabled={disabled || !page.connection?.active || !page.connection.available} onClick={() => setTestOpen(true)}>Queue shared-channel TEST</Button><Button icon="refresh" variant="outline" pending={pending} onClick={() => void refresh()}>Refresh Teams receipts</Button>
    <ConfirmationDialog open={testOpen} onOpenChange={setTestOpen} title="Queue shared-channel TEST?" description="This posts the fixed TEST message to everyone in the configured shared channel. It sends no campaign or private recipient content." confirmLabel="Queue TEST message" finalFocus={testTrigger} pending={pending} disabled={disabled || !page.connection?.active || !page.connection.available} onConfirm={() => { if (!writing.current) void update('test').finally(() => { if (active.current) setTestOpen(false) }) }}><dl><dt>Audience</dt><dd>Everyone in {page.connection?.label} · connection version {page.connection?.version}</dd><dt>Subject</dt><dd>TEST — Hack-in-a-Box Teams connection check</dd><dt>Message</dt><dd>This synthetic connection test is a broadcast to the configured event channel. Confirm the destination and workflow run with its owner. It is not a personal campaign self-test.</dd></dl></ConfirmationDialog>
    {unknown && <div role="alert"><p>Outcome unknown for {unknown.action === 'test' ? 'the shared-channel TEST' : 'the connection change'}. No replacement command will be sent. Refresh current facts and reconcile with the workflow owner; current facts are not the original acknowledgement.</p>{unknown.action === 'test' && <Button icon="refresh" disabled={pending || history.pending || !canEdit} onClick={() => void update('test', true)}>Retry same TEST acknowledgement</Button>}</div>}
    <p>Provider accepted means the webhook accepted the request; it does not prove the workflow posted to Teams. Reconcile uncertain outcomes in Microsoft Workflows run history before explicitly queuing any new message. Uncertain sends are never replayed automatically.</p>
    <ul>{page.history.map(receipt => <li key={receipt.id}>{receipt.kind === 'connection_test' ? 'Connection TEST' : 'Approved broadcast'} · <StatusBadge>{receipt.status.replaceAll('_', ' ')}</StatusBadge> · {receipt.reason.replaceAll('_', ' ')} · {receipt.attempts} attempts{receipt.retry_at && ` · retry after ${new Date(receipt.retry_at).toLocaleString()}`}</li>)}</ul>{pending && <p role="status">Updating Teams connection…</p>}{notice && <p role="status">{notice}</p>}{!!failure && <Failure error={failure} />}
  </CommunicationStep>
}

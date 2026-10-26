import { usePrivateRequestScope, type PrivateRequestScope } from './private-request-scope'
import { repositoryAppExplanation } from './repository-diagnostics'
import { useRepositoryDeparture, useRepositoryDraft } from './repository-drafts'
import { RepositoryActionReview } from './repository-review'
import { useLayoutEffect, useRef, useState } from 'react'
import { api, unwrap, type EventRecord } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { useRepositoryRead } from './repository-read'
import { RepositoryCommandStatus, useRepositoryCommand } from './repository-command'
import { Button } from './components/ui/button'
import { Field, Input, Select, focusFirstInvalid } from './components/ui/fields'
import { Failure, Loading } from './components/states'
import { GitHubSetupFields, gitHubSetupErrors } from './github-setup-fields'
type S = components['schemas']

export function RepositorySetup({ event }: { event: EventRecord }) {
  const leave = useRepositoryDeparture()
  const [open, setOpen] = useState(false)
  return <section className="fleet-repository-section"><h2>Organisation and template setup</h2><p>The organisation-installed App provisions private team repositories. People verify their own GitHub account separately.</p><Button icon="repository" variant="outline" onClick={() => { if (!open || leave()) setOpen(!open) }}>{open ? 'Hide' : 'Open'} App setup</Button>{open && <SetupReader key={String(event.can_edit)} event={event} />}</section>
}
function SetupReader({ event }: { event: EventRecord }) {
  const identity = useIdentity(), lastRevision = useRef<number | undefined>(undefined), configurationCommands = useRef<PrivateRequestScope | null>(null)
  const owner = useRepositoryRead(event.id, 'app', async signal => {
    const value = unwrap(await api.GET('/api/v1/events/{event_id}/github-app', { signal, cache: 'no-store', params: { path: { event_id: event.id } } }))
    // A fresh configuration revision fences an old installation continuation
    // synchronously; unsent form values remain local for explicit correction.
    if (lastRevision.current !== undefined && lastRevision.current !== value.revision) configurationCommands.current?.renew()
    lastRevision.current = value.revision
    return value
  })
  const commandScope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${event.id}:repository-app-command`, owner.scope)
  useLayoutEffect(() => { configurationCommands.current = commandScope; return () => { configurationCommands.current = null } }, [commandScope])
  return owner.denied ? <><Failure error={owner.denied} /><Button icon="refresh" onClick={owner.check}>Check current App setup access</Button></> : owner.query.isPending ? <Loading /> : owner.query.error && !owner.query.data ? <Failure error={owner.query.error} retry={() => void owner.query.refetch()} /> : <>{owner.query.error ? <Failure error={owner.query.error} retry={() => void owner.query.refetch()} /> : null}<SetupEditor {...{ event, owner, commandScope }} value={owner.query.data!} /></>
}
function SetupEditor({ event, owner, commandScope, value }: { event: EventRecord; commandScope: PrivateRequestScope; owner: ReturnType<typeof useRepositoryRead<S['GitHubAppStatus']>>; value: S['GitHubAppStatus'] }) {
  const identity = useIdentity(), operation = useRepositoryCommand(event.id, 'app', commandScope, owner.deny)
  const [action, setAction] = useState<S['GitHubAppCommand']['action']>('configure'), [appID, setAppID] = useState(value.configuration?.app_id.toString() ?? ''), [slug, setSlug] = useState(value.configuration?.app_slug ?? ''), [orgID, setOrgID] = useState(value.configuration?.organization_id.toString() ?? ''), [organization, setOrganization] = useState(value.configuration?.organization ?? ''), [template, setTemplate] = useState(value.configuration?.template_repository ?? ''), [privateKey, setPrivateKey] = useState(''), [webhook, setWebhook] = useState('')
  const [review, setReview] = useState<{ command: S['GitHubAppCommand'] | 'install'; revision: number; configuration?: S['GitHubAppConfiguration'] }>(), [notice, setNotice] = useState(''), opener = useRef<HTMLElement | null>(null), readControl = useRef<HTMLButtonElement>(null)
  const settleOwnDraft = useRepositoryDraft(!!privateKey || !!webhook || !!review || operation.blocked || action !== 'configure' || appID !== (value.configuration?.app_id.toString() ?? '') || slug !== (value.configuration?.app_slug ?? '') || orgID !== (value.configuration?.organization_id.toString() ?? '') || organization !== (value.configuration?.organization ?? '') || template !== (value.configuration?.template_repository ?? ''))
  const [attempted, setAttempted] = useState(false), [editing, setEditing] = useState(!value.configured)
  const installationHeading = useRef<HTMLHeadingElement>(null), editor = useRef<HTMLFormElement>(null)
  const setupValues = { appID, slug, orgID, organization, template, privateKey }
  const setupSetters = { appID: setAppID, slug: setSlug, orgID: setOrgID, organization: setOrganization, template: setTemplate, privateKey: setPrivateKey }
  const stale = !!review && review.revision !== value.revision
  function prepare(control: HTMLElement, install = false) {
    if (owner.paused || operation.blocked) return
    if (!install && action === 'configure') {
      setAttempted(true)
      if (Object.keys(gitHubSetupErrors(setupValues)).length) {
        requestAnimationFrame(() => { const form = control.closest('form'); if (form) focusFirstInvalid(form) }); return
      }
    }
    opener.current = control
    setReview({ revision: value.revision, configuration: value.configuration ? structuredClone(value.configuration) : undefined, command: install ? 'install' : { action, ...(action === 'configure' ? { app_id: Number(appID), app_slug: slug.trim(), organization_id: Number(orgID), organization: organization.trim(), template_repository: template.trim() } : {}), ...(action === 'configure' ? { private_key: privateKey } : {}), ...(action !== 'disconnect' && webhook ? { webhook_secret: webhook } : {}) } })
  }
  function correct() { setReview(undefined); requestAnimationFrame(() => { if (opener.current?.isConnected && !(opener.current instanceof HTMLButtonElement && opener.current.disabled)) opener.current.focus(); else readControl.current?.focus() }) }
  async function send() {
    if (!review || stale || !event.can_edit || owner.paused) return
    const intent = review
    setReview(undefined); setPrivateKey(''); setWebhook(''); setNotice('')
    if (intent.command === 'install') await operation.send('installation authorisation', intent.revision, async () => { const result = unwrap(await api.POST('/api/v1/events/{event_id}/github-app/installation-authorizations', { cache: 'no-store', params: { path: { event_id: event.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${intent.revision}"` } } })); const url = new URL(result.url); if (url.origin !== 'https://github.com' || !!url.username || !!url.password || !intent.configuration || url.pathname !== `/apps/${encodeURIComponent(intent.configuration.app_slug)}/installations/new`) throw new Error('Unexpected installation destination.'); return result }, result => { settleOwnDraft(); window.location.assign(result.url) })
    else await operation.send(`App ${intent.command.action}`, intent.revision, async () => unwrap(await api.POST('/api/v1/events/{event_id}/github-app', { cache: 'no-store', params: { path: { event_id: event.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${intent.revision}"` } }, body: intent.command as S['GitHubAppCommand'] })), result => { setAction('configure'); setAppID(result.configuration?.app_id.toString() ?? ''); setSlug(result.configuration?.app_slug ?? ''); setOrgID(result.configuration?.organization_id.toString() ?? ''); setOrganization(result.configuration?.organization ?? ''); setTemplate(result.configuration?.template_repository ?? ''); setAttempted(false); setEditing(!result.configured); setNotice(intent.command !== 'install' && intent.command.action === 'configure' ? 'The App configuration request was acknowledged. Next, install the saved App on GitHub, then check readiness.' : 'The App configuration request was acknowledged. Check current App readiness below.'); if (intent.command !== 'install' && intent.command.action === 'configure') requestAnimationFrame(() => installationHeading.current?.focus()); void owner.query.refetch() })
  }
  return <div className="fleet-repository-section"><ol className="fleet-setup-steps" aria-label="GitHub connection steps">
      <li aria-current={!value.configured ? 'step' : undefined}><strong>1. App details</strong><span>{value.configured ? 'Saved' : 'Enter and review'}</span></li>
      <li aria-current={value.configured && !value.installation_id ? 'step' : undefined}><strong>2. GitHub installation</strong><span>{value.installation_id ? 'Installation recorded' : 'Approve on GitHub'}</span></li>
      <li aria-current={!!value.installation_id && value.diagnostics.code !== 'healthy' ? 'step' : undefined}><strong>3. Check connection</strong><span>{value.diagnostics.code === 'healthy' ? 'Ready' : 'Verify access and template'}</span></li>
    </ol>
    {event.can_edit && <><div hidden={editing || !!review}>
      <h3>1. Saved App details</h3><p>{value.configuration?.app_slug} · {value.configuration?.organization}/{value.configuration?.template_repository}</p>
      <Button variant="outline" icon="repository" disabled={operation.blocked || owner.paused} onClick={() => { setEditing(true); requestAnimationFrame(() => editor.current?.querySelector('select')?.focus()) }}>Change App details</Button>
    </div><form ref={editor} noValidate hidden={!!review || !editing} onSubmit={e => { e.preventDefault(); prepare((e.nativeEvent as SubmitEvent).submitter ?? e.currentTarget.querySelector<HTMLButtonElement>('button[type=submit]')!) }} className="fleet-repository-fields"><fieldset disabled={operation.blocked || owner.paused}><legend>1. App details</legend><Field label="App setup action" description="Changing action clears entered secrets. Re-encryption keeps the GitHub key and installation unchanged."><Select value={action} onChange={e => { setAction(e.target.value as typeof action); setPrivateKey(''); setWebhook(''); setAttempted(false) }}><option value="configure">Configure organisation and template</option><option value="rotate" disabled={!value.configured}>Re-encrypt stored App credentials</option><option value="disconnect" disabled={!value.configured}>Disconnect App configuration</option></Select></Field>{action === 'configure' && <GitHubSetupFields key={value.revision} values={setupValues} attempted={attempted} onChange={(name, next) => setupSetters[name](next)} />}{action !== 'disconnect' && <><Field label="Optional replacement webhook secret" description="Leave empty to retain the current secret for the same App."><Input type="password" autoComplete="new-password" value={webhook} onChange={e => setWebhook(e.target.value)} /></Field></>}<Button type="submit" icon="next" disabled={action !== 'configure' && !value.configured}>Review App setup change</Button></fieldset></form>
    {notice && <p role="status">{notice}</p>}
    <section hidden={!!review} className="fleet-repository-section" aria-label="GitHub installation">
      <h3 ref={installationHeading} tabIndex={-1}>2. Install on GitHub</h3>
      <p>{value.configured ? 'Install the saved App in your organisation and select the template repository. GitHub will return you here.' : 'Save your App details above to enable installation.'}</p>
      <Button icon="external" disabled={!value.configured || operation.blocked || owner.paused} onClick={e => prepare(e.currentTarget, true)}>Review organisation installation</Button>
    </section>
    {review && <RepositoryActionReview correctDisabled={operation.history.unconfirmed} title="Review App setup change" pending={operation.pending} disabled={operation.history.unconfirmed || stale || owner.paused} correct={correct} confirm={send} confirmLabel={review.command === 'install' ? 'Continue to GitHub installation' : 'Confirm App setup change'}><p>Event {event.name} · {event.id} · Revision {review.revision}</p>{review.command === 'install' ? <><p>Install saved App {review.configuration?.app_slug} · ID {review.configuration?.app_id} for {review.configuration?.organization} · Organisation ID {review.configuration?.organization_id}.</p><p>Leave this app for GitHub, then return through the verified installation callback. Unsent setup fields are not applied or forwarded. This does not link a competitor’s personal GitHub account or provision repositories. Reloading this app clears its in-memory draft and unconfirmed-command history.</p></> : <><p>Action: {review.command.action}</p>{review.command.action === 'configure' && <p>App {review.command.app_slug} · ID {review.command.app_id} · Organisation {review.command.organization} · ID {review.command.organization_id} · Template {review.command.template_repository}</p>}<p>{review.command.action === 'configure' ? 'Configuration resets the installation and pending attempts. Existing repositories are retained.' : review.command.action === 'rotate' ? 'Re-encrypt the stored App credentials. This does not replace the GitHub key or change the installation. Existing repositories are retained.' : 'Disconnect removes App configuration only. Existing repositories and access are retained.'}</p>{review.command.action !== 'disconnect' && <p>{review.command.action === 'configure' ? 'Private key supplied; value hidden.' : 'No private key is read or sent for re-encryption.'} Webhook secret {review.command.webhook_secret ? 'replacement supplied; value hidden' : 'unchanged for the same App'}.</p>}</>}{stale && <p>Current App facts changed. Correct details and review the current base.</p>}</RepositoryActionReview>}</>}
    <section className="fleet-repository-section" aria-label="Connection readiness">
      <h3>Current App readiness</h3><p role="status">{repositoryAppExplanation(value.diagnostics.code)}</p><p>Installation {value.installation_id || 'not connected'} · Webhook {value.webhook_configured ? 'configured' : 'not configured'}</p>{value.configuration && <dl className="fleet-repository-facts"><div><dt>Organisation</dt><dd>{value.configuration.organization} · ID {value.configuration.organization_id}</dd></div><div><dt>App</dt><dd>{value.configuration.app_slug} · ID {value.configuration.app_id}</dd></div><div><dt>Repository template</dt><dd>{value.configuration.template_repository}</dd></div></dl>}<ul>{value.diagnostics.checks.map(check => <li key={check.operation}>{check.operation.replaceAll('_', ' ')} · {check.allowed ? 'Allowed' : 'Missing permission'} · Required: {check.required.join(', ')}</li>)}</ul><Button ref={readControl} icon="refresh" variant="outline" onClick={() => void owner.query.refetch()}>Check current App readiness</Button>
      <p>Check this after returning from GitHub. Field format checks do not verify App permissions or repository access.</p>
    </section>
    <RepositoryCommandStatus command={operation} />
  </div>
}

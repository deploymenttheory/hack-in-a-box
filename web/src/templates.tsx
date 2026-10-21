import { useState } from 'react'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useBlocker, useNavigate, useParams } from '@tanstack/react-router'
import { api, APIError, privateKey, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { DefinitionEditor, TextField } from './definition-editor'
import { brandingErrors } from './branding'

type Template = components['schemas']['Template']
function differences(before: unknown, after: unknown, path = ''): string[] {
  if (JSON.stringify(before) === JSON.stringify(after)) return []
  if (before && after && typeof before === 'object' && typeof after === 'object') {
    const a = before as Record<string, unknown>, b = after as Record<string, unknown>
    return [...new Set([...Object.keys(a), ...Object.keys(b)])].filter(k => k !== 'id').flatMap(k => differences(a[k], b[k], `${path}${path ? ' / ' : ''}${k.replaceAll('_', ' ')}`))
  }
  const display = (v: unknown) => typeof v === 'object' ? 'definition' : String(v ?? 'not set').slice(0, 100)
  return [path.endsWith('content') ? `${path}: image changed` : `${path}: ${display(before)} → ${display(after)}`]
}
export function TemplatesPage() {
  const identity = useIdentity(), navigate = useNavigate(), cache = useQueryClient()
  const [name, setName] = useState(''), [selected, setSelected] = useState('all-team'), [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>()
  const [id] = useState(() => crypto.randomUUID())
  const starters = useQuery({ queryKey: privateKey(identity.person.id, 'templates', 'starters'), queryFn: async () => unwrap(await api.GET('/api/v1/template-starters')), enabled: identity.can_create_events })
  const templates = useInfiniteQuery({ queryKey: privateKey(identity.person.id, 'templates', 'list'), initialPageParam: undefined as string | undefined, queryFn: async ({ pageParam }) => unwrap(await api.GET('/api/v1/templates', { params: { query: { after: pageParam, limit: 25 } } })), getNextPageParam: page => page.next_cursor ?? undefined })
  const starter = starters.data?.items.find(s => s.key === selected)
  async function create() {
    if (!starter) return
    setPending(true); setFailure(undefined)
    try {
      const created = unwrap(await api.POST('/api/v1/templates', { params: { header: { 'X-CSRF-Token': identity.csrf_token } }, body: { id, name: name.trim() || starter.name, configuration: starter.configuration } }))
      await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, 'templates', 'list') })
      await navigate({ to: '/templates/$templateId', params: { templateId: created.id } })
    } catch (error) { setFailure(error) } finally { setPending(false) }
  }
  return <section className="workspace"><Link to="/">← Your events</Link><div className="page-heading"><div><p className="eyebrow">Reusable processes</p><h1>Event templates</h1><p>Publish a reusable version, then create independent draft events.</p></div></div>
    {identity.can_create_events && <form className="panel" onSubmit={e => { e.preventDefault(); void create() }}><h2>Start a template</h2>{starters.isPending ? <Loading /> : starters.error ? <Failure error={starters.error} /> : <><label htmlFor="starter">Starter process</label><select id="starter" value={selected} onChange={e => setSelected(e.target.value)}>{starters.data?.items.map(s => <option key={s.key} value={s.key}>{s.name}</option>)}</select><TextField label="Template name" value={name} change={setName} />{starter && <p>{starter.configuration.event.description}</p>}<p>Starters contain generic examples. Review dates, responsibilities and judging guidance before using them for an event.</p><Button type="submit" disabled={pending || !starter}>Create template draft</Button></>}{!!failure && <Failure error={failure} />}</form>}
    <h2 className="section-heading">Your templates</h2>{templates.isPending ? <Loading /> : templates.error ? <Failure error={templates.error} /> : <><div className="event-grid">{templates.data.pages.flatMap(p => p.items).map(t => <Link className="event-card" key={t.id} to="/templates/$templateId" params={{ templateId: t.id }}><h3>{t.name}</h3><p>Latest version {t.latest_version} · {t.published_version ? `Published version ${t.published_version}` : 'Unpublished draft'}</p></Link>)}</div>{!templates.data.pages[0].items.length && <p>No templates yet.</p>}{templates.hasNextPage && <Button onClick={() => void templates.fetchNextPage()}>More templates</Button>}</>}
  </section>
}
export function TemplatePage() {
  const { templateId } = useParams({ from: '/templates/$templateId' }), identity = useIdentity()
  const query = useQuery({ queryKey: privateKey(identity.person.id, templateId, 'template'), queryFn: async () => unwrap(await api.GET('/api/v1/templates/{template_id}', { params: { path: { template_id: templateId } } })) })
  if (query.isPending) return <Loading />
  if (query.error) return <Failure error={query.error} />
  return <TemplateEditor key={templateId} initial={query.data} />
}
function TemplateEditor({ initial }: { initial: Template }) {
  const identity = useIdentity(), cache = useQueryClient()
  const [saved, setSaved] = useState(initial), [name, setName] = useState(initial.name), [bundle, setBundle] = useState(initial.configuration)
  const [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState('')
  const dirty = name !== saved.name || JSON.stringify(bundle) !== JSON.stringify(saved.configuration)
  const changes = differences({ name: saved.name, ...saved.configuration }, { name, ...bundle })
  useBlocker({ shouldBlockFn: () => dirty && !window.confirm('Leave this template with unsaved changes?'), enableBeforeUnload: dirty })
  const published = useQuery({ queryKey: [...privateKey(identity.person.id, saved.id, 'published-template'), saved.published_version], enabled: !!saved.published_version, queryFn: async () => unwrap(await api.GET('/api/v1/templates/{template_id}', { params: { path: { template_id: saved.id }, query: { version: saved.published_version! } } })) })
  async function mutate(publish: boolean) {
    setPending(true); setFailure(undefined); setNotice('')
    try {
      const params = { path: { template_id: saved.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${saved.revision}"` } }
      const result = publish ? unwrap(await api.POST('/api/v1/templates/{template_id}/publish', { params, body: { version: saved.latest_version } })) : unwrap(await api.PUT('/api/v1/templates/{template_id}', { params, body: { id: saved.id, name, configuration: bundle } }))
      setSaved(result); setName(result.name); setBundle(result.configuration); setNotice(publish ? `Version ${result.version} published. Existing events are unchanged.` : `Version ${result.version} saved as a draft.`)
      cache.setQueryData(privateKey(identity.person.id, saved.id, 'template'), result)
      await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, 'templates', 'list') })
    } catch (error) { setFailure(error) } finally { setPending(false) }
  }
  return <section className="workspace"><Link to="/templates">← Event templates</Link><div className="page-heading"><div><p className="eyebrow">Version {saved.latest_version} · {saved.published_version ? `Published ${saved.published_version}` : 'Unpublished'}</p><h1>{saved.name}</h1></div></div><form className="setup-form" onSubmit={e => { e.preventDefault(); void mutate(false) }}><TextField label="Template name" value={name} change={setName} /><DefinitionEditor value={bundle.definitions} change={definitions => setBundle({ ...bundle, definitions })} configuration={bundle.event} changeConfiguration={event => setBundle({ ...bundle, event })} />
    <section className="panel"><h2>Review changes</h2>{changes.length ? <ul className="change-list">{changes.map((change, i) => <li key={i}>{change}</li>)}</ul> : <p>No unsaved changes.</p>}<div className="actions"><Button type="submit" disabled={pending || !dirty || !name.trim() || !!brandingErrors(bundle.definitions.branding).length}>Save new draft version</Button><Button type="button" variant="outline" disabled={pending || dirty || saved.published_version === saved.latest_version} onClick={() => void mutate(true)}>Publish version {saved.latest_version}</Button></div><p>Saving creates an immutable version. Publishing makes that version available for new events. Existing events retain their independent copies.</p>{notice && <p role="status">{notice}</p>}{!!failure && <Failure error={failure} />}{failure instanceof APIError && failure.blockers.map(b => <p role="alert" key={b.field}>{b.message}</p>)}</section>
    </form>
    {published.error && <Failure error={published.error} />}{published.data && <section className="panel section-heading"><h2>Compared with published version {published.data.version}</h2><ul className="change-list">{differences(published.data.configuration, bundle).map((change, i) => <li key={i}>{change}</li>)}</ul>{!differences(published.data.configuration, bundle).length && <p>Definitions match the published version.</p>}</section>}
    {saved.published_version && <CloneTemplate template={saved} disabled={dirty || pending} />}
  </section>
}
function CloneTemplate({ template, disabled }: { template: Template; disabled: boolean }) {
  const identity = useIdentity(), navigate = useNavigate(), cache = useQueryClient()
  const [id] = useState(() => crypto.randomUUID()), [name, setName] = useState(''), [slug, setSlug] = useState(''), [start, setStart] = useState(''), [timezone, setTimezone] = useState(template.configuration.event.timezone), [dst, setDST] = useState<'' | 'earlier' | 'later'>(''), [ack, setAck] = useState(false)
  const [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>()
  async function clone() {
    setPending(true); setFailure(undefined)
    try {
      const resolved = unwrap(await api.POST('/api/v1/time/resolve', { params: { header: { 'X-CSRF-Token': identity.csrf_token } }, body: { wall_time: start.length === 16 ? start + ':00' : start, timezone, dst_choice: dst } }))
      const created = unwrap(await api.POST('/api/v1/templates/{template_id}/clone', { params: { path: { template_id: template.id }, header: { 'X-CSRF-Token': identity.csrf_token } }, body: { id, name, slug, live_start: resolved.instant, timezone, dst_choice: dst, version: template.published_version!, acknowledge_competition_restriction: ack } }))
      await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, 'directory', 'events') })
      await navigate({ to: '/events/$eventId', params: { eventId: created.id } })
    } catch (error) { setFailure(error) } finally { setPending(false) }
  }
  return <form className="panel section-heading" onSubmit={e => { e.preventDefault(); void clone() }}><h2>Create an event from published version {template.published_version}</h2><p>Only reusable definitions are copied. Attendees, assignments, assessments, credentials and delivery history are excluded.</p><TextField label="New event name" value={name} change={setName} /><TextField label="New event slug" value={slug} change={setSlug} /><TextField label="Event timezone" value={timezone} change={setTimezone} /><label htmlFor="clone-start">Live start in event timezone</label><input id="clone-start" type="datetime-local" required value={start} onChange={e => setStart(e.target.value)} /><label htmlFor="clone-dst">Repeated local time</label><select id="clone-dst" value={dst} onChange={e => setDST(e.target.value as typeof dst)}><option value="">Not ambiguous</option><option value="earlier">Earlier occurrence</option><option value="later">Later occurrence</option></select><label className="check"><input type="checkbox" checked={ack} onChange={e => setAck(e.target.checked)} />I understand that becoming this event's organiser permanently prevents me from competing in it.</label><Button type="submit" disabled={disabled || pending || !ack || !name.trim() || !slug.trim() || !start}>Create independent event draft</Button>{!!failure && <Failure error={failure} />}{failure instanceof APIError && failure.blockers.map(b => <p role="alert" key={b.field}>{b.message}</p>)}</form>
}

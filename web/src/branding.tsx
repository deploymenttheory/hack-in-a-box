import { PlanningEditorSections, PlanningStep } from './planning-layout'
import { Input, Label } from './components/ui/fields'
import { useEffect, useState, type ReactNode } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useBlocker } from '@tanstack/react-router'
import { api, APIError, privateKey, unwrap, type EventRecord } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Theme } from './components/theme'
import { Failure, Loading } from './components/states'
import { Button } from './components/ui/button'

export { brandAsset, brandStyle, brandingErrors, contrast, defaultBranding } from './design-system/palette'
export type { Branding } from './design-system/palette'
import { brandAsset, brandStyle, brandingErrors, type Branding } from './design-system/palette'

function useBrand(eventID: string) {
  const identity = useIdentity()
  return useQuery({ queryKey: privateKey(identity.person.id, eventID, 'branding'), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/branding', { params: { path: { event_id: eventID } } })), refetchOnWindowFocus: 'always' })
}
function EventIcon({ branding }: { branding: Branding }) {
  const href = brandAsset(branding, 'icon')
  useEffect(() => {
    if (!href) return
    const link = document.createElement('link'); link.rel = 'icon'; link.href = href; document.head.append(link)
    return () => link.remove()
  }, [href])
  return null
}
export function EventBrand({ eventID, children }: { eventID: string; children: ReactNode }) {
  const query = useBrand(eventID)
  if (query.isPending) return <Loading />
  if (!query.data || (query.error instanceof APIError && [401,403,404].includes(query.error.status))) return <Failure error={query.error} retry={() => void query.refetch()} />
  const branding = query.data.branding, logo = brandAsset(branding, 'logo'), hero = brandAsset(branding, 'hero')
  return <Theme style={brandStyle(branding)}><EventIcon branding={branding} /><div className="fleet-event-brand-assets">{logo && <img className="event-logo" src={logo} alt="Event logo" />}{hero && <details><summary>Event image</summary><img src={hero} alt="" /></details>}</div>{query.error && <div role="status"><p>Current event branding access could not be checked. Your unsaved inputs remain; changes are paused.</p><Button variant="outline" icon="refresh" onClick={()=>void query.refetch()}>Check event branding access</Button></div>}<fieldset className="fleet-planning-authority" disabled={!!query.error}>{children}</fieldset></Theme>
}

export function BrandFields({ value, onChange }: { value: Branding; onChange: (value: Branding) => void }) {
  const [failure, setFailure] = useState('')
  async function upload(kind: 'logo' | 'icon' | 'hero', file?: File) {
    if (!file) return
    setFailure('')
    if (!['image/png', 'image/jpeg'].includes(file.type) || file.size > 512 * 1024) { setFailure('Choose a PNG or JPEG at or below 512 KiB.'); return }
    try {
      const content = await new Promise<string>((resolve, reject) => { const reader = new FileReader(); reader.onload = () => resolve(String(reader.result).split(',')[1]); reader.onerror = reject; reader.readAsDataURL(file) })
      onChange({ ...value, assets: [...value.assets.filter(a => a.kind !== kind), { id: crypto.randomUUID(), kind, media_type: file.type as 'image/png' | 'image/jpeg', content }] })
    } catch { setFailure('The image could not be read. Choose it again.') }
  }
  return <><PlanningEditorSections label="Event branding sections"><PlanningStep title="Colours"><div className="form-grid">{(['primary', 'accent', 'background', 'foreground'] as const).map(field => <div key={field}><Label htmlFor={`brand-${field}`}>{field === 'foreground' ? 'Text colour' : field[0].toUpperCase() + field.slice(1) + ' colour'}</Label><Input id={`brand-${field}`} value={value[field]} onChange={e => onChange({ ...value, [field]: e.target.value })} maxLength={7} /></div>)}</div>
    </PlanningStep><PlanningStep title="Images"><p className="muted">PNG or JPEG, at most 512 KiB and 4096 pixels per side. Accent is decorative; text and controls use contrast-checked colours.</p>
    {(['logo', 'icon', 'hero'] as const).map(kind => <div key={kind}><Label htmlFor={`brand-${kind}`}>{kind[0].toUpperCase() + kind.slice(1)} image</Label><Input id={`brand-${kind}`} type="file" accept="image/png,image/jpeg" onChange={e => void upload(kind, e.target.files?.[0])} />{brandAsset(value, kind) && <div className="brand-image-preview"><img src={brandAsset(value, kind)} alt={`${kind} preview`} /><Button type="button" variant="outline" onClick={() => onChange({ ...value, assets: value.assets.filter(a => a.kind !== kind) })}>Remove {kind}</Button></div>}</div>)}
    {failure && <p role="alert">{failure}</p>}</PlanningStep><PlanningStep title="Preview">
    <Theme style={brandStyle(value)}><section className="fleet-brand-preview" aria-label="Branding preview"><h3>Your event preview</h3><p>Build something together. These colours apply after you save.</p><Button type="button" icon="check">Example action</Button><Label htmlFor="brand-preview-name">Example field</Label><Input id="brand-preview-name" placeholder="Event-local controls" /><Label className="check"><Input type="checkbox" />Example choice</Label></section></Theme></PlanningStep></PlanningEditorSections>{brandingErrors(value).map(error=><p className="field-error" role="alert" key={error}>{error}</p>)}</>
}
export function BrandingEditor({ event, onDirty }: { event: EventRecord; onDirty: (dirty: boolean) => void }) {
  const query = useBrand(event.id)
  if (query.isPending) return <Loading />
  if (query.error) return <Failure error={query.error} />
  return <BrandingForm key={event.id} event={event} initial={query.data} onDirty={onDirty} />
}
function BrandingForm({ event, initial, onDirty }: { event: EventRecord; initial: components['schemas']['EventBranding']; onDirty: (dirty: boolean) => void }) {
  const identity = useIdentity(), cache = useQueryClient()
  const [saved, setSaved] = useState(initial), [value, setValue] = useState(initial.branding)
  const [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState('')
  const dirty = JSON.stringify(value) !== JSON.stringify(saved.branding)
  useEffect(() => { onDirty(dirty); return () => onDirty(false) }, [dirty, onDirty])
  useBlocker({ shouldBlockFn: () => dirty && !window.confirm('Leave with unsaved branding changes?'), enableBeforeUnload: dirty })
  async function save() {
    setPending(true); setFailure(undefined); setNotice('')
    try {
      const result = unwrap(await api.PUT('/api/v1/events/{event_id}/branding', { params: { path: { event_id: event.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${saved.revision}"` } }, body: value }))
      setSaved(result); setValue(result.branding); setNotice('Branding saved.'); cache.setQueryData(privateKey(identity.person.id, event.id, 'branding'), result)
      await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, event.id, 'event') })
    } catch (error) { setFailure(error) } finally { setPending(false) }
  }
  return <form className="fleet-planning-form" onSubmit={e => { e.preventDefault(); void save() }}><h2>Event branding</h2><BrandFields value={value} onChange={setValue} /><div className="actions"><Button type="submit" disabled={pending || !dirty || !!brandingErrors(value).length}>Save branding</Button><Button type="button" variant="outline" onClick={() => { setValue(initial.branding); setSaved(initial); setFailure(undefined) }}>Reset to saved branding</Button></div>{notice && <p role="status">{notice}</p>}{!!failure && <Failure error={failure} />}{failure instanceof APIError && failure.blockers.map(b => <p role="alert" key={b.field}>{b.message}</p>)}</form>
}

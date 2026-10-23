import { useEffect, useState, type CSSProperties, type ReactNode } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useBlocker } from '@tanstack/react-router'
import { api, APIError, privateKey, unwrap, type EventRecord } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Theme } from './components/theme'
import { Failure, Loading } from './components/states'
import { Button } from './components/ui/button'

export type Branding = components['schemas']['TemplateBranding']
export const defaultBranding: Branding = { primary: '#173a35', accent: '#d17c32', background: '#f8f9f5', foreground: '#173a35', assets: [] }
function luminance(hex: string) {
  return [0.2126, 0.7152, 0.0722].reduce((sum, weight, i) => { const n = parseInt(hex.slice(1 + i * 2, 3 + i * 2), 16) / 255; return sum + weight * (n <= 0.04045 ? n / 12.92 : ((n + 0.055) / 1.055) ** 2.4) }, 0)
}
export function contrast(a: string, b: string) { const x = luminance(a), y = luminance(b); return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05) }
export function brandingErrors(b: Branding) {
  if (![b.primary, b.accent, b.foreground, b.background].every(value => /^#[\da-f]{6}$/i.test(value))) return ['Use six-digit hexadecimal colours, for example #173a35.']
  return [contrast(b.foreground, b.background) < 4.5 && 'Text and background need contrast of at least 4.5:1.', contrast(b.primary, b.background) < 4.5 && 'Primary and background need contrast of at least 4.5:1.'].filter(Boolean) as string[]
}
function brandSurface(foreground: string, background: string, amount: number) {
  const mixed = '#' + [1, 3, 5].map(i => Math.round(parseInt(background.slice(i, i + 2), 16) * (1 - amount) + parseInt(foreground.slice(i, i + 2), 16) * amount).toString(16).padStart(2, '0')).join('')
  return contrast(foreground, mixed) >= 4.5 ? mixed : background
}
// Shared, bounded render inputs for web pages and future email/display adapters.
export function brandStyle(b: Branding): CSSProperties {
  const safe = brandingErrors(b).length ? defaultBranding : b
  const card = brandSurface(safe.foreground, safe.background, .025)
  const muted = brandSurface(safe.foreground, safe.background, .06)
  const destructive = ['#a32632', '#ffb4b5', safe.foreground].find(color => contrast(color, card) >= 4.5) ?? safe.foreground
  return {
    '--primary': safe.primary,
    '--primary-foreground': contrast('#ffffff', safe.primary) >= 4.5 ? '#ffffff' : '#000000',
    '--accent': safe.accent,
    '--background': safe.background,
    '--foreground': safe.foreground,
    '--card': card,
    '--muted': muted,
    '--accent-surface': brandSurface(safe.foreground, safe.background, .1),
    '--muted-foreground': safe.foreground,
    '--input': safe.foreground,
    '--destructive': destructive,
    '--destructive-foreground': contrast('#ffffff', destructive) >= 4.5 ? '#ffffff' : '#000000',
    '--destructive-surface': card,
    '--warning': safe.foreground,
    '--warning-surface': muted,
    '--success': safe.foreground,
    colorScheme: luminance(safe.background) < .18 ? 'dark' : 'light',
  } as CSSProperties
}
export function brandAsset(b: Branding, kind: 'logo' | 'icon' | 'hero') {
  const a = b.assets.find(asset => asset.kind === kind)
  return a && ['image/png', 'image/jpeg'].includes(a.media_type) && /^[A-Za-z0-9+/=]+$/.test(a.content) ? `data:${a.media_type};base64,${a.content}` : undefined
}
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
  if (query.error) return <Failure error={query.error} retry={() => void query.refetch()} />
  const branding = query.data.branding, logo = brandAsset(branding, 'logo'), hero = brandAsset(branding, 'hero')
  return <Theme style={brandStyle(branding)}><EventIcon branding={branding} /><div className="event-brand-assets">{logo && <img className="event-logo" src={logo} alt="Event logo" />}{hero && <img className="event-hero" src={hero} alt="" />}</div>{children}</Theme>
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
  return <><div className="form-grid">{(['primary', 'accent', 'background', 'foreground'] as const).map(field => <div key={field}><label htmlFor={`brand-${field}`}>{field === 'foreground' ? 'Text colour' : field[0].toUpperCase() + field.slice(1) + ' colour'}</label><input id={`brand-${field}`} value={value[field]} onChange={e => onChange({ ...value, [field]: e.target.value })} maxLength={7} /></div>)}</div>
    <p className="muted">PNG or JPEG, at most 512 KiB and 4096 pixels per side. Accent is decorative; text and controls use contrast-checked colours.</p>
    {(['logo', 'icon', 'hero'] as const).map(kind => <div key={kind}><label htmlFor={`brand-${kind}`}>{kind[0].toUpperCase() + kind.slice(1)} image</label><input id={`brand-${kind}`} type="file" accept="image/png,image/jpeg" onChange={e => void upload(kind, e.target.files?.[0])} />{brandAsset(value, kind) && <div className="brand-image-preview"><img src={brandAsset(value, kind)} alt={`${kind} preview`} /><Button type="button" variant="outline" onClick={() => onChange({ ...value, assets: value.assets.filter(a => a.kind !== kind) })}>Remove {kind}</Button></div>}</div>)}
    {failure && <p role="alert">{failure}</p>}{brandingErrors(value).map(error => <p className="field-error" role="alert" key={error}>{error}</p>)}
    <section className="brand-preview theme" style={brandStyle(value)} aria-label="Branding preview"><h3>Your event preview</h3><p>Build something together. These colours apply after you save.</p><Button type="button">Example action</Button></section></>
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
  return <form className="panel" onSubmit={e => { e.preventDefault(); void save() }}><h2>Event branding</h2><BrandFields value={value} onChange={setValue} /><div className="actions"><Button type="submit" disabled={pending || !dirty || !!brandingErrors(value).length}>Save branding</Button><Button type="button" variant="outline" onClick={() => { setValue(initial.branding); setSaved(initial); setFailure(undefined) }}>Reset to saved branding</Button></div>{notice && <p role="status">{notice}</p>}{!!failure && <Failure error={failure} />}{failure instanceof APIError && failure.blockers.map(b => <p role="alert" key={b.field}>{b.message}</p>)}</form>
}

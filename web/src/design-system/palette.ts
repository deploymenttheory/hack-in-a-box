// Pure palette projection shared by product scopes and the static guide.
import type { CSSProperties } from 'react'
import type { components } from '../api/schema.gen'

export type Branding = components['schemas']['TemplateBranding']
export const defaultBranding: Branding = { primary: '#007e67', accent: '#007e67', background: '#ffffff', foreground: '#192147', assets: [] }
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
  const card = safe.background
  const muted = brandSurface(safe.foreground, safe.background, .06)
  const status = (colours: string[]) => colours.find(colour => [card, muted].every(surface => contrast(colour, surface) >= 4.5)) ?? safe.foreground
  const destructive = status(['#a33049', '#ffadc0'])
  const primaryForeground = contrast('#ffffff', safe.primary) >= 4.5 ? '#ffffff' : '#000000'
  return {
    '--primary': safe.primary,
    '--primary-foreground': primaryForeground,
    '--primary-hover': safe.primary,
    '--link': safe.primary,
    '--ring': safe.primary,
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
    '--warning': status(['#885900', '#f2cf83']),
    '--warning-surface': muted,
    '--success': status(['#147250', '#78d7b1']),
    colorScheme: luminance(safe.background) < .18 ? 'dark' : 'light',
  } as CSSProperties
}
export function brandAsset(b: Branding, kind: 'logo' | 'icon' | 'hero') {
  const a = b.assets.find(asset => asset.kind === kind)
  return a && ['image/png', 'image/jpeg'].includes(a.media_type) && /^[A-Za-z0-9+/=]+$/.test(a.content) ? `data:${a.media_type};base64,${a.content}` : undefined
}

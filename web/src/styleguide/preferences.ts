import { defaultBranding, type Branding } from '../design-system/palette'
export type Mode = 'system' | 'light' | 'dark'
export type Palette = 'platform' | 'light' | 'dark' | 'threshold'
export type Density = 'comfortable' | 'compact'
export type PreviewOptions = { kind: 'guide-preview-options'; mode: Mode; palette: Palette; density: Density; resetKey: number; portal: 'admin' | 'participant' }
export const palettes: Record<Exclude<Palette, 'platform'>, Branding> = {
  light: { ...defaultBranding, primary: '#233c78', foreground: '#283b51', background: '#fff8e9', accent: '#ffedbb' },
  dark: { ...defaultBranding, primary: '#b1d2ff', foreground: '#eaf1ff', background: '#172238', accent: '#160f30' },
  threshold: { ...defaultBranding, primary: '#767676', foreground: '#767676', background: '#ffffff' },
}
export function isPreviewOptions(value: unknown): value is PreviewOptions {
  if (!value || typeof value !== 'object') return false
  const v = value as Record<string, unknown>
  return v.kind === 'guide-preview-options' && typeof v.mode === 'string' && ['system', 'light', 'dark'].includes(v.mode) && typeof v.palette === 'string' && ['platform', 'light', 'dark', 'threshold'].includes(v.palette) && typeof v.density === 'string' && ['comfortable', 'compact'].includes(v.density) && (v.portal === 'admin' || v.portal === 'participant') && typeof v.resetKey === 'number' && Number.isSafeInteger(v.resetKey) && v.resetKey >= 0
}

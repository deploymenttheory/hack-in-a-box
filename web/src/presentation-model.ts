import type { components } from './api/schema.gen'
export type Cue = components['schemas']['PresentationCue']
export type ConsolePage = components['schemas']['PresentationPage']
export type CueCommand = components['schemas']['PresentationCommand']
export function emptyCue(): Cue { return { id: crypto.randomUUID(), title: '', text: '', url: '', duration_seconds: 120, content_id: null } }
export function cueIssue(cue: Cue) {
  if (!cue.title.trim() || cue.title.length > 200 || cue.text.length > 4000) return 'Give every cue a title and keep its text within the stated limits.'
  if (!Number.isInteger(cue.duration_seconds) || cue.duration_seconds < 0 || cue.duration_seconds > 86400) return 'Countdowns must be whole seconds between 0 and 86400.'
  if (cue.url) {
    try { const url = new URL(cue.url); if (url.protocol !== 'https:' || !url.hostname || url.username || url.password || cue.url.length > 2048) return 'Cue links must be HTTPS URLs without an embedded username or password.' } catch { return 'Enter a complete HTTPS cue link.' }
  }
}
export function countdown(seconds: number) { const value = Math.max(0, Math.ceil(seconds)); return `${Math.floor(value / 60).toString().padStart(2, '0')}:${(value % 60).toString().padStart(2, '0')}` }

import type { Cue } from './presentation-model'
// Read-only production cue presentation. No account, API or command owner.
export function CuePreview({ cue, label }: { cue?: Cue | null; label: string }) {
  return <section className="fleet-cue-preview" aria-label={label}><p className="fleet-scope-label">{label}</p><h3>{cue?.title || 'No cue selected'}</h3>{cue && <><p className="whitespace-pre-wrap break-words">{cue.text}</p>{cue.url && <p className="break-all text-sm">{cue.url}</p>}<p className="text-sm text-muted-foreground">{cue.duration_seconds} seconds</p>{cue.content_id && <p>The room withholds this linked cue until its resource is released for the full audience.</p>}</>}</section>
}

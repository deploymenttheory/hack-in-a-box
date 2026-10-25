import { z } from 'zod'
import type { components } from './api/schema.gen'
// Two cues plus the latest finalist and award releases are bounded by the
// native limits:100 finalists,100 reserves,50 awards and160-byte names.
// Go HTML escaping expands each name byte to at most6 characters. The measured
// full legal frame is below512 KiB; keep this ceiling and reject larger frames.
const maximumFrameCharacters = 512 * 1024
export type Snapshot = components['schemas']['DisplaySnapshot']
const cue = z.object({ id: z.uuid(), title: z.string().max(200), text: z.string().max(4000), url: z.string().max(2048), content_title: z.string().max(200) })
const namedTeam = z.object({id:z.uuid(),name:z.string().min(1).max(160)}).strict()
const announcedAward = z.object({prize_id:z.uuid(),name:z.string().min(1).max(160),category_id:z.uuid().nullable(),team:namedTeam,amount:z.string().max(64).regex(/^(?:[0-9]+(?:\.[0-9]+)?)?$/),currency:z.string().regex(/^(?:[A-Z]{3})?$/)}).strict().refine(a=>(a.amount==='')===(a.currency===''))
const announcement = z.object({id:z.uuid(),kind:z.enum(['finalists','awards']),version:z.number().int().positive(),finalists:z.array(namedTeam).max(100),reserves:z.array(namedTeam).max(100),awards:z.array(announcedAward).max(50),published_at:z.iso.datetime({offset:true})}).strict()
const results = z.array(announcement).max(2).refine(items=>new Set(items.map(i=>i.kind)).size===items.length).default([])
const snapshot = z.object({ results, event_id: z.uuid(), event_name: z.string(), timezone: z.string(), sequence: z.number().int(), server_now: z.iso.datetime({ offset: true }), current: cue.nullable(), next: cue.nullable(), mode: z.enum(['held', 'running']), remaining_seconds: z.number().int().nonnegative().max(86400), timer_ends_at: z.iso.datetime({ offset: true }).nullable(), primary_colour: z.string(), background_colour: z.string(), foreground_colour: z.string(), override_active: z.boolean(), assets: z.array(z.object({ id: z.uuid(), kind: z.enum(['logo','icon','hero']), media_type: z.enum(['image/png','image/jpeg']), digest: z.string().regex(/^[a-f0-9]{64}$/) })).max(3) })
export function parseSnapshot(value: unknown, eventID: string): Snapshot {
  const safe = snapshot.parse(value)
  if (safe.event_id !== eventID) throw new Error('wrong_display_event')
  return safe
}
export function safeDestination(value: string) {
  try { const url = new URL(value); return url.protocol === 'https:' && !url.username && !url.password ? url.href : undefined } catch { return undefined }
}
export async function consumeFrames(response: Response, signal: AbortSignal, receive: (event: string, data: string) => void) {
  if (!response.body || !response.headers.get('content-type')?.startsWith('text/event-stream')) throw new Error('invalid_display_stream')
  const reader = response.body.getReader(), decoder = new TextDecoder()
  let buffer = ''
  try {
    while (!signal.aborted) {
      const { done, value } = await reader.read()
      if (done) break
      buffer = (buffer + decoder.decode(value, { stream: true })).replaceAll('\r\n','\n')
      let boundary: number
      while ((boundary = buffer.indexOf('\n\n')) >= 0) {
        if (boundary > maximumFrameCharacters) throw new Error('oversized_display_frame')
        const frame = buffer.slice(0,boundary); buffer = buffer.slice(boundary+2)
        let event = 'message'; const data: string[] = []
        for (const line of frame.split('\n')) { if (line.startsWith('event:')) event = line.slice(6).trim(); if (line.startsWith('data:')) data.push(line.slice(5).trimStart()) }
        if (data.length) receive(event,data.join('\n'))
      }
      if (buffer.length > maximumFrameCharacters) throw new Error('oversized_display_frame')
    }
  } finally { await reader.cancel().catch(() => undefined); reader.releaseLock() }
}

import type { CSSProperties, ReactNode } from 'react'
import { QRCodeSVG } from 'qrcode.react'
import { brandStyle, contrast, defaultBranding } from './design-system/palette'
import { safeDestination, type Snapshot } from './room-stream'
import { Button } from './components/ui/button'
// Only a complete allowlisted room projection enters this presentation. The
// adapter owns credentials, clock offset, revocation, streams and asset URLs.
export function RoomCanvas({ snapshot, serverNow, connection, assets, fullscreen, disconnect }: {
  snapshot: Snapshot; serverNow: number; connection: 'connecting' | 'connected' | 'reconnecting'; assets?: ReactNode; fullscreen: () => void; disconnect: () => void
}) {
  const safeColour = (value: string) => /^#[0-9a-f]{6}$/i.test(value)
  const readable = [snapshot.background_colour,snapshot.foreground_colour,snapshot.primary_colour].every(safeColour) && contrast(snapshot.foreground_colour,snapshot.background_colour)>=4.5 && contrast(snapshot.primary_colour,snapshot.background_colour)>=4.5
  const palette = readable ? { ...defaultBranding, primary: snapshot.primary_colour, background: snapshot.background_colour, foreground: snapshot.foreground_colour } : defaultBranding
  const style = { ...brandStyle(palette), '--room-background': palette.background, '--room-foreground': palette.foreground, '--room-primary': palette.primary } as CSSProperties
  const remaining = snapshot.mode === 'running' && snapshot.timer_ends_at ? Math.max(0,Math.ceil((Date.parse(snapshot.timer_ends_at)-(serverNow))/1000)) : snapshot.remaining_seconds
  const time = `${Math.floor(remaining/60).toString().padStart(2,'0')}:${(remaining%60).toString().padStart(2,'0')}`
  const destination = safeDestination(snapshot.current?.url ?? '')
  return <main className="room-display" style={style}><header>{assets}<p className="room-event">{snapshot.event_name}</p><div className="room-controls"><Button icon="external" variant="outline" onClick={fullscreen}>Fullscreen</Button><Button icon="close" variant="outline" onClick={disconnect}>Disconnect</Button></div></header>
    <div className="room-live-status" role="status">{connection === 'connected' ? 'Live room feed' : 'Connection lost · reconnecting · showing last-known room state'} · Last update {new Intl.DateTimeFormat('en-GB',{timeZone:snapshot.timezone,dateStyle:'medium',timeStyle:'medium'}).format(new Date(snapshot.server_now))}</div>
    <section className="room-current"><div className="room-copy"><p className="room-kicker">{snapshot.override_active ? 'Urgent announcement' : 'Now'}</p><h1>{snapshot.current?.title ?? 'Waiting for the MC'}</h1><p className="room-instructions">{snapshot.current?.text ?? 'The next approved activity will appear here.'}</p>{snapshot.current?.content_title && <p className="room-content">{snapshot.current.content_title}</p>}</div><aside><div className="room-countdown" aria-label={`Countdown ${time}`}>{time}</div><p>{snapshot.mode === 'held' ? 'Timer held' : remaining === 0 ? 'Time complete' : 'Time remaining'}</p>{destination && <div className="room-qr"><p className="room-call-to-action">Scan or open this link</p><QRCodeSVG value={destination} size={320} marginSize={4} level="M" title="Scan to open the displayed link" /><a href={destination} target="_blank" rel="noopener noreferrer">{destination}</a></div>}</aside></section>
    <footer><span>Next</span><strong>{snapshot.next?.title ?? 'Await the MC’s next instruction'}</strong></footer>
  </main>
}

import { useEffect, useLayoutEffect, useMemo, useState } from 'react'
import { consumeFrames, parseSnapshot, type Snapshot } from './room-stream'
import { RoomCanvas } from './room-canvas'
import { Theme } from './components/theme'
import { Button } from './components/ui/button'

type Connection = 'connecting' | 'connected' | 'reconnecting'
type Received = { snapshot: Snapshot; offset: number; receivedAt: number }
function now() { return Date.now() }
export function App() {
  const eventID = window.location.pathname.split('/')[2] ?? ''
  const [token, setToken] = useState(''), [session, setSession] = useState(''), [notice, setNotice] = useState('')
  return <Theme>{session ? <ConnectedRoom key={`${eventID}:${session}`} eventID={eventID} token={session} onEnd={() => { setSession(''); setNotice('This display pairing has ended. Ask the MC for a new pairing.')}} /> : <main className="display-pairing"><h1>Connect a room display</h1><p>Ask the MC for this event's temporary display key. Keep the key private.</p><form onSubmit={e => { e.preventDefault(); if (/^[0-9a-f-]{36}$/i.test(eventID) && token.trim()) { setSession(token.trim()); setToken(''); setNotice('') } else setNotice('Open the event display address supplied by the MC and enter its key.') }}><label htmlFor="display-key">Display key</label><input id="display-key" type="password" autoComplete="off" value={token} onChange={e => setToken(e.target.value)} maxLength={200} /><Button type="submit">Connect display</Button></form>{notice && <p role="status">{notice}</p>}</main>}</Theme>
}
function ConnectedRoom({ eventID, token, onEnd }: { eventID: string; token: string; onEnd: () => void }) {
  const [received,setReceived] = useState<Received>(), [connection,setConnection] = useState<Connection>('connecting'), [clock,setClock] = useState(now)
  useEffect(() => { const timer = window.setInterval(() => setClock(now()),250); return () => window.clearInterval(timer) },[])
  useEffect(() => {
    let ended = false, retry = 1000, timeout: ReturnType<typeof setTimeout> | undefined, stream: AbortController | undefined
    const terminal = () => { if (!ended) { ended=true; stream?.abort(); setReceived(undefined); onEnd() } }
    async function connect() {
      if (ended) return
      stream = new AbortController(); const controller = stream
      let heartbeat = window.setTimeout(() => controller.abort(),8_000)
      try {
        const response = await fetch(`/api/v1/events/${eventID}/display/stream`, { headers: { Authorization: `Bearer ${token}`, Accept: 'text/event-stream' }, credentials: 'omit', cache: 'no-store', signal: controller.signal })
        if ([401,403,404].includes(response.status)) { terminal(); return }
        if (!response.ok) throw new Error('display_unavailable')
        await consumeFrames(response,controller.signal,(event,data) => {
          if (ended || controller.signal.aborted) return
          if (event === 'revoked') { terminal(); return }
          if (event === 'reconnect') { controller.abort(); return }
          if (event !== 'snapshot') return
          const snapshot = parseSnapshot(JSON.parse(data),eventID), receivedAt = now()
          // Complete snapshots supersede the prior projection even at equal sequence.
          setReceived({ snapshot, receivedAt, offset: Date.parse(snapshot.server_now)-receivedAt }); setConnection('connected'); retry=1000
          window.clearTimeout(heartbeat); heartbeat=window.setTimeout(() => controller.abort(),8_000)
        })
      } catch { /* Keep only the last validated safe snapshot during disconnection. */ }
      finally { window.clearTimeout(heartbeat) }
      if (!ended) { setConnection('reconnecting'); timeout=setTimeout(() => void connect(),retry); retry=Math.min(15_000,retry*2) }
    }
    void connect()
    return () => { ended=true; stream?.abort(); clearTimeout(timeout) }
  },[eventID,token,onEnd])
  const snapshot = received?.snapshot
  if (!snapshot) return <main className="display-pairing"><h1>Waiting for the room feed</h1><p role="status">{connection === 'reconnecting' ? 'Connection lost. Reconnecting…' : 'Connecting…'}</p><Button onClick={onEnd}>Disconnect display</Button></main>
  return <RoomCanvas snapshot={snapshot} serverNow={Math.max(clock,received.receivedAt)+received.offset} connection={connection} assets={<RoomAssets key={snapshot.assets.map(a=>a.digest).join(':')} eventID={eventID} token={token} assets={snapshot.assets} onRevoked={onEnd}/>} fullscreen={() => void document.documentElement.requestFullscreen().catch(() => undefined)} disconnect={onEnd}/>

}
function RoomAssets({ eventID,token,assets,onRevoked }: { eventID: string; token: string; assets: Snapshot['assets']; onRevoked: () => void }) {
  const assetKey = JSON.stringify(assets)
  const [loaded,setLoaded] = useState<{source:string;images:{kind:string;url:string}[]}>({source:'',images:[]})
  const images=useMemo(()=>loaded.source===assetKey?loaded.images:[],[assetKey,loaded])
  useLayoutEffect(() => {
    const controller = new AbortController(), urls: string[] = []
    void Promise.all((JSON.parse(assetKey) as Snapshot['assets']).map(async asset => {
      try {
        const response = await fetch(`/api/v1/events/${eventID}/display/assets/${asset.id}`, { headers:{ Authorization:`Bearer ${token}` }, credentials:'omit',cache:'no-store',signal:controller.signal })
        if ([401,403].includes(response.status)) { onRevoked(); return undefined }
        if (!response.ok || response.headers.get('content-type') !== asset.media_type) return undefined
        const blob = await response.blob()
        if (controller.signal.aborted || blob.size > 512*1024) return undefined
        const digest=Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256',await blob.arrayBuffer()))).map(v=>v.toString(16).padStart(2,'0')).join('')
        if (controller.signal.aborted || digest !== asset.digest) return undefined
        const url=URL.createObjectURL(blob); urls.push(url); return {kind:asset.kind,url}
      } catch { return undefined }
    })).then(values => { if (!controller.signal.aborted) setLoaded({source:assetKey,images:values.filter(v=>v!==undefined)}) })
    return () => { controller.abort(); urls.forEach(url=>URL.revokeObjectURL(url)) }
  },[eventID,token,assetKey,onRevoked])
  useEffect(() => { const icon=images.find(i=>i.kind==='icon'); if (!icon) return; const link=document.createElement('link');link.rel='icon';link.href=icon.url;document.head.append(link);return ()=>link.remove() },[images])
  return <>{images.filter(image=>image.kind!=='icon').map(image=><img key={image.url} src={image.url} className={`room-${image.kind}`} alt={image.kind==='logo'?'Event logo':''} onError={e=>{e.currentTarget.hidden=true}} />)}</>
}

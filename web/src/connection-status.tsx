import { useEffect, useState } from 'react'

type State = 'checking' | 'online' | 'offline' | 'unavailable' | 'recovering'
// This is transport status only. Each recovered mentor submission still needs
// its own fresh current-resource GET and normal server authorisation/If-Match.
export function ConnectionStatus() {
  const [state, setState] = useState<State>(() => navigator.onLine ? 'checking' : 'offline')
  useEffect(() => {
    const online = () => setState('recovering'), offline = () => setState('offline')
    const failed = () => setState(navigator.onLine ? 'unavailable' : 'offline')
    // A request started online can finish after the browser reports network loss.
    const response = () => setState(navigator.onLine ? 'online' : 'offline')
    window.addEventListener('online', online); window.addEventListener('offline', offline)
    window.addEventListener('workspace-network-unavailable', failed); window.addEventListener('workspace-response', response)
    return () => { window.removeEventListener('online', online); window.removeEventListener('offline', offline); window.removeEventListener('workspace-network-unavailable', failed); window.removeEventListener('workspace-response', response) }
  }, [])
  const text: Record<State, string> = { checking: 'Checking connection', online: 'Connected to server', offline: 'Offline · server changes unavailable', unavailable: 'Server unavailable · information may be stale', recovering: 'Reconnecting · checking current information' }
  return <p className={`connection-status connection-${state}`} aria-label="Connection status" role={state === 'online' || state === 'checking' ? undefined : 'status'}>{text[state]}{state !== 'online' && <span> Local drafts are not saved server notes. Assessment and judging require a connection.</span>}</p>
}

import { currentMentorDraftScope, invalidateMentorDrafts, purgeExpiredMentorDrafts } from './mentor-drafts'

const channelName = 'hackbox-private-draft-lifecycle-v1'
const signalKey = 'hackbox:session-signal:v1'
type Signal = { kind: 'ended' } | { kind: 'scope'; scope: string }
// Signals carry only non-authorizing scope metadata, never a person, draft,
// session/CSRF token or resource. The storage fallback is removed immediately.
export function installDraftInvalidation(onRemote: () => void) {
  let announcedEnd = false
  let channel: BroadcastChannel | undefined
  try { channel = new BroadcastChannel(channelName) } catch { /* Use the metadata-only storage event fallback. */ }
  const receive = (value: unknown) => {
    if (!value || typeof value !== 'object') return
    const signal = value as Signal
    if (signal.kind !== 'ended' && (signal.kind !== 'scope' || typeof signal.scope !== 'string' || !/^[a-f0-9-]{36}$/i.test(signal.scope) || signal.scope === currentMentorDraftScope())) return
    announcedEnd = true
    invalidateMentorDrafts(); onRemote()
  }
  const broadcast = (signal: Signal) => {
    try { channel?.postMessage(signal) } catch { /* Storage events still invalidate peers. */ }
    try { localStorage.setItem(signalKey, JSON.stringify({ signal, nonce: crypto.randomUUID() })); localStorage.removeItem(signalKey) } catch { /* Session scope is also rechecked on focus. */ }
  }
  const message = (event: MessageEvent) => receive(event.data)
  const storage = (event: StorageEvent) => { if (event.key === signalKey && event.newValue) { try { receive(JSON.parse(event.newValue).signal) } catch { /* Ignore malformed signals. */ } } }
  const end = () => { invalidateMentorDrafts(); if (!announcedEnd) { announcedEnd = true; broadcast({ kind: 'ended' }) } }
  const scope = (event: Event) => { announcedEnd = false; broadcast({ kind: 'scope', scope: (event as CustomEvent<string>).detail }) }
  const purge = () => purgeExpiredMentorDrafts()
  const timer = window.setInterval(purge, 30_000)
  window.addEventListener('focus', purge); document.addEventListener('visibilitychange', purge)
  purge()
  channel?.addEventListener('message', message)
  window.addEventListener('storage', storage); window.addEventListener('mentor-draft-scope', scope)
  window.addEventListener('session-ending', end); window.addEventListener('session-expired', end)
  return () => { window.clearInterval(timer); window.removeEventListener('focus', purge); document.removeEventListener('visibilitychange', purge); window.removeEventListener('session-ending', end); window.removeEventListener('session-expired', end); window.removeEventListener('storage', storage); window.removeEventListener('mentor-draft-scope', scope); channel?.removeEventListener('message', message); channel?.close() }
}

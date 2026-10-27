import { useEffect, useRef } from 'react'

// Retain only local DOM targets and a captured private-lifetime check. A second
// read must not lose the opener restored by the first. Deliberate navigation
// ends the return intention, so a later read cannot steal the user's focus.
export function useReviewReturnFocus({ paused, pending, unknown, reviewing, capture }: { paused: boolean; pending: boolean; unknown: boolean; reviewing: boolean; capture: () => () => boolean }) {
  const deferred = useRef<{ source: HTMLElement; fallback: HTMLElement; current: () => boolean } | undefined>(undefined)
  useEffect(() => {
    const leave = (event: Event) => {
      const target = event.target, value = deferred.current
      if (value && target instanceof Node && target !== document.body && !value.source.contains(target) && !value.fallback.contains(target)) deferred.current = undefined
    }
    document.addEventListener('focusin', leave); document.addEventListener('pointerdown', leave)
    return () => { deferred.current = undefined; document.removeEventListener('focusin', leave); document.removeEventListener('pointerdown', leave) }
  }, [])
  useEffect(() => {
    const value = deferred.current
    if (!value?.current() || !value.source.isConnected || !value.fallback.isConnected) { deferred.current = undefined; return }
    const active = document.activeElement
    const returning = active === value.source || active === value.fallback || active === document.body
    if (!returning) { deferred.current = undefined; return }
    if (paused || pending) {
      if (active === document.body) value.fallback.focus()
    } else if (!unknown && !value.source.matches(':disabled') && !value.source.closest('[hidden]')) value.source.focus()
  }, [paused, pending, unknown, reviewing])
  return (source: HTMLElement | null, fallback: HTMLElement | null) => {
    deferred.current = source?.isConnected && fallback ? { source, fallback, current: capture() } : undefined
    // Base UI resolves this destination during teardown, before React unhides
    // the editor. Its final focus runs after that commit; keep the real opener
    // for ordinary cancellation and use the fallback only while disabled.
    return source?.isConnected && !source.matches(':disabled') ? source : fallback ?? false
  }
}

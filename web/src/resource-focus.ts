import { useRef, type RefObject } from 'react'

// Owners capture the actual trigger, including its responsive replacement key.
// Returning to a title link must not silently move focus to a different action.
export function useResourceReturnFocus(root: RefObject<HTMLElement | null>) {
  const opener = useRef<{ element: HTMLElement; key?: string; tag: string }>(undefined)
  function capture(element: HTMLElement | null = document.activeElement instanceof HTMLElement ? document.activeElement : null) {
    if (element) opener.current = { element, key: element.dataset.resourceOpener ?? element.dataset.recordFocus, tag: element.tagName }
  }
  function restore() {
    requestAnimationFrame(() => {
      const saved = opener.current, candidates = [...(root.current?.querySelectorAll<HTMLElement>('[data-resource-opener], [data-record-focus]') ?? [])]
      const replacement = saved?.element.isConnected ? saved.element : candidates.find(element => (element.dataset.resourceOpener ?? element.dataset.recordFocus) === saved?.key && element.tagName === saved?.tag)
      if (replacement && replacement.getClientRects().length && !replacement.matches(':disabled')) { replacement.focus(); return }
      const fallback = root.current?.querySelector<HTMLElement>('h1, h2, h3')
      if (fallback) { fallback.tabIndex = -1; fallback.focus() }
    })
  }
  return { capture, restore }
}

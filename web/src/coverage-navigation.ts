import { useBlocker } from '@tanstack/react-router'
import { useEffect, useLayoutEffect, useReducer, useRef, type RefObject } from 'react'

export type CoverageFocusTarget = { attribute: 'data-coverage-focus' | 'data-record-focus' | 'id'; value: string } | { button: string }
// Store the actual source control's stable identity, not its detached DOM node.
// CollectionRecords replaces wide table controls with narrow list counterparts.
export function useCoverageFocus(root: RefObject<HTMLElement | null>) {
  const pending = useRef<{ target: CoverageFocusTarget | undefined; unlessFocusMoved: boolean; originalActive: Element | null } | undefined>(undefined)
  const mounted = useRef(false), [generation, requestCommit] = useReducer((value: number) => value + 1, 0)
  useLayoutEffect(() => { mounted.current = true; return () => { mounted.current = false; pending.current = undefined } }, [])
  useEffect(() => {
    const restoration = pending.current
    pending.current = undefined
    if (!restoration || !mounted.current) return
    // Request from the committed passive-effect flush, so inspector heading
    // autofocus finishes before this one existing restoration frame.
    const frame = requestAnimationFrame(() => {
      if (!mounted.current) return
      const { target, unlessFocusMoved, originalActive } = restoration
      // Resolve the stable source after the closing render has committed. An
      // animation frame alone does not establish that React mounted that source.
      if (unlessFocusMoved && document.activeElement !== originalActive && document.activeElement !== document.body && document.activeElement?.isConnected) return
      const controls = root.current?.querySelectorAll<HTMLElement>('button,input,textarea,select,a,[tabindex]') ?? []
      const matches = [...controls].filter(element => element.getClientRects().length > 0 && !element.matches(':disabled') && (target && 'attribute' in target ? element.getAttribute(target.attribute) === target.value : target && element instanceof HTMLButtonElement && (element.getAttribute('aria-label') ?? element.textContent?.trim().replace(/\s+/g, ' ') ?? '') === target.button))
      const source = matches.length === 1 ? matches[0] : undefined
      ;(source ?? root.current)?.focus()
    })
    return () => cancelAnimationFrame(frame)
  }, [generation, root])
  function capture(): CoverageFocusTarget | undefined {
    const active = document.activeElement
    if (!(active instanceof HTMLElement) || !root.current?.contains(active)) return
    for (const attribute of ['data-coverage-focus', 'data-record-focus', 'id'] as const) {
      const value = active.getAttribute(attribute)
      if (value) return { attribute, value }
    }
    if (active instanceof HTMLButtonElement) return { button: active.getAttribute('aria-label') ?? active.textContent?.trim().replace(/\s+/g, ' ') ?? '' }
  }
  function restore(target: CoverageFocusTarget | undefined, unlessFocusMoved = false) {
    if (!mounted.current) return
    pending.current = { target, unlessFocusMoved, originalActive: document.activeElement }
    requestCommit()
  }
  return { capture, restore }
}
export function useCoverageDraftGuard({ dirty, pending, unknown }: { dirty: boolean; pending: boolean; unknown: boolean }) {
  useBlocker({ shouldBlockFn: () => pending || (!unknown && dirty && !window.confirm('Discard unsaved roster changes and leave this page?')), enableBeforeUnload: dirty || pending || unknown })
  function discard(hasDraft: boolean, leave: () => void) {
    if (pending || (!unknown && hasDraft && !window.confirm('Discard unsaved changes in this task?'))) return false
    leave()
    return true
  }
  return { discard }
}

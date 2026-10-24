import { useBlocker } from '@tanstack/react-router'
import type { RefObject } from 'react'

export type CoverageFocusTarget = { attribute: 'data-coverage-focus' | 'data-record-focus' | 'id'; value: string } | { button: string }
// Store the actual source control's stable identity, not its detached DOM node.
// CollectionRecords replaces wide table controls with narrow list counterparts.
export function useCoverageFocus(root: RefObject<HTMLElement | null>) {
  function capture(): CoverageFocusTarget | undefined {
    const active = document.activeElement
    if (!(active instanceof HTMLElement) || !root.current?.contains(active)) return
    for (const attribute of ['data-coverage-focus', 'data-record-focus', 'id'] as const) {
      const value = active.getAttribute(attribute)
      if (value) return { attribute, value }
    }
    if (active instanceof HTMLButtonElement) return { button: active.getAttribute('aria-label') ?? active.textContent?.trim().replace(/\s+/g, ' ') ?? '' }
  }
  function restore(target: CoverageFocusTarget | undefined) {
    requestAnimationFrame(() => {
      const controls = root.current?.querySelectorAll<HTMLElement>('button,input,textarea,select,a,[tabindex]') ?? []
      const source = [...controls].find(element => element.getClientRects().length > 0 && !element.matches(':disabled') && (target && 'attribute' in target ? element.getAttribute(target.attribute) === target.value : target && element instanceof HTMLButtonElement && (element.getAttribute('aria-label') ?? element.textContent?.trim().replace(/\s+/g, ' ') ?? '') === target.button))
      ;(source ?? root.current)?.focus()
    })
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

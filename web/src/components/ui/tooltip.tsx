import { Tooltip as Base } from '@base-ui/react/tooltip'
import { useId, useState, type ReactElement, type ReactNode } from 'react'
import { descriptionIds } from './fields'
import { useThemePortal } from '../theme'
export const TooltipProvider = Base.Provider
// Guidance required to finish a task must remain visible outside this hint.
export function Tooltip({ children, content }: { children: ReactElement; content: ReactNode }) {
  const portal = useThemePortal(), id = useId(), [open, setOpen] = useState(false)
  const describedBy = (children.props as { 'aria-describedby'?: string })['aria-describedby']
  return <Base.Root open={open} onOpenChange={setOpen}><Base.Trigger render={children} aria-describedby={descriptionIds(describedBy, open && portal ? id : undefined)} />{portal && <Base.Portal container={portal}><Base.Positioner sideOffset={6} className="fleet-positioner"><Base.Popup id={id} role="tooltip" className="fleet-tooltip">{content}</Base.Popup></Base.Positioner></Base.Portal>}</Base.Root>
}

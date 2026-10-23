import { Popover as Base } from '@base-ui/react/popover'
import type { ReactNode } from 'react'
import { useThemePortal } from '../theme'
import { cn } from '@/lib/utils'
export const Popover = Base.Root
export const PopoverTrigger = Base.Trigger
export const PopoverClose = Base.Close
export function PopoverContent({ title, children, className, ...props }: Omit<Base.Popup.Props, 'title' | 'className'> & { className?: string } & { title: ReactNode }) {
  const portal = useThemePortal()
  if (!portal) return null
  return <Base.Portal container={portal}><Base.Positioner sideOffset={6} className="fleet-positioner"><Base.Popup {...props} className={cn('fleet-popover', className)}><Base.Title className="fleet-dialog-title">{title}</Base.Title>{children}</Base.Popup></Base.Positioner></Base.Portal>
}

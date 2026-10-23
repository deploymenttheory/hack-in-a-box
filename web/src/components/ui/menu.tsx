import { Menu as Base } from '@base-ui/react/menu'
import { useThemePortal } from '../theme'
import { cn } from '@/lib/utils'
import { Icon, type IconName } from './icon'

export const Menu = Base.Root
export const MenuTrigger = Base.Trigger
export function MenuContent({ children, className, ...props }: Omit<Base.Popup.Props, 'className'> & { className?: string }) {
  const portal = useThemePortal()
  if (!portal) return null
  return <Base.Portal container={portal}><Base.Positioner sideOffset={6} className="fleet-positioner"><Base.Popup {...props} className={cn('fleet-menu', className)}>{children}</Base.Popup></Base.Positioner></Base.Portal>
}
export function MenuItem({ children, icon, className, ...props }: Omit<Base.Item.Props, 'className'> & { className?: string } & { icon?: IconName }) {
  return <Base.Item {...props} className={cn('fleet-menu-item', className)}>{icon && <Icon name={icon} />}{children}</Base.Item>
}
export const MenuSeparator = Base.Separator

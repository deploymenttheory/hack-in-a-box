import { Tabs as Base } from '@base-ui/react/tabs'
import { cn } from '@/lib/utils'
export const Tabs = Base.Root
export function TabList({ className, ...props }: Omit<Base.List.Props, 'className'> & { className?: string }) { return <Base.List {...props} className={cn('fleet-tabs', className)} /> }
export function Tab({ className, ...props }: Omit<Base.Tab.Props, 'className'> & { className?: string }) { return <Base.Tab {...props} className={cn('fleet-tab', className)} /> }
export const TabPanel = Base.Panel

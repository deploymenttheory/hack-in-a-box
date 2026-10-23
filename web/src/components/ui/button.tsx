import { Button as ButtonPrimitive } from '@base-ui/react/button'
import { cva, type VariantProps } from 'class-variance-authority'
import type { ComponentProps } from 'react'
import { cn } from '@/lib/utils'
import { Icon, type IconName } from './icon'

const buttonVariants = cva('fleet-button', { variants: {
  variant: { default: 'fleet-button-primary', outline: 'fleet-button-outline', secondary: 'fleet-button-secondary', ghost: 'fleet-button-ghost', destructive: 'fleet-button-danger', link: 'fleet-button-link' },
  size: { default: '', xs: 'fleet-button-small', sm: 'fleet-button-small', lg: 'fleet-button-large', icon: 'fleet-button-icon', 'icon-xs': 'fleet-button-icon', 'icon-sm': 'fleet-button-icon', 'icon-lg': 'fleet-button-icon fleet-button-large' },
}, defaultVariants: { variant: 'default', size: 'default' } })
export type ButtonProps = Omit<ButtonPrimitive.Props, 'className'> & VariantProps<typeof buttonVariants> & { className?: string; icon?: IconName; pending?: boolean }
export function Button({ className, variant = 'default', size = 'default', icon, pending = false, disabled, children, ...props }: ButtonProps) {
  return <ButtonPrimitive {...props} data-slot="button" data-variant={variant} data-size={size} className={cn(buttonVariants({ variant, size, className }))} disabled={disabled || pending} aria-busy={pending || undefined}>
    {icon && <Icon name={pending ? 'pending' : icon} className={pending ? 'fleet-spinner' : undefined} />}{children}
  </ButtonPrimitive>
}
export function IconButton({ label, icon, variant = 'ghost', ...props }: Omit<ButtonProps, 'children' | 'icon' | 'aria-label' | 'size'> & { label: string; icon: IconName }) {
  return <Button {...props} icon={icon} variant={variant} size="icon" aria-label={label} />
}
// Links retain href, browser context-menu and keyboard navigation semantics.
export function ActionLink({ className, variant = 'outline', size = 'default', icon, children, ...props }: ComponentProps<'a'> & VariantProps<typeof buttonVariants> & { icon?: IconName }) {
  return <a {...props} className={cn(buttonVariants({ variant, size, className }))}>{icon && <Icon name={icon} />}{children}</a>
}
export { buttonVariants }

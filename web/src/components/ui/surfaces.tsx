import type { ComponentProps } from 'react'
import { cn } from '@/lib/utils'
// Shared aliases add no implicit headings, live regions or interaction.
export function Card({ className, ...props }: ComponentProps<'section'>) {
  return <section data-slot="card" className={cn('ui-card', className)} {...props} />
}
export function Badge({ className, ...props }: ComponentProps<'span'>) {
  return <span data-slot="badge" className={cn('ui-badge', className)} {...props} />
}
export function Alert({ className, tone = 'neutral', ...props }: ComponentProps<'div'> & { tone?: 'neutral' | 'destructive' | 'warning' | 'success' }) {
  return <div data-slot="alert" data-tone={tone} className={cn('ui-alert', className)} {...props} />
}

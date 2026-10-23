import type { ComponentProps } from 'react'
export { StatusBadge as Badge } from './feedback'
import { cn } from '@/lib/utils'
// Shared aliases add no implicit headings, live regions or interaction.
export function Card({ className, ...props }: ComponentProps<'section'>) {
  return <section data-slot="card" className={cn('fleet-surface', className)} {...props} />
}
export function Alert({ className, tone = 'neutral', ...props }: ComponentProps<'div'> & { tone?: 'neutral' | 'destructive' | 'warning' | 'success' }) {
  return <div data-slot="alert" data-tone={tone} className={cn('fleet-alert', className)} {...props} />
}

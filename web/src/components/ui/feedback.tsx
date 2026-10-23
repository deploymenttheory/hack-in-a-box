import type { ComponentProps } from 'react'
import { Icon, type IconName } from './icon'
import { cn } from '@/lib/utils'
export type Tone = 'neutral' | 'success' | 'warning' | 'destructive'
const toneIcons: Record<Tone, IconName> = { neutral: 'info', success: 'success', warning: 'warning', destructive: 'warning' }
export function StatusBadge({ tone = 'neutral', children, className, ...props }: ComponentProps<'span'> & { tone?: Tone }) {
  return <span {...props} data-tone={tone} className={cn('fleet-status', className)}><Icon name={toneIcons[tone]} />{children}</span>
}
export function LiveMessage({ children, urgent = false, className, ...props }: ComponentProps<'div'> & { urgent?: boolean }) {
  return <div {...props} className={cn('fleet-live', className)} role={urgent ? 'alert' : 'status'} aria-atomic="true">{children}</div>
}
export function Progress({ label, value, max = 100 }: { label: string; value?: number; max?: number }) {
  const known = value !== undefined && Number.isFinite(value) && Number.isFinite(max) && max > 0 && value >= 0 && value <= max
  return <label className="fleet-progress"><span>{label}</span><progress aria-label={label} value={known ? value : undefined} max={Number.isFinite(max) && max > 0 ? max : 100} />{known && <span>{value} / {max}</span>}</label>
}

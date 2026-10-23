import type { ComponentProps } from 'react'
import { cn } from '@/lib/utils'
// Native props, labels, refs and ARIA relationships pass through unchanged.
export function Input({ className, ...props }: ComponentProps<'input'>) {
  return <input data-slot="input" className={cn('ui-input', className)} {...props} />
}
export function Textarea({ className, ...props }: ComponentProps<'textarea'>) {
  return <textarea data-slot="textarea" className={cn('ui-textarea', className)} {...props} />
}
export function Select({ className, ...props }: ComponentProps<'select'>) {
  return <select data-slot="select" className={cn('ui-select', className)} {...props} />
}
export function Label({ className, ...props }: ComponentProps<'label'>) {
  return <label data-slot="label" className={cn('ui-label', className)} {...props} />
}

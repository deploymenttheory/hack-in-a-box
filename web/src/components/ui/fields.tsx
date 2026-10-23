import { createContext, useContext, useId, type ComponentProps, type ReactNode } from 'react'
import { cn } from '@/lib/utils'

const FieldContext = createContext<{ id: string; description?: string; error?: string }>({ id: '' })
export function descriptionIds(...values: (string | undefined)[]) { return [...new Set(values.flatMap(value => value?.split(/\s+/).filter(Boolean) ?? []))].join(' ') || undefined }
export function Field({ id: suppliedID, label, description, error, children, className }: { id?: string; label: ReactNode; description?: ReactNode; error?: string; children: ReactNode; className?: string }) {
  const generated = useId(), id = suppliedID ?? `field-${generated}`
  return <FieldContext.Provider value={{ id, description: description ? `${id}-help` : undefined, error: error ? `${id}-error` : undefined }}>
    <div className={cn('fleet-field', className)}><Label htmlFor={id}>{label}</Label>{description && <p id={`${id}-help`} className="fleet-field-help">{description}</p>}{children}{error && <FieldError id={`${id}-error`}>{error}</FieldError>}</div>
  </FieldContext.Provider>
}
function useControl(props: { id?: string; 'aria-describedby'?: string; 'aria-invalid'?: ComponentProps<'input'>['aria-invalid'] }) {
  const field = useContext(FieldContext)
  return { id: props.id ?? (field.id || undefined), 'aria-describedby': descriptionIds(props['aria-describedby'], field.description, field.error), 'aria-invalid': field.error ? true : props['aria-invalid'] }
}
export function Input({ className, ...props }: ComponentProps<'input'>) {
  const semantics = useControl(props)
  return <input {...props} {...semantics} data-slot="input" className={cn('fleet-input', className)} />
}
export function Textarea({ className, ...props }: ComponentProps<'textarea'>) {
  const semantics = useControl(props)
  return <textarea {...props} {...semantics} data-slot="textarea" className={cn('fleet-input fleet-textarea', className)} />
}
export function Select({ className, ...props }: ComponentProps<'select'>) {
  const semantics = useControl(props)
  return <select {...props} {...semantics} data-slot="select" className={cn('fleet-input', className)} />
}
export function Checkbox({ className, type: _type, ...props }: ComponentProps<'input'>) {
  void _type
  const semantics = useControl(props)
  return <input {...props} {...semantics} type="checkbox" data-slot="checkbox" className={cn('fleet-checkbox', className)} />
}
export function Label({ className, ...props }: ComponentProps<'label'>) { return <label {...props} data-slot="label" className={cn('fleet-label', className)} /> }
export function FieldError({ className, ...props }: ComponentProps<'p'>) { return <p {...props} className={cn('fleet-field-error', className)} /> }
// Call after the owning validator has rendered its current errors. Native/RHF
// validation still owns submission; async visual validation cannot authorise it.
export function focusFirstInvalid(form: HTMLElement) {
  const control = [...form.querySelectorAll<HTMLElement>('[aria-invalid="true"], :invalid')].find(element => (element.matches('input:not([type=hidden]), select, textarea, button') || (element.hasAttribute('tabindex') && element.tabIndex >= 0)) && !element.matches(':disabled') && !element.closest('[hidden], [inert]'))
  control?.focus()
}

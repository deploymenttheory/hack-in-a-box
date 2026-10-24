import { Dialog as Base } from '@base-ui/react/dialog'
import { AlertDialog as Alert } from '@base-ui/react/alert-dialog'
import { useRef, type ReactNode } from 'react'
import { useThemePortal } from '../theme'
import { Button, IconButton } from './button'
import { cn } from '@/lib/utils'

export const Dialog = Base.Root
export const DialogTrigger = Base.Trigger
export const DialogClose = Base.Close
export function DialogContent({ title, description, children, footer, className, closeLabel = 'Close dialog', ...props }: Omit<Base.Popup.Props, 'title' | 'className'> & { className?: string } & { title: ReactNode; description?: ReactNode; footer?: ReactNode; closeLabel?: string }) {
  const portal = useThemePortal()
  if (!portal) return null
  return <Base.Portal container={portal}><Base.Backdrop className="fleet-backdrop" /><Base.Viewport className="fleet-dialog-viewport"><Base.Popup {...props} className={cn('fleet-dialog', className)}>
    <div className="fleet-dialog-heading"><Base.Title className="fleet-dialog-title">{title}</Base.Title><Base.Close render={<IconButton icon="close" label={closeLabel} />} /></div>
    {description && <Base.Description className="fleet-description">{description}</Base.Description>}<div className="fleet-dialog-body">{children}</div>{footer && <div className="fleet-dialog-footer">{footer}</div>}
  </Base.Popup></Base.Viewport></Base.Portal>
}
export function ConfirmationDialog({ open, onOpenChange, title, description, confirmLabel, onConfirm, pending = false, disabled = false, printExcluded = false, finalFocus, children }: { open: boolean; onOpenChange: (open: boolean) => void; title: string; description: string; confirmLabel: string; onConfirm: () => void; pending?: boolean; disabled?: boolean; printExcluded?: boolean; finalFocus?: Base.Popup.Props['finalFocus']; children?: ReactNode }) {
  const portal = useThemePortal(), cancel = useRef<HTMLButtonElement>(null)
  return <Alert.Root open={open} onOpenChange={(next, details) => { if (!next && pending) details.cancel(); else onOpenChange(next) }}>
    {portal && <Alert.Portal container={portal}><Alert.Backdrop className={cn("fleet-backdrop", printExcluded && "fleet-print-excluded")} /><Alert.Viewport className={cn("fleet-dialog-viewport", printExcluded && "fleet-print-excluded")}><Alert.Popup className="fleet-dialog" initialFocus={cancel} finalFocus={finalFocus}>
      <div className="fleet-dialog-heading"><Alert.Title className="fleet-dialog-title">{title}</Alert.Title><Alert.Close disabled={pending} render={<IconButton icon="close" label="Close confirmation" />} /></div>
      <Alert.Description className="fleet-description">{description}</Alert.Description>{children && <div className="fleet-dialog-body">{children}</div>}
      <div className="fleet-dialog-footer"><Alert.Close disabled={pending} render={<Button ref={cancel} variant="outline" />}>Cancel</Alert.Close><Button icon="check" pending={pending} disabled={disabled} onClick={onConfirm}>{confirmLabel}</Button></div>
    </Alert.Popup></Alert.Viewport></Alert.Portal>}
  </Alert.Root>
}

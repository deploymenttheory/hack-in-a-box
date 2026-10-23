import { Toast as Base } from '@base-ui/react/toast'
import type { ReactNode } from 'react'
import { useThemePortal } from '../theme'
import { IconButton } from './button'

// Mount per identity/event scope, never use a module-global manager. Critical
// failures and receipts also belong inline; a toast is only secondary feedback.
export function ToastProvider({ children, scopeKey }: { children: ReactNode; scopeKey: string }) {
  return <Base.Provider key={scopeKey} timeout={5000} limit={3}>{children}<ToastViewport /></Base.Provider>
}
function ToastViewport() {
  const portal = useThemePortal(), manager = Base.useToastManager()
  if (!portal) return null
  return <Base.Portal container={portal}><Base.Viewport className="fleet-toasts">{manager.toasts.map(toast => <Base.Root key={toast.id} toast={toast} className="fleet-toast"><Base.Content><Base.Title className="fleet-label" /><Base.Description /></Base.Content><Base.Close render={<IconButton icon="close" label="Dismiss notification" />} /></Base.Root>)}</Base.Viewport></Base.Portal>
}
export function useToast() {
  const manager = Base.useToastManager()
  return { notify: ({ title, description, urgent = false }: { title: string; description?: string; urgent?: boolean }) => manager.add({ title, description, priority: urgent ? 'high' : 'low', timeout: urgent ? 0 : 5000 }), dismiss: manager.close }
}

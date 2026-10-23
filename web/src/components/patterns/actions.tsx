import { ActionLink, Button } from '../ui/button'
import type { PatternAction } from './model'

export function PatternActionControl({ action, primary = false }: { action: PatternAction; primary?: boolean }) {
  const variant = primary ? 'default' : 'outline'
  if (action.href !== undefined) return action.disabled ? <span className="fleet-disabled-action" aria-disabled="true">{action.label}</span> : <ActionLink href={action.href} icon={action.icon} variant={variant}>{action.label}</ActionLink>
  return <Button type="button" icon={action.icon} variant={variant} disabled={action.disabled} onClick={action.onSelect}>{action.label}</Button>
}

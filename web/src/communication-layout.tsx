import type { ReactNode } from 'react'
import { Icon, type IconName } from './components/ui/icon'

/** Flat form sections keep the explanation beside its fields on wide screens. */
export function CommunicationStep({ id, title, icon, description, children }: { id?: string; title: string; icon: IconName; description: string; children: ReactNode }) {
  return <section id={id} className="fleet-communication-step" aria-label={title}><header><Icon name={icon} /><h3>{title}</h3><p>{description}</p></header><div className="fleet-communication-fields">{children}</div></section>
}

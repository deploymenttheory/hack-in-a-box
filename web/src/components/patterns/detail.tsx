import { useEffect, useRef, type ReactNode } from 'react'
import { Icon } from '../ui/icon'
import type { NavigationTarget, ScopeLabel } from './model'

export function ContextLink({ target, back = false }: { target: NavigationTarget; back?: boolean }) {
  return <a href={target.href} onClick={event => { if (target.onNavigate && event.button === 0 && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey) { event.preventDefault(); target.onNavigate() } }}>{back && <Icon name="back" />}{target.label}</a>
}
export function Breadcrumbs({ items }: { items: readonly NavigationTarget[] }) {
  return <nav aria-label="Breadcrumb"><ol className="fleet-breadcrumbs">{items.map((item, index) => <li key={item.href}>{index > 0 && <span aria-hidden="true">/</span>}<ContextLink target={item} /></li>)}</ol></nav>
}
export function Inspector({ identity, title, scope, back, children }: { identity: string; title: string; scope: ScopeLabel; back: NavigationTarget; children: ReactNode }) {
  const heading = useRef<HTMLHeadingElement>(null)
  useEffect(() => { heading.current?.focus() }, [identity])
  return <section className="fleet-inspector" aria-label={title}><ContextLink target={back} back /><p className="fleet-scope-label">{scope.event && <>{scope.event} · </>}{scope.entity}</p><h2 tabIndex={-1} ref={heading}>{title}</h2>{children}</section>
}
// Selection and URLs are supplied by the router/domain owner. At narrow widths
// the selected route owns the canvas rather than becoming a cramped overlay.
export function DetailWorkspace({ collection, detail }: { collection: ReactNode; detail?: ReactNode }) {
  return <div className="fleet-detail-workspace" data-detail-open={detail ? 'true' : undefined}><div className="fleet-detail-collection">{collection}</div>{detail && <aside className="fleet-detail-region">{detail}</aside>}</div>
}

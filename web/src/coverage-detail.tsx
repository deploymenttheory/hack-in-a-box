import { useEffect, useRef, type ReactNode } from 'react'
import { Button } from './components/ui/button'
export function CoverageDetail({ title, children, close, disabled = false }: { title: string; children: ReactNode; close: () => void; disabled?: boolean }) {
  const heading = useRef<HTMLHeadingElement>(null)
  useEffect(() => { heading.current?.focus() }, [title])
  return <section className="fleet-inspector" aria-label={title}><Button icon="back" variant="ghost" onClick={close} disabled={disabled}>Back to roster</Button><h2 tabIndex={-1} ref={heading}>{title}</h2>{children}</section>
}

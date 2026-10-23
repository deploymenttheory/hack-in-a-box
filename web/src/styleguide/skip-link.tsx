import type { ReactNode } from 'react'

// Guide sections and synthetic request owners use URL state. A skip action moves
// focus within the current document without creating a navigation/history event.
export function GuideSkipLink({ target, children }: { target: 'guide-main' | 'preview-main'; children: ReactNode }) {
  return <a className="skip-link" href={`#${target}`} onClick={event => { event.preventDefault(); document.getElementById(target)?.focus() }}>{children}</a>
}

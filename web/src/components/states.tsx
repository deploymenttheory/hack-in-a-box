import { Alert } from './ui/surfaces'
import { Button } from './ui/button'
import { useId } from 'react'
import { Icon } from './ui/icon'
import { LiveMessage } from './ui/feedback'
import { APIError } from '../api/client'

export function Loading() { return <LiveMessage className="state">Loading your workspace…</LiveMessage> }
export function Failure({ error, retry }: { error: unknown; retry?: () => void }) {
  return <Alert role="alert" tone="destructive" className="state">
    <h2><Icon name="warning" /> Something needs attention</h2>
    <p>{error instanceof APIError ? error.message : 'We could not load this page. Please try again.'}</p>
    {error instanceof APIError && error.blockers.length > 0 && <ul>{error.blockers.map((item, index) => <li key={`${item.field}-${index}`}><a href={`#${item.field}`}>{item.message}</a></li>)}</ul>}
    {error instanceof APIError && error.requestID && <p className="muted">Reference: {error.requestID}</p>}
    {retry && <Button icon="refresh" onClick={retry}>Try again</Button>}
  </Alert>
}
export function FieldError({ message, id }: { message?: string; id?: string }) {
  const generated = useId()
  return message ? <p id={id ?? generated} role="alert" className="field-error">{message}</p> : null
}

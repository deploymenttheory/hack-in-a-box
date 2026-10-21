import { Button } from './ui/button'
import { APIError } from '../api/client'

export function Loading() { return <p role="status" className="state">Loading your workspace…</p> }
export function Failure({ error, retry }: { error: unknown; retry?: () => void }) {
  return <div role="alert" className="state">
    <h2>Something needs attention</h2>
    <p>{error instanceof APIError ? error.message : 'We could not load this page. Please try again.'}</p>
    {error instanceof APIError && error.blockers.length > 0 && <ul>{error.blockers.map((item, index) => <li key={`${item.field}-${index}`}><a href={`#${item.field}`}>{item.message}</a></li>)}</ul>}
    {error instanceof APIError && error.requestID && <p className="muted">Reference: {error.requestID}</p>}
    {retry && <Button onClick={retry}>Try again</Button>}
  </div>
}
export function FieldError({ message }: { message?: string }) {
  return message ? <p id="event-error" role="alert" className="field-error">{message}</p> : null
}

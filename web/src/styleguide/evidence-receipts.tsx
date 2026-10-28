import { useState } from 'react'
import { sameEvidence, type Evidence } from '../deliverable-model'
import { Button } from '../components/ui/button'
import { Snippet } from './snippet'

const reviewed: Evidence = { deliverable_id: '11111111-1111-4111-8111-111111111111', kind: 'repository', label: 'Synthetic reviewed repository', url: 'https://example.test/repository', file_id: null }
const native: Evidence = { deliverable_id: reviewed.deliverable_id, file_id: null, kind: reviewed.kind, label: reviewed.label, url: reviewed.url }

export function EvidenceReceiptExample() {
  const [received, setReceived] = useState(native)
  const matches = sameEvidence([reviewed], [received])
  return <section aria-label="Team evidence receipt comparison" className="guide-section"><h2>Compare reviewed evidence by value</h2><p>This local example uses the production comparison function with synthetic evidence. The native DTO places file identity before evidence kind. JSON object member order does not change the meaning of an exact original response.</p><dl><dt>Reviewed URL</dt><dd>{reviewed.url}</dd><dt>Example returned URL</dt><dd>{received.url}</dd></dl><div className="fleet-pattern-actions"><Button icon="refresh" variant="outline" onClick={() => setReceived(native)}>Use reordered identical evidence</Button><Button icon="warning" variant="outline" onClick={() => setReceived({ ...native, url: 'https://example.test/changed' })}>Change returned URL</Button></div><p role="status" aria-label="Evidence comparison result" aria-live="polite">{matches ? 'Evidence fields match the reviewed values.' : 'Evidence changed. Do not acknowledge this as the reviewed response.'}</p><p>Use named keyboard-operable controls and textual status feedback. Production separately validates the original operation, owner, authority, resource and revision; this example makes no request and creates no native acknowledgement. A current GET never settles an earlier unknown write. Compare every contracted field exactly, including nullable file identity; do not trim or normalise away a changed URL, label or kind.</p><Snippet>{'sameEvidence(originalResponse.evidence, capturedReview.evidence)\n// Compare evidence values independently of JSON member order.\n// Validate resource, authority and revision in the native owner too.\n// A mismatched or missing response retains unknown-outcome recovery.'}</Snippet></section>
}

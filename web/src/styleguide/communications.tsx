import { useRef, useState } from 'react'
import { CommunicationStep } from '../communication-layout'
import { Button } from '../components/ui/button'
import { ConfirmationDialog } from '../components/ui/dialog'
import { Field, Input } from '../components/ui/fields'
import { StatusBadge } from '../components/ui/feedback'
import { CommandState } from '../components/patterns/review'
import { Snippet } from './snippet'

export function CommunicationExamples() {
  const [subject, setSubject] = useState('Synthetic organiser-only reminder'), [version, setVersion] = useState(1), [review, setReview] = useState<number>(), [open, setOpen] = useState(false)
  const [outcome, setOutcome] = useState<'idle' | 'queued' | 'accepted' | 'unknown'>('idle'), [read, setRead] = useState(false)
  const trigger = useRef<HTMLButtonElement>(null)
  const blocked = outcome !== 'idle', currentReview = review === version
  return <section aria-label="Communications review example" className="guide-section"><h2>Communications: review before TEST</h2><p>Local synthetic state only. No application, email or Teams requests are made. Use the real owner’s current native preview and fingerprint; never reproduce personalisation or authorisation in the browser.</p>
    <CommunicationStep title="Synthetic TEST source" icon="type" description="Keep the exact source, audience and channel together. A source or connection change invalidates the review.">
      <Field id="guide-communication-subject" label="Synthetic message subject"><Input value={subject} disabled={blocked} onChange={event => { setSubject(event.target.value); setVersion(value => value + 1) }} /></Field>
      <p>Audience: requesting synthetic organiser only. Channel: synthetic email adapter. Review version {version}.</p>
      <div className="fleet-pattern-actions"><Button ref={trigger} icon="user" disabled={blocked || !subject.trim()} onClick={() => { setReview(version); setOpen(true) }}>Review synthetic TEST</Button><Button icon="refresh" variant="outline" disabled={blocked} onClick={() => setVersion(value => value + 1)}>Simulate connection change</Button></div>
      <ConfirmationDialog open={open} onOpenChange={setOpen} title="Review synthetic organiser-only TEST" description="This is a local guide demonstration, with no provider operation." confirmLabel="Confirm synthetic TEST" finalFocus={trigger} disabled={!currentReview || blocked} onConfirm={() => { if (currentReview && !blocked) { setOpen(false); setOutcome('queued') } }}><dl><dt>Audience</dt><dd>Requesting synthetic organiser only</dd><dt>Subject</dt><dd>[TEST] {subject}</dd><dt>Message</dt><dd>This is a synthetic organiser-only review. No message will be sent.</dd><dt>Reviewed version</dt><dd>{review}</dd></dl></ConfirmationDialog>
    </CommunicationStep>
    {outcome !== 'idle' && <StatusBadge tone={outcome === 'unknown' ? 'warning' : 'neutral'}>{outcome === 'queued' ? 'Synthetic queued — no provider acknowledgement' : outcome === 'accepted' ? 'Synthetic provider accepted — delivery not confirmed' : 'Synthetic outcome unknown'}</StatusBadge>}
    {outcome === 'queued' && <div className="fleet-pattern-actions"><Button icon="check" variant="outline" onClick={() => setOutcome('accepted')}>Simulate provider acceptance</Button><Button icon="warning" variant="outline" onClick={() => setOutcome('unknown')}>Simulate lost response</Button></div>}
    <CommandState value={outcome === 'unknown' ? { kind: 'uncertain', intentID: 'synthetic-local-intent', message: read ? 'Synthetic current facts loaded. They do not establish the original acknowledgement. No replacement TEST is permitted.' : 'Synthetic response lost. Keep the original outcome unknown; never generate a replacement request identity to retry.', readStatus: () => setRead(true) } : { kind: 'idle' }} />
    <p>Keep unsaved fields, an in-flight request and an unknown outcome separate. SMTP/webhook credentials and private addresses never enter shared command history, query metadata, storage or this guide. Native recovery may offer exact retries only where the backend replay contract proves them safe.</p>
    <Snippet>{'<CommunicationStep title="Organiser-only TEST" icon="send" description="Review current native output">\n  {exactNativePreview}\n</CommunicationStep>\n// Confirm with the retained native review fingerprint.\n// Current GET facts are not the original command receipt.'}</Snippet>
  </section>
}

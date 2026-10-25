import { useRef, useState, useLayoutEffect, useEffect, type ReactNode } from 'react'
import { useBlocker } from '@tanstack/react-router'
import { APIError } from './api/client'
import { useIdentity } from './session'
import { useCommunicationCommands } from './communication-tests'
import { usePrivateRequestScope, type PrivateRequestScope } from './private-request-scope'
import { Button } from './components/ui/button'
import { ConfirmationDialog } from './components/ui/dialog'
import { Failure } from './components/states'
import { usePlanningDepartureBarrier } from './planning-operations'
export type DecisionMetadata = { requestID: string; actorID: string; action: string; revision: number; version: number; fingerprint: string; resourceID?: string; subjectID?: string; intentID?: string }
export type DecisionBody = { request_id: string; action: string; reason: string }
export const decisionNamespace = (actor: string, session: string, eventID: string, resource = 'results') => `${actor}:${session}:${eventID}:${resource}`

// The payload/review stays inside this disposable private owner. Only the
// whitelisted original request identity enters authenticated session memory.
export function useResultDecision<B extends DecisionBody, P, R>(options: {
 owner: string; resourceID?: string; metadata?: (body: B, page: P) => Pick<DecisionMetadata, 'resourceID' | 'subjectID' | 'intentID'>; parent: PrivateRequestScope; paused: boolean; page: P;
 version: (page: P) => number; revision: (page: P) => number; read: (signal: AbortSignal) => Promise<P>;
 fingerprint: (body: B, revision: number) => Promise<string>; ready: (page: P, body: B) => void;
 send: (body: B, revision: number) => Promise<R>; matches: (result: R, intent: DecisionMetadata) => boolean;
 contextDenied?: (error: unknown) => boolean; refresh: () => Promise<unknown>; acknowledged?: (current?: P) => void;
}) {
 const identity = useIdentity(), commands = useCommunicationCommands(options.owner), scope = usePrivateRequestScope(`${options.owner}:editor`, options.parent)
 const [review, setReview] = useState<{ body: B; intent: DecisionMetadata; summary: ReactNode }>(), [preparing, setPreparing] = useState(false), [failure, setFailure] = useState<unknown>(), [failureStage, setFailureStage] = useState<'before_write' | 'rejected' | 'unknown'>(), [notice, setNotice] = useState(''), [base, setBase] = useState<{ revision: number; version: number }>()
 const sending = useRef(false), opener = useRef<HTMLElement | null>(null), baseOpener = useRef<HTMLElement | null>(null), fallback = useRef<HTMLHeadingElement | null>(null)
 const pending = preparing || commands.pending, stale = !!review && (options.version(options.page) !== review.intent.version || options.revision(options.page) !== review.intent.revision), blocked = options.paused || pending || commands.unconfirmed || stale
 usePlanningDepartureBarrier(pending)
 async function prepare(body: B, summary: ReactNode, source: HTMLElement) {
  if (blocked || sending.current) return
  const ticket = scope.capture(); sending.current = true; setPreparing(true); setFailure(undefined); setFailureStage(undefined); setNotice('')
  try {
   ticket.assert(); options.ready(options.page, body)
   const captured = structuredClone(body), revision = options.revision(options.page), fingerprint = await options.fingerprint(captured, revision); ticket.assert()
   opener.current = source
   setReview({ body: captured, summary, intent: { requestID: captured.request_id, actorID: identity.person.id, action: captured.action, revision, version: options.version(options.page), fingerprint, ...options.metadata?.(captured, options.page) } })
  } catch (error) { if (ticket.current()) { setFailure(error); setFailureStage('before_write') } }
  finally { sending.current = false; if (ticket.current()) setPreparing(false) }
 }
 async function confirm() {
  if (!review || blocked || sending.current) return
  const captured = review, ticket = scope.capture(); sending.current = true; setPreparing(true); setFailure(undefined); setFailureStage(undefined)
  let attempt: string | undefined
  try {
   const current = await options.read(ticket.signal); ticket.assert()
   if (options.revision(current) !== captured.intent.revision || options.version(current) !== captured.intent.version) throw new Error('The current starting point changed. Nothing was sent. Cancel, read current facts and review again.')
   options.ready(current, captured.body); ticket.assert()
   attempt = commands.begin({ operation: 'Reviewed ' + captured.body.action.replaceAll('_', ' '), revision: captured.intent.revision, command: { requestID: captured.intent.requestID, action: captured.intent.action, expectedVersion: captured.intent.version, intentFingerprint: captured.intent.fingerprint, resourceID: captured.intent.resourceID ?? options.resourceID, subjectID: captured.intent.subjectID, intentID: captured.intent.intentID } })
   const result = await options.send(captured.body, captured.intent.revision)
   if (!options.matches(result, captured.intent)) throw new Error('The response did not acknowledge the exact original request. Its outcome remains unknown.')
   commands.complete(attempt)
   if (ticket.current()) { setReview(undefined); setNotice('Original ' + captured.body.action.replaceAll('_', ' ') + ' acknowledged. Read current facts for the next action.'); options.acknowledged?.(); await options.refresh() }
  } catch (error) {
   const rejected = error instanceof APIError && error.status >= 400 && error.status < 500 && error.status !== 408
   if (attempt) { if (rejected) commands.complete(attempt); else commands.uncertain(attempt) }
   if (ticket.current()) { setFailure(error); setFailureStage(!attempt ? 'before_write' : rejected ? 'rejected' : 'unknown'); if (attempt) setReview(undefined); if (error instanceof APIError && [401, 403, 404].includes(error.status) || options.contextDenied?.(error)) { scope.invalidate(); await options.refresh() } }
  } finally { sending.current = false; if (ticket.current()) setPreparing(false) }
 }
 async function readBase(source: HTMLElement) {
  if (pending || options.paused || sending.current) return
  const ticket = scope.capture(); sending.current = true; setPreparing(true); setFailure(undefined)
  try { const current = await options.read(ticket.signal); ticket.assert(); baseOpener.current = source; setBase({ revision: options.revision(current), version: options.version(current) }) }
  catch (error) { if (ticket.current()) { setFailure(error); await options.refresh() } }
  finally { sending.current = false; if (ticket.current()) setPreparing(false) }
 }
 async function chooseBase() {
  if (!base || pending || options.paused || sending.current) return
  const ticket = scope.capture(); sending.current = true; setPreparing(true); setFailure(undefined)
  try {
   const current = await options.read(ticket.signal); ticket.assert()
   if (options.revision(current) !== base.revision || options.version(current) !== base.version) throw new Error('The current starting point changed again. Review current facts.')
   commands.selectNewBase(); setBase(undefined); setReview(undefined); options.acknowledged?.(current); await options.refresh()
  } catch (error) { if (ticket.current()) setFailure(error) }
  finally { sending.current = false; if (ticket.current()) setPreparing(false) }
 }
 const focus = (source: HTMLElement | null) => source?.isConnected && !source.matches(':disabled') ? source : fallback.current ?? false
 return { review, pending, blocked, stale, unknown: commands.unconfirmed, failure, failureStage, notice, prepare, confirm, cancel: () => { if (!pending) setReview(undefined) }, returnFocus: () => focus(opener.current), headingRef: (element: HTMLHeadingElement | null) => { fallback.current = element }, base, readBase, chooseBase, cancelBase: () => { if (!pending) setBase(undefined) }, returnBaseFocus: () => focus(baseOpener.current) }
}
export function ResultDecisionState({ failure, failureStage, notice, unknown, pending, stale, headingRef, base, readBase, chooseBase, cancelBase, returnBaseFocus }: Pick<ReturnType<typeof useResultDecision>, 'failure' | 'failureStage' | 'notice' | 'unknown' | 'pending' | 'stale' | 'headingRef' | 'base' | 'readBase' | 'chooseBase' | 'cancelBase' | 'returnBaseFocus'>) {
 return <div className="fleet-result-history"><h3 ref={headingRef} tabIndex={-1}>Decision status</h3>{failureStage === 'before_write' && <p role="alert">Current access, source or starting point could not be confirmed. Nothing was sent. Read current facts and review again.</p>}{!!failure && <Failure error={failure} />}{notice && <p role="status">{notice}</p>}{stale && <p role="status">The starting point changed. Cancel this review, read current facts and review a fresh decision.</p>}{unknown && <><p role="status">The original outcome is unknown. Read its original acknowledgement below. A current read does not establish success or failure.</p><Button icon="refresh" variant="outline" disabled={pending} onClick={event => void readBase(event.currentTarget)}>Review a fresh starting point</Button></>}{base && <ConfirmationDialog open title="Review a fresh starting point" description="Permit a different deliberate request while retaining the earlier unknown request in history." confirmLabel="Use reviewed current starting point" pending={pending} onConfirm={() => void chooseBase()} onOpenChange={open => { if (!open) cancelBase() }} finalFocus={returnBaseFocus} printExcluded><p>Version {base.version} · Source revision {base.revision}</p><p>This clears unsent private inputs. It does not acknowledge or replay the earlier request.</p></ConfirmationDialog>}</div>
}
export function OriginalResultHistory<R>({ owner, read, matches }: { owner: string; read: (requestID: string, signal: AbortSignal) => Promise<R>; matches: (result: R, intent: DecisionMetadata) => boolean }) {
 const identity = useIdentity(), commands = useCommunicationCommands(owner), scope = usePrivateRequestScope(`${owner}:history`)
 const [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState('')
 const outcome = useRef<HTMLParagraphElement>(null)
 useEffect(() => { if (notice) { const frame = requestAnimationFrame(() => outcome.current?.focus()); return () => cancelAnimationFrame(frame) } }, [notice])
 const departure = () => window.confirm('Leave with an original result or report outcome still unknown? Safe request metadata remains in this signed-in session. No request will be replayed.')
 usePlanningDepartureBarrier(commands.pending)
 useBlocker({ shouldBlockFn: () => commands.pending || commands.unconfirmed && !departure(), enableBeforeUnload: commands.pending || commands.unconfirmed })
 useLayoutEffect(() => {
  if (!commands.unconfirmed) return
  const guard = (event: Event) => { if (!departure()) event.preventDefault() }
  window.addEventListener('planning-departure', guard); return () => window.removeEventListener('planning-departure', guard)
 }, [commands.unconfirmed])
 async function recover(attempt: typeof commands.attempts[number]) {
  if (pending || !attempt.command.requestID || !attempt.command.intentFingerprint || attempt.command.expectedVersion === undefined) return
  const ticket = scope.capture(); setPending(true); setFailure(undefined)
  try {
   const result = await read(attempt.command.requestID, ticket.signal); ticket.assert()
   if (!matches(result, { requestID: attempt.command.requestID, actorID: identity.person.id, action: attempt.command.action, revision: attempt.revision, version: attempt.command.expectedVersion, fingerprint: attempt.command.intentFingerprint, resourceID: attempt.command.resourceID, subjectID: attempt.command.subjectID, intentID: attempt.command.intentID })) throw new Error('No exact original acknowledgement is available. Current facts do not settle this request.')
   commands.complete(attempt.id); setNotice('The exact original request was acknowledged. Read current facts separately before the next action.')
  } catch (error) { if (ticket.current()) setFailure(error) } finally { if (ticket.current()) setPending(false) }
 }
 return <>{commands.attempts.length > 0 && <aside className="fleet-result-history fleet-results-private" aria-label="Original request history"><h2>Original request history</h2><p>Request identity only. Private prose, scores, reasons and recipients are cleared when the editor closes.</p>{commands.attempts.map(attempt => <div key={attempt.id}><p>Request {attempt.command.requestID} · Version {attempt.command.expectedVersion} · {attempt.state === 'pending' ? 'Awaiting original acknowledgement' : 'Original outcome unknown'}</p>{attempt.state === 'unknown' && <Button icon="refresh" variant="outline" pending={pending} onClick={() => void recover(attempt)}>Read original acknowledgement</Button>}</div>)}{!!failure && <Failure error={failure} />}</aside>}{notice && <p role="status" tabIndex={-1} ref={outcome}>{notice}</p>}</>
}

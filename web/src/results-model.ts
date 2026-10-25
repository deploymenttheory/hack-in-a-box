import type { components } from './api/schema.gen'
type S = components['schemas']
export type ResultPage = S['ResultPage']
export type ResultCommand = S['ResultCommand']
export type ResultAnnouncement = S['ResultAnnouncement']
export type ResultAward = S['ResultAward']
export type ResultIntent = { requestID: string; actorID: string; action: string; revision: number; version: number; fingerprint: string }
export const nilUUID = '00000000-0000-0000-0000-000000000000'
export async function nativeFingerprint(value: unknown) {
 const encoded = JSON.stringify(value).replace(/[<>&\u2028\u2029]/g, c => `\\u${c.charCodeAt(0).toString(16).padStart(4, '0')}`)
 return [...new Uint8Array(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(encoded)))].map(v => v.toString(16).padStart(2, '0')).join('')
}
export function resultFingerprint(body: ResultCommand, revision: number) {
 return nativeFingerprint({ Command: { request_id: body.request_id.toLowerCase(), version: body.version, action: body.action, source_binding: body.source_binding, tie_id: '', team_id: nilUUID, evidence: null, awards: body.awards?.map(a => ({ prize_id: a.prize_id.toLowerCase(), name: a.name, category_id: a.category_id?.toLowerCase() ?? null, team_id: a.team_id.toLowerCase(), amount: a.amount, currency: a.currency })) ?? null, reason: body.reason }, ExpectedRevision: revision })
}
export function matchesResultReceipt(receipt: S['ResultReceipt'], intent: ResultIntent) {
 const publication = intent.action === 'publish_finalists' || intent.action === 'publish_awards'
 return receipt.actor_id === intent.actorID && receipt.request_id === intent.requestID && receipt.action === intent.action && receipt.expected_revision === intent.revision && receipt.expected_version === intent.version && receipt.version === intent.version + 1 && receipt.fingerprint === intent.fingerprint && receipt.run_id === nilUUID && receipt.tie_id === '' && !receipt.team_ids?.length && (publication ? !!receipt.publication_id && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(receipt.publication_id) && receipt.publication_id !== nilUUID : receipt.publication_id === null)
}
export function resultBinding(page: ResultPage, action: ResultCommand['action']) {
 return action === 'publish_finalists' || action === 'prepare' && page.preview.blockers.length > 0 ? page.preview.finalists_binding : page.preview.binding
}
export function resultActionReady(page: ResultPage, action: ResultCommand['action']) {
 if (action === 'publish_finalists') return !!page.preview.selection && !!page.preview.finalists_binding
 if (action === 'prepare') return !!resultBinding(page, action)
 const sourceReady = page.current && !!page.preview.binding && page.preview.blockers.length === 0 && page.round.prepared_by !== nilUUID && page.round.source.binding === page.preview.binding
 if (!sourceReady || page.tie_states.some(t => !t.winner_id || !!t.blocker)) return false
 return action === 'confirm' || !!page.round.confirmation && page.round.confirmation.source_binding === page.preview.binding
}
export function assertResultReady(page: ResultPage, body: ResultCommand) {
 if (body.version !== page.round.version || body.source_binding !== resultBinding(page, body.action) || !resultActionReady(page, body.action)) throw new Error('The reviewed result source changed or is blocked. Read current results and review again.')
}
export const resultActionLabel = { prepare: 'Prepare results', confirm: 'Confirm awards privately', publish_finalists: 'Announce finalists', publish_awards: 'Announce awards' } as const

import type { components } from './api/schema.gen'
export type FreezePage = components['schemas']['SubmissionFreezePage']
export type FreezeCommand = components['schemas']['SubmissionFreezeCommand']
export type FreezeReceipt = components['schemas']['SubmissionFreezeReceipt']
export type FreezeIntent = { command: FreezeCommand; revision: number; fingerprint: string }
export type FreezeAcknowledgement = { requestID: string; action: string; revision: number; bodyAvailable: boolean }
// Match the native Go command encoding, including zero values and reviewed head.
// This is intent identity, not a signature or a permission decision.
export const trimFreezeEvidence = (value:string) => value.replace(/^\p{White_Space}+|\p{White_Space}+$/gu,'')
export async function freezeFingerprint(c: FreezeCommand, revision: number) {
  const bytes = new TextEncoder().encode(JSON.stringify({ submission_revision:c.submission_revision??0,native_revision:c.native_revision??0,request_id:c.request_id,action:c.action,source_id:c.source_id??'00000000-0000-0000-0000-000000000000',branch:c.branch??'',sha:c.sha??'',reason:trimFreezeEvidence(c.reason??''),observation_ids:c.observation_ids??null,push_ids:c.push_ids??null,external_evidence:trimFreezeEvidence(c.external_evidence??''),prior_decision_id:c.prior_decision_id??null,expected_revision:revision }).replace(/[<>&]/g, char=>({'<':'\\u003c','>':'\\u003e','&':'\\u0026'}[char]!)).replace(/\u2028/g,'\\u2028').replace(/\u2029/g,'\\u2029'))
  return [...new Uint8Array(await crypto.subtle.digest('SHA-256',bytes))].map(b=>b.toString(16).padStart(2,'0')).join('')
}
export function matchesFreezeReceipt(receipt: FreezeReceipt, original: { requestID:string; action:string; revision:number; fingerprint:string }) {
  return receipt.request_id===original.requestID && receipt.action===original.action && receipt.expected_revision===original.revision && receipt.revision===original.revision+1 && receipt.fingerprint===original.fingerprint
}
export function verifiedFreeze(page: FreezePage) {
  return page.native_outcome==='frozen' && page.status==='completed' && page.binding_current && !!page.current && page.current.submission_revision===page.current_submission_revision
}
export function frozenCommitURL(repository:string,sha:string) {
  try {const u=new URL(repository);return u.protocol==='https:'&&u.hostname==='github.com'&&!u.port&&!u.username&&!u.password&&!u.search&&!u.hash&&/^\/[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(u.pathname)&&/^(?:[a-f0-9]{40}|[a-f0-9]{64})$/.test(sha)?`${u.origin}${u.pathname}/commit/${sha}`:undefined}catch{return undefined}
}
export function freezeLabel(page:FreezePage) {
  if(page.native_outcome==='withdrawn')return 'Withdrawn · unused and unscored'
  if(page.native_outcome==='not_submitted')return 'Not submitted'
  if(verifiedFreeze(page))return 'Reviewed frozen code'
  if(page.status==='not_applicable')return 'No repository evidence in this final'
  if(page.status==='pending')return 'Awaiting cutoff capture'
  if(page.status==='native_revision_mismatch')return 'Current final needs a separate code amendment'
  return 'Unresolved code evidence · organiser action required'
}

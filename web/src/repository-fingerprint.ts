export type RepositoryIntentIdentity = { action: 'review' | 'approve'; actorID: string; eventID: string; teamID: string; repositoryID: number; planID: string; version?: number; reason: string }
// Same Unicode White_Space trimming as native strings.TrimSpace, including NEL.
export function repositoryReason(reason: string) { return reason.replace(/^\p{White_Space}+|\p{White_Space}+$/gu, '') }
export async function repositoryIntentFingerprint(input: RepositoryIntentIdentity) {
  const ids = [input.actorID, input.eventID, input.teamID, input.planID]
  if (ids.some(id => !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(id) || id === '00000000-0000-0000-0000-000000000000') || !Number.isSafeInteger(input.repositoryID) || input.repositoryID <= 0 || (input.action === 'approve' && (!Number.isSafeInteger(input.version) || input.version! <= 0))) throw new Error('The original repository command identity is not representable safely.')
  const fields = [input.action, input.actorID.toLowerCase(), input.eventID.toLowerCase(), input.teamID.toLowerCase(), String(input.repositoryID), input.planID.toLowerCase(), String(input.action === 'review' ? 0 : input.version), repositoryReason(input.reason)]
  const encoder = new TextEncoder(), framed = 'hackbox:repository-access-command:v1\n' + fields.map(value => `${encoder.encode(value).length}:${value}`).join('')
  const digest = await crypto.subtle.digest('SHA-256', encoder.encode(framed))
  return [...new Uint8Array(digest)].map(value => value.toString(16).padStart(2, '0')).join('')
}

import type { components } from './api/schema.gen'
export type SessionReceipt = components['schemas']['SessionBookingReceipt']
export type SessionOriginal = { requestID: string; sessionID: string; personID: string; action: 'reserve' | 'cancel' | 'present' | 'absent'; revision: number }
export function matchSessionReceipt(receipt: SessionReceipt, original: SessionOriginal): boolean {
  const attendance = original.action === 'present' || original.action === 'absent'
  return receipt.request_id === original.requestID && receipt.session_id === original.sessionID && receipt.person_id === original.personID
    && receipt.action === (attendance ? 'attendance' : original.action)
    && (original.action === 'reserve' ? ['booked', 'waitlisted'].includes(receipt.state) : receipt.state === (original.action === 'cancel' ? 'cancelled' : original.action))
    && Number.isSafeInteger(receipt.revision) && receipt.revision === original.revision + 1
}

export class SessionBookingError extends Error {}

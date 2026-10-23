import { createContext, useContext, useEffect, useRef } from 'react'
import { APIError, type EventRecord } from './api/client'
import type { components } from './api/schema.gen'

export type Delivery = components['schemas']['DeliveryHistoryItem']
export type Detail = components['schemas']['DeliveryHistoryDetail']
export type Decision = components['schemas']['DeliveryReviewCommand']
export type Action = Decision['action']
export const DeliveryContext = createContext<{ event: EventRecord; key: readonly unknown[]; deny: (error: unknown) => void } | null>(null)
export function useDelivery() { const value = useContext(DeliveryContext); if (!value) throw new Error('Missing current delivery workspace'); return value }
export function denied(error: unknown) { return error instanceof APIError && [401, 403, 404].includes(error.status) }
export function uncertain(error: unknown) { return !(error instanceof APIError) || error.status >= 500 || error.status === 408 }
export function useCurrentDelivery() { const active = useRef(true); useEffect(() => { active.current = true; return () => { active.current = false } }, []); return active }
export function useDeliveryDenial(error: unknown) { const { deny } = useDelivery(); useEffect(() => { if (denied(error)) deny(error) }, [deny, error]) }
export const actionName: Record<Action, string> = { retry: 'Retry a definite temporary failure', cancel: 'Cancel a pending attempt', cancel_resend: 'Cancel only the queued resend', resolve_no_resend: 'Resolve without another send', resend_acknowledged: 'Explicitly authorise one more attempt' }
export function stateName(value: string) { return ({ never_attempted: 'Never attempted', provider_accepted: 'Provider accepted — delivery not confirmed', retryable: 'Retryable failure', permanent: 'Permanent failure', ambiguous: 'Uncertain outcome', in_progress: 'Attempt in progress', local_persisted: 'Recorded in the private in-app inbox', unavailable: 'Not attempted — recipient unavailable' } as Record<string, string>)[value] ?? 'Unknown evidence' }

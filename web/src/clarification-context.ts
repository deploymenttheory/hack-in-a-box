import { createContext, useContext, useEffect, useLayoutEffect, useRef } from 'react'
import { useBlocker } from '@tanstack/react-router'
import { APIError, type EventRecord } from './api/client'
import type { components } from './api/schema.gen'

export type Workspace = components['schemas']['ClarificationWorkspace']
export type Question = components['schemas']['PrivateQuestion']
export type AnswerDraft = components['schemas']['ClarificationConfiguration']
export type AnswerDocument = components['schemas']['ClarificationDocument']
export type Source = components['schemas']['ClarificationSource']
export type Scope = { event: EventRecord; workspace: Workspace; key: readonly unknown[]; writable: boolean; deny: (error: unknown) => void; refresh: (capability?: 'ask' | 'review') => Promise<boolean> }
export const ClarificationContext = createContext<Scope | null>(null)
export function useClarifications() { const value = useContext(ClarificationContext); if (!value) throw new Error('Missing native clarification workspace'); return value }
export function denied(error: unknown) { return error instanceof APIError && [401, 403, 404].includes(error.status) }
export function uncertain(error: unknown) { return !(error instanceof APIError) || error.status >= 500 || error.status === 408 }
export function sameSource(a: Source, b: Source) { return a.challenge_id === b.challenge_id && a.challenge_version === b.challenge_version && a.rubric.id === b.rubric.id && a.rubric.version === b.rubric.version && a.rubric.digest === b.rubric.digest }
export function sameDocument(a: AnswerDocument, b: AnswerDocument) { return a.title === b.title && a.body === b.body && a.kind === b.kind }
export function textBytes(value: string) { return new TextEncoder().encode(value).length }
export function useCurrentPage() {
  const active = useRef(true)
  useLayoutEffect(() => { active.current = true; const end = () => { active.current = false }; window.addEventListener('session-ending', end); window.addEventListener('session-expired', end); return () => { end(); window.removeEventListener('session-ending', end); window.removeEventListener('session-expired', end) } }, [])
  return active
}
export function usePrivateLeaveWarning(dirty: boolean) {
  useBlocker({ shouldBlockFn: () => dirty && !window.confirm('Leave this private unsent text or uncertain operation? Text is kept only in this page and will be cleared.'), enableBeforeUnload: dirty })
}
export function useAccessFailure(error: unknown) {
  const { deny } = useClarifications()
  useEffect(() => { if (denied(error)) deny(error) }, [deny, error])
}

import { useEffect, useRef } from 'react'
import { useBlocker } from '@tanstack/react-router'
import { APIError } from './api/client'
import type { components } from './api/schema.gen'
export type ParticipantSurvey = components['schemas']['SurveyParticipant']
export type SurveyConfiguration = components['schemas']['SurveyConfiguration']
export type SurveyDefinition = components['schemas']['SurveyDefinition']
export type SurveyReport = components['schemas']['SurveyReport']
export type SurveyDashboard = components['schemas']['SurveyDashboard']
export function canonical(value: unknown): string { if (Array.isArray(value)) return `[${value.map(canonical).join(',')}]`; if (value && typeof value === 'object') return `{${Object.entries(value).filter(([, v]) => v !== undefined).sort(([a], [b]) => a.localeCompare(b)).map(([k, v]) => `${JSON.stringify(k)}:${canonical(v)}`).join(',')}}`; return JSON.stringify(value) ?? 'undefined' }
export const same = (a: unknown, b: unknown) => canonical(a) === canonical(b)
export const nativeDenied = (error: unknown) => error instanceof APIError && [401, 403, 404].includes(error.status)
export const unknownOutcome = (error: unknown) => !(error instanceof APIError) || error.status >= 500 || error.status === 408
export function useSurveyLeaveWarning(dirty: boolean, pending = false) {
  useBlocker({ shouldBlockFn: () => pending || dirty && !window.confirm('Leave unsaved survey text or an uncertain operation? Private text and its retained retry command are kept only in this page and will be cleared.'), enableBeforeUnload: dirty || pending })
}
export function useActivePage() {
  const active = useRef(true)
  useEffect(() => { active.current = true; return () => { active.current = false } }, [])
  return active
}
export function answerText(value: unknown): string {
  if (value === undefined || value === null) return 'Unknown or not answered'
  if (typeof value === 'boolean') return value ? 'Yes' : 'No'
  if (Array.isArray(value)) return value.map(String).join(', ') || 'No choices selected'
  return String(value)
}

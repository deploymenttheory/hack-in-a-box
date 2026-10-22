import type { components } from './api/schema.gen'
export type MentorPage = components['schemas']['MentorPage']
export type MentorAssignment = components['schemas']['MentorAssignment']
export type MentorCommand = components['schemas']['MentorCommand']
export type MentorProfile = components['schemas']['MentorProfile']
export type MentorProposal = components['schemas']['MentorProposal']
export type MentorWindow = components['schemas']['MentorWindow']
export function workload(assignments: MentorAssignment[], person: string) { return new Set(assignments.filter(a => a.person_id === person).map(a => a.team_id)).size }
export function slotLabel(slot: MentorAssignment['slot']) { return slot === 'primary' ? 'Primary mentor' : 'Second reviewer' }
export function replaceSlot(assignments: MentorAssignment[], team: string, slot: MentorAssignment['slot'], replacement?: MentorAssignment) {
  const result = assignments.filter(a => a.team_id !== team || a.slot !== slot)
  return replacement ? [...result, replacement] : result
}
export function declaredAvailabilityCovers(windows: MentorWindow[], need: MentorWindow) {
  let cursor = new Date(need.start).getTime()
  const end = new Date(need.end).getTime()
  for (const window of [...windows].sort((a, b) => new Date(a.start).getTime() - new Date(b.start).getTime())) {
    const start = new Date(window.start).getTime(), finish = new Date(window.end).getTime()
    if (finish <= cursor) continue
    if (start > cursor) return false
    cursor = Math.max(cursor, finish)
    if (cursor >= end) return true
  }
  return false
}

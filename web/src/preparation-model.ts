import type { components } from './api/schema.gen'

export type PreparationTask = components['schemas']['PreparationTask']
export type PreparationPage = components['schemas']['PreparationPage']
export type Owner = { id: string; name: string }
export const taskStatuses = ['not_started', 'in_progress', 'blocked', 'complete', 'waived'] as const
export const taskRoles = ['organiser', 'mentor', 'judge', 'mc', 'logistics', 'photographer', 'attendee'] as const
export const words = (value: string) => value.replaceAll('_', ' ')
export function ownerName(task: PreparationTask, owners: Owner[]) {
  const d = task.definition
  return [d.owner_id ? owners.find(p => p.id === d.owner_id)?.name ?? 'Previous assignee' : 'No named owner', d.owner_role].filter(Boolean).join(' · ')
}
